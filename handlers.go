package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"pareshpawar.com/simple-http-server/utils"
)

const (
	// maxEchoBody caps how much of a request body is read and echoed back.
	maxEchoBody = 1 << 20 // 1 MiB
	// maxDelay caps /delay so a single request cannot hold a connection forever.
	maxDelay = 60 * time.Second
)

// requestInfo is everything the server reports about a request. It backs the
// text, HTML and JSON outputs.
type requestInfo struct {
	Time          time.Time         `json:"time"`
	Hostname      string            `json:"hostname"`
	Version       string            `json:"version"`
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Path          string            `json:"path"`
	Query         url.Values        `json:"query,omitempty"`
	Proto         string            `json:"proto"`
	Host          string            `json:"host"`
	RemoteAddr    string            `json:"remote_addr"`
	ClientIP      string            `json:"client_ip"`
	ServerAddr    string            `json:"server_addr,omitempty"`
	LocalIP       string            `json:"local_ip"`
	TLS           bool              `json:"tls"`
	Headers       http.Header       `json:"headers"`
	Form          url.Values        `json:"form,omitempty"`
	Body          string            `json:"body,omitempty"`
	BodySize      int               `json:"body_size"`
	BodyTruncated bool              `json:"body_truncated,omitempty"`
	Env           map[string]string `json:"env"`
}

func (s *server) collectInfo(r *http.Request) *requestInfo {
	info := &requestInfo{
		Time:       time.Now(),
		Hostname:   s.hostname,
		Version:    version,
		Method:     r.Method,
		URL:        r.URL.String(),
		Path:       r.URL.Path,
		Proto:      r.Proto,
		Host:       r.Host,
		RemoteAddr: r.RemoteAddr,
		ClientIP:   utils.ClientIP(r),
		LocalIP:    s.localIP,
		TLS:        r.TLS != nil,
		Headers:    r.Header,
		Env:        make(map[string]string, len(s.cfg.EchoEnv)),
	}
	if q := r.URL.Query(); len(q) > 0 {
		info.Query = q
	}
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		info.ServerAddr = addr.String()
	}
	for _, name := range s.cfg.EchoEnv {
		info.Env[name] = os.Getenv(name)
	}

	// Read (a bounded amount of) the body so it can be echoed, then put it
	// back so ParseForm can still decode url-encoded form posts.
	if r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxEchoBody+1))
		if err != nil {
			log.Printf("reading request body: %v", err)
		}
		if len(body) > maxEchoBody {
			body = body[:maxEchoBody]
			info.BodyTruncated = true
		}
		info.Body = string(body)
		info.BodySize = len(body)
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if err := r.ParseForm(); err != nil {
		log.Printf("parsing form: %v", err)
	}
	if len(r.Form) > 0 {
		info.Form = r.Form
	}
	return info
}

// handleEcho serves "/". The format comes from ?format=, then the Accept
// header (application/json), then the configured default.
func (s *server) handleEcho(w http.ResponseWriter, r *http.Request) {
	switch s.negotiateFormat(r) {
	case "json":
		s.writeJSON(w, r)
	case "html":
		s.writeHTML(w, r)
	default:
		s.writeText(w, r)
	}
}

func (s *server) negotiateFormat(r *http.Request) string {
	if f := strings.ToLower(r.URL.Query().Get("format")); validFormats[f] {
		return f
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		first, _, _ := strings.Cut(accept, ",")
		if mt, _, err := mime.ParseMediaType(strings.TrimSpace(first)); err == nil && mt == "application/json" {
			return "json"
		}
	}
	return s.cfg.OutputFormat
}

func (s *server) handleHTML(w http.ResponseWriter, r *http.Request) { s.writeHTML(w, r) }
func (s *server) handleJSON(w http.ResponseWriter, r *http.Request) { s.writeJSON(w, r) }

