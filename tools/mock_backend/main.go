package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	port := flag.Int("port", 9001, "Port to listen on")
	id := flag.String("id", "backend-1", "Backend identifier name")
	flag.Parse()

	// Fallback to environment variables if specified
	if envPort := os.Getenv("PORT"); envPort != "" && *port == 9001 {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}
	if envID := os.Getenv("BACKEND_ID"); envID != "" && *id == "backend-1" {
		*id = envID
	}

	addr := fmt.Sprintf(":%d", *port)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] Received request: %s %s from %s\n", *id, r.Method, r.URL.Path, r.RemoteAddr)
		fmt.Fprintf(w, "Response from %s (port %d)\n", *id, *port)
	})

	log.Printf("Starting mock backend server [%s] on %s\n", *id, addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
