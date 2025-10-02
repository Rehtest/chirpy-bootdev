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
	"time"

	"github.com/Rehtest/chirpy-bootdev/internal/auth"
	"github.com/Rehtest/chirpy-bootdev/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	secretKey      string
}

type returnChirp struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Body      string `json:"body"`
	UserID    string `json:"user_id"`
}

type userCreation struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	IsChirpyRed bool   `json:"is_chirpy_red"`
}

type userLogin struct {
	userCreation
	ExpiresIn time.Duration `json:"expires_in_seconds"`
}

type userCreationResponse struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Email       string `json:"email"`
	IsChirpyRed bool   `json:"is_chirpy_red"`
}

type userLoginResponse struct {
	ID           string `json:"id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Email        string `json:"email"`
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	IsChirpyRed  bool   `json:"is_chirpy_red"`
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
	secretKey := os.Getenv("SECRET_KEY")

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
		platform:  platform,
		secretKey: secretKey,
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

	// Add Handler for Get Chirps
	mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		chirps, err := cfg.dbQueries.GetChirpsByAscendingCreatedAt(r.Context())
		if err != nil {
			log.Printf("Error getting chirps: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error getting chirps")
			return
		}

		var resp []returnChirp
		for _, chirp := range chirps {
			resp = append(resp, returnChirp{
				ID:        chirp.ID.String(),
				CreatedAt: chirp.CreatedAt.String(),
				UpdatedAt: chirp.UpdatedAt.String(),
				Body:      chirp.Body,
				UserID:    chirp.UserID.String(),
			})
		}

		respondWithJSON(w, http.StatusOK, resp)
	})

	// Add Handler to Get specific chirp by ID
	mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		// Extract the chirp ID from the URL
		chirpID := r.PathValue("chirpID")
		chirpUUID, err := uuid.Parse(chirpID)
		if err != nil {
			log.Printf("Error parsing chirp ID: %s", err)
			respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
			return
		}

		// Fetch the chirp from the database
		chirp, err := cfg.dbQueries.GetChirpByID(r.Context(), chirpUUID)
		if err != nil {
			log.Printf("Error getting chirp: %s", err)
			respondWithError(w, http.StatusNotFound, "Error getting chirp")
			return
		}

		// Respond with the chirp details
		resp := returnChirp{
			ID:        chirp.ID.String(),
			CreatedAt: chirp.CreatedAt.String(),
			UpdatedAt: chirp.UpdatedAt.String(),
			Body:      chirp.Body,
			UserID:    chirp.UserID.String(),
		}

		respondWithJSON(w, http.StatusOK, resp)
	})

	// Add Handler for Post validation
	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
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

		// Validate the Bearer token
		token, err := auth.GetBearerToken(r)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		userID, err := auth.ValidateJWT(token, cfg.secretKey)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
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
				UserID: userID,
			})
			if err != nil {
				log.Printf("Error creating chirp: %s", err)
				respondWithError(w, http.StatusInternalServerError, "Error creating chirp")
				return
			}
			log.Printf("Created chirp with ID: %s", chirp.ID)

			// Respond with the chirp details
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

	// Add Handler to Delete a chirp by ID
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		// Extract the chirp ID from the URL
		chirpID := r.PathValue("chirpID")
		chirpUUID, err := uuid.Parse(chirpID)
		if err != nil {
			log.Printf("Error parsing chirp ID: %s", err)
			respondWithError(w, http.StatusBadRequest, "Invalid chirp ID")
			return
		}

		// Validate the Bearer token
		token, err := auth.GetBearerToken(r)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		userID, err := auth.ValidateJWT(token, cfg.secretKey)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// Fetch the chirp from the database to verify ownership
		chirp, err := cfg.dbQueries.GetChirpByID(r.Context(), chirpUUID)
		if err != nil {
			log.Printf("Error getting chirp: %s", err)
			respondWithError(w, http.StatusNotFound, "Chirp not found")
			return
		}

		if chirp.UserID != userID {
			respondWithError(w, http.StatusForbidden, "You do not have permission to delete this chirp")
			return
		}

		// Delete the chirp from the database
		err = cfg.dbQueries.DeleteChirp(r.Context(), chirpUUID)
		if err != nil {
			log.Printf("Error deleting chirp: %s", err)
			respondWithError(w, http.StatusNotFound, "Chirp not found")
			return
		}

		respondWithJSON(w, http.StatusNoContent, nil)
	})

	// Add Handler for User Creation
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		// Store the parameters in a userCreation struct
		params := userCreation{}

		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			w.WriteHeader(500)
			return
		}

		hashedPassword, err := auth.HashPassword(params.Password)
		if err != nil {
			log.Printf("Error hashing password: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error creating user")
			return
		}

		// Insert new user into the database
		createParams := database.CreateUserParams{
			Email:          strings.ToLower(params.Email),
			HashedPassword: sql.NullString{String: hashedPassword, Valid: true},
		}

		user, err := cfg.dbQueries.CreateUser(r.Context(), createParams)
		if err != nil {
			log.Printf("Error creating user: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error creating user")
			return
		}

		resp := userCreationResponse{
			ID:          user.ID.String(),
			CreatedAt:   user.CreatedAt.String(),
			UpdatedAt:   user.UpdatedAt.String(),
			Email:       user.Email,
			IsChirpyRed: user.IsChirpyRed,
		}
		respondWithJSON(w, http.StatusCreated, resp)

	})

	// Add Handler for User Login
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		// Store the parameters in a userLogin struct
		params := userLogin{}

		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			w.WriteHeader(500)
			return
		}

		user, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
			return
		}

		valid, err := auth.CheckPasswordHash(params.Password, user.HashedPassword.String)
		if err != nil || !valid {
			respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
			return
		}

		token, err := auth.MakeJWT(user.ID, cfg.secretKey)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error creating JWT")
			return
		}

		refreshToken, err := auth.MakeRefreshToken()
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error creating refresh token")
			return
		}

		// Store the refresh token in the database
		_, err = cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
			Token:  refreshToken,
			UserID: user.ID,
		})
		if err != nil {
			log.Printf("Error storing refresh token: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error creating refresh token")
			return
		}

		log.Printf("User %s logged in, issued JWT and refresh token", user.Email)

		// Respond with the user details and JWT

		resp := userLoginResponse{
			ID:           user.ID.String(),
			CreatedAt:    user.CreatedAt.String(),
			UpdatedAt:    user.UpdatedAt.String(),
			Email:        user.Email,
			Token:        token,
			RefreshToken: refreshToken,
			IsChirpyRed:  user.IsChirpyRed,
		}

		respondWithJSON(w, http.StatusOK, resp)
	})

	// Add Handler for user email and password update
	mux.HandleFunc("PUT /api/users", func(w http.ResponseWriter, r *http.Request) {
		// Store the parameters in a userUpdateParams struct
		type userUpdateParams struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		params := userUpdateParams{}

		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			w.WriteHeader(500)
			return
		}

		accessToken, err := auth.GetBearerToken(r)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		userID, err := auth.ValidateJWT(accessToken, cfg.secretKey)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		hashedPassword, err := auth.HashPassword(params.Password)
		if err != nil {
			log.Printf("Error hashing password: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error updating user")
			return
		}

		// Update the user's email and password
		updateParams := database.UpdateUserEmailAndPasswordParams{
			ID:             userID,
			Email:          strings.ToLower(params.Email),
			HashedPassword: sql.NullString{String: hashedPassword, Valid: true},
		}

		updatedUser, err := cfg.dbQueries.UpdateUserEmailAndPassword(r.Context(), updateParams)
		if err != nil {
			log.Printf("Error updating user: %s", err)
			respondWithError(w, http.StatusInternalServerError, "Error updating user")
			return
		}

		resp := userCreationResponse{
			ID:          updatedUser.ID.String(),
			CreatedAt:   updatedUser.CreatedAt.String(),
			UpdatedAt:   updatedUser.UpdatedAt.String(),
			Email:       updatedUser.Email,
			IsChirpyRed: updatedUser.IsChirpyRed,
		}

		respondWithJSON(w, http.StatusOK, resp)
	})

	// Add Handler for Token Refresh
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.GetBearerToken(r)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		user, err := cfg.dbQueries.GetUserFromRefreshToken(r.Context(), token)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		newToken, err := auth.MakeJWT(user.ID, cfg.secretKey)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error creating JWT")
			return
		}

		// Respond with the new JWT
		respondWithJSON(w, http.StatusOK, map[string]string{"token": newToken})
	})

	// Add handler for revoke token
	mux.HandleFunc("POST /api/revoke", func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.GetBearerToken(r)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Missing or invalid token")
			return
		}

		err = cfg.dbQueries.RevokeRefreshToken(r.Context(), token)
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// Respond with success
		respondWithJSON(w, http.StatusNoContent, map[string]string{"status": "success"})
	})

	// Add handler for webhook to upgrade user to chirpy red
	mux.HandleFunc("POST /api/polka/webhooks", func(w http.ResponseWriter, r *http.Request) {
		type webhookParams struct {
			Event string `json:"event"`
			Data  struct {
				UserID string `json:"user_id"`
			} `json:"data"`
		}
		params := webhookParams{}

		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			w.WriteHeader(500)
			return
		}

		if params.Event != "user.upgraded" {
			respondWithJSON(w, http.StatusNoContent, nil)
		}

		// Upgrade the user to chirpy red
		userUUID, err := uuid.Parse(params.Data.UserID)
		if err != nil {
			log.Printf("Error parsing user ID: %s", err)
			respondWithError(w, http.StatusBadRequest, "Invalid user ID")
			return
		}

		updatedUser, err := cfg.dbQueries.UpgradeUserToChirpyRed(r.Context(), userUUID)
		if err != nil {
			log.Printf("Error upgrading user: %s", err)
			respondWithError(w, http.StatusNotFound, "User not found")
			return
		}

		log.Printf("Upgraded user %s to Chirpy Red", updatedUser.Email)
		respondWithJSON(w, http.StatusNoContent, nil)
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
	w.WriteHeader(code)
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
