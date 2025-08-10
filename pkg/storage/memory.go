package storage

import (
	storageerrors "github.com/wozozo/s3pit/pkg/errors"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type memoryObject struct {
	data         []byte
	contentType  string
	lastModified time.Time
	etag         string
}

type memoryBucket struct {
	creationDate time.Time
	objects      map[string]*memoryObject
}

type MemoryStorage struct {
	buckets         map[string]*memoryBucket
	mu              sync.RWMutex
	multipartMgr    *MultipartManager
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		buckets:      make(map[string]*memoryBucket),
		multipartMgr: NewMultipartManager(),
	}
}

func (m *MemoryStorage) CreateBucket(bucket string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.buckets[bucket]; exists {
		return false, nil
	}

	m.buckets[bucket] = &memoryBucket{
		creationDate: time.Now().UTC(),
		objects:      make(map[string]*memoryObject),
	}

	return true, nil
}

func (m *MemoryStorage) DeleteBucket(bucket string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return ErrBucketNotFound
	}

	if len(b.objects) > 0 {
		return ErrBucketNotEmpty
	}

	delete(m.buckets, bucket)
	return nil
}

func (m *MemoryStorage) ListBuckets() ([]BucketInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var buckets []BucketInfo
	for name, b := range m.buckets {
		buckets = append(buckets, BucketInfo{
			Name:         name,
			CreationDate: b.creationDate,
		})
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Name < buckets[j].Name
	})

	return buckets, nil
}

func (m *MemoryStorage) BucketExists(bucket string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.buckets[bucket]
	return exists, nil
}

func (m *MemoryStorage) PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return "", ErrBucketNotFound
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	etag := CalculateETag(data)

	b.objects[key] = &memoryObject{
		data:         data,
		contentType:  contentType,
		lastModified: time.Now().UTC(),
		etag:         etag,
	}

	return etag, nil
}

func (m *MemoryStorage) GetObject(bucket, key string) (io.ReadCloser, *ObjectMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return nil, nil, ErrBucketNotFound
	}

	obj, exists := b.objects[key]
	if !exists {
		return nil, nil, ErrObjectNotFound
	}

	meta := &ObjectMetadata{
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		LastModified: obj.lastModified,
		ETag:         obj.etag,
	}

	return io.NopCloser(bytes.NewReader(obj.data)), meta, nil
}

func (m *MemoryStorage) GetObjectMetadata(bucket, key string) (*ObjectMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return nil, ErrBucketNotFound
	}

	obj, exists := b.objects[key]
	if !exists {
		return nil, ErrObjectNotFound
	}

	return &ObjectMetadata{
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		LastModified: obj.lastModified,
		ETag:         obj.etag,
	}, nil
}

func (m *MemoryStorage) DeleteObject(bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return ErrBucketNotFound
	}

	if _, exists := b.objects[key]; !exists {
		return ErrObjectNotFound
	}

	delete(b.objects, key)
	return nil
}

func (m *MemoryStorage) ListObjects(bucket, prefix, delimiter string, maxKeys int, continuationToken string) ([]ObjectInfo, []string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return nil, nil, "", ErrBucketNotFound
	}

	var keys []string
	for k := range b.objects {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			if continuationToken == "" || k > continuationToken {
				keys = append(keys, k)
			}
		}
	}

	sort.Strings(keys)

	var objects []ObjectInfo
	commonPrefixes := make(map[string]bool)

	for _, key := range keys {
		if len(objects) >= maxKeys {
			break
		}

		if delimiter != "" {
			afterPrefix := key
			if prefix != "" {
				afterPrefix = strings.TrimPrefix(key, prefix)
			}

			if idx := strings.Index(afterPrefix, delimiter); idx >= 0 {
				commonPrefix := key[:len(key)-len(afterPrefix)+idx+len(delimiter)]
				if prefix != "" {
					commonPrefix = prefix + afterPrefix[:idx+len(delimiter)]
				}
				commonPrefixes[commonPrefix] = true
				continue
			}
		}

		obj := b.objects[key]
		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         int64(len(obj.data)),
			LastModified: obj.lastModified,
			ETag:         obj.etag,
		})
	}

	var nextToken string
	if len(objects) > 0 && len(keys) > len(objects) {
		nextToken = objects[len(objects)-1].Key
	}

	var prefixes []string
	for p := range commonPrefixes {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	return objects, prefixes, nextToken, nil
}

