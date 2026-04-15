package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/common-nighthawk/go-figure"
	"pareshpawar.com/simple-http-server/utils"
)

var (
	cachedOutboundIP string
	indexTemplate    *template.Template
)

func main() {
	// Resolve outbound IP once at startup
	ip, err := utils.GetMyOutboundIP()
	if err != nil {
		log.Printf("Warning: could not determine outbound IP: %v", err)
		cachedOutboundIP = "unavailable"
	} else {
		cachedOutboundIP = ip.String()
	}

	// Parse HTML template once at startup
	file, err := os.ReadFile("html/index.html")
	if err != nil {
		log.Printf("Warning: could not read html/index.html: %v (the /html/ endpoint will be unavailable)", err)
	} else {
		tmpl, err := template.New("webpage").Parse(string(file))
		if err != nil {
			log.Printf("Warning: could not parse html/index.html: %v (the /html/ endpoint will be unavailable)", err)
		} else {
			indexTemplate = tmpl
		}
	}

	http.HandleFunc("/", handler)
	http.HandleFunc("/html/", htmlhandler)
	http.HandleFunc("/healthcheck", healthhandler)

	serverBrand := figure.NewColorFigure("Simple HTTP Server", "straight", "green", true)
	serverBrand.Print()
	myBrand := figure.NewColorFigure("by PareshPawar.com", "term", "green", true)
	myBrand.Print()
	log.Print("pareshpawar/simple-http-server: Simple HTTP Server Running on port 8081")

	srv := &http.Server{
		Addr:         "0.0.0.0:8081",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	log.Println("Server exited")
}

func logRequest(r *http.Request) {
	colors := map[string]string{
		"GET":    "\033[34m",
		"POST":   "\033[33m",
		"PUT":    "\033[35m",
		"DELETE": "\033[31m",
	}
	color, ok := colors[r.Method]
	if !ok {
		color = "\033[36m"
	}
	fmt.Printf("%s%s %s %s %s  ===> from %s\033[0m\n",
		color, time.Now().Local(), r.Method, r.URL, r.Proto, r.RemoteAddr)
}

func htmlhandler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	if indexTemplate == nil {
		http.Error(w, "HTML template not available", http.StatusInternalServerError)
		return
	}

	timestamp := time.Now()
	type reqDataStruct struct{ ReqTime, ReqType, Host, Remote, RemoteAddr string }
	reqData := reqDataStruct{
		ReqTime:    timestamp.String(),
		ReqType:    r.Proto + " " + r.Method + " " + r.URL.Path,
		Host:       r.Host,
		Remote:     r.RemoteAddr,
		RemoteAddr: cachedOutboundIP,
	}

	data := struct {
		Request reqDataStruct
		Headers http.Header
	}{
		Request: reqData,
		Headers: r.Header,
	}

	if err := indexTemplate.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	logRequest(r)

	timestamp := time.Now()
	fmt.Fprintf(w, "Request Time\t==> %s\n", timestamp)
	fmt.Fprintf(w, "Request Type\t==> %s %s %s\n", r.Method, r.URL, r.Proto)
	fmt.Fprintf(w, "Hostname/Host \t==> %s\n", r.Host)
	fmt.Fprintf(w, "Remote Address \t==> %s\n", r.RemoteAddr)
	fmt.Fprintf(w, "Local Address \t==> %s\n\n", cachedOutboundIP)

	// print request headers
	for key, value := range r.Header {
		fmt.Fprintf(w, "Header[%q] = %s\n", key, value)
	}

	// log form errors
	if err := r.ParseForm(); err != nil {
		log.Print(err)
	}

	// print form data key-value pairs
	for key, value := range r.Form {
		fmt.Fprintf(w, "FormData[%q] = %q\n", key, value)
	}

	// print the environment variable
	fmt.Fprintf(w, "\nYOUR_ENV = %s\n", os.Getenv("YOUR_ENV"))

	// print brand
	serverBrand := figure.NewColorFigure("Simple HTTP Server", "straight", "green", true)
	fmt.Fprintf(w, "__________________________________________________________\n")
	figure.Write(w, serverBrand)
	fmt.Fprintf(w, "----------------------------------------------------------\n")
	fmt.Fprintf(w, "                    by PareshPawar.com                    \n")
	fmt.Fprintf(w, "----------------------------------------------------------\n")
}

func healthhandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK\n")
}
