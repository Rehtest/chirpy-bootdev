package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Step 1: Set Content-Type
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	// Step 2: Write status code
	w.WriteHeader(http.StatusOK)

	// Step 3: Write body
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) getFileServerHits() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		metrics := fmt.Sprintf("Hits: %d\n", cfg.fileserverHits.Load())
		w.Write([]byte(metrics))
	})
}

func (cfg *apiConfig) resetFileServerHits() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		metrics := fmt.Sprintf("Hits reset to 0 from %d\n", cfg.fileserverHits.Load())
		cfg.fileserverHits.Store(0)
		w.Write([]byte(metrics))
	})
}

func main() {
	fmt.Println("Hello, Chirpy Bootdev!")

	// Step 1: Create a new ServeMux
	mux := http.NewServeMux()

	// Step 2: Use the http.FileServer to serve static files from the current directory
	fileServer := http.FileServer(http.Dir("."))

	// Step 3: Initialize config for middleware
	cfg := &apiConfig{}

	// Use StripPrefix so that /app/index.html resolves to ./index.html
	appHandler := http.StripPrefix("/app/", fileServer)
	mux.Handle("/app/", cfg.middlewareMetricsInc(appHandler))

	// Step 4: Add readiness endpoint
	mux.HandleFunc("/healthz", readinessHandler)

	// Step 5: Add metrics endpoint
	mux.Handle("/metrics", cfg.getFileServerHits())

	// Step 6: Add reset metrics endpoint
	mux.Handle("/reset", cfg.resetFileServerHits())

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
