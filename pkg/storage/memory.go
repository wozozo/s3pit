package storage

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
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
	buckets  map[string]*memoryBucket
	mu       sync.RWMutex
	uploads  map[string]*MultipartUpload
	uploadMu sync.RWMutex
	parts    map[string]map[int][]byte // uploadId -> partNumber -> data
}

func NewMemoryStorage() Storage {
	return &MemoryStorage{
		buckets: make(map[string]*memoryBucket),
		uploads: make(map[string]*MultipartUpload),
		parts:   make(map[string]map[int][]byte),
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

	hash := md5.Sum(data)
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))

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

	hash := md5.Sum(dataCopy)
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))

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

	uploadId := fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), key)

	m.uploadMu.Lock()
	defer m.uploadMu.Unlock()

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

// UploadPart uploads a part for a multipart upload
func (m *MemoryStorage) UploadPart(bucket, key, uploadId string, partNumber int, reader io.Reader, size int64) (string, error) {
	m.uploadMu.RLock()
	upload, exists := m.uploads[uploadId]
	m.uploadMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("upload not found: %s", uploadId)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return "", fmt.Errorf("upload mismatch")
	}

	// Read part data
	data := make([]byte, size)
	n, err := io.ReadFull(reader, data)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	data = data[:n]

	// Calculate ETag
	hash := md5.Sum(data)
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))

	m.uploadMu.Lock()
	defer m.uploadMu.Unlock()

	m.parts[uploadId][partNumber] = data
	upload.Parts[partNumber] = PartInfo{
		PartNumber:   partNumber,
		Size:         int64(n),
		ETag:         etag,
		LastModified: time.Now().UTC(),
	}

	return etag, nil
}

// CompleteMultipartUpload completes a multipart upload
func (m *MemoryStorage) CompleteMultipartUpload(bucket, key, uploadId string, parts []CompletedPart) (string, error) {
	m.uploadMu.Lock()
	upload, exists := m.uploads[uploadId]
	if !exists {
		m.uploadMu.Unlock()
		return "", fmt.Errorf("upload not found: %s", uploadId)
	}

	partData := m.parts[uploadId]
	delete(m.uploads, uploadId)
	delete(m.parts, uploadId)
	m.uploadMu.Unlock()

	if upload.Bucket != bucket || upload.Key != key {
		return "", fmt.Errorf("upload mismatch")
	}

	// Ensure all parts are present
	for _, part := range parts {
		if _, ok := upload.Parts[part.PartNumber]; !ok {
			return "", fmt.Errorf("part %d not found", part.PartNumber)
		}
	}

	// Sort parts by part number
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	// Concatenate all parts
	var finalData []byte
	for _, part := range parts {
		data, ok := partData[part.PartNumber]
		if !ok {
			return "", fmt.Errorf("part %d data not found", part.PartNumber)
		}
		finalData = append(finalData, data...)
	}

	// Calculate final ETag
	hash := md5.Sum(finalData)
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))

	// Store the final object
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.buckets[bucket]
	if !exists {
		return "", ErrBucketNotFound
	}

	b.objects[key] = &memoryObject{
		data:         finalData,
		contentType:  "application/octet-stream",
		lastModified: time.Now().UTC(),
		etag:         etag,
	}

	return etag, nil
}

// AbortMultipartUpload cancels a multipart upload
func (m *MemoryStorage) AbortMultipartUpload(bucket, key, uploadId string) error {
	m.uploadMu.Lock()
	defer m.uploadMu.Unlock()

	upload, exists := m.uploads[uploadId]
	if !exists {
		return fmt.Errorf("upload not found: %s", uploadId)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return fmt.Errorf("upload mismatch")
	}

	delete(m.uploads, uploadId)
	delete(m.parts, uploadId)

	return nil
}

// ListParts lists the parts of a multipart upload
func (m *MemoryStorage) ListParts(bucket, key, uploadId string) ([]PartInfo, error) {
	m.uploadMu.RLock()
	defer m.uploadMu.RUnlock()

	upload, exists := m.uploads[uploadId]
	if !exists {
		return nil, fmt.Errorf("upload not found: %s", uploadId)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return nil, fmt.Errorf("upload mismatch")
	}

	var parts []PartInfo
	for _, part := range upload.Parts {
		parts = append(parts, part)
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	return parts, nil
}
