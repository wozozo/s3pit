package testutil

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequest represents a test HTTP request
type TestRequest struct {
	Method  string
	Path    string
	Body    []byte
	Headers map[string]string
}

// TestResponse wraps httptest.ResponseRecorder for easier testing
type TestResponse struct {
	*httptest.ResponseRecorder
}

// BodyString returns the response body as a string
func (r *TestResponse) BodyString() string {
	return r.Body.String()
}

// BodyBytes returns the response body as bytes
func (r *TestResponse) BodyBytes() []byte {
	return r.Body.Bytes()
}

// MakeTestRequest performs a test HTTP request against a gin router
func MakeTestRequest(t *testing.T, router *gin.Engine, req TestRequest) *TestResponse {
	var body io.Reader
	if req.Body != nil {
		body = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.Path, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Add headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Create response recorder
	recorder := httptest.NewRecorder()
	
	// Process request
	router.ServeHTTP(recorder, httpReq)

	return &TestResponse{recorder}
}

// AssertStatus checks if the response has the expected status code
func AssertStatus(t *testing.T, resp *TestResponse, expected int) {
	if resp.Code != expected {
		t.Errorf("Expected status %d, got %d", expected, resp.Code)
	}
}

// AssertContains checks if the response body contains the expected string
func AssertContains(t *testing.T, resp *TestResponse, expected string) {
	body := resp.BodyString()
	if !bytes.Contains([]byte(body), []byte(expected)) {
		t.Errorf("Response body does not contain expected string: %s", expected)
	}
}

// AssertHeader checks if a response header has the expected value
func AssertHeader(t *testing.T, resp *TestResponse, header, expected string) {
	actual := resp.Header().Get(header)
	if actual != expected {
		t.Errorf("Expected header %s to be %s, got %s", header, expected, actual)
	}
}