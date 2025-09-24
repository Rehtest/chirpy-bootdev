package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	fmt.Println("Hello, Chirpy Bootdev!")

	// Step 1: Create a new ServeMux
	mux := http.NewServeMux()

	// Step 2: Use the http.FileServer to serve static files from the current directory
	fileServer := http.FileServer(http.Dir("."))

	// Step 3: Use Handle() method to add a handler for the root path "/"
	mux.Handle("/", fileServer)

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
