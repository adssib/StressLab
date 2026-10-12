package telemetry

import (
	"io"
	"log/slog"
	"math/rand/v2"
	"time"
)

// NewLogger returns the API's structured logger: JSON lines on w (stdout in
// production), which Docker captures and Alloy ships to Loki. The API never
// talks to Loki itself.
func NewLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, nil))
}

// The logging policy (SPEC, "Logging"), held fixed across every run.
const (
	SlowRequest = 500 * time.Millisecond // the latency SLO
	SampleRate  = 0.01                   // of everything else
)

// ShouldLogRequest applies the policy: every non-2xx, every request slower
// than the latency SLO, and a 1% sample of the rest. Until tracing exists
// (ROADMAP 0.9) the sample is random; then it becomes "the requests that were
// traced", so every sampled line has a trace to click into.
func ShouldLogRequest(status int, d time.Duration) bool {
	if status < 200 || status > 299 || d > SlowRequest {
		return true
	}
	return rand.Float64() < SampleRate
}
