package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wozozo/s3pit/pkg/tenant"
)

func TestTenantAwareStorage_GetStorageForTenant(t *testing.T) {
	// Create temporary directories
	baseDir, err := os.MkdirTemp("", "s3pit-test-base")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	tenant1Dir, err := os.MkdirTemp("", "s3pit-test-tenant1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tenant1Dir)

	tenant2Dir, err := os.MkdirTemp("", "s3pit-test-tenant2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tenant2Dir)

	// Create tenant manager with test tenants
	tenantManager := tenant.NewManager("")
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tenant1",
		SecretAccessKey: "secret1",
		CustomDir:       tenant1Dir,
	})
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tenant2",
		SecretAccessKey: "secret2",
		CustomDir:       tenant2Dir,
	})

	// Create tenant-aware storage
	tas := NewTenantAwareStorage(baseDir, tenantManager, false)

	// Test getting storage for different tenants
	storage1, err := tas.GetStorageForTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get storage for tenant1: %v", err)
	}

	storage2, err := tas.GetStorageForTenant("tenant2")
	if err != nil {
		t.Fatalf("Failed to get storage for tenant2: %v", err)
	}

	// Verify they are different instances
	if storage1 == storage2 {
		t.Error("Expected different storage instances for different tenants")
	}

	// Test that same tenant returns same storage instance
	storage1Again, err := tas.GetStorageForTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get storage for tenant1 again: %v", err)
	}

	if storage1 != storage1Again {
		t.Error("Expected same storage instance for same tenant")
	}

	// Test default tenant
	storageDefault, err := tas.GetStorageForTenant("")
	if err != nil {
		t.Fatalf("Failed to get storage for default tenant: %v", err)
	}

	if storageDefault == nil {
		t.Error("Expected non-nil storage for default tenant")
	}
}

func TestTenantAwareStorage_Isolation(t *testing.T) {
	// Create temporary directories
	baseDir, err := os.MkdirTemp("", "s3pit-test-base")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	tenant1Dir, err := os.MkdirTemp("", "s3pit-test-tenant1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tenant1Dir)

	tenant2Dir, err := os.MkdirTemp("", "s3pit-test-tenant2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tenant2Dir)

	// Create tenant manager
	tenantManager := tenant.NewManager("")
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tenant1",
		SecretAccessKey: "secret1",
		CustomDir:       tenant1Dir,
	})
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tenant2",
		SecretAccessKey: "secret2",
		CustomDir:       tenant2Dir,
	})

	// Create tenant-aware storage
	tas := NewTenantAwareStorage(baseDir, tenantManager, false)

	// Get storage for each tenant
	storage1, _ := tas.GetStorageForTenant("tenant1")
	storage2, _ := tas.GetStorageForTenant("tenant2")

	// Create bucket and object for tenant1
	_, _ = storage1.CreateBucket("test-bucket")
	content1 := "Content for tenant1"
	_, _ = storage1.PutObject("test-bucket", "test-file.txt",
		bytes.NewReader([]byte(content1)), int64(len(content1)), "text/plain")

	// Create bucket and object for tenant2
	_, _ = storage2.CreateBucket("test-bucket")
	content2 := "Content for tenant2"
	_, _ = storage2.PutObject("test-bucket", "test-file.txt",
		bytes.NewReader([]byte(content2)), int64(len(content2)), "text/plain")

	// Verify tenant1's file
	reader1, _, err := storage1.GetObject("test-bucket", "test-file.txt")
	if err != nil {
		t.Fatalf("Failed to get tenant1's object: %v", err)
	}
	defer reader1.Close()

	data1, _ := io.ReadAll(reader1)
	if string(data1) != content1 {
		t.Errorf("Tenant1 content mismatch. Got %s, want %s", string(data1), content1)
	}

	// Verify tenant2's file
	reader2, _, err := storage2.GetObject("test-bucket", "test-file.txt")
	if err != nil {
		t.Fatalf("Failed to get tenant2's object: %v", err)
	}
	defer reader2.Close()

	data2, _ := io.ReadAll(reader2)
	if string(data2) != content2 {
		t.Errorf("Tenant2 content mismatch. Got %s, want %s", string(data2), content2)
	}

	// Verify files are in correct directories
	file1Path := filepath.Join(tenant1Dir, "test-bucket", "test-file.txt")
	if _, err := os.Stat(file1Path); os.IsNotExist(err) {
		t.Error("Tenant1's file not found in correct directory")
	}

	file2Path := filepath.Join(tenant2Dir, "test-bucket", "test-file.txt")
	if _, err := os.Stat(file2Path); os.IsNotExist(err) {
		t.Error("Tenant2's file not found in correct directory")
	}
}

