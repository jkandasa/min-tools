package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

// Server exposes the webhook receivers and a health endpoint over HTTP.
type Server struct {
	address    string
	healthPath string
	receivers  []*Receiver
	httpServer *http.Server
}

// NewServer creates a server bound to address.
func NewServer(address, healthPath string) *Server {
	return &Server{
		address:    address,
		healthPath: healthPath,
	}
}

// Handle registers a receiver.
func (s *Server) Handle(receiver *Receiver) {
	s.receivers = append(s.receivers, receiver)
}

// Receivers returns the registered receivers.
func (s *Server) Receivers() []*Receiver { return s.receivers }

// ListenAndServe starts the server and blocks until it is shut down. A clean
// shutdown returns nil.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	for _, receiver := range s.receivers {
		mux.Handle(receiver.Path(), receiver)
	}
	mux.HandleFunc(s.healthPath, s.health)

	s.httpServer = &http.Server{
		Addr:              s.address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// Audit bodies can be large on a busy cluster, keep write and idle
		// timeouts generous but bounded.
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		ErrorLog:     logx.StdLogger(),
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("webhook server failed on %s: %w", s.address, err)
	}
	return nil
}

// Shutdown stops the server without dropping in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// health reports the state of every receiver, useful as a load balancer probe
// and as a quick way to confirm MinIO is actually posting events.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	type stream struct {
		Events   int64  `json:"events"`
		Rejected int64  `json:"rejected"`
		Errors   int64  `json:"errors"`
		File     string `json:"file"`
	}

	response := struct {
		Status  string            `json:"status"`
		Streams map[string]stream `json:"streams"`
	}{
		Status:  "ok",
		Streams: make(map[string]stream, len(s.receivers)),
	}

	for _, receiver := range s.receivers {
		events, rejected, errs := receiver.Stats()
		response.Streams[receiver.Name()] = stream{
			Events:   events,
			Rejected: rejected,
			Errors:   errs,
			File:     receiver.File(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logx.Debugf("failed to write health response: %v", err)
	}
}
