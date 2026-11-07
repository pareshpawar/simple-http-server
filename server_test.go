package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func newTestServer(t *testing.T, mutate func(*config)) (*server, http.Handler) {
	t.Helper()
	cfg, err := loadConfig(nil, env(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	cfg.NoColor = true
	if mutate != nil {
		mutate(&cfg)
	}
	s, err := newServer(cfg, io.Discard)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	s.ready.Store(true)
	return s, s.routes()
}

func do(t *testing.T, h http.Handler, req *http.Request) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestLoadConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := loadConfig(nil, env(nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Port != 8081 || cfg.BindAddress != "0.0.0.0" || cfg.OutputFormat != "text" {
			t.Errorf("unexpected defaults: %+v", cfg)
		}
		if len(cfg.EchoEnv) != 1 || cfg.EchoEnv[0] != "YOUR_ENV" {
			t.Errorf("EchoEnv = %v, want [YOUR_ENV]", cfg.EchoEnv)
		}
	})

	t.Run("env vars", func(t *testing.T) {
		dir := t.TempDir()
		cfg, err := loadConfig(nil, env(map[string]string{
			"PORT":           "9000",
			"OUTPUT_FORMAT":  "JSON",
			"STATIC_DIR":     dir,
			"ECHO_ENV":       "POD_NAME, NODE_NAME,,YOUR_ENV",
			"SHUTDOWN_DELAY": "5s",
			"NO_COLOR":       "1",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Port != 9000 || cfg.OutputFormat != "json" || cfg.StaticDir != dir ||
			cfg.ShutdownDelay != 5*time.Second || !cfg.NoColor {
			t.Errorf("env not applied: %+v", cfg)
		}
		if got := strings.Join(cfg.EchoEnv, ","); got != "YOUR_ENV,POD_NAME,NODE_NAME" {
			t.Errorf("EchoEnv = %s", got)
		}
	})

	t.Run("flags override env", func(t *testing.T) {
		cfg, err := loadConfig([]string{"-port", "7000", "-format", "html"},
			env(map[string]string{"PORT": "9000", "OUTPUT_FORMAT": "json"}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Port != 7000 || cfg.OutputFormat != "html" {
			t.Errorf("flags not applied: %+v", cfg)
		}
	})

	errorCases := map[string]struct {
		args []string
		env  map[string]string
	}{
		"non-numeric PORT":      {env: map[string]string{"PORT": "abc"}},
		"port out of range":     {args: []string{"-port", "70000"}},
		"unknown format":        {args: []string{"-format", "xml"}},
		"bad SHUTDOWN_DELAY":    {env: map[string]string{"SHUTDOWN_DELAY": "soon"}},
		"missing static dir":    {env: map[string]string{"STATIC_DIR": "/does/not/exist"}},
		"unexpected positional": {args: []string{"extra"}},
	}
	for name, tc := range errorCases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadConfig(tc.args, env(tc.env)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestEchoText(t *testing.T) {
	t.Setenv("YOUR_ENV", "hello-env")
	_, h := newTestServer(t, nil)

	req := httptest.NewRequest("GET", "/some/path?x=1", nil)
	req.Header.Set("X-Test", "abc")
	resp, body := do(t, h, req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.Header.Get("X-Served-By") == "" {
		t.Error("missing X-Served-By header")
	}
	for _, want := range []string{
		"Request Type\t==> GET /some/path?x=1 HTTP/1.1",
		`Header["X-Test"] = [abc]`,
		`FormData["x"] = ["1"]`,
		"YOUR_ENV = hello-env",
		"by PareshPawar.com",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "\033[") {
		t.Error("body contains ANSI codes despite NoColor")
	}
}

func TestEchoFormPost(t *testing.T) {
	_, h := newTestServer(t, nil)
	req := httptest.NewRequest("POST", "/", strings.NewReader("name=paresh&lang=go"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, body := do(t, h, req)

	for _, want := range []string{`FormData["name"] = ["paresh"]`, `FormData["lang"] = ["go"]`, "name=paresh&lang=go"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

func decodeInfo(t *testing.T, body string) requestInfo {
	t.Helper()
	var info requestInfo
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, body)
	}
	return info
}

func TestEchoJSON(t *testing.T) {
	_, h := newTestServer(t, nil)

	cases := map[string]*http.Request{
		"/json endpoint":     httptest.NewRequest("POST", "/json?a=b", strings.NewReader(`{"k":"v"}`)),
		"format query param": httptest.NewRequest("POST", "/?format=json&a=b", strings.NewReader(`{"k":"v"}`)),
		"accept header": func() *http.Request {
			r := httptest.NewRequest("POST", "/?a=b", strings.NewReader(`{"k":"v"}`))
			r.Header.Set("Accept", "application/json")
			return r
		}(),
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			req.Header.Set("Content-Type", "application/json")
			resp, body := do(t, h, req)
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q", ct)
			}
			info := decodeInfo(t, body)
			if info.Method != "POST" || info.Query.Get("a") != "b" {
				t.Errorf("unexpected method/query: %+v", info)
			}
			if info.Body != `{"k":"v"}` || info.BodySize != 9 {
				t.Errorf("body = %q (%d bytes)", info.Body, info.BodySize)
			}
			if info.ClientIP != "203.0.113.9" {
				t.Errorf("client_ip = %q", info.ClientIP)
			}
			if info.Headers.Get("X-Forwarded-For") != "203.0.113.9" {
				t.Errorf("headers not echoed: %v", info.Headers)
			}
			if _, ok := info.Env["YOUR_ENV"]; !ok {
				t.Error("env missing YOUR_ENV")
			}
		})
	}
}

func TestEchoDefaultFormatFromConfig(t *testing.T) {
	_, h := newTestServer(t, func(c *config) { c.OutputFormat = "html" })
	resp, body := do(t, h, httptest.NewRequest("GET", "/", nil))
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || !strings.Contains(body, "<html") {
		t.Errorf("expected HTML output, got %q", resp.Header.Get("Content-Type"))
	}
}

func TestBodyTruncation(t *testing.T) {
	_, h := newTestServer(t, nil)
	big := strings.Repeat("a", maxEchoBody+100)
	_, body := do(t, h, httptest.NewRequest("POST", "/json", strings.NewReader(big)))
	info := decodeInfo(t, body)
	if !info.BodyTruncated || info.BodySize != maxEchoBody {
		t.Errorf("truncated=%v size=%d", info.BodyTruncated, info.BodySize)
	}
}

func TestHTMLEscapesInput(t *testing.T) {
	_, h := newTestServer(t, nil)
	req := httptest.NewRequest("GET", "/html/", nil)
	req.Header.Set("X-Evil", "<script>alert(1)</script>")
	resp, body := do(t, h, req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("header value was not HTML-escaped")
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("escaped header value missing from page")
	}
}

func TestTemplateLoading(t *testing.T) {
	dir := t.TempDir()

	t.Run("custom template with legacy fields", func(t *testing.T) {
		path := filepath.Join(dir, "custom.html")
		src := `<p>{{.Request.ReqType}}|{{.Info.Hostname}}|{{range $k, $v := .Headers}}{{$k}};{{end}}</p>`
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		s, h := newTestServer(t, func(c *config) { c.TemplateFile = path })
		req := httptest.NewRequest("GET", "/html/x", nil)
		req.Header.Set("X-A", "1")
		_, body := do(t, h, req)
		want := "<p>HTTP/1.1 GET /html/x|" + s.hostname + "|X-A;</p>"
		if body != want {
			t.Errorf("body = %q, want %q", body, want)
		}
	})

	t.Run("missing explicit template is an error", func(t *testing.T) {
		if _, err := loadTemplate(filepath.Join(dir, "nope.html")); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("invalid template is an error", func(t *testing.T) {
		path := filepath.Join(dir, "bad.html")
		os.WriteFile(path, []byte("{{ .Broken "), 0o644)
		if _, err := loadTemplate(path); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("missing default template falls back to embedded", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chdir(wd) })
		if _, err := loadTemplate(defaultTemplateFile); err != nil {
			t.Errorf("expected embedded fallback, got %v", err)
		}
	})
}

func TestHealthAndReadiness(t *testing.T) {
	s, h := newTestServer(t, nil)

	for _, path := range []string{"/healthz", "/healthcheck"} {
		resp, body := do(t, h, httptest.NewRequest("GET", path, nil))
		if resp.StatusCode != http.StatusOK || body != "OK\n" {
			t.Errorf("%s: %d %q", path, resp.StatusCode, body)
		}
	}

	resp, _ := do(t, h, httptest.NewRequest("GET", "/readyz", nil))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/readyz while ready: %d", resp.StatusCode)
	}
	s.ready.Store(false)
	resp, _ = do(t, h, httptest.NewRequest("GET", "/readyz", nil))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/readyz while not ready: %d", resp.StatusCode)
	}
}

func TestVersion(t *testing.T) {
	_, h := newTestServer(t, nil)
	_, body := do(t, h, httptest.NewRequest("GET", "/version", nil))
	var v map[string]string
	if err := json.Unmarshal([]byte(body), &v); err != nil || v["version"] != version {
		t.Errorf("unexpected /version body %q (err %v)", body, err)
	}
}

func TestStatus(t *testing.T) {
	_, h := newTestServer(t, nil)
	tests := []struct {
		path     string
		want     int
		wantBody string
	}{
		{"/status/418", 418, "418 I'm a teapot\n"},
		{"/status/503", 503, "503 Service Unavailable\n"},
		{"/status/204", 204, ""},
		{"/status/99", 400, ""},
		{"/status/600", 400, ""},
		{"/status/abc", 400, ""},
	}
	for _, tt := range tests {
		resp, body := do(t, h, httptest.NewRequest("GET", tt.path, nil))
		if resp.StatusCode != tt.want {
			t.Errorf("%s: status %d, want %d", tt.path, resp.StatusCode, tt.want)
		}
		if tt.wantBody != "" && body != tt.wantBody {
			t.Errorf("%s: body %q, want %q", tt.path, body, tt.wantBody)
		}
	}
}

func TestDelay(t *testing.T) {
	_, h := newTestServer(t, nil)

	start := time.Now()
	resp, _ := do(t, h, httptest.NewRequest("GET", "/delay/50ms", nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("responded after %s, want >= 50ms", elapsed)
	}

	resp, _ = do(t, h, httptest.NewRequest("GET", "/delay/0.01", nil))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("numeric seconds: status = %d", resp.StatusCode)
	}

	for _, path := range []string{"/delay/forever", "/delay/-1s", "/delay/10m"} {
		resp, _ := do(t, h, httptest.NewRequest("GET", path, nil))
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", path, resp.StatusCode)
		}
	}
}

func TestDelayStopsWhenClientDisconnects(t *testing.T) {
	_, h := newTestServer(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/delay/30s", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}
}

func TestStaticFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, h := newTestServer(t, func(c *config) { c.StaticDir = dir })

	resp, body := do(t, h, httptest.NewRequest("GET", "/static/hello.txt", nil))
	if resp.StatusCode != http.StatusOK || body != "hi there" {
		t.Errorf("static file: %d %q", resp.StatusCode, body)
	}
	resp, _ = do(t, h, httptest.NewRequest("GET", "/static/missing.txt", nil))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing static file: %d", resp.StatusCode)
	}
	resp, _ = do(t, h, httptest.NewRequest("GET", "/static/../server.go", nil))
	if resp.StatusCode == http.StatusOK {
		t.Error("path traversal outside static dir succeeded")
	}
}

func TestRequestLogging(t *testing.T) {
	s, _ := newTestServer(t, nil)
	var buf strings.Builder
	s.out = &buf
	h := s.routes()

	do(t, h, httptest.NewRequest("GET", "/status/404", nil))
	do(t, h, httptest.NewRequest("GET", "/healthz", nil))

	logs := buf.String()
	if !strings.Contains(logs, "GET /status/404 HTTP/1.1") || !strings.Contains(logs, "<=== 404") {
		t.Errorf("request not logged with status: %q", logs)
	}
	if strings.Contains(logs, "/healthz") {
		t.Errorf("health checks should not be logged: %q", logs)
	}
}

func TestRunAndHealthCheck(t *testing.T) {
	// Reserve a free port, then hand it to the server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg, err := loadConfig([]string{"-bind", "127.0.0.1", "-port", strconv.Itoa(port), "-no-color"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if err := healthCheck(cfg); err == nil {
		t.Fatal("health check passed before the server started")
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for healthCheck(cfg) != nil {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("server never became healthy")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
	if s.ready.Load() {
		t.Error("server still reports ready after shutdown")
	}
}
