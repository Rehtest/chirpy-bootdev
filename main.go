package main

import (
	"fmt"
	"log"
	"net/http"
)

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Step 1: Set Content-Type
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	// Step 2: Write status code
	w.WriteHeader(http.StatusOK)

	// Step 3: Write body
	w.Write([]byte("OK"))
}

func main() {
	fmt.Println("Hello, Chirpy Bootdev!")

	// Step 1: Create a new ServeMux
	mux := http.NewServeMux()

	// Step 2: Use the http.FileServer to serve static files from the current directory
	fileServer := http.FileServer(http.Dir("."))

	// Use StripPrefix so that /app/index.html resolves to ./index.html
	mux.Handle("/app/", http.StripPrefix("/app/", fileServer))

	// Step 3: Add readiness endpoint
	mux.HandleFunc("/healthz", readinessHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Step 4: Start the server
	log.Println("Starting server on :8080")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal("Server failed:", err)
	}

}
