package main

import (
	"fmt"
	"net/http"
	"time"
)

// statusRecorder captures the response status and size for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer
// (used by /delay to extend the write deadline).
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

var methodColors = map[string]string{
	http.MethodGet:    "\033[34m",
	http.MethodPost:   "\033[33m",
	http.MethodPut:    "\033[35m",
	http.MethodDelete: "\033[31m",
}

const (
	defaultColor = "\033[36m"
	resetColor   = "\033[0m"
)

func isHealthPath(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/healthcheck"
}

// logRequests writes one colored line per request, including the response
// status, size and latency. Health probes are skipped to keep logs readable.
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isHealthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		color, reset := "", ""
		if !s.cfg.NoColor {
			color, reset = defaultColor, resetColor
			if c, ok := methodColors[r.Method]; ok {
				color = c
			}
		}
		fmt.Fprintf(s.out, "%s%s %s %s %s  ===> from %s  <=== %d %dB in %s%s\n",
			color, start.Format(time.RFC3339), r.Method, r.URL, r.Proto, r.RemoteAddr,
			rec.status, rec.bytes, time.Since(start).Round(time.Microsecond), reset)
	})
}

// withServerHeaders identifies which instance answered, which is useful when
// testing load balancing across several replicas.
func (s *server) withServerHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Served-By", s.hostname)
		w.Header().Set("X-Server-Version", version)
		// Responses echo client input; never let browsers sniff it as HTML.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