func (m *MemoryStorage) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	srcB, exists := m.buckets[srcBucket]
	if !exists {
		return "", ErrBucketNotFound
	}

	srcObj, exists := srcB.objects[srcKey]
	if !exists {
		return "", ErrObjectNotFound
	}

	dstB, exists := m.buckets[dstBucket]
	if !exists {
		return "", ErrBucketNotFound
	}

	dataCopy := make([]byte, len(srcObj.data))
	copy(dataCopy, srcObj.data)

	etag := CalculateETag(dataCopy)

	dstB.objects[dstKey] = &memoryObject{
		data:         dataCopy,
		contentType:  srcObj.contentType,
		lastModified: time.Now().UTC(),
		etag:         etag,
	}

	return etag, nil
}

// InitiateMultipartUpload starts a new multipart upload
func (m *MemoryStorage) InitiateMultipartUpload(bucket, key string) (string, error) {
	m.mu.RLock()
	if _, exists := m.buckets[bucket]; !exists {
		m.mu.RUnlock()
		return "", ErrBucketNotFound
	}
	m.mu.RUnlock()

	return m.multipartMgr.InitiateUpload(bucket, key)
}

// UploadPart uploads a part for a multipart upload
func (m *MemoryStorage) UploadPart(bucket, key, uploadId string, partNumber int, reader io.Reader, size int64) (string, error) {
	m.mu.RLock()
	if _, exists := m.buckets[bucket]; !exists {
		m.mu.RUnlock()
		return "", ErrBucketNotFound
	}
	m.mu.RUnlock()

	upload, exists := m.multipartMgr.GetUpload(uploadId)
	if !exists {
		return "", storageerrors.ErrUploadNotFound
	}

	if upload.Bucket != bucket || upload.Key != key {
		return "", storageerrors.ErrUploadMismatch
	}

	// Read the data from the reader
	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	if err != nil {
		return "", storageerrors.WrapStorageError("read part data", err)
	}

	etag := CalculateETag(data)
	if err := m.multipartMgr.StorePart(uploadId, partNumber, data, etag); err != nil {
		return "", err
	}

	return etag, nil
}

// CompleteMultipartUpload completes a multipart upload
func (m *MemoryStorage) CompleteMultipartUpload(bucket, key, uploadId string, parts []CompletedPart) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.buckets[bucket]; !exists {
		return "", ErrBucketNotFound
	}

	upload, exists := m.multipartMgr.GetUpload(uploadId)
	if !exists {
		return "", storageerrors.ErrUploadNotFound
	}

	if upload.Bucket != bucket || upload.Key != key {
		return "", storageerrors.ErrUploadMismatch
	}

	uploadParts, _ := m.multipartMgr.GetParts(uploadId)
	
	// Combine all parts
	var finalData []byte
	for _, part := range parts {
		partData, exists := uploadParts[part.PartNumber]
		if !exists {
			return "", storageerrors.WrapMultipartError(fmt.Sprintf("part %d", part.PartNumber), storageerrors.ErrPartNotFound)
		}
		finalData = append(finalData, partData...)
	}

	// Calculate final ETag
	etag := CalculateETag(finalData)

	// Store the combined object
	m.buckets[bucket].objects[key] = &memoryObject{
		data:         finalData,
		contentType:  "application/octet-stream",
		lastModified: time.Now().UTC(),
		etag:         etag,
	}

	// Clean up the multipart upload
	_ = m.multipartMgr.DeleteUpload(uploadId)

	return etag, nil
}

// AbortMultipartUpload cancels a multipart upload
func (m *MemoryStorage) AbortMultipartUpload(bucket, key, uploadId string) error {
	m.mu.RLock()
	if _, exists := m.buckets[bucket]; !exists {
		m.mu.RUnlock()
		return ErrBucketNotFound
	}
	m.mu.RUnlock()

	upload, exists := m.multipartMgr.GetUpload(uploadId)
	if !exists {
		return storageerrors.ErrUploadNotFound
	}

	if upload.Bucket != bucket || upload.Key != key {
		return storageerrors.ErrUploadMismatch
	}

	return m.multipartMgr.DeleteUpload(uploadId)
}

// ListParts lists the parts of a multipart upload
func (m *MemoryStorage) ListParts(bucket, key, uploadId string) ([]PartInfo, error) {
	m.mu.RLock()
	if _, exists := m.buckets[bucket]; !exists {
		m.mu.RUnlock()
		return nil, ErrBucketNotFound
	}
	m.mu.RUnlock()

	upload, exists := m.multipartMgr.GetUpload(uploadId)
	if !exists {
		return nil, storageerrors.ErrUploadNotFound
	}

	if upload.Bucket != bucket || upload.Key != key {
		return nil, storageerrors.ErrUploadMismatch
	}

	return m.multipartMgr.ListParts(uploadId)
}
