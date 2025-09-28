package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInt(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Initialize the multiplexer
	mux := http.NewServeMux()

	cfg := &apiConfig{}

	// Add file server for static files
	fileServer := http.FileServer(http.Dir("."))

	// Add Handler for root path
	mux.Handle("/app/", cfg.middlewareMetricsInt(http.StripPrefix("/app/", fileServer)))

	// Add Handler for Health path
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Add Handler for Metrics path
	mux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		response_text := fmt.Sprintf(`
		<html>
			<body>
				<h1>Welcome, Chirpy Admin</h1>
				<p>Chirpy has been visited %d times!</p>
			</body>
		</html>`,
			cfg.fileserverHits.Load())
		w.Write([]byte(response_text))
	})

	// Add Handler for Reset path
	mux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Store(0)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Add Handler for Post validation
	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		// Decode the JSON body
		type parameters struct {
			Body string `json:"body"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			w.WriteHeader(500)
			return
		}

		// Encode the response
		type returnSuccess struct {
			Body string `json:"cleaned_body"`
		}
		type returnError struct {
			Error string `json:"error"`
		}

		var respBody any
		if len(params.Body) > 140 {
			respBody = returnError{Error: "Chirp is too long"}
			w.WriteHeader(http.StatusBadRequest)
		} else {
			respText := cleanText(params.Body)
			respBody = returnSuccess{Body: respText}
			w.WriteHeader(http.StatusOK)
		}

		dat, err := json.Marshal(respBody)
		if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(dat)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Start the server
	server.ListenAndServe()
}

func cleanText(input string) string {
	wordsToRemove := []string{"kerfuffle", "sharbert", "fornax"}
	cleanedText := strings.Split(input, " ")
	for i, word := range cleanedText {
		for _, removeWord := range wordsToRemove {
			if strings.ToLower(word) == removeWord {
				cleanedText[i] = "****"
			}
		}
	}
	return strings.Join(cleanedText, " ")
}