func (s *server) writeText(w http.ResponseWriter, r *http.Request) {
	info := s.collectInfo(r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	var b strings.Builder
	fmt.Fprintf(&b, "Request Time\t==> %s\n", info.Time)
	fmt.Fprintf(&b, "Request Type\t==> %s %s %s\n", info.Method, info.URL, info.Proto)
	fmt.Fprintf(&b, "Hostname/Host \t==> %s\n", info.Host)
	fmt.Fprintf(&b, "Remote Address \t==> %s\n", info.RemoteAddr)
	fmt.Fprintf(&b, "Client IP \t==> %s\n", info.ClientIP)
	fmt.Fprintf(&b, "Local Address \t==> %s\n", info.LocalIP)
	fmt.Fprintf(&b, "Server Hostname\t==> %s\n", info.Hostname)
	fmt.Fprintf(&b, "Server Version\t==> %s\n\n", info.Version)

	for _, key := range sortedKeys(info.Headers) {
		fmt.Fprintf(&b, "Header[%q] = %s\n", key, info.Headers[key])
	}
	for _, key := range sortedKeys(info.Form) {
		fmt.Fprintf(&b, "FormData[%q] = %q\n", key, info.Form[key])
	}
	if info.BodySize > 0 {
		truncated := ""
		if info.BodyTruncated {
			truncated = ", truncated"
		}
		fmt.Fprintf(&b, "\nBody (%d bytes%s):\n%s\n", info.BodySize, truncated, info.Body)
	}

	b.WriteString("\n")
	for _, name := range s.cfg.EchoEnv {
		fmt.Fprintf(&b, "%s = %s\n", name, info.Env[name])
	}
	b.WriteString(s.bannerResp)

	io.WriteString(w, b.String())
}

// legacyRequest keeps the fields older custom templates rely on.
type legacyRequest struct{ ReqTime, ReqType, Host, Remote, RemoteAddr string }

type templateData struct {
	Info    *requestInfo
	Request legacyRequest // Deprecated: use Info.
	Headers http.Header   // Deprecated: use Info.Headers.
}

func (s *server) writeHTML(w http.ResponseWriter, r *http.Request) {
	info := s.collectInfo(r)
	data := templateData{
		Info: info,
		Request: legacyRequest{
			ReqTime:    info.Time.String(),
			ReqType:    info.Proto + " " + info.Method + " " + info.Path,
			Host:       info.Host,
			Remote:     info.RemoteAddr,
			RemoteAddr: info.LocalIP, // historically held the server's local IP
		},
		Headers: info.Headers,
	}

	// Render into a buffer so a template error yields a clean 500 instead of
	// a half-written page.
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, data); err != nil {
		log.Printf("executing template: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (s *server) writeJSON(w http.ResponseWriter, r *http.Request) {
	writeJSONValue(w, http.StatusOK, s.collectInfo(r))
}

func writeJSONValue(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("encoding JSON: %v", err)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "OK\n")
}

// handleReady reports 503 before the listener is up and while shutting down,
// so orchestrators stop routing traffic before connections are closed.
func (s *server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "NOT READY\n")
		return
	}
	io.WriteString(w, "READY\n")
}

func (s *server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSONValue(w, http.StatusOK, map[string]string{"version": version, "hostname": s.hostname})
}

// handleStatus responds with the status code from the path, e.g. /status/503.
func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 200 || code > 599 {
		http.Error(w, "status code must be a number between 200 and 599", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	if code == http.StatusNoContent || code == http.StatusNotModified {
		return // these must not carry a body
	}
	fmt.Fprintf(w, "%d %s\n", code, http.StatusText(code))
}

// handleDelay waits before responding, e.g. /delay/1500ms or /delay/2 (seconds).
func (s *server) handleDelay(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("duration")
	d, err := time.ParseDuration(raw)
	if err != nil {
		secs, convErr := strconv.ParseFloat(raw, 64)
		if convErr != nil {
			http.Error(w, "invalid duration: use e.g. 500ms, 2s or a number of seconds", http.StatusBadRequest)
			return
		}
		d = time.Duration(secs * float64(time.Second))
	}
	if d < 0 || d > maxDelay {
		http.Error(w, fmt.Sprintf("duration must be between 0 and %s", maxDelay), http.StatusBadRequest)
		return
	}

	// The server's WriteTimeout would otherwise cut long delays short.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(d + 10*time.Second)); err != nil && err != http.ErrNotSupported {
		log.Printf("extending write deadline: %v", err)
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-r.Context().Done():
		return // client went away
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Responded after %s from %s\n", d, s.hostname)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
