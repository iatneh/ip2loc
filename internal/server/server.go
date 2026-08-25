// Package server exposes the public HTTP API.
//
// Endpoints
//
//	GET /         → client's own IP (string body)
//	GET /ip2loc   → { ip, country, city, asn, ... }
//	                query params: ip=<address>, lang=<locale>
//	GET /healthz  → liveness probe (200 if process is up)
//	GET /readyz   → readiness probe (200 only when the City db is loaded)
//
// Conventions
//   - All JSON responses follow the same envelope: { code, msg, data }.
//   - The original project used a uniform 200-with-code for errors; we keep
//     that, and additionally return the proper HTTP status code so well-behaved
//     clients can branch on it.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/iatneh/ip2loc/internal/config"
	"github.com/iatneh/ip2loc/internal/geoip"
	"github.com/iatneh/ip2loc/internal/iputil"
)

// Server is the HTTP layer. It is fully constructed by New; the caller only
// needs to invoke Run.
type Server struct {
	cfg    config.HTTPConfig
	reader *geoip.Reader
	log    *slog.Logger
	srv    *http.Server
}

// New wires routes and prepares the listener. addr is "host:port".
func New(cfg config.HTTPConfig, reader *geoip.Reader, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{
		cfg:    cfg,
		reader: reader,
		log:    log.With("component", "http"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePublicIP)
	mux.HandleFunc("/ip2loc", s.handleIp2Loc)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	s.srv = &http.Server{
		Addr:              net.JoinHostPort(cfg.Address, fmt.Sprintf("%d", cfg.Port)),
		Handler:           s.middleware(mux),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Run starts the listener and blocks until ctx is cancelled. Shutdown is
// graceful with a 10s grace period.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http listening", "addr", s.srv.Addr)
		err := s.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.log.Info("http shutting down")
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// --- middleware --------------------------------------------------------------

type respWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *respWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *respWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (s *Server) middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &respWriter{ResponseWriter: w}
		h.ServeHTTP(rw, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"bytes", rw.bytes,
			"remote", r.RemoteAddr,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

// --- handlers ---------------------------------------------------------------

func (s *Server) handlePublicIP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	ip, err := iputil.ClientIP(r)
	if err != nil {
		// Fallback to RemoteAddr raw; client wants *something* stringified.
		ip = r.RemoteAddr
	}
	writeText(w, http.StatusOK, ip)
}

func (s *Server) handleIp2Loc(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("ip")
	lang := r.URL.Query().Get("lang")

	var ip string
	if raw == "" {
		got, err := iputil.ClientIP(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, 4000, "no ip provided and client ip could not be determined")
			return
		}
		ip = got
	} else {
		got, err := iputil.Normalise(raw)
		if err != nil {
			code := 4000
			msg := "invalid ip"
			if errors.Is(err, iputil.ErrEmpty) {
				msg = "empty ip"
			}
			writeError(w, http.StatusBadRequest, code, msg)
			return
		}
		ip = got
	}

	info, err := s.reader.Lookup(ip, lang)
	if err != nil {
		switch {
		case errors.Is(err, geoip.ErrEmptyIP):
			writeError(w, http.StatusBadRequest, 4000, "empty ip")
		case errors.Is(err, geoip.ErrInvalidIP):
			writeError(w, http.StatusBadRequest, 4001, "invalid ip")
		case errors.Is(err, geoip.ErrNoCityDB):
			writeError(w, http.StatusServiceUnavailable, 5001, "city database not loaded")
		default:
			writeError(w, http.StatusInternalServerError, 5000, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, 0, "success", info)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, 0, "ok", map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.reader.HasCity() {
		writeError(w, http.StatusServiceUnavailable, 5001, "city db not ready")
		return
	}
	writeJSON(w, http.StatusOK, 0, "ready", map[string]string{"status": "ready"})
}

// --- response helpers --------------------------------------------------------

type envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Data    interface{} `json:"data"`
}

func writeJSON(w http.ResponseWriter, status, code int, msg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Code: code, Message: msg, Data: data})
}

func writeError(w http.ResponseWriter, status, code int, msg string) {
	writeJSON(w, status, code, msg, nil)
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
