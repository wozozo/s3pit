package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/wozozo/s3pit/internal/config"
)

func TestCalculateDelay(t *testing.T) {
	tests := []struct {
		name        string
		fixedMs     int
		randomMinMs int
		randomMaxMs int
		expectMin   int
		expectMax   int
	}{
		{
			name:        "Fixed delay only",
			fixedMs:     100,
			randomMinMs: 0,
			randomMaxMs: 0,
			expectMin:   100,
			expectMax:   100,
		},
		{
			name:        "Random delay range",
			fixedMs:     0,
			randomMinMs: 50,
			randomMaxMs: 150,
			expectMin:   50,
			expectMax:   150,
		},
		{
			name:        "Random takes precedence over fixed",
			fixedMs:     200,
			randomMinMs: 50,
			randomMaxMs: 150,
			expectMin:   50,
			expectMax:   150,
		},
		{
			name:        "Same min and max",
			fixedMs:     0,
			randomMinMs: 100,
			randomMaxMs: 100,
			expectMin:   100,
			expectMax:   100,
		},
		{
			name:        "No delay configured",
			fixedMs:     0,
			randomMinMs: 0,
			randomMaxMs: 0,
			expectMin:   0,
			expectMax:   0,
		},
		{
			name:        "Invalid random range (min > max)",
			fixedMs:     0,
			randomMinMs: 150,
			randomMaxMs: 50,
			expectMin:   0,
			expectMax:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test multiple times for random delays
			for i := 0; i < 20; i++ {
				delay := calculateDelay(tt.fixedMs, tt.randomMinMs, tt.randomMaxMs)
				assert.GreaterOrEqual(t, delay, tt.expectMin, "Delay should be >= expectMin")
				assert.LessOrEqual(t, delay, tt.expectMax, "Delay should be <= expectMax")
			}
		})
	}
}

