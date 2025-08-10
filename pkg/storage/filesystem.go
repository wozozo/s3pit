package storage

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type FileSystemStorage struct {
	baseDir     string
	bucketLocks sync.Map // Per-bucket locks for better concurrency
	uploads     sync.Map // Use sync.Map for better concurrent access
}

func NewFileSystemStorage(baseDir string) (Storage, error) {
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &FileSystemStorage{
		baseDir: absPath,
	}, nil
}

// getBucketLock returns a lock for the specific bucket
func (fs *FileSystemStorage) getBucketLock(bucket string) *sync.RWMutex {
	lock, _ := fs.bucketLocks.LoadOrStore(bucket, &sync.RWMutex{})
	return lock.(*sync.RWMutex)
}

func (fs *FileSystemStorage) CreateBucket(bucket string) (bool, error) {
	lock := fs.getBucketLock(bucket)
	lock.Lock()
	defer lock.Unlock()

	bucketPath := filepath.Join(fs.baseDir, bucket)

	if _, err := os.Stat(bucketPath); err == nil {
		return false, nil
	}

	if err := os.MkdirAll(bucketPath, 0755); err != nil {
		return false, fmt.Errorf("failed to create bucket directory: %w", err)
	}

	metaPath := filepath.Join(bucketPath, ".s3pit_bucket_meta.json")
	meta := map[string]interface{}{
		"created": time.Now().UTC(),
		"name":    bucket,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return false, err
	}

	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return false, err
	}

	return true, nil
}

func (fs *FileSystemStorage) PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (string, error) {
	lock := fs.getBucketLock(bucket)
	lock.Lock()
	defer lock.Unlock()

	objectPath := filepath.Join(fs.baseDir, bucket, key)
	objectDir := filepath.Dir(objectPath)

	if err := os.MkdirAll(objectDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create object directory: %w", err)
	}

	// Use a temporary file for atomic writes
	tempFile, err := os.CreateTemp(objectDir, ".upload_*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Clean up on error
	defer func() {
		if tempFile != nil {
			tempFile.Close()
			os.Remove(tempPath)
		}
	}()

	hash := md5.New()
	writer := io.MultiWriter(tempFile, hash)

	written, err := io.Copy(writer, reader)
	if err != nil {
		return "", fmt.Errorf("failed to write object data: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return "", err
	}
	tempFile = nil // Mark as closed

	// Atomic rename
	if err := os.Rename(tempPath, objectPath); err != nil {
		return "", fmt.Errorf("failed to move object to final location: %w", err)
	}

	etag := hex.EncodeToString(hash.Sum(nil))

	// Save metadata
	metaPath := objectPath + ".s3pit_meta.json"
	meta := map[string]interface{}{
		"content-type": contentType,
		"etag":         etag,
		"size":         written,
		"modified":     time.Now().UTC(),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Sprintf("\"%s\"", etag), nil
	}

	_ = os.WriteFile(metaPath, data, 0644)

	return fmt.Sprintf("\"%s\"", etag), nil
}

func (fs *FileSystemStorage) GetObject(bucket, key string) (io.ReadCloser, *ObjectMetadata, error) {
	lock := fs.getBucketLock(bucket)
	lock.RLock()
	defer lock.RUnlock()

	objectPath := filepath.Join(fs.baseDir, bucket, key)

	file, err := os.Open(objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrObjectNotFound
		}
		return nil, nil, err
	}

	// Load metadata from disk
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}

	meta := &ObjectMetadata{
		Size:         stat.Size(),
		LastModified: stat.ModTime(),
		ContentType:  "application/octet-stream",
	}

	metaPath := objectPath + ".s3pit_meta.json"
	if data, err := os.ReadFile(metaPath); err == nil {
		var storedMeta map[string]interface{}
		if json.Unmarshal(data, &storedMeta) == nil {
			if ct, ok := storedMeta["content-type"].(string); ok {
				meta.ContentType = ct
			}
			if etag, ok := storedMeta["etag"].(string); ok {
				if !strings.HasPrefix(etag, "\"") {
					etag = fmt.Sprintf("\"%s\"", etag)
				}
				meta.ETag = etag
			}
		}
	}

	return file, meta, nil
}

