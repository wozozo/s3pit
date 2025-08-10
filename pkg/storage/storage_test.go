package storage

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func TestMemoryStorage(t *testing.T) {
	store := NewMemoryStorage()

	t.Run("CreateAndListBuckets", func(t *testing.T) {
		// Create bucket
		created, err := store.CreateBucket("test-bucket")
		if err != nil {
			t.Fatalf("Failed to create bucket: %v", err)
		}
		if !created {
			t.Error("Expected bucket to be created")
		}

		// List buckets
		buckets, err := store.ListBuckets()
		if err != nil {
			t.Fatalf("Failed to list buckets: %v", err)
		}

		found := false
		for _, b := range buckets {
			if b.Name == "test-bucket" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Created bucket not found in list")
		}
	})

	t.Run("PutAndGetObject", func(t *testing.T) {
		bucket := "test-bucket-2"
		key := "test-object.txt"
		content := []byte("Hello, World!")

		// Create bucket first
		_, _ = store.CreateBucket(bucket)

		// Put object
		_, err := store.PutObject(bucket, key, bytes.NewReader(content), int64(len(content)), "text/plain")
		if err != nil {
			t.Fatalf("Failed to put object: %v", err)
		}

		// Get object
		reader, objMeta, err := store.GetObject(bucket, key)
		if err != nil {
			t.Fatalf("Failed to get object: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read object: %v", err)
		}

		if !bytes.Equal(data, content) {
			t.Errorf("Object content mismatch. Got %s, want %s", data, content)
		}

		if objMeta.ContentType != "text/plain" {
			t.Errorf("Content-Type mismatch. Got %s, want text/plain", objMeta.ContentType)
		}
	})

	t.Run("DeleteObject", func(t *testing.T) {
		bucket := "test-bucket-3"
		key := "delete-me.txt"

		// Setup
		_, _ = store.CreateBucket(bucket)
		_, _ = store.PutObject(bucket, key, bytes.NewReader([]byte("delete")), 6, "text/plain")

		// Delete object
		err := store.DeleteObject(bucket, key)
		if err != nil {
			t.Fatalf("Failed to delete object: %v", err)
		}

		// Verify deletion
		_, _, err = store.GetObject(bucket, key)
		if err == nil {
			t.Error("Object should not exist after deletion")
		}
	})

	t.Run("ListObjects", func(t *testing.T) {
		bucket := "test-bucket-4"
		_, _ = store.CreateBucket(bucket)

		// Create multiple objects
		objects := []string{
			"dir1/file1.txt",
			"dir1/file2.txt",
			"dir2/file3.txt",
			"file4.txt",
		}

		for _, key := range objects {
			_, _ = store.PutObject(bucket, key, bytes.NewReader([]byte("test")), 4, "text/plain")
		}

		// List with prefix
		contents, _, _, err := store.ListObjects(bucket, "dir1/", "/", 100, "")
		if err != nil {
			t.Fatalf("Failed to list objects: %v", err)
		}

		if len(contents) != 2 {
			t.Errorf("Expected 2 objects with prefix 'dir1/', got %d", len(contents))
		}

		// List with delimiter
		_, prefixes, _, err := store.ListObjects(bucket, "", "/", 100, "")
		if err != nil {
			t.Fatalf("Failed to list objects with delimiter: %v", err)
		}

		if len(prefixes) != 2 {
			t.Errorf("Expected 2 common prefixes, got %d", len(prefixes))
		}
	})

	t.Run("CopyObject", func(t *testing.T) {
		srcBucket := "test-bucket-5"
		dstBucket := "test-bucket-6"
		srcKey := "source.txt"
		dstKey := "destination.txt"
		content := []byte("Copy me!")

		// Setup
		_, _ = store.CreateBucket(srcBucket)
		_, _ = store.CreateBucket(dstBucket)
		_, _ = store.PutObject(srcBucket, srcKey, bytes.NewReader(content), int64(len(content)), "text/plain")

		// Copy object
		_, err := store.CopyObject(srcBucket, srcKey, dstBucket, dstKey)
		if err != nil {
			t.Fatalf("Failed to copy object: %v", err)
		}

		// Verify copy
		reader, _, err := store.GetObject(dstBucket, dstKey)
		if err != nil {
			t.Fatalf("Failed to get copied object: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read copied object: %v", err)
		}

		if !bytes.Equal(data, content) {
			t.Errorf("Copied content mismatch. Got %s, want %s", data, content)
		}
	})

	t.Run("MultipartUpload", func(t *testing.T) {
		bucket := "test-bucket-7"
		key := "multipart-object.txt"
		_, _ = store.CreateBucket(bucket)

		// Initiate multipart upload
		uploadID, err := store.InitiateMultipartUpload(bucket, key)
		if err != nil {
			t.Fatalf("Failed to initiate multipart upload: %v", err)
		}

		// Upload parts
		parts := []struct {
			partNumber int
			content    []byte
		}{
			{1, []byte("Part 1 ")},
			{2, []byte("Part 2 ")},
			{3, []byte("Part 3")},
		}

		var completedParts []CompletedPart
		for _, part := range parts {
			etag, err := store.UploadPart(bucket, key, uploadID, part.partNumber,
				bytes.NewReader(part.content), int64(len(part.content)))
			if err != nil {
				t.Fatalf("Failed to upload part %d: %v", part.partNumber, err)
			}
			completedParts = append(completedParts, CompletedPart{
				PartNumber: part.partNumber,
				ETag:       etag,
			})
		}

		// Complete multipart upload
		_, err = store.CompleteMultipartUpload(bucket, key, uploadID, completedParts)
		if err != nil {
			t.Fatalf("Failed to complete multipart upload: %v", err)
		}

		// Verify the assembled object
		reader, _, err := store.GetObject(bucket, key)
		if err != nil {
			t.Fatalf("Failed to get multipart object: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read multipart object: %v", err)
		}

		expected := "Part 1 Part 2 Part 3"
		if string(data) != expected {
			t.Errorf("Multipart content mismatch. Got %s, want %s", data, expected)
		}
	})

	t.Run("AbortMultipartUpload", func(t *testing.T) {
		bucket := "test-bucket-8"
		key := "aborted-multipart.txt"
		_, _ = store.CreateBucket(bucket)

		// Initiate multipart upload
		uploadID, err := store.InitiateMultipartUpload(bucket, key)
		if err != nil {
			t.Fatalf("Failed to initiate multipart upload: %v", err)
		}

		// Upload a part
		_, err = store.UploadPart(bucket, key, uploadID, 1,
			bytes.NewReader([]byte("test")), 4)
		if err != nil {
			t.Fatalf("Failed to upload part: %v", err)
		}

		// Abort multipart upload
		err = store.AbortMultipartUpload(bucket, key, uploadID)
		if err != nil {
			t.Fatalf("Failed to abort multipart upload: %v", err)
		}

		// Verify object doesn't exist
		_, _, err = store.GetObject(bucket, key)
		if err == nil {
			t.Error("Object should not exist after aborting multipart upload")
		}
	})
}

