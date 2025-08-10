package errors

import (
	"errors"
	"fmt"
)

// Storage errors
var (
	// Bucket-related errors
	ErrBucketNotFound      = errors.New("bucket not found")
	ErrBucketNotEmpty      = errors.New("bucket not empty")
	ErrBucketExists        = errors.New("bucket already exists")
	ErrInvalidBucketName   = errors.New("invalid bucket name")
	ErrBucketNameEmpty     = errors.New("bucket name cannot be empty")
	ErrBucketNameTooLong   = errors.New("bucket name must be between 3 and 63 characters")
	ErrBucketNameInvalidChar = errors.New("bucket name contains invalid character")
	ErrBucketNameInvalidFormat = errors.New("bucket name cannot be formatted as an IP address")
	
	// Object-related errors
	ErrObjectNotFound      = errors.New("object not found")
	ErrInvalidObjectKey    = errors.New("invalid object key")
	ErrObjectKeyEmpty      = errors.New("object key cannot be empty")
	ErrObjectKeyTooLong    = errors.New("object key is too long (max 1024 bytes)")
	ErrObjectKeyNullBytes  = errors.New("object key cannot contain null bytes")
	
	// Multipart upload errors
	ErrUploadNotFound      = errors.New("upload not found")
	ErrUploadMismatch      = errors.New("upload mismatch")
	ErrPartNotFound        = errors.New("part not found")
	
	// Directory/file system errors
	ErrDirectoryCreation   = errors.New("failed to create directory")
	ErrFileCreation        = errors.New("failed to create file")
	ErrFileWrite           = errors.New("failed to write file")
	ErrFileRead            = errors.New("failed to read file")
	ErrFileMove            = errors.New("failed to move file")
	
	// Policy errors
	ErrPolicyParseFailed   = errors.New("failed to parse bucket policy")
)

// WrapStorageError wraps a storage error with operation context
func WrapStorageError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("storage %s failed: %w", operation, err)
}

// WrapBucketError wraps a bucket-related error with bucket name context
func WrapBucketError(bucket string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("bucket %s: %w", bucket, err)
}

// WrapObjectError wraps an object-related error with bucket and key context
func WrapObjectError(bucket, key string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("object %s/%s: %w", bucket, key, err)
}

// WrapMultipartError wraps a multipart upload error with upload ID context
func WrapMultipartError(uploadID string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("multipart upload %s: %w", uploadID, err)
}

// WrapFileSystemError wraps a file system error with path context
func WrapFileSystemError(path string, operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %s: %w", operation, path, err)
}

// IsNotFound checks if an error is a not found error (bucket or object)
func IsNotFound(err error) bool {
	return errors.Is(err, ErrBucketNotFound) || errors.Is(err, ErrObjectNotFound)
}

// IsAlreadyExists checks if an error is an already exists error
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrBucketExists)
}

// IsInvalidInput checks if an error is due to invalid input
func IsInvalidInput(err error) bool {
	return errors.Is(err, ErrInvalidBucketName) ||
		errors.Is(err, ErrBucketNameEmpty) ||
		errors.Is(err, ErrBucketNameTooLong) ||
		errors.Is(err, ErrBucketNameInvalidChar) ||
		errors.Is(err, ErrBucketNameInvalidFormat) ||
		errors.Is(err, ErrInvalidObjectKey) ||
		errors.Is(err, ErrObjectKeyEmpty) ||
		errors.Is(err, ErrObjectKeyTooLong) ||
		errors.Is(err, ErrObjectKeyNullBytes)
}