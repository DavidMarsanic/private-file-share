package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DavidMarsanic/private-file-share/internal/browser"
	"github.com/DavidMarsanic/private-file-share/internal/dialog"
	"github.com/DavidMarsanic/private-file-share/internal/engine"
	"github.com/DavidMarsanic/private-file-share/internal/jobs"
)

func (s *Server) handleChooseFiles(w http.ResponseWriter, r *http.Request) {
	paths, err := dialog.ChooseFiles()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"paths": paths})
}

func (s *Server) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	path, err := dialog.ChooseFolder()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// ---- local (Wi-Fi / nearby phone) share ------------------------------------

func (s *Server) handleLocalShareStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	share, err := engine.StartLocalShare(req.Paths)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "bad-request"})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"url":        share.URL,
		"qrCodePng":  base64.StdEncoding.EncodeToString(qr),
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
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
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
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Paths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing to send", "code": "bad-request"})
		return
	}

	code := engine.NewTransferCode()
	job, ctx := s.Jobs.Create(s.ctx)

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

	writeJSON(w, http.StatusOK, map[string]string{"jobId": job.ID, "code": code})
}

func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter the code you were given", "code": "bad-request"})
		return
	}

	job, ctx := s.Jobs.Create(s.ctx)

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

	writeJSON(w, http.StatusOK, map[string]string{"jobId": job.ID})
}

// ---- shared job/reveal/open plumbing ---------------------------------

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := s.Jobs.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := job.Subscribe()
	defer cancel()

	for {
		select {
		case e, open := <-ch:
			if !open {
				return
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if e.Stage == "done" || e.Stage == "error" || e.Stage == "canceled" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	job, ok := s.Jobs.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	job.Cancel()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := browser.Reveal(req.Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := browser.Open(req.Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body", "code": "bad-request"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
