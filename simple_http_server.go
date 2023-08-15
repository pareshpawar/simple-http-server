package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/common-nighthawk/go-figure"
)

func main() {
	http.HandleFunc("/", handler)
	serverBrand := figure.NewColorFigure("Simple HTTP Server", "straight", "green", true)
	serverBrand.Print()
	myBrand := figure.NewColorFigure("by PareshPawar.com", "term", "green", true)
	myBrand.Print()
	log.Print("pareshpawar/simple-http-server: Simple HTTP Server Running on port 8081")
	log.Fatal(http.ListenAndServe("0.0.0.0:8081", nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	timestamp := time.Now()
	if r.Method == "GET" {
		fmt.Print(string("\033[34m"))
	} else if r.Method == "POST" {
		fmt.Print(string("\033[33m"))
	} else if r.Method == "PUT" {
		fmt.Print(string("\033[35m"))
	} else if r.Method == "DELETE" {
		fmt.Print(string("\033[31m"))
	} else {
		fmt.Print(string("\033[36m"))
	}
	fmt.Printf("%s %s %s %s  ===> from %s\n", timestamp.Local(), r.Method, r.URL, r.Proto, r.RemoteAddr)
	fmt.Fprintf(w, "Request Time	==> %s\n", timestamp)
	fmt.Fprintf(w, "Request Type	==> %s %s %s\n", r.Method, r.URL, r.Proto)
	fmt.Fprintf(w, "Hostname/Host 	==> %s\n", r.Host)
	fmt.Fprintf(w, "Remote Address 	==> %s\n", r.RemoteAddr)
	fmt.Fprintf(w, "Local Address 	==> %s\n\n", GetOutboundIP())

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

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}
