package storage

import (
	"fmt"
	storageerrors "github.com/wozozo/s3pit/pkg/errors"
	"sync"
	"time"
)

// MultipartManager handles multipart upload operations with thread-safe access
type MultipartManager struct {
	uploads map[string]*MultipartUpload
	parts   map[string]map[int][]byte
	mu      sync.RWMutex
}

// NewMultipartManager creates a new multipart upload manager
func NewMultipartManager() *MultipartManager {
	return &MultipartManager{
		uploads: make(map[string]*MultipartUpload),
		parts:   make(map[string]map[int][]byte),
	}
}

// InitiateUpload creates a new multipart upload
func (m *MultipartManager) InitiateUpload(bucket, key string) (string, error) {
	uploadId := fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), key)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.uploads[uploadId] = &MultipartUpload{
		Bucket:    bucket,
		Key:       key,
		UploadId:  uploadId,
		Initiated: time.Now().UTC(),
		Parts:     make(map[int]PartInfo),
	}
	m.parts[uploadId] = make(map[int][]byte)

	return uploadId, nil
}

// GetUpload retrieves a multipart upload by ID
func (m *MultipartManager) GetUpload(uploadId string) (*MultipartUpload, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	upload, exists := m.uploads[uploadId]
	return upload, exists
}

// StorePart stores a part for a multipart upload
func (m *MultipartManager) StorePart(uploadId string, partNumber int, data []byte, etag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	upload, exists := m.uploads[uploadId]
	if !exists {
		return storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}

	// Store part data
	if m.parts[uploadId] == nil {
		m.parts[uploadId] = make(map[int][]byte)
	}
	m.parts[uploadId][partNumber] = data

	// Update part info
	upload.Parts[partNumber] = PartInfo{
		PartNumber: partNumber,
		ETag:       etag,
		Size:       int64(len(data)),
	}

	return nil
}

// GetParts retrieves all parts for an upload
func (m *MultipartManager) GetParts(uploadId string) (map[int][]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	parts, exists := m.parts[uploadId]
	return parts, exists
}

// ListParts lists all part info for an upload
func (m *MultipartManager) ListParts(uploadId string) ([]PartInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	upload, exists := m.uploads[uploadId]
	if !exists {
		return nil, storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}

	parts := make([]PartInfo, 0, len(upload.Parts))
	for _, part := range upload.Parts {
		parts = append(parts, part)
	}

	return parts, nil
}

// DeleteUpload removes an upload and its parts
func (m *MultipartManager) DeleteUpload(uploadId string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.uploads, uploadId)
	delete(m.parts, uploadId)

	return nil
}

// ListUploads returns all active uploads for a bucket
func (m *MultipartManager) ListUploads(bucket string) []*MultipartUpload {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var uploads []*MultipartUpload
	for _, upload := range m.uploads {
		if upload.Bucket == bucket {
			uploads = append(uploads, upload)
		}
	}

	return uploads
}
