package storage

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// ValidateBucketName validates that a bucket name is not empty and follows S3 naming rules
func ValidateBucketName(bucket string) error {
	if bucket == "" {
		return fmt.Errorf("bucket name cannot be empty")
	}
	
	// S3 bucket naming rules
	if len(bucket) < 3 || len(bucket) > 63 {
		return fmt.Errorf("bucket name must be between 3 and 63 characters")
	}
	
	// Must start and end with lowercase letter or number
	if !isAlphanumeric(bucket[0]) || !isAlphanumeric(bucket[len(bucket)-1]) {
		return fmt.Errorf("bucket name must start and end with a lowercase letter or number")
	}
	
	// Check for invalid characters and consecutive dots/hyphens
	for i, ch := range bucket {
		if !isValidBucketChar(ch) {
			return fmt.Errorf("bucket name contains invalid character: %c", ch)
		}
		
		// No consecutive dots or hyphens
		if i > 0 && (ch == '.' || ch == '-') && bucket[i-1] == byte(ch) {
			return fmt.Errorf("bucket name cannot contain consecutive dots or hyphens")
		}
	}
	
	// Cannot be formatted as IP address
	if looksLikeIPAddress(bucket) {
		return fmt.Errorf("bucket name cannot be formatted as an IP address")
	}
	
	return nil
}

// ValidateObjectKey validates that an object key is valid
func ValidateObjectKey(key string) error {
	if key == "" {
		return fmt.Errorf("object key cannot be empty")
	}
	
	// Check for null bytes
	if strings.Contains(key, "\x00") {
		return fmt.Errorf("object key cannot contain null bytes")
	}
	
	// Key length limit (S3 limit is 1024 bytes)
	if len(key) > 1024 {
		return fmt.Errorf("object key is too long (max 1024 bytes)")
	}
	
	return nil
}

// CalculateETag calculates the ETag (MD5 hash) for the given data
func CalculateETag(data []byte) string {
	hash := md5.Sum(data)
	return fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))
}

// CalculateETagFromReader calculates the ETag from an io.Reader
func CalculateETagFromReader(reader io.Reader) (string, int64, []byte, error) {
	hash := md5.New()
	
	// Buffer to store the data for potential reuse
	var buf []byte
	teeReader := io.TeeReader(reader, &writerBuffer{&buf})
	
	written, err := io.Copy(hash, teeReader)
	if err != nil {
		return "", 0, nil, fmt.Errorf("failed to calculate hash: %w", err)
	}
	
	etag := fmt.Sprintf("\"%s\"", hex.EncodeToString(hash.Sum(nil)))
	return etag, written, buf, nil
}

// FormatETag ensures the ETag is properly formatted with quotes
func FormatETag(etag string) string {
	// Remove existing quotes if any
	etag = strings.Trim(etag, "\"")
	// Add quotes
	return fmt.Sprintf("\"%s\"", etag)
}

// StripETagQuotes removes the quotes from an ETag
func StripETagQuotes(etag string) string {
	return strings.Trim(etag, "\"")
}

// Helper functions

func isAlphanumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

func isValidBucketChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || 
		   (ch >= '0' && ch <= '9') || 
		   ch == '.' || ch == '-'
}

func looksLikeIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		
		// Check if all characters are digits
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
		
		// Check if it's a valid octet (0-255)
		if len(part) > 1 && part[0] == '0' {
			return false // No leading zeros
		}
		
		val := 0
		for _, ch := range part {
			val = val*10 + int(ch-'0')
		}
		if val > 255 {
			return false
		}
	}
	
	return true
}

// writerBuffer is a simple buffer that implements io.Writer
type writerBuffer struct {
	buf *[]byte
}

func (w *writerBuffer) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// CalculateMultipartETag calculates the ETag for a multipart upload
func CalculateMultipartETag(partETags []string) string {
	h := md5.New()
	for _, etag := range partETags {
		// Remove quotes from individual part ETags
		etag = StripETagQuotes(etag)
		data, _ := hex.DecodeString(etag)
		h.Write(data)
	}
	return fmt.Sprintf("\"%s-%d\"", hex.EncodeToString(h.Sum(nil)), len(partETags))
}