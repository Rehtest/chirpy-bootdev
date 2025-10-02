package auth

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestHashPasswod(t *testing.T) {
	password := "mysecretpassword"
	hashedPassword, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Error hashing password: %v", err)
	}

	match, err := CheckPasswordHash(password, hashedPassword)
	if err != nil {
		t.Fatalf("Error checking password hash: %v", err)
	}
	if !match {
		t.Fatalf("Password and hash do not match")
	}
}

func TestJWT(t *testing.T) {
	userID := "550e8400-e29b-41d4-a716-446655440000"
	token, err := MakeJWT(uuid.MustParse(userID), "mysecret")
	if err != nil {
		t.Fatalf("Error creating JWT: %v", err)
	}

	parsedUserID, err := ValidateJWT(token, "mysecret")
	if err != nil {
		t.Fatalf("Error validating JWT: %v", err)
	}

	if parsedUserID != uuid.MustParse(userID) {
		t.Fatalf("Parsed user ID does not match original: %v != %v", parsedUserID, userID)
	}
}

func TestInvalidJWT(t *testing.T) {
	_, err := ValidateJWT("invalid.token.here", "mysecret")
	if err == nil {
		t.Fatalf("Expected error validating invalid JWT, got nil")
	}
}

func TestBearerToken(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("Error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer mytoken")
	token, err := GetBearerToken(req)
	if err != nil {
		t.Fatalf("Error getting bearer token: %v", err)
	}
	if token != "mytoken" {
		t.Fatalf("Expected token 'mytoken', got '%s'", token)
	}
}
