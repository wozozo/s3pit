package testutil

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/storage"
)

// InitTestMode sets Gin to test mode
func InitTestMode() {
	gin.SetMode(gin.TestMode)
}

// ConfigOption allows customizing test configurations
type ConfigOption func(*config.Config)

// WithPort sets a custom port for the test config
func WithPort(port int) ConfigOption {
	return func(cfg *config.Config) {
		cfg.Port = port
	}
}

// WithAuthMode sets the authentication mode
func WithAuthMode(mode string) ConfigOption {
	return func(cfg *config.Config) {
		cfg.AuthMode = mode
	}
}

// WithAutoCreateBucket enables/disables auto bucket creation
func WithAutoCreateBucket(enabled bool) ConfigOption {
	return func(cfg *config.Config) {
		cfg.AutoCreateBucket = enabled
	}
}



// WithInMemory sets whether to use in-memory storage
func WithInMemory(inMemory bool) ConfigOption {
	return func(cfg *config.Config) {
		cfg.InMemory = inMemory
	}
}

// NewTestConfig creates a standard test configuration
func NewTestConfig(t *testing.T, opts ...ConfigOption) *config.Config {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Port:             3333,
		GlobalDir:        t.TempDir(),
		InMemory:         true,
		AutoCreateBucket: true,
		AuthMode:         "none",
	}

	// Apply custom options
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// SetupTestStorage creates a test storage instance
func SetupTestStorage(t *testing.T, inMemory bool) storage.Storage {
	if inMemory {
		return storage.NewMemoryStorage()
	}
	
	tempDir := t.TempDir()
	fs, err := storage.NewFileSystemStorage(tempDir)
	if err != nil {
		t.Fatalf("Failed to create filesystem storage: %v", err)
	}
	
	return fs
}

// SetupTestStorageWithBuckets creates storage with pre-created buckets
func SetupTestStorageWithBuckets(t *testing.T, inMemory bool, buckets ...string) storage.Storage {
	store := SetupTestStorage(t, inMemory)
	
	for _, bucket := range buckets {
		_, err := store.CreateBucket(bucket)
		if err != nil {
			t.Fatalf("Failed to create bucket %s: %v", bucket, err)
		}
	}
	
	return store
}

// CleanupTestStorage performs any necessary cleanup
func CleanupTestStorage(t *testing.T, store storage.Storage) {
	// For now, this is a no-op as temp directories are automatically cleaned
	// But we keep this for future extensions
}