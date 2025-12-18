package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type FileInfo struct {
	Name         string
	Size         int64
	LastModified time.Time
	IsDir        bool
}

// DownloadPricelist fetches pricelist data from the web API with all parameters
func DownloadPricelist(baseURL, username, apiKey, remotePath string) ([]byte, error) {
	// Only allow access to the specific pricelist file
	if remotePath != "/Hinnat/salhydro_kaikki.zip" && remotePath != "salhydro_kaikki.zip" {
		return nil, fmt.Errorf("access denied: only salhydro_kaikki.zip is available")
	}

	// Create local HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	url := fmt.Sprintf("%s/pricelist", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add API key from user (password) to request headers
	req.Header.Set("X-ApiKey", apiKey)
	req.Header.Set("User-Agent", "SFTP-Service/1.0")

	log.Printf("Downloading pricelist for user %s from web API: %s", username, url)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed: HTTP %d", resp.StatusCode)
	}

	// Read the response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	log.Printf("Successfully downloaded pricelist: %d bytes", len(data))
	return data, nil
}

type OrderRequest struct {
	Username  string `json:"username"`
	Filename  string `json:"filename"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
	FileSize  int    `json:"file_size"`
}

// SendOrderToAPI sends the order data to the HTTP API with all parameters
func SendOrderToAPI(apiURL, username, apiKey, filename, content string) error {
	// Check file size limit (100KB = 102400 bytes)
	if len(content) > 102400 {
		return fmt.Errorf("file size exceeds 100KB limit")
	}

	// Generate timestamp for the order
	timestamp := time.Now().Format("20060102_150405")

	// Create local HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	orderReq := OrderRequest{
		Username:  username,
		Filename:  filename,
		Content:   content,
		Timestamp: timestamp,
		FileSize:  len(content),
	}

	jsonData, err := json.Marshal(orderReq)
	if err != nil {
		return fmt.Errorf("failed to marshal order: %w", err)
	}

	url := fmt.Sprintf("%s/order", apiURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SFTP-Service/1.0")
	req.Header.Set("X-ApiKey", apiKey)

	log.Printf("Sending order to API: %s (user: %s, file: %s)", url, username, filename)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API request failed: HTTP %d - %s", resp.StatusCode, string(body))
	}

	log.Printf("Order successfully sent to API: %s", string(body))
	log.Printf("Successfully processed incoming order: %s/%s (%d bytes)", username, filename, len(content))
	return nil
}