func (fs *FileSystemStorage) GetObjectMetadata(bucket, key string) (*ObjectMetadata, error) {
	lock := fs.getBucketLock(bucket)
	lock.RLock()
	defer lock.RUnlock()

	objectPath := filepath.Join(fs.baseDir, bucket, key)

	stat, err := os.Stat(objectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	meta := &ObjectMetadata{
		Size:         stat.Size(),
		LastModified: stat.ModTime(),
		ContentType:  "application/octet-stream",
	}

	metaPath := objectPath + ".s3pit_meta.json"
	if data, err := os.ReadFile(metaPath); err == nil {
		var storedMeta map[string]interface{}
		if json.Unmarshal(data, &storedMeta) == nil {
			if ct, ok := storedMeta["content-type"].(string); ok {
				meta.ContentType = ct
			}
			if etag, ok := storedMeta["etag"].(string); ok {
				if !strings.HasPrefix(etag, "\"") {
					etag = fmt.Sprintf("\"%s\"", etag)
				}
				meta.ETag = etag
			}
		}
	}

	return meta, nil
}

func (fs *FileSystemStorage) DeleteObject(bucket, key string) error {
	lock := fs.getBucketLock(bucket)
	lock.Lock()
	defer lock.Unlock()

	objectPath := filepath.Join(fs.baseDir, bucket, key)

	if err := os.Remove(objectPath); err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return err
	}

	metaPath := objectPath + ".s3pit_meta.json"
	os.Remove(metaPath)

	return nil
}

func (fs *FileSystemStorage) DeleteBucket(bucket string) error {
	lock := fs.getBucketLock(bucket)
	lock.Lock()
	defer lock.Unlock()

	bucketPath := filepath.Join(fs.baseDir, bucket)

	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		return ErrBucketNotFound
	}

	entries, err := os.ReadDir(bucketPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".s3pit_") {
			return ErrBucketNotEmpty
		}
	}

	return os.RemoveAll(bucketPath)
}

func (fs *FileSystemStorage) ListBuckets() ([]BucketInfo, error) {
	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return nil, err
	}

	var buckets []BucketInfo
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			creationTime := info.ModTime()
			metaPath := filepath.Join(fs.baseDir, entry.Name(), ".s3pit_bucket_meta.json")
			if data, err := os.ReadFile(metaPath); err == nil {
				var meta map[string]interface{}
				if json.Unmarshal(data, &meta) == nil {
					if created, ok := meta["created"].(string); ok {
						if t, err := time.Parse(time.RFC3339, created); err == nil {
							creationTime = t
						}
					}
				}
			}

			buckets = append(buckets, BucketInfo{
				Name:         entry.Name(),
				CreationDate: creationTime,
			})
		}
	}

	return buckets, nil
}

func (fs *FileSystemStorage) BucketExists(bucket string) (bool, error) {
	bucketPath := filepath.Join(fs.baseDir, bucket)
	_, err := os.Stat(bucketPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (fs *FileSystemStorage) ListObjects(bucket, prefix, delimiter string, maxKeys int, continuationToken string) ([]ObjectInfo, []string, string, error) {
	lock := fs.getBucketLock(bucket)
	lock.RLock()
	defer lock.RUnlock()

	bucketPath := filepath.Join(fs.baseDir, bucket)
	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		return nil, nil, "", ErrBucketNotFound
	}

	var objects []ObjectInfo
	commonPrefixes := make(map[string]bool)

	startAfter := continuationToken

	err := filepath.Walk(bucketPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() || strings.Contains(info.Name(), ".s3pit_") {
			return nil
		}

		relPath, err := filepath.Rel(bucketPath, path)
		if err != nil {
			return nil
		}

		key := filepath.ToSlash(relPath)

		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}

		if startAfter != "" && key <= startAfter {
			return nil
		}

		if delimiter != "" && prefix != "" {
			afterPrefix := strings.TrimPrefix(key, prefix)
			if idx := strings.Index(afterPrefix, delimiter); idx >= 0 {
				commonPrefix := prefix + afterPrefix[:idx+len(delimiter)]
				commonPrefixes[commonPrefix] = true
				return nil
			}
		} else if delimiter != "" {
			if idx := strings.Index(key, delimiter); idx >= 0 {
				commonPrefix := key[:idx+len(delimiter)]
				commonPrefixes[commonPrefix] = true
				return nil
			}
		}

		// Get metadata from disk
		var etag string
		metaPath := path + ".s3pit_meta.json"
		if data, err := os.ReadFile(metaPath); err == nil {
			var meta map[string]interface{}
			if json.Unmarshal(data, &meta) == nil {
				if e, ok := meta["etag"].(string); ok {
					if !strings.HasPrefix(e, "\"") {
						etag = fmt.Sprintf("\"%s\"", e)
					} else {
						etag = e
					}
				}
			}
		}

		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         etag,
		})

		return nil
	})

	if err != nil {
		return nil, nil, "", err
	}

	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})

	var nextToken string
	if len(objects) > maxKeys {
		nextToken = objects[maxKeys-1].Key
		objects = objects[:maxKeys]
	}

	var prefixes []string
	for p := range commonPrefixes {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	return objects, prefixes, nextToken, nil
}