func TestIsReadOperation(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		expected bool
	}{
		{
			name:     "GET request is read",
			method:   "GET",
			path:     "/bucket/object",
			expected: true,
		},
		{
			name:     "HEAD request is read",
			method:   "HEAD",
			path:     "/bucket",
			expected: true,
		},
		{
			name:     "PUT request is write",
			method:   "PUT",
			path:     "/bucket/object",
			expected: false,
		},
		{
			name:     "DELETE request is write",
			method:   "DELETE",
			path:     "/bucket/object",
			expected: false,
		},
		{
			name:     "POST request is write",
			method:   "POST",
			path:     "/bucket",
			expected: false,
		},
		{
			name:     "Unknown method defaults to read",
			method:   "OPTIONS",
			path:     "/bucket",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tt.method, tt.path, nil)

			result := isReadOperation(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetOperationType(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		expected OperationType
	}{
		{
			name:     "GET returns READ",
			method:   "GET",
			path:     "/bucket/object",
			expected: OpTypeRead,
		},
		{
			name:     "HEAD returns READ",
			method:   "HEAD",
			path:     "/bucket",
			expected: OpTypeRead,
		},
		{
			name:     "PUT returns WRITE",
			method:   "PUT",
			path:     "/bucket/object",
			expected: OpTypeWrite,
		},
		{
			name:     "DELETE returns WRITE",
			method:   "DELETE",
			path:     "/bucket",
			expected: OpTypeWrite,
		},
		{
			name:     "POST returns WRITE",
			method:   "POST",
			path:     "/bucket",
			expected: OpTypeWrite,
		},
		{
			name:     "Unknown method defaults to READ",
			method:   "OPTIONS",
			path:     "/bucket",
			expected: OpTypeRead,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tt.method, tt.path, nil)

			result := GetOperationType(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDelayMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		readDelayMs   int
		writeDelayMs  int
		expectedMinMs int
		expectedMaxMs int
		shouldSkip    bool
	}{
		{
			name:          "Read operation with read delay",
			method:        "GET",
			path:          "/bucket/object",
			readDelayMs:   100,
			writeDelayMs:  200,
			expectedMinMs: 90, // Allow some tolerance
			expectedMaxMs: 110,
		},
		{
			name:          "Write operation with write delay",
			method:        "PUT",
			path:          "/bucket/object",
			readDelayMs:   100,
			writeDelayMs:  200,
			expectedMinMs: 190,
			expectedMaxMs: 210,
		},
		{
			name:          "Dashboard request skips delay",
			method:        "GET",
			path:          "/dashboard",
			readDelayMs:   100,
			writeDelayMs:  200,
			expectedMinMs: 0,
			expectedMaxMs: 10,
			shouldSkip:    true,
		},
		{
			name:          "Static files skip delay",
			method:        "GET",
			path:          "/static/app.js",
			readDelayMs:   100,
			writeDelayMs:  200,
			expectedMinMs: 0,
			expectedMaxMs: 10,
			shouldSkip:    true,
		},
		{
			name:          "Health check skips delay",
			method:        "GET",
			path:          "/health",
			readDelayMs:   100,
			writeDelayMs:  200,
			expectedMinMs: 0,
			expectedMaxMs: 10,
			shouldSkip:    true,
		},
		{
			name:          "No delay configured",
			method:        "GET",
			path:          "/bucket/object",
			readDelayMs:   0,
			writeDelayMs:  0,
			expectedMinMs: 0,
			expectedMaxMs: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			// Create server with test config
			cfg := &config.Config{
				ReadDelayMs:  tt.readDelayMs,
				WriteDelayMs: tt.writeDelayMs,
			}
			s := &Server{config: cfg}

			// Create test context
			w := httptest.NewRecorder()
			c, router := gin.CreateTestContext(w)

			// Add middleware and test handler
			router.Use(s.delayMiddleware())
			router.Any("/*path", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			// Create request
			c.Request = httptest.NewRequest(tt.method, tt.path, nil)

			// Measure execution time
			start := time.Now()
			router.ServeHTTP(w, c.Request)
			elapsed := time.Since(start).Milliseconds()

			// Check delay was applied correctly
			assert.GreaterOrEqual(t, int(elapsed), tt.expectedMinMs,
				"Elapsed time should be >= expected minimum")
			assert.LessOrEqual(t, int(elapsed), tt.expectedMaxMs,
				"Elapsed time should be <= expected maximum")

			// Check response
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestDelayMiddlewareWithRandomDelay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create server with random delay config
	cfg := &config.Config{
		ReadDelayRandomMin:  50,
		ReadDelayRandomMax:  150,
		WriteDelayRandomMin: 100,
		WriteDelayRandomMax: 300,
	}
	s := &Server{config: cfg}

	// Test read operation multiple times
	t.Run("Random read delays", func(t *testing.T) {
		delays := make([]int64, 10)

		for i := 0; i < 10; i++ {
			w := httptest.NewRecorder()
			_, router := gin.CreateTestContext(w)

			router.Use(s.delayMiddleware())
			router.GET("/bucket/object", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/bucket/object", nil)

			start := time.Now()
			router.ServeHTTP(w, req)
			delays[i] = time.Since(start).Milliseconds()

			// Check delay is within range
			assert.GreaterOrEqual(t, int(delays[i]), 45, "Should be >= min (with tolerance)")
			assert.LessOrEqual(t, int(delays[i]), 155, "Should be <= max (with tolerance)")
		}

		// Check that we got different delays (randomness)
		uniqueDelays := make(map[int64]bool)
		for _, d := range delays {
			uniqueDelays[d/10] = true // Group by 10ms buckets
		}
		assert.Greater(t, len(uniqueDelays), 1, "Should have varied delays")
	})

	// Test write operation multiple times
	t.Run("Random write delays", func(t *testing.T) {
		delays := make([]int64, 10)

		for i := 0; i < 10; i++ {
			w := httptest.NewRecorder()
			_, router := gin.CreateTestContext(w)

			router.Use(s.delayMiddleware())
			router.PUT("/bucket/object", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("PUT", "/bucket/object", nil)

			start := time.Now()
			router.ServeHTTP(w, req)
			delays[i] = time.Since(start).Milliseconds()

			// Check delay is within range
			assert.GreaterOrEqual(t, int(delays[i]), 95, "Should be >= min (with tolerance)")
			assert.LessOrEqual(t, int(delays[i]), 305, "Should be <= max (with tolerance)")
		}

		// Check that we got different delays (randomness)
		uniqueDelays := make(map[int64]bool)
		for _, d := range delays {
			uniqueDelays[d/20] = true // Group by 20ms buckets
		}
		assert.Greater(t, len(uniqueDelays), 1, "Should have varied delays")
	})
}

func BenchmarkDelayMiddleware(b *testing.B) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		ReadDelayMs: 10,
	}
	s := &Server{config: cfg}

	router := gin.New()
	router.Use(s.delayMiddleware())
	router.GET("/bucket/object", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/bucket/object", nil)
			router.ServeHTTP(w, req)
		}
	})
}

func BenchmarkCalculateDelay(b *testing.B) {
	b.Run("Fixed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = calculateDelay(100, 0, 0)
		}
	})

	b.Run("Random", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = calculateDelay(0, 50, 150)
		}
	})
}
