package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	
	storageerrors "github.com/wozozo/s3pit/pkg/errors"
)

// FileSystemMultipartManager handles multipart uploads with filesystem storage
type FileSystemMultipartManager struct {
	baseDir string
	uploads sync.Map // uploadId -> *MultipartUpload
}

// NewFileSystemMultipartManager creates a new filesystem-based multipart manager
func NewFileSystemMultipartManager(baseDir string) *FileSystemMultipartManager {
	return &FileSystemMultipartManager{
		baseDir: baseDir,
	}
}

// InitiateUpload creates a new multipart upload and prepares filesystem
func (m *FileSystemMultipartManager) InitiateUpload(bucket, key string) (string, error) {
	uploadId := fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), key)
	
	upload := &MultipartUpload{
		Bucket:    bucket,
		Key:       key,
		UploadId:  uploadId,
		Initiated: time.Now().UTC(),
		Parts:     make(map[int]PartInfo),
	}
	
	m.uploads.Store(uploadId, upload)
	
	// Create temporary directory for parts
	tempDir := filepath.Join(m.baseDir, ".s3pit_uploads", uploadId)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		m.uploads.Delete(uploadId)
		return "", storageerrors.WrapFileSystemError(tempDir, "create directory", err)
	}
	
	return uploadId, nil
}

// GetUpload retrieves a multipart upload by ID
func (m *FileSystemMultipartManager) GetUpload(uploadId string) (*MultipartUpload, bool) {
	uploadInterface, exists := m.uploads.Load(uploadId)
	if !exists {
		return nil, false
	}
	return uploadInterface.(*MultipartUpload), true
}

// StorePart stores a part to filesystem
func (m *FileSystemMultipartManager) StorePart(uploadId string, partNumber int, reader io.Reader, size int64) (string, error) {
	upload, exists := m.GetUpload(uploadId)
	if !exists {
		return "", storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}
	
	// Save part to temporary file
	partPath := filepath.Join(m.baseDir, ".s3pit_uploads", uploadId, fmt.Sprintf("part-%d", partNumber))
	
	partFile, err := os.Create(partPath)
	if err != nil {
		return "", err
	}
	defer partFile.Close()
	
	// Read part data
	data := make([]byte, size)
	n, err := io.ReadFull(reader, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	data = data[:n]
	
	// Write to part file
	written, err := partFile.Write(data)
	if err != nil {
		return "", err
	}
	
	// Calculate ETag
	etag := CalculateETag(data)
	
	// Update upload parts
	upload.Parts[partNumber] = PartInfo{
		PartNumber:   partNumber,
		Size:         int64(written),
		ETag:         etag,
		LastModified: time.Now().UTC(),
	}
	
	return etag, nil
}

// GetPartPath returns the filesystem path for a part
func (m *FileSystemMultipartManager) GetPartPath(uploadId string, partNumber int) string {
	return filepath.Join(m.baseDir, ".s3pit_uploads", uploadId, fmt.Sprintf("part-%d", partNumber))
}

// ListParts lists all parts for an upload
func (m *FileSystemMultipartManager) ListParts(uploadId string) ([]PartInfo, error) {
	upload, exists := m.GetUpload(uploadId)
	if !exists {
		return nil, storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}
	
	parts := make([]PartInfo, 0, len(upload.Parts))
	for _, part := range upload.Parts {
		parts = append(parts, part)
	}
	
	return parts, nil
}

// DeleteUpload removes an upload and its parts from filesystem
func (m *FileSystemMultipartManager) DeleteUpload(uploadId string) error {
	m.uploads.Delete(uploadId)
	
	// Remove temporary directory
	uploadDir := filepath.Join(m.baseDir, ".s3pit_uploads", uploadId)
	return os.RemoveAll(uploadDir)
}

// ListUploads returns all active uploads for a bucket
func (m *FileSystemMultipartManager) ListUploads(bucket string) []*MultipartUpload {
	var uploads []*MultipartUpload
	
	m.uploads.Range(func(key, value interface{}) bool {
		upload := value.(*MultipartUpload)
		if upload.Bucket == bucket {
			uploads = append(uploads, upload)
		}
		return true
	})
	
	return uploads
}