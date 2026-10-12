package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/adssib/StressLab/internal/store"
	"github.com/adssib/StressLab/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/bcrypt"
)

// FeedSize is how many posts the feed returns (SPEC, E1).
const FeedSize = 20

// Server holds everything the handlers share.
type Server struct {
	store     store.Store
	tokens    *Tokens
	dummyHash []byte
	log       *slog.Logger
	metrics   *telemetry.HTTPMetrics
	registry  *prometheus.Registry
}

// NewServer builds the API. It computes one throwaway bcrypt hash up front,
// used to make a login for an unknown user cost as much as a wrong password.
func NewServer(st store.Store, tokens *Tokens, log *slog.Logger, reg *prometheus.Registry) (*Server, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	dummy, err := bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &Server{
		store:     st,
		tokens:    tokens,
		dummyHash: dummy,
		log:       log,
		metrics:   telemetry.NewHTTPMetrics(reg),
		registry:  reg,
	}, nil
}

// Handler returns the routed API. Route patterns double as metric labels.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, h http.Handler) { mux.Handle(pattern, s.instrument(pattern, h)) }

	route("POST /v1/login", http.HandlerFunc(s.login))
	route("GET /v1/feed", s.authed(s.feed))
	route("GET /v1/posts/{id}", s.authed(s.getPost))
	route("POST /v1/posts/{id}/like", s.authed(s.like))
	route("POST /v1/posts", s.authed(s.createPost))
	route("GET /healthz", http.HandlerFunc(s.healthz))

	// Private-network only (ARCHITECTURE, ports); not counted as API traffic.
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return mux
}

// requestInfo lets inner handlers (auth) tell the outer instrumentation who
// the caller was, for the log line.
type requestInfo struct{ userID int64 }

type infoKey struct{}

// instrument records metrics for every request and logs it under the policy.
func (s *Server) instrument(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		done := s.metrics.Start()
		info := &requestInfo{}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), infoKey{}, info)))

		d := time.Since(start)
		done(r.Method, route, rec.status, d)
		if telemetry.ShouldLogRequest(rec.status, d) {
			s.log.LogAttrs(r.Context(), levelFor(rec.status), "request",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", rec.status),
				slog.Float64("duration_ms", float64(d.Microseconds())/1000),
				slog.Int64("user_id", info.userID),
			)
		}
	})
}

func levelFor(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.ErrorContext(r.Context(), "internal error", slog.String("path", r.URL.Path), slog.Any("err", err))
	writeError(w, http.StatusInternalServerError, "internal error")
}