func TestTenantAwareStorage_Concurrency(t *testing.T) {
	// Create temporary directories
	baseDir, err := os.MkdirTemp("", "s3pit-test-base")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	// Create tenant manager with multiple tenants
	tenantManager := tenant.NewManager("")
	numTenants := 5
	tenantDirs := make([]string, numTenants)

	for i := 0; i < numTenants; i++ {
		dir, err := os.MkdirTemp("", fmt.Sprintf("s3pit-test-tenant%d", i))
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		tenantDirs[i] = dir

		tenantID := fmt.Sprintf("tenant%d", i)
		_ = tenantManager.AddTenant(&tenant.Tenant{
			AccessKeyID:     tenantID,
			SecretAccessKey: fmt.Sprintf("secret%d", i),
			CustomDir:       dir,
		})
	}

	// Create tenant-aware storage
	tas := NewTenantAwareStorage(baseDir, tenantManager, false)

	// Test concurrent access to different tenants
	var wg sync.WaitGroup
	numGoroutines := 100
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Select a tenant based on goroutine index
			tenantID := fmt.Sprintf("tenant%d", index%numTenants)

			// Get storage for tenant
			storage, err := tas.GetStorageForTenant(tenantID)
			if err != nil {
				errors <- fmt.Errorf("Failed to get storage for %s: %v", tenantID, err)
				return
			}

			// Create bucket
			bucketName := fmt.Sprintf("bucket-%d", index)
			_, err = storage.CreateBucket(bucketName)
			if err != nil {
				errors <- fmt.Errorf("Failed to create bucket %s for %s: %v",
					bucketName, tenantID, err)
				return
			}

			// Put object
			objectKey := fmt.Sprintf("object-%d.txt", index)
			content := fmt.Sprintf("Content from goroutine %d", index)
			_, err = storage.PutObject(bucketName, objectKey,
				bytes.NewReader([]byte(content)), int64(len(content)), "text/plain")
			if err != nil {
				errors <- fmt.Errorf("Failed to put object %s for %s: %v",
					objectKey, tenantID, err)
				return
			}

			// Verify object
			reader, _, err := storage.GetObject(bucketName, objectKey)
			if err != nil {
				errors <- fmt.Errorf("Failed to get object %s for %s: %v",
					objectKey, tenantID, err)
				return
			}
			defer reader.Close()

			data, _ := io.ReadAll(reader)
			if string(data) != content {
				errors <- fmt.Errorf("Content mismatch for %s in %s",
					objectKey, tenantID)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errCount int
	for err := range errors {
		t.Error(err)
		errCount++
		if errCount >= 10 {
			t.Fatal("Too many errors, stopping test")
		}
	}

	if errCount > 0 {
		t.Fatalf("Test failed with %d errors", errCount)
	}
}

func TestTenantAwareStorage_TildeExpansion(t *testing.T) {
	// This test verifies that tilde paths are expanded correctly
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot determine home directory")
	}

	// Create a test directory in home
	testDir := filepath.Join(homeDir, ".s3pit-test")
	err = os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	// Create tenant manager with tilde path
	tenantManager := tenant.NewManager("")
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tilde-tenant",
		SecretAccessKey: "secret",
		CustomDir:       "~/.s3pit-test",
	})

	// Note: The actual tilde expansion happens in tenant.Manager
	// This test verifies the integration works correctly

	baseDir, err := os.MkdirTemp("", "s3pit-test-base")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	tas := NewTenantAwareStorage(baseDir, tenantManager, false)

	storage, err := tas.GetStorageForTenant("tilde-tenant")
	if err != nil {
		t.Fatalf("Failed to get storage for tilde-tenant: %v", err)
	}

	// Create a bucket and verify it's created in the expanded path
	_, err = storage.CreateBucket("tilde-bucket")
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	// Check if bucket directory exists in the expanded path
	bucketPath := filepath.Join(testDir, "tilde-bucket")
	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		t.Error("Bucket not created in expanded tilde path")
	}
}
