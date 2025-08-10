package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	
	storageerrors "github.com/wozozo/s3pit/pkg/errors"
)

type FileSystemStorage struct {
	baseDir      string
	bucketLocks  sync.Map // Per-bucket locks for better concurrency
	multipartMgr *FileSystemMultipartManager
}

func NewFileSystemStorage(baseDir string) (*FileSystemStorage, error) {
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, storageerrors.WrapFileSystemError(baseDir, "resolve directory", err)
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, storageerrors.WrapFileSystemError(absPath, "create directory", err)
	}

	return &FileSystemStorage{
		baseDir:      absPath,
		multipartMgr: NewFileSystemMultipartManager(absPath),
	}, nil
}

// getBucketLock returns a lock for the specific bucket
func (fs *FileSystemStorage) getBucketLock(bucket string) *sync.RWMutex {
	lock, _ := fs.bucketLocks.LoadOrStore(bucket, &sync.RWMutex{})
	return lock.(*sync.RWMutex)
}

// saveMetadata saves object metadata to a JSON file
func (fs *FileSystemStorage) saveMetadata(bucket, key string, metadata *ObjectMetadata) error {
	objectPath := filepath.Join(fs.baseDir, bucket, key)
	metaPath := objectPath + ".s3pit_meta.json"
	
	meta := map[string]interface{}{
		"content-type": metadata.ContentType,
		"etag":         StripETagQuotes(metadata.ETag),
		"size":         metadata.Size,
		"modified":     metadata.LastModified,
	}
	
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	
	return os.WriteFile(metaPath, metaData, 0644)
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
		return false, storageerrors.WrapFileSystemError(bucketPath, "create directory", err)
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
		return "", storageerrors.WrapFileSystemError(objectDir, "create directory", err)
	}

	// Use a temporary file for atomic writes
	tempFile, err := os.CreateTemp(objectDir, ".upload_*")
	if err != nil {
		return "", storageerrors.WrapFileSystemError(objectDir, "create temp file", err)
	}
	tempPath := tempFile.Name()

	// Clean up on error
	defer func() {
		if tempFile != nil {
			tempFile.Close()
			os.Remove(tempPath)
		}
	}()

	// Read all data to calculate ETag
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", storageerrors.WrapStorageError("read object data", err)
	}

	// Calculate ETag using helper
	etag := CalculateETag(data)

	// Write data to temp file
	if _, err := tempFile.Write(data); err != nil {
		return "", storageerrors.WrapFileSystemError(tempPath, "write file", err)
	}

	if err := tempFile.Close(); err != nil {
		return "", err
	}
	tempFile = nil // Mark as closed

	// Atomic rename
	if err := os.Rename(tempPath, objectPath); err != nil {
		return "", storageerrors.WrapFileSystemError(objectPath, "move file", err)
	}

	// Save metadata
	metaPath := objectPath + ".s3pit_meta.json"
	meta := map[string]interface{}{
		"content-type": contentType,
		"etag":         StripETagQuotes(etag),
		"size":         int64(len(data)),
		"modified":     time.Now().UTC(),
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return etag, nil
	}

	_ = os.WriteFile(metaPath, metaData, 0644)

	return etag, nil
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

	// Read source file data
	srcData, err := io.ReadAll(srcFile)
	if err != nil {
		return "", err
	}

	// Write to destination
	if _, err := dstFile.Write(srcData); err != nil {
		return "", err
	}

	// Calculate ETag using helper
	etag := CalculateETag(srcData)

	// Copy and update metadata
	srcMetaPath := srcPath + ".s3pit_meta.json"
	dstMetaPath := dstPath + ".s3pit_meta.json"

	if srcMetaData, err := os.ReadFile(srcMetaPath); err == nil {
		var meta map[string]interface{}
		if json.Unmarshal(srcMetaData, &meta) == nil {
			meta["etag"] = StripETagQuotes(etag)
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

	return fs.multipartMgr.InitiateUpload(bucket, key)
}

// UploadPart uploads a part for a multipart upload
func (fs *FileSystemStorage) UploadPart(bucket, key, uploadId string, partNumber int, reader io.Reader, size int64) (string, error) {
	upload, exists := fs.multipartMgr.GetUpload(uploadId)
	if !exists {
		return "", storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return "", storageerrors.ErrUploadMismatch
	}

	return fs.multipartMgr.StorePart(uploadId, partNumber, reader, size)
}

// CompleteMultipartUpload completes a multipart upload
func (fs *FileSystemStorage) CompleteMultipartUpload(bucket, key, uploadId string, parts []CompletedPart) (string, error) {
	upload, exists := fs.multipartMgr.GetUpload(uploadId)
	if !exists {
		return "", storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return "", storageerrors.ErrUploadMismatch
	}

	// Combine all parts
	objectPath := filepath.Join(fs.baseDir, bucket, key)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0755); err != nil {
		return "", err
	}

	outFile, err := os.Create(objectPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	// Copy each part to the final file
	for _, part := range parts {
		partPath := fs.multipartMgr.GetPartPath(uploadId, part.PartNumber)
		partFile, err := os.Open(partPath)
		if err != nil {
			return "", storageerrors.WrapMultipartError(fmt.Sprintf("part %d", part.PartNumber), err)
		}

		if _, err := io.Copy(outFile, partFile); err != nil {
			partFile.Close()
			return "", storageerrors.WrapMultipartError(fmt.Sprintf("part %d copy", part.PartNumber), err)
		}
		partFile.Close()
	}

	// Calculate final ETag
	finalFile, err := os.Open(objectPath)
	if err != nil {
		return "", err
	}
	defer finalFile.Close()

	data, err := io.ReadAll(finalFile)
	if err != nil {
		return "", err
	}
	etag := CalculateETag(data)

	// Save metadata
	metadata := &ObjectMetadata{
		ContentType:  "application/octet-stream",
		Size:         int64(len(data)),
		LastModified: time.Now().UTC(),
		ETag:         etag,
	}

	if err := fs.saveMetadata(bucket, key, metadata); err != nil {
		return "", err
	}

	// Clean up the multipart upload
	_ = fs.multipartMgr.DeleteUpload(uploadId)

	return etag, nil
}

// AbortMultipartUpload cancels a multipart upload
func (fs *FileSystemStorage) AbortMultipartUpload(bucket, key, uploadId string) error {
	upload, exists := fs.multipartMgr.GetUpload(uploadId)
	if !exists {
		return storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return storageerrors.ErrUploadMismatch
	}

	return fs.multipartMgr.DeleteUpload(uploadId)
}

// ListParts lists the parts of a multipart upload
func (fs *FileSystemStorage) ListParts(bucket, key, uploadId string) ([]PartInfo, error) {
	upload, exists := fs.multipartMgr.GetUpload(uploadId)
	if !exists {
		return nil, storageerrors.WrapMultipartError(uploadId, storageerrors.ErrUploadNotFound)
	}

	if upload.Bucket != bucket || upload.Key != key {
		return nil, storageerrors.ErrUploadMismatch
	}

	return fs.multipartMgr.ListParts(uploadId)
}
