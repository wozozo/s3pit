package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/server"
)

func TestServeWithDelayConfiguration(t *testing.T) {
	t.Skip("Integration test - requires server setup")
	tests := []struct {
		name          string
		cfg           *config.Config
		method        string
		path          string
		expectedMinMs int
		expectedMaxMs int
	}{
		{
			name: "Fixed read delay",
			cfg: &config.Config{
				Host:            "127.0.0.1",
				Port:            0, // Use random port
				GlobalDir:       t.TempDir(),
				AuthMode:        "sigv4",
				InMemory:        true,
				EnableDashboard: false,
				ReadDelayMs:     100,
				WriteDelayMs:    0,
			},
			method:        "GET",
			path:          "/",
			expectedMinMs: 90,
			expectedMaxMs: 120,
		},
		{
			name: "Fixed write delay",
			cfg: &config.Config{
				Host:             "127.0.0.1",
				Port:             0,
				GlobalDir:        t.TempDir(),
				AuthMode:         "sigv4",
				InMemory:         true,
				EnableDashboard:  false,
				AutoCreateBucket: true,
				ReadDelayMs:      0,
				WriteDelayMs:     150,
			},
			method:        "PUT",
			path:          "/test-bucket",
			expectedMinMs: 140,
			expectedMaxMs: 170,
		},
		{
			name: "Random read delay",
			cfg: &config.Config{
				Host:               "127.0.0.1",
				Port:               0,
				GlobalDir:          t.TempDir(),
				AuthMode:           "sigv4",
				InMemory:           true,
				EnableDashboard:    false,
				ReadDelayRandomMin: 50,
				ReadDelayRandomMax: 100,
			},
			method:        "GET",
			path:          "/",
			expectedMinMs: 45,
			expectedMaxMs: 110,
		},
		{
			name: "No delay",
			cfg: &config.Config{
				Host:            "127.0.0.1",
				Port:            0,
				GlobalDir:       t.TempDir(),
				AuthMode:        "sigv4",
				InMemory:        true,
				EnableDashboard: false,
			},
			method:        "GET",
			path:          "/",
			expectedMinMs: 0,
			expectedMaxMs: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start server with test configuration
			srv, err := server.New(tt.cfg)
			require.NoError(t, err)

			// Start server in background
			serverURL := startTestServer(t, srv)

			// Make request and measure delay
			start := time.Now()
			resp, err := makeRequest(tt.method, serverURL+tt.path)
			elapsed := time.Since(start).Milliseconds()

			require.NoError(t, err)
			defer resp.Body.Close()

			// Check delay is within expected range
			assert.GreaterOrEqual(t, int(elapsed), tt.expectedMinMs,
				"Request should take at least %dms, took %dms", tt.expectedMinMs, elapsed)
			assert.LessOrEqual(t, int(elapsed), tt.expectedMaxMs,
				"Request should take at most %dms, took %dms", tt.expectedMaxMs, elapsed)
		})
	}
}

func TestServeWithRandomDelayVariation(t *testing.T) {
	t.Skip("Integration test - requires server setup")
	cfg := &config.Config{
		Host:               "127.0.0.1",
		Port:               0,
		GlobalDir:          t.TempDir(),
		AuthMode:           "sigv4",
		InMemory:           true,
		EnableDashboard:    false,
		ReadDelayRandomMin: 20,
		ReadDelayRandomMax: 80,
	}

	srv, err := server.New(cfg)
	require.NoError(t, err)

	serverURL := startTestServer(t, srv)

	// Make multiple requests to verify randomness
	delays := make([]int64, 10)
	for i := 0; i < 10; i++ {
		start := time.Now()
		resp, err := makeRequest("GET", serverURL+"/")
		delays[i] = time.Since(start).Milliseconds()

		require.NoError(t, err)
		resp.Body.Close()

		// Each delay should be within range
		assert.GreaterOrEqual(t, int(delays[i]), 15, "Delay should be >= min")
		assert.LessOrEqual(t, int(delays[i]), 90, "Delay should be <= max")
	}

	// Check that we got different delays (verifying randomness)
	uniqueDelays := make(map[int64]bool)
	for _, d := range delays {
		uniqueDelays[d/10] = true // Group by 10ms buckets
	}
	assert.Greater(t, len(uniqueDelays), 2, "Should have at least 3 different delay values")
}

