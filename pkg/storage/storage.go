package storage

import (
	"io"
	"time"

	storageerrors "github.com/wozozo/s3pit/pkg/errors"
)

// Re-export common errors for backward compatibility
var (
	ErrBucketNotFound = storageerrors.ErrBucketNotFound
	ErrBucketNotEmpty = storageerrors.ErrBucketNotEmpty
	ErrObjectNotFound = storageerrors.ErrObjectNotFound
	ErrBucketExists   = storageerrors.ErrBucketExists
)

type Storage interface {
	CreateBucket(bucket string) (bool, error)
	DeleteBucket(bucket string) error
	ListBuckets() ([]BucketInfo, error)
	BucketExists(bucket string) (bool, error)

	PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (string, error)
	GetObject(bucket, key string) (io.ReadCloser, *ObjectMetadata, error)
	GetObjectMetadata(bucket, key string) (*ObjectMetadata, error)
	DeleteObject(bucket, key string) error
	ListObjects(bucket, prefix, delimiter string, maxKeys int, continuationToken string) ([]ObjectInfo, []string, string, error)

	CopyObject(srcBucket, srcKey, dstBucket, dstKey string) (string, error)

	// Multipart upload operations
	InitiateMultipartUpload(bucket, key string) (string, error)
	UploadPart(bucket, key, uploadId string, partNumber int, reader io.Reader, size int64) (string, error)
	CompleteMultipartUpload(bucket, key, uploadId string, parts []CompletedPart) (string, error)
	AbortMultipartUpload(bucket, key, uploadId string) error
	ListParts(bucket, key, uploadId string) ([]PartInfo, error)
}

type BucketInfo struct {
	Name         string
	CreationDate time.Time
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

type ObjectMetadata struct {
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
	Metadata     map[string]string
}

type CompletedPart struct {
	PartNumber int
	ETag       string
}

type PartInfo struct {
	PartNumber   int
	Size         int64
	ETag         string
	LastModified time.Time
}

type MultipartUpload struct {
	Bucket    string
	Key       string
	UploadId  string
	Initiated time.Time
	Parts     map[int]PartInfo
}
