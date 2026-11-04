package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := loadConfig(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	if cfg.ShowVersion {
		fmt.Println(version)
		return
	}
	if cfg.HealthCheck {
		if err := healthCheck(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}
		return
	}

	s, err := newServer(cfg, os.Stdout)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	s.printBanner()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := s.run(ctx); err != nil {
		log.Fatalf("error: %v", err)
	}
	log.Println("Server exited")
}

// healthCheck queries /healthz on the locally running server. It lets the
// container image run a HEALTHCHECK without shipping curl or wget.
func healthCheck(cfg config) error {
	host := cfg.BindAddress
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/healthz"

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return nil
}
