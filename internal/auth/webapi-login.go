package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type WebAPIAuthenticator struct {
	baseURL    string
	timeout    time.Duration
	httpClient *http.Client
}

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	UserID  string `json:"user_id,omitempty"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	ApiKey   string `json:"api_key"` // Store password for API calls
}

// NewWebAPIAuthenticator creates a new web API authenticator
func NewWebAPIAuthenticator(baseURL string) *WebAPIAuthenticator {
	return &WebAPIAuthenticator{
		baseURL: baseURL,
		timeout: 10 * time.Second,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AuthenticateUser authenticates a user against the web API (with fallback to hardcoded user)
func (w *WebAPIAuthenticator) AuthenticateUser(username, password string) (*User, error) {

	// Try API authentication (username already includes customer_ prefix)
	authReq := AuthRequest{
		Username: username,
		Password: password,
	}

	jsonData, err := json.Marshal(authReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	url := fmt.Sprintf("%s/login", w.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("HTTP request creation failed, falling back to hardcoded check: %v", err)
		return nil, fmt.Errorf("authentication failed")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SFTP-Service/1.0")

	log.Printf("Authenticating user %s against web API: %s", username, url)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		log.Printf("HTTP request failed, API might be down: %v", err)
		return nil, fmt.Errorf("authentication failed: API unavailable")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Authentication failed for user %s: HTTP %d - %s", username, resp.StatusCode, string(body))
		return nil, fmt.Errorf("authentication failed: HTTP %d", resp.StatusCode)
	}

	var authResp AuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auth response: %w", err)
	}

	if !authResp.Success {
		log.Printf("Authentication failed for user %s: %s", username, authResp.Message)
		return nil, fmt.Errorf("authentication failed: %s", authResp.Message)
	}

	log.Printf("Authentication successful for user: %s (ID: %s)", username, authResp.UserID)

	return &User{
		ID:       authResp.UserID,
		Username: username,
		ApiKey:   password, // Store password as API key for subsequent API calls
	}, nil
}

// SetTimeout allows customizing the HTTP timeout
func (w *WebAPIAuthenticator) SetTimeout(timeout time.Duration) {
	w.timeout = timeout
	w.httpClient.Timeout = timeout
}
