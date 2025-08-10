package tenant

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTenants(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	tenantsFile := filepath.Join(tempDir, "tenants.json")

	// Create test tenants.json
	config := TenantsConfig{
		Tenants: []Tenant{
			{
				AccessKeyID:     "test-key-1",
				SecretAccessKey: "test-secret-1",
				CustomDir: "tenant1",
			},
			{
				AccessKeyID:     "test-key-2",
				SecretAccessKey: "test-secret-2",
				CustomDir: "tenant2",
			},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	err = os.WriteFile(tenantsFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test loading
	manager := NewManager(tenantsFile)
	err = manager.LoadFromFile()
	if err != nil {
		t.Fatalf("Failed to load tenants: %v", err)
	}

	// Verify loaded data
	allTenants := manager.GetAllTenants()
	if len(allTenants) != 2 {
		t.Errorf("Expected 2 tenants, got %d", len(allTenants))
	}

	tenant1, exists := manager.GetTenant("test-key-1")
	if !exists {
		t.Error("test-key-1 not found")
	} else {
		if tenant1.SecretAccessKey != "test-secret-1" {
			t.Errorf("Expected secret key 'test-secret-1', got %s", tenant1.SecretAccessKey)
		}
		if tenant1.CustomDir != "tenant1" {
			t.Errorf("Expected directory 'tenant1', got %s", tenant1.CustomDir)
		}
	}
}

func TestGetTenant(t *testing.T) {
	manager := NewManager("")

	// Add test tenant directly
	manager.tenants = map[string]*Tenant{
		"existing-key": {
			AccessKeyID:     "existing-key",
			SecretAccessKey: "secret",
			CustomDir: "dir",
		},
	}

	t.Run("ExistingTenant", func(t *testing.T) {
		tenant, exists := manager.GetTenant("existing-key")
		if !exists {
			t.Error("Expected tenant to exist")
			return
		}
		if tenant.SecretAccessKey != "secret" {
			t.Errorf("Expected secret 'secret', got %s", tenant.SecretAccessKey)
		}
	})

	t.Run("NonExistingTenant", func(t *testing.T) {
		_, exists := manager.GetTenant("non-existing-key")
		if exists {
			t.Error("Expected tenant to not exist")
		}
	})
}

func TestGetAllTenants(t *testing.T) {
	manager := NewManager("")

	// Add test tenants
	manager.tenants = map[string]*Tenant{
		"key1": {AccessKeyID: "key1", SecretAccessKey: "secret1", CustomDir: "dir1"},
		"key2": {AccessKeyID: "key2", SecretAccessKey: "secret2", CustomDir: "dir2"},
		"key3": {AccessKeyID: "key3", SecretAccessKey: "secret3", CustomDir: "dir3"},
	}

	allTenants := manager.GetAllTenants()

	if len(allTenants) != 3 {
		t.Errorf("Expected 3 tenants, got %d", len(allTenants))
	}

	// Verify all tenants are present (GetAllTenants returns map[string]string)
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, exists := allTenants[key]; !exists {
			t.Errorf("Tenant %s not found in GetAllTenants result", key)
		}
	}
}

func TestGetDirectory(t *testing.T) {
	manager := NewManager("")

	manager.tenants = map[string]*Tenant{
		"tenant-key": {
			AccessKeyID:     "tenant-key",
			SecretAccessKey: "secret",
			CustomDir: "custom-dir",
		},
	}

	t.Run("WithTenantMapping", func(t *testing.T) {
		dir := manager.GetDirectory("tenant-key")
		expected := "custom-dir"
		if dir != expected {
			t.Errorf("Expected directory %s, got %s", expected, dir)
		}
	})

	t.Run("WithoutTenantMapping", func(t *testing.T) {
		dir := manager.GetDirectory("unknown-key")
		expected := "unknown-key"
		if dir != expected {
			t.Errorf("Expected directory %s, got %s", expected, dir)
		}
	})

	t.Run("EmptyAccessKey", func(t *testing.T) {
		dir := manager.GetDirectory("")
		if dir != "" {
			t.Errorf("Expected empty string for empty key, got %s", dir)
		}
	})
}

func TestLoadInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	tenantsFile := filepath.Join(tempDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(tenantsFile, []byte("{ invalid json }"), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	manager := NewManager(tenantsFile)
	err = manager.LoadFromFile()
	if err == nil {
		t.Error("Expected error loading invalid JSON, got nil")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	manager := NewManager("/non/existent/file.json")
	err := manager.LoadFromFile()

	// Should handle gracefully - either return error or use empty tenants
	if err != nil {
		// Error is fine
		t.Logf("Got expected error: %v", err)
	} else if len(manager.GetAllTenants()) != 0 {
		t.Error("Expected empty tenants for non-existent file")
	}
}

func TestConcurrentAccess(t *testing.T) {
	manager := NewManager("")
	manager.tenants = map[string]*Tenant{
		"key1": {AccessKeyID: "key1", SecretAccessKey: "secret1", CustomDir: "dir1"},
	}

	// Test concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = manager.GetTenant("key1")
			_ = manager.GetAllTenants()
			_ = manager.GetDirectory("key1")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If test doesn't panic or deadlock, concurrent access is safe
}
