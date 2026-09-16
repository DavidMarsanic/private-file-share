// Package server exposes the local-share and internet-transfer engines
// over a small JSON+SSE HTTP API, bound to loopback only, for the
// embedded browser-based UI. Shared server plumbing (loopback bind,
// idle-timeout shutdown, job events/cancel/reveal/open routes) comes
// from brightencode-appkit; this package adds the routes specific to
// sharing, and uses ExtraBusy so an open local-share session (not
// itself a Job) also holds off idle shutdown.
package server

import (
	"context"
	"net/http"
	"sync"

	appkit "github.com/DavidMarsanic/brightencode-appkit/server"
	"github.com/DavidMarsanic/private-file-share/internal/engine"
	"github.com/DavidMarsanic/private-file-share/web"
)

type Server struct {
	*appkit.Server
	DefaultOutputDir string

	mu         sync.Mutex
	localShare *engine.LocalShare
}

func New(ctx context.Context, defaultOutputDir string) *Server {
	s := &Server{
		Server:           appkit.New(ctx, 0),
		DefaultOutputDir: defaultOutputDir,
	}
	s.Server.ExtraBusy = func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.localShare != nil
	}
	return s
}

func (s *Server) Start(port int) (string, error) {
	return s.Server.Start(port, web.Static, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/dialog/files", s.handleChooseFiles)
		mux.HandleFunc("POST /api/dialog/folder", s.handleChooseFolder)
		mux.HandleFunc("POST /api/local-share/start", s.handleLocalShareStart)
		mux.HandleFunc("POST /api/local-share/stop", s.handleLocalShareStop)
		mux.HandleFunc("GET /api/local-share/status", s.handleLocalShareStatus)
		mux.HandleFunc("POST /api/send", s.handleSend)
		mux.HandleFunc("POST /api/receive", s.handleReceive)
	})
}
