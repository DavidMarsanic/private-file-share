package server

import (
	"encoding/base64"
	"net/http"

	appkit "github.com/DavidMarsanic/brightencode-appkit/server"
	"github.com/DavidMarsanic/brightencode-appkit/jobs"
	"github.com/DavidMarsanic/private-file-share/internal/dialog"
	"github.com/DavidMarsanic/private-file-share/internal/engine"
)

func (s *Server) handleChooseFiles(w http.ResponseWriter, r *http.Request) {
	paths, err := dialog.ChooseFiles()
	if err != nil {
		appkit.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	appkit.WriteJSON(w, http.StatusOK, map[string]any{"paths": paths})
}

func (s *Server) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	path, err := dialog.ChooseFolder()
	if err != nil {
		appkit.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	appkit.WriteJSON(w, http.StatusOK, map[string]string{"path": path})
}

// ---- local (Wi-Fi / nearby phone) share ------------------------------------

func (s *Server) handleLocalShareStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if !appkit.DecodeJSON(w, r, &req) {
		return
	}

	share, err := engine.StartLocalShare(req.Paths)
	if err != nil {
		appkit.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "bad-request"})
		return
	}

	s.mu.Lock()
	if s.localShare != nil {
		_ = s.localShare.Stop()
	}
	s.localShare = share
	s.mu.Unlock()

	qr, err := share.QRCodePNG()
	if err != nil {
		appkit.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	appkit.WriteJSON(w, http.StatusOK, map[string]any{
		"url":       share.URL,
		"qrCodePng": base64.StdEncoding.EncodeToString(qr),
	})
}

func (s *Server) handleLocalShareStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	share := s.localShare
	s.localShare = nil
	s.mu.Unlock()

	if share != nil {
		_ = share.Stop()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLocalShareStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	share := s.localShare
	s.mu.Unlock()

	if share == nil {
		appkit.WriteJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	appkit.WriteJSON(w, http.StatusOK, map[string]any{
		"active":    true,
		"url":       share.URL,
		"downloads": share.Downloads(),
	})
}

// ---- internet transfer (croc) ----------------------------------------

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if !appkit.DecodeJSON(w, r, &req) {
		return
	}
	if len(req.Paths) == 0 {
		appkit.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing to send", "code": "bad-request"})
		return
	}

	code := engine.NewTransferCode()
	job, ctx := s.Jobs.Create(s.Ctx)

	go func() {
		onProgress := func(p engine.TransferProgress) {
			job.Publish(jobs.Event{Stage: p.Stage, Percent: p.Percent})
		}
		err := engine.SendOverInternet(ctx, req.Paths, code, onProgress)
		if err != nil {
			if ctx.Err() != nil {
				job.Publish(jobs.Event{Stage: "canceled"})
				return
			}
			job.Publish(jobs.Event{Stage: "error", Message: err.Error()})
			return
		}
		job.Publish(jobs.Event{Stage: "done", Percent: 100})
	}()

	appkit.WriteJSON(w, http.StatusOK, map[string]string{"jobId": job.ID, "code": code})
}

func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !appkit.DecodeJSON(w, r, &req) {
		return
	}
	if req.Code == "" {
		appkit.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "enter the code you were given", "code": "bad-request"})
		return
	}

	job, ctx := s.Jobs.Create(s.Ctx)

	go func() {
		onProgress := func(p engine.TransferProgress) {
			job.Publish(jobs.Event{Stage: p.Stage, Percent: p.Percent})
		}
		err := engine.ReceiveFromInternet(ctx, req.Code, s.DefaultOutputDir, onProgress)
		if err != nil {
			if ctx.Err() != nil {
				job.Publish(jobs.Event{Stage: "canceled"})
				return
			}
			job.Publish(jobs.Event{Stage: "error", Message: err.Error()})
			return
		}
		job.Publish(jobs.Event{Stage: "done", Percent: 100, Path: s.DefaultOutputDir})
	}()

	appkit.WriteJSON(w, http.StatusOK, map[string]string{"jobId": job.ID})
}
