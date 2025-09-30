package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/Rehtest/chirpy-bootdev/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetricsInt(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	dbQueries := database.New(db)

	// Initialize the multiplexer
	mux := http.NewServeMux()

	cfg := &apiConfig{
		dbQueries: dbQueries,
	}

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
		if platform != "dev" {
			respondWithError(w, http.StatusForbidden, "Reset is only allowed in dev environment")
			return
		}

		err := cfg.dbQueries.DeleteAllUsers(r.Context())
		if err != nil {
			log.Printf("Error deleting users: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error deleting users")
			return
		}

		// Reset the fileserver hits counter
		cfg.fileserverHits.Store(0)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Add Handler for Post validation
	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		// Decode the JSON body
		type parameters struct {
			Body   string    `json:"body"`
			UserID uuid.UUID `json:"user_id"`
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

		// Validate the chirp length
		if len(params.Body) > 140 {
			respondWithError(w, http.StatusBadRequest, "Chirp is too long")
			return
		} else if len(params.Body) == 0 {
			respondWithError(w, http.StatusBadRequest, "Chirp is too short")
			return
		} else {
			// Insert new chirp into the database
			chirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
				Body:   cleanText(params.Body),
				UserID: params.UserID,
			})
			if err != nil {
				log.Printf("Error creating chirp: %s", err)
				respondWithError(w, http.StatusInternalServerError, "Error creating chirp")
				return
			}
			log.Printf("Created chirp with ID: %s", chirp.ID)

			// Respond with the chirp details
			type returnChirp struct {
				ID        string `json:"id"`
				CreatedAt string `json:"created_at"`
				UpdatedAt string `json:"updated_at"`
				Body      string `json:"body"`
				UserID    string `json:"user_id"`
			}
			resp := returnChirp{
				ID:        chirp.ID.String(),
				CreatedAt: chirp.CreatedAt.String(),
				UpdatedAt: chirp.UpdatedAt.String(),
				Body:      chirp.Body,
				UserID:    chirp.UserID.String(),
			}

			respondWithJSON(w, http.StatusCreated, resp)
		}
	})

	// Add Handler for User Creation
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		// Decode the JSON body
		type parameters struct {
			Email string `json:"email"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			w.WriteHeader(500)
			return
		}

		// Respond with the user ID
		type User struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Email     string `json:"email"`
		}

		user, err := cfg.dbQueries.CreateUser(r.Context(), params.Email)
		if err != nil {
			log.Printf("Error creating user: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error creating user")
			return
		}

		resp := User{
			ID:        user.ID.String(),
			CreatedAt: user.CreatedAt.String(),
			UpdatedAt: user.UpdatedAt.String(),
			Email:     user.Email,
		}
		respondWithJSON(w, http.StatusCreated, resp)

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

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type returnError struct {
		Error string `json:"error"`
	}
	respBody := returnError{Error: msg}

	dat, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	w.WriteHeader(code)
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(dat)
}
