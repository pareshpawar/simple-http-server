package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultTemplateFile = "html/index.html"

// config holds all runtime settings. Every option can be set with an
// environment variable (handy for containers) and overridden by a flag.
type config struct {
	BindAddress   string
	Port          int
	OutputFormat  string
	StaticDir     string
	TemplateFile  string
	EchoEnv       []string
	ShutdownDelay time.Duration
	NoColor       bool

	HealthCheck bool
	ShowVersion bool
}

var validFormats = map[string]bool{"text": true, "html": true, "json": true}

func loadConfig(args []string, getenv func(string) string) (config, error) {
	var cfg config

	envOr := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	port := 8081
	if v := getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid PORT %q: must be a number", v)
		}
		port = p
	}

	var shutdownDelay time.Duration
	if v := getenv("SHUTDOWN_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid SHUTDOWN_DELAY %q: %v", v, err)
		}
		shutdownDelay = d
	}

	var echoEnv string
	fs := flag.NewFlagSet("simple_http_server", flag.ContinueOnError)
	fs.StringVar(&cfg.BindAddress, "bind", envOr("BIND_ADDRESS", "0.0.0.0"), "address to listen on (env BIND_ADDRESS)")
	fs.IntVar(&cfg.Port, "port", port, "port to listen on (env PORT)")
	fs.StringVar(&cfg.OutputFormat, "format", envOr("OUTPUT_FORMAT", "text"), "default response format for /: text, html or json (env OUTPUT_FORMAT)")
	fs.StringVar(&cfg.StaticDir, "static-dir", getenv("STATIC_DIR"), "directory to serve under /static/; disabled when empty (env STATIC_DIR)")
	fs.StringVar(&cfg.TemplateFile, "template", envOr("TEMPLATE_FILE", defaultTemplateFile), "HTML template used by /html/; falls back to the built-in one if the default path is missing (env TEMPLATE_FILE)")
	fs.StringVar(&echoEnv, "echo-env", getenv("ECHO_ENV"), "comma-separated extra environment variables to include in responses, in addition to YOUR_ENV (env ECHO_ENV)")
	fs.DurationVar(&cfg.ShutdownDelay, "shutdown-delay", shutdownDelay, "time to keep serving after SIGTERM while /readyz reports not ready (env SHUTDOWN_DELAY)")
	fs.BoolVar(&cfg.NoColor, "no-color", getenv("NO_COLOR") != "", "disable ANSI colors in logs and text responses (env NO_COLOR)")
	fs.BoolVar(&cfg.HealthCheck, "healthcheck", false, "probe the running server's /healthz endpoint and exit 0 if healthy (for container HEALTHCHECK)")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "print the version and exit")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	cfg.EchoEnv = []string{"YOUR_ENV"}
	for _, name := range strings.Split(echoEnv, ",") {
		if name = strings.TrimSpace(name); name != "" && name != "YOUR_ENV" {
			cfg.EchoEnv = append(cfg.EchoEnv, name)
		}
	}

	cfg.OutputFormat = strings.ToLower(strings.TrimSpace(cfg.OutputFormat))
	if !validFormats[cfg.OutputFormat] {
		return cfg, fmt.Errorf("invalid output format %q: must be text, html or json", cfg.OutputFormat)
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("invalid port %d: must be between 0 and 65535", cfg.Port)
	}
	if cfg.ShutdownDelay < 0 {
		return cfg, fmt.Errorf("invalid shutdown delay %s: must not be negative", cfg.ShutdownDelay)
	}
	if cfg.StaticDir != "" && !cfg.HealthCheck {
		info, err := os.Stat(cfg.StaticDir)
		if err != nil {
			return cfg, fmt.Errorf("static dir: %v", err)
		}
		if !info.IsDir() {
			return cfg, fmt.Errorf("static dir %q is not a directory", cfg.StaticDir)
		}
	}
	return cfg, nil
}
