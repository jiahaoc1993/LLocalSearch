package utils

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// MaxResponseBodySize limits response body to 10MB
	MaxResponseBodySize = 10 * 1024 * 1024
	// HTTPClientTimeout is the timeout for HTTP requests
	HTTPClientTimeout = 30 * time.Second
)

func DownloadWebsiteText(url string) (string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: HTTPClientTimeout,
	}
	
	// Make an HTTP GET request to the URL
	resp, err := client.Get(url)
	if err != nil {
		return "", err // Return the error if the request failed
	}
	defer resp.Body.Close()

	// Limit the response body size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, MaxResponseBodySize)
	
	// Read the response body
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err // Return the error if reading the body failed
	}
	
	// Check if we hit the size limit
	if int64(len(body)) >= MaxResponseBodySize {
		return "", fmt.Errorf("response body exceeds maximum size of %d bytes", MaxResponseBodySize)
	}

	// Convert the body to a string and return it
	return string(body), nil
}
