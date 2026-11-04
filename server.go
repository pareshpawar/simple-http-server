package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/common-nighthawk/go-figure"
	"pareshpawar.com/simple-http-server/utils"
)

//go:embed html/index.html
var embeddedTemplate string

const shutdownTimeout = 15 * time.Second

type server struct {
	cfg      config
	tmpl     *template.Template
	hostname string
	localIP  string
	ready    atomic.Bool
	out      io.Writer // request log destination

	// ASCII-art brand, rendered once at startup.
	bannerTerm string
	bannerResp string
}

func newServer(cfg config, out io.Writer) (*server, error) {
	s := &server{cfg: cfg, out: out}

	tmpl, err := loadTemplate(cfg.TemplateFile)
	if err != nil {
		return nil, err
	}
	s.tmpl = tmpl

	if s.hostname, err = os.Hostname(); err != nil {
		s.hostname = "unknown"
	}

	// Resolve the local IP once at startup instead of on every request.
	if ip, err := utils.GetLocalIP(); err != nil {
		log.Printf("Warning: could not determine local IP: %v", err)
		s.localIP = "unavailable"
	} else {
		s.localIP = ip.String()
	}

	s.bannerTerm = renderFigure("Simple HTTP Server", "straight", !cfg.NoColor) +
		renderFigure("by PareshPawar.com", "term", !cfg.NoColor)

	var b strings.Builder
	b.WriteString("__________________________________________________________\n")
	b.WriteString(renderFigure("Simple HTTP Server", "straight", !cfg.NoColor))
	b.WriteString("----------------------------------------------------------\n")
	b.WriteString("                    by PareshPawar.com                    \n")
	b.WriteString("----------------------------------------------------------\n")
	s.bannerResp = b.String()

	return s, nil
}

func renderFigure(text, font string, color bool) string {
	var buf bytes.Buffer
	if color {
		figure.Write(&buf, figure.NewColorFigure(text, font, "green", true))
	} else {
		figure.Write(&buf, figure.NewFigure(text, font, true))
	}
	return buf.String()
}

// loadTemplate parses the HTML template from path. If path is the default
// location and does not exist, the template built into the binary is used, so
// the server works without the html/ directory but can still be customised
// by mounting a file there.
func loadTemplate(path string) (*template.Template, error) {
	tmpl := template.New("webpage").Funcs(template.FuncMap{"join": strings.Join})

	src, err := os.ReadFile(path)
	switch {
	case err == nil:
		t, err := tmpl.Parse(string(src))
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", path, err)
		}
		return t, nil
	case errors.Is(err, fs.ErrNotExist) && path == defaultTemplateFile:
		return tmpl.Parse(embeddedTemplate)
	default:
		return nil, fmt.Errorf("reading template: %w", err)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleEcho)
	mux.HandleFunc("/html/", s.handleHTML)
	mux.HandleFunc("/json", s.handleJSON)
	mux.HandleFunc("/healthcheck", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/version", s.handleVersion)
	mux.HandleFunc("/status/{code}", s.handleStatus)
	mux.HandleFunc("/delay/{duration}", s.handleDelay)
	if s.cfg.StaticDir != "" {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.cfg.StaticDir))))
	}
	return s.withServerHeaders(s.logRequests(mux))
}

func (s *server) addr() string {
	return net.JoinHostPort(s.cfg.BindAddress, strconv.Itoa(s.cfg.Port))
}

func (s *server) printBanner() {
	fmt.Fprint(s.out, s.bannerTerm)
}

// run serves HTTP until ctx is cancelled, then shuts down gracefully: it
// flips /readyz to 503, waits ShutdownDelay so load balancers can drain the
// instance, and finally lets in-flight requests finish.
func (s *server) run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr())
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	s.ready.Store(true)
	log.Printf("pareshpawar/simple-http-server %s: listening on http://%s", version, ln.Addr())
	if s.cfg.StaticDir != "" {
		log.Printf("Serving files from %s under /static/", s.cfg.StaticDir)
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.ready.Store(false)
	if s.cfg.ShutdownDelay > 0 {
		log.Printf("Shutdown requested; draining for %s before closing", s.cfg.ShutdownDelay)
		time.Sleep(s.cfg.ShutdownDelay)
	}
	log.Println("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	return nil
}