func TestFileSystemStorage(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	store, err := NewFileSystemStorage(tempDir)
	if err != nil {
		t.Fatalf("Failed to create FileSystemStorage: %v", err)
	}

	t.Run("CreateAndListBuckets", func(t *testing.T) {
		// Create bucket
		created, err := store.CreateBucket("fs-test-bucket")
		if err != nil {
			t.Fatalf("Failed to create bucket: %v", err)
		}
		if !created {
			t.Error("Expected bucket to be created")
		}

		// List buckets
		buckets, err := store.ListBuckets()
		if err != nil {
			t.Fatalf("Failed to list buckets: %v", err)
		}

		found := false
		for _, b := range buckets {
			if b.Name == "fs-test-bucket" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Created bucket not found in list")
		}
	})

	t.Run("PutAndGetObject", func(t *testing.T) {
		bucket := "fs-test-bucket-2"
		key := "test/object.txt"
		content := []byte("FileSystem Storage Test")

		// Create bucket first
		_, _ = store.CreateBucket(bucket)

		// Put object
		_, err := store.PutObject(bucket, key, bytes.NewReader(content), int64(len(content)), "text/plain")
		if err != nil {
			t.Fatalf("Failed to put object: %v", err)
		}

		// Get object
		reader, objMeta, err := store.GetObject(bucket, key)
		if err != nil {
			t.Fatalf("Failed to get object: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read object: %v", err)
		}

		if !bytes.Equal(data, content) {
			t.Errorf("Object content mismatch. Got %s, want %s", data, content)
		}

		if objMeta.ContentType != "text/plain" {
			t.Errorf("Content-Type mismatch. Got %s, want text/plain", objMeta.ContentType)
		}
	})

	t.Run("ListObjectsWithDelimiter", func(t *testing.T) {
		bucket := "fs-test-bucket-3"
		_, _ = store.CreateBucket(bucket)

		// Create nested directory structure
		objects := []string{
			"docs/readme.txt",
			"docs/guide.txt",
			"images/logo.png",
			"images/banner.jpg",
			"index.html",
		}

		for _, key := range objects {
			_, _ = store.PutObject(bucket, key, bytes.NewReader([]byte("test")), 4, "text/plain")
		}

		// List root level with delimiter
		contents, prefixes, _, err := store.ListObjects(bucket, "", "/", 100, "")
		if err != nil {
			t.Fatalf("Failed to list objects: %v", err)
		}

		// Should have 1 object (index.html) and 2 common prefixes (docs/, images/)
		if len(contents) != 1 {
			t.Errorf("Expected 1 root object, got %d", len(contents))
		}

		if len(prefixes) != 2 {
			t.Errorf("Expected 2 common prefixes, got %d", len(prefixes))
		}
	})
}

