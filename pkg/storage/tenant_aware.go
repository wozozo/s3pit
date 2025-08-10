package storage

import (
	"fmt"
	"io"
	"path/filepath"
	"sync"

	storageerrors "github.com/wozozo/s3pit/pkg/errors"
	"github.com/wozozo/s3pit/pkg/tenant"
)

// TenantAwareStorage wraps storage operations with tenant isolation
// It creates separate storage instances for each tenant directory
type TenantAwareStorage struct {
	baseDir       string
	tenantManager *tenant.Manager
	storages      map[string]Storage
	mu            sync.RWMutex
	inMemory      bool
}

// NewTenantAwareStorage creates a new tenant-aware storage wrapper
func NewTenantAwareStorage(baseDir string, tenantManager *tenant.Manager, inMemory bool) *TenantAwareStorage {
	return &TenantAwareStorage{
		baseDir:       baseDir,
		tenantManager: tenantManager,
		storages:      make(map[string]Storage),
		inMemory:      inMemory,
	}
}

// GetStorageForTenant returns or creates a storage instance for the specified tenant
// This is the thread-safe method to use for getting tenant-specific storage
func (t *TenantAwareStorage) GetStorageForTenant(tenantID string) (Storage, error) {
	if tenantID == "" {
		tenantID = "default"
	}

	t.mu.RLock()
	if storage, exists := t.storages[tenantID]; exists {
		t.mu.RUnlock()
		return storage, nil
	}
	t.mu.RUnlock()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Double-check after acquiring write lock
	if storage, exists := t.storages[tenantID]; exists {
		return storage, nil
	}

	// Get tenant directory
	dir := t.baseDir
	if t.tenantManager != nil {
		dir = t.tenantManager.GetDirectory(tenantID)
		// If GetDirectory returns just the accessKeyID (no custom dir), use baseDir/accessKeyID
		if dir == tenantID {
			dir = filepath.Join(t.baseDir, tenantID)
		}
	}

	// Create storage instance for this tenant
	var storage Storage
	var err error
	if t.inMemory {
		storage = NewMemoryStorage()
	} else {
		storage, err = NewFileSystemStorage(dir)
		if err != nil {
			return nil, storageerrors.WrapStorageError(fmt.Sprintf("create storage for tenant %s", tenantID), err)
		}
	}

	t.storages[tenantID] = storage
	return storage, nil
}

// CreateBucket creates a new bucket for the default tenant
func (t *TenantAwareStorage) CreateBucket(bucket string) (bool, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return false, err
	}
	return storage.CreateBucket(bucket)
}

// DeleteBucket deletes a bucket for the default tenant
func (t *TenantAwareStorage) DeleteBucket(bucket string) error {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return err
	}
	return storage.DeleteBucket(bucket)
}

// ListBuckets lists all buckets for the default tenant
func (t *TenantAwareStorage) ListBuckets() ([]BucketInfo, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return nil, err
	}
	return storage.ListBuckets()
}

// BucketExists checks if a bucket exists for the default tenant
func (t *TenantAwareStorage) BucketExists(bucket string) (bool, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return false, err
	}
	return storage.BucketExists(bucket)
}

// PutObject stores an object for the default tenant
func (t *TenantAwareStorage) PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (string, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return "", err
	}
	return storage.PutObject(bucket, key, reader, size, contentType)
}

// GetObject retrieves an object for the default tenant
func (t *TenantAwareStorage) GetObject(bucket, key string) (io.ReadCloser, *ObjectMetadata, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return nil, nil, err
	}
	return storage.GetObject(bucket, key)
}

// GetObjectMetadata retrieves object metadata for the default tenant
func (t *TenantAwareStorage) GetObjectMetadata(bucket, key string) (*ObjectMetadata, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return nil, err
	}
	return storage.GetObjectMetadata(bucket, key)
}

// DeleteObject deletes an object for the default tenant
func (t *TenantAwareStorage) DeleteObject(bucket, key string) error {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return err
	}
	return storage.DeleteObject(bucket, key)
}

// ListObjects lists objects in a bucket for the default tenant
func (t *TenantAwareStorage) ListObjects(bucket, prefix, delimiter string, maxKeys int, continuationToken string) ([]ObjectInfo, []string, string, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return nil, nil, "", err
	}
	return storage.ListObjects(bucket, prefix, delimiter, maxKeys, continuationToken)
}

// CopyObject copies an object within the default tenant's storage
func (t *TenantAwareStorage) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) (string, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return "", err
	}
	return storage.CopyObject(srcBucket, srcKey, dstBucket, dstKey)
}

// InitiateMultipartUpload initiates a multipart upload for the default tenant
func (t *TenantAwareStorage) InitiateMultipartUpload(bucket, key string) (string, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return "", err
	}
	return storage.InitiateMultipartUpload(bucket, key)
}

// UploadPart uploads a part of a multipart upload for the default tenant
func (t *TenantAwareStorage) UploadPart(bucket, key, uploadId string, partNumber int, reader io.Reader, size int64) (string, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return "", err
	}
	return storage.UploadPart(bucket, key, uploadId, partNumber, reader, size)
}

// CompleteMultipartUpload completes a multipart upload for the default tenant
func (t *TenantAwareStorage) CompleteMultipartUpload(bucket, key, uploadId string, parts []CompletedPart) (string, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return "", err
	}
	return storage.CompleteMultipartUpload(bucket, key, uploadId, parts)
}

// AbortMultipartUpload aborts a multipart upload for the default tenant
func (t *TenantAwareStorage) AbortMultipartUpload(bucket, key, uploadId string) error {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return err
	}
	return storage.AbortMultipartUpload(bucket, key, uploadId)
}

// ListParts lists parts of a multipart upload for the default tenant
func (t *TenantAwareStorage) ListParts(bucket, key, uploadId string) ([]PartInfo, error) {
	storage, err := t.GetStorageForTenant("default")
	if err != nil {
		return nil, err
	}
	return storage.ListParts(bucket, key, uploadId)
}