func (fs *FileSystemStorage) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) (string, error) {
	// Use per-bucket locks to avoid deadlock
	srcLock := fs.getBucketLock(srcBucket)
	dstLock := fs.getBucketLock(dstBucket)

	// Lock in consistent order to prevent deadlock
	if srcBucket < dstBucket {
		srcLock.RLock()
		defer srcLock.RUnlock()
		dstLock.Lock()
		defer dstLock.Unlock()
	} else if srcBucket > dstBucket {
		dstLock.Lock()
		defer dstLock.Unlock()
		srcLock.RLock()
		defer srcLock.RUnlock()
	} else {
		srcLock.Lock()
		defer srcLock.Unlock()
	}

	srcPath := filepath.Join(fs.baseDir, srcBucket, srcKey)
	dstPath := filepath.Join(fs.baseDir, dstBucket, dstKey)

	srcFile, err := os.Open(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrObjectNotFound
		}
		return "", err
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", err
	}

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dstFile.Close()

	hash := md5.New()
	writer := io.MultiWriter(dstFile, hash)

	if _, err := io.Copy(writer, srcFile); err != nil {
		return "", err
	}

	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash.Sum(nil)))

	// Copy and update metadata
	srcMetaPath := srcPath + ".s3pit_meta.json"
	dstMetaPath := dstPath + ".s3pit_meta.json"

	if srcMetaData, err := os.ReadFile(srcMetaPath); err == nil {
		var meta map[string]interface{}
		if json.Unmarshal(srcMetaData, &meta) == nil {
			meta["etag"] = strings.Trim(etag, "\"")
			meta["modified"] = time.Now().UTC()

			if data, err := json.Marshal(meta); err == nil {
				_ = os.WriteFile(dstMetaPath, data, 0644)
			}
		}
	}

	return etag, nil
}

// InitiateMultipartUpload starts a new multipart upload
func (fs *FileSystemStorage) InitiateMultipartUpload(bucket, key string) (string, error) {
	lock := fs.getBucketLock(bucket)
	lock.RLock()
	bucketPath := filepath.Join(fs.baseDir, bucket)
	if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
		lock.RUnlock()
		return "", ErrBucketNotFound
	}
	lock.RUnlock()

	uploadId := fmt.Sprintf("upload-%d-%s", time.Now().UnixNano(), key)

	upload := &MultipartUpload{
		Bucket:    bucket,
		Key:       key,
		UploadId:  uploadId,
		Initiated: time.Now().UTC(),
		Parts:     make(map[int]PartInfo),
	}

	fs.uploads.Store(uploadId, upload)

	// Create temporary directory for parts
	tempDir := filepath.Join(fs.baseDir, ".s3pit_uploads", uploadId)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		fs.uploads.Delete(uploadId)
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}

	return uploadId, nil
}