func TestObjectMetadata(t *testing.T) {
	store := NewMemoryStorage()
	bucket := "metadata-test"
	key := "test-object"

	_, _ = store.CreateBucket(bucket)

	t.Run("PreserveMetadata", func(t *testing.T) {
		content := []byte(`{"test": "data"}`)
		_, err := store.PutObject(bucket, key, bytes.NewReader(content), int64(len(content)), "application/json")
		if err != nil {
			t.Fatalf("Failed to put object: %v", err)
		}

		// Get object metadata
		meta, err := store.GetObjectMetadata(bucket, key)
		if err != nil {
			t.Fatalf("Failed to get object metadata: %v", err)
		}

		if meta.ContentType != "application/json" {
			t.Errorf("Content-Type not preserved. Got %s, want application/json", meta.ContentType)
		}
	})
}

func TestPagination(t *testing.T) {
	store := NewMemoryStorage()
	bucket := "pagination-test"

	_, _ = store.CreateBucket(bucket)

	// Create many objects
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("object-%02d.txt", i)
		_, _ = store.PutObject(bucket, key, bytes.NewReader([]byte("test")), 4, "text/plain")
	}

	t.Run("ListWithMaxKeys", func(t *testing.T) {
		// First page
		contents, _, nextToken, err := store.ListObjects(bucket, "", "", 3, "")
		if err != nil {
			t.Fatalf("Failed to list objects: %v", err)
		}

		if len(contents) != 3 {
			t.Errorf("Expected 3 objects in first page, got %d", len(contents))
		}

		if nextToken == "" {
			t.Error("Expected continuation token for more results")
		}

		// Second page
		contents2, _, _, err := store.ListObjects(bucket, "", "", 3, nextToken)
		if err != nil {
			t.Fatalf("Failed to list second page: %v", err)
		}

		if len(contents2) != 3 {
			t.Errorf("Expected 3 objects in second page, got %d", len(contents2))
		}
	})
}
