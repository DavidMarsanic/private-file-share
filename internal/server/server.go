// Package server exposes the local-share and internet-transfer engines
// over a small JSON+SSE HTTP API, bound to loopback only, for the
// embedded browser-based UI.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DavidMarsanic/private-file-share/internal/engine"
	"github.com/DavidMarsanic/private-file-share/internal/jobs"
	"github.com/DavidMarsanic/private-file-share/web"
)

const idleTimeout = 30 * time.Minute

type Server struct {
	Jobs             *jobs.Registry
	DefaultOutputDir string
	ctx              context.Context

	mu         sync.Mutex
	localShare *engine.LocalShare

	lastActivity atomic.Int64
}

func New(ctx context.Context, defaultOutputDir string) *Server {
	s := &Server{
		ctx:              ctx,
		Jobs:             jobs.NewRegistry(),
		DefaultOutputDir: defaultOutputDir,
	}
	s.touch()
	return s
}

func (s *Server) Start(port int) (string, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", fmt.Errorf("starting local server: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dialog/files", s.handleChooseFiles)
	mux.HandleFunc("POST /api/dialog/folder", s.handleChooseFolder)
	mux.HandleFunc("POST /api/local-share/start", s.handleLocalShareStart)
	mux.HandleFunc("POST /api/local-share/stop", s.handleLocalShareStop)
	mux.HandleFunc("GET /api/local-share/status", s.handleLocalShareStatus)
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("POST /api/receive", s.handleReceive)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleJobCancel)
	mux.HandleFunc("POST /api/reveal", s.handleReveal)
	mux.HandleFunc("POST /api/open", s.handleOpen)
	mux.Handle("GET /", http.FileServer(http.FS(web.Static)))

	httpSrv := &http.Server{Handler: s.trackActivity(mux)}
	go func() {
		_ = httpSrv.Serve(ln)
	}()
	go s.watchIdle()

	return "http://" + ln.Addr().String(), nil
}

func (s *Server) trackActivity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.touch()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) touch() {
	s.lastActivity.Store(time.Now().Unix())
}

func (s *Server) watchIdle() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		idleFor := time.Now().Unix() - s.lastActivity.Load()
		s.mu.Lock()
		hasShare := s.localShare != nil
		s.mu.Unlock()
		if idleFor > int64(idleTimeout.Seconds()) && !s.Jobs.HasActive() && !hasShare {
			os.Exit(0)
		}
	}
}