// UploadPart uploads a part for a multipart upload
func (fs *FileSystemStorage) UploadPart(bucket, key, uploadId string, partNumber int, reader io.Reader, size int64) (string, error) {
	uploadInterface, exists := fs.uploads.Load(uploadId)
	if !exists {
		return "", fmt.Errorf("upload not found: %s", uploadId)
	}

	upload := uploadInterface.(*MultipartUpload)
	if upload.Bucket != bucket || upload.Key != key {
		return "", fmt.Errorf("upload mismatch")
	}

	// Save part to temporary file
	partPath := filepath.Join(fs.baseDir, ".s3pit_uploads", uploadId, fmt.Sprintf("part-%d", partNumber))

	partFile, err := os.Create(partPath)
	if err != nil {
		return "", err
	}
	defer partFile.Close()

	hash := md5.New()
	writer := io.MultiWriter(partFile, hash)

	written, err := io.CopyN(writer, reader, size)
	if err != nil && err != io.EOF {
		return "", err
	}

	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash.Sum(nil)))

	// Update upload parts
	upload.Parts[partNumber] = PartInfo{
		PartNumber:   partNumber,
		Size:         written,
		ETag:         etag,
		LastModified: time.Now().UTC(),
	}

	return etag, nil
}

// CompleteMultipartUpload completes a multipart upload
func (fs *FileSystemStorage) CompleteMultipartUpload(bucket, key, uploadId string, parts []CompletedPart) (string, error) {
	uploadInterface, exists := fs.uploads.LoadAndDelete(uploadId)
	if !exists {
		return "", fmt.Errorf("upload not found: %s", uploadId)
	}

	upload := uploadInterface.(*MultipartUpload)
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

	lock := fs.getBucketLock(bucket)
	lock.Lock()
	defer lock.Unlock()

	// Create the final object
	objectPath := filepath.Join(fs.baseDir, bucket, key)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0755); err != nil {
		return "", err
	}

	finalFile, err := os.Create(objectPath)
	if err != nil {
		return "", err
	}
	defer finalFile.Close()

	hash := md5.New()
	writer := io.MultiWriter(finalFile, hash)

	// Concatenate all parts
	var totalSize int64

	for _, part := range parts {
		partPath := filepath.Join(fs.baseDir, ".s3pit_uploads", uploadId, fmt.Sprintf("part-%d", part.PartNumber))
		partFile, err := os.Open(partPath)
		if err != nil {
			return "", err
		}

		written, err := io.Copy(writer, partFile)
		if err != nil {
			partFile.Close()
			return "", err
		}
		partFile.Close()
		totalSize += written
	}

	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash.Sum(nil)))

	// Save metadata
	meta := map[string]interface{}{
		"size":         totalSize,
		"content_type": "application/octet-stream",
		"etag":         strings.Trim(etag, "\""),
		"modified":     time.Now().UTC(),
	}

	metaPath := objectPath + ".s3pit_meta.json"
	metaData, _ := json.Marshal(meta)
	_ = os.WriteFile(metaPath, metaData, 0644)

	// Clean up temporary files
	tempDir := filepath.Join(fs.baseDir, ".s3pit_uploads", uploadId)
	os.RemoveAll(tempDir)

	return etag, nil
}

// AbortMultipartUpload cancels a multipart upload
func (fs *FileSystemStorage) AbortMultipartUpload(bucket, key, uploadId string) error {
	uploadInterface, exists := fs.uploads.LoadAndDelete(uploadId)
	if !exists {
		return fmt.Errorf("upload not found: %s", uploadId)
	}

	upload := uploadInterface.(*MultipartUpload)
	if upload.Bucket != bucket || upload.Key != key {
		return fmt.Errorf("upload mismatch")
	}

	// Clean up temporary files
	tempDir := filepath.Join(fs.baseDir, ".s3pit_uploads", uploadId)
	return os.RemoveAll(tempDir)
}

// ListParts lists the parts of a multipart upload
func (fs *FileSystemStorage) ListParts(bucket, key, uploadId string) ([]PartInfo, error) {
	uploadInterface, exists := fs.uploads.Load(uploadId)
	if !exists {
		return nil, fmt.Errorf("upload not found: %s", uploadId)
	}

	upload := uploadInterface.(*MultipartUpload)
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