func TestDelayDoesNotAffectDashboard(t *testing.T) {
	t.Skip("Integration test - requires server setup")
	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		GlobalDir:       t.TempDir(),
		AuthMode:        "sigv4",
		InMemory:        true,
		EnableDashboard: true,
		ReadDelayMs:     200, // High delay for regular requests
	}

	srv, err := server.New(cfg)
	require.NoError(t, err)

	serverURL := startTestServer(t, srv)

	// Dashboard request should not be delayed
	start := time.Now()
	resp, err := makeRequest("GET", serverURL+"/dashboard")
	elapsed := time.Since(start).Milliseconds()

	require.NoError(t, err)
	defer resp.Body.Close()

	// Dashboard should respond quickly despite delay configuration
	assert.Less(t, int(elapsed), 50, "Dashboard should not be delayed")
}

func TestEnvironmentVariableConfiguration(t *testing.T) {
	// Set environment variables
	t.Setenv("S3PIT_READ_DELAY_MS", "75")
	t.Setenv("S3PIT_WRITE_DELAY_MS", "125")

	cfg := config.LoadFromEnv()

	assert.Equal(t, 75, cfg.ReadDelayMs)
	assert.Equal(t, 125, cfg.WriteDelayMs)
}

func TestEnvironmentVariableRandomConfiguration(t *testing.T) {
	// Set environment variables for random delays
	t.Setenv("S3PIT_READ_DELAY_RANDOM_MIN_MS", "100")
	t.Setenv("S3PIT_READ_DELAY_RANDOM_MAX_MS", "200")
	t.Setenv("S3PIT_WRITE_DELAY_RANDOM_MIN_MS", "300")
	t.Setenv("S3PIT_WRITE_DELAY_RANDOM_MAX_MS", "400")

	cfg := config.LoadFromEnv()

	assert.Equal(t, 100, cfg.ReadDelayRandomMin)
	assert.Equal(t, 200, cfg.ReadDelayRandomMax)
	assert.Equal(t, 300, cfg.WriteDelayRandomMin)
	assert.Equal(t, 400, cfg.WriteDelayRandomMax)
}

// Helper functions

func startTestServer(t testing.TB, srv *server.Server) string {
	// Use a random port by setting it to 0
	if srv.GetConfig().Port == 0 {
		// Find a free port
		srv.GetConfig().Port = findFreePort(t)
	}

	// Start server in background
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Return the server URL
	return fmt.Sprintf("http://127.0.0.1:%d", srv.GetConfig().Port)
}

func findFreePort(t testing.TB) int {
	// Find a free port by binding to port 0
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)

	listener, err := net.ListenTCP("tcp", addr)
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func makeRequest(method, url string) (*http.Response, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	// Add basic auth for authenticated endpoints
	if method != "GET" || !isPublicEndpoint(url) {
		req.SetBasicAuth("testuser", "testpass")
	}

	return client.Do(req)
}

func isPublicEndpoint(url string) bool {
	// Dashboard and static files are public
	return false // Simplified for testing
}

// Benchmark tests

func BenchmarkServerWithDelay(b *testing.B) {
	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		GlobalDir:       b.TempDir(),
		AuthMode:        "sigv4",
		InMemory:        true,
		EnableDashboard: false,
		ReadDelayMs:     10,
	}

	srv, err := server.New(cfg)
	require.NoError(b, err)

	serverURL := startTestServer(b, srv)
	client := &http.Client{Timeout: 5 * time.Second}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(serverURL + "/")
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

func BenchmarkServerWithoutDelay(b *testing.B) {
	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		GlobalDir:       b.TempDir(),
		AuthMode:        "sigv4",
		InMemory:        true,
		EnableDashboard: false,
	}

	srv, err := server.New(cfg)
	require.NoError(b, err)

	serverURL := startTestServer(b, srv)
	client := &http.Client{Timeout: 5 * time.Second}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(serverURL + "/")
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}
