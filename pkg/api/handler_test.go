package api

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/auth"
	"github.com/wozozo/s3pit/pkg/storage"
	"github.com/wozozo/s3pit/pkg/tenant"
)

func setupTestHandler() (*Handler, *gin.Engine) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Port:             8080,
		GlobalDirectory:  "/tmp/test",
		AuthMode:         "none",
		AutoCreateBucket: true,
	}

	authHandler, _ := auth.NewHandler("none", "", "")

	handler := NewHandler(
		storage.NewMemoryStorage(),
		authHandler,
		tenant.NewManager(""),
		cfg,
	)

	router := gin.New()
	// Setup routes manually for testing
	router.GET("/", handler.ListBuckets)
	router.HEAD("/:bucket", handler.HeadBucket)
	router.PUT("/:bucket", handler.CreateBucket)
	router.DELETE("/:bucket", handler.DeleteBucket)
	router.GET("/:bucket", handler.ListObjectsV2)

	router.PUT("/:bucket/*key", func(c *gin.Context) {
		if c.GetHeader("x-amz-copy-source") != "" {
			handler.CopyObject(c)
		} else if c.Query("uploadId") != "" && c.Query("partNumber") != "" {
			handler.UploadPart(c)
		} else {
			handler.PutObject(c)
		}
	})
	router.GET("/:bucket/*key", handler.GetObject)
	router.HEAD("/:bucket/*key", handler.HeadObject)
	router.DELETE("/:bucket/*key", handler.DeleteObject)
	router.POST("/:bucket/*key", func(c *gin.Context) {
		if _, exists := c.GetQuery("uploads"); exists {
			handler.InitiateMultipartUpload(c)
		} else if c.Query("uploadId") != "" {
			handler.CompleteMultipartUpload(c)
		}
	})

	return handler, router
}

func TestCreateBucket(t *testing.T) {
	handler, router := setupTestHandler()

	t.Run("CreateNewBucket", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test-bucket", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Verify x-s3pit-bucket-created header
		if w.Header().Get("x-s3pit-bucket-created") != "true" {
			t.Error("Expected x-s3pit-bucket-created header to be true")
		}
	})

	t.Run("CreateExistingBucket", func(t *testing.T) {
		// Create bucket first
		_, _ = handler.storage.CreateBucket("existing-bucket")

		req := httptest.NewRequest("PUT", "/existing-bucket", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Should not have x-s3pit-bucket-created header for existing bucket
		if w.Header().Get("x-s3pit-bucket-created") == "true" {
			t.Error("Should not have x-s3pit-bucket-created header for existing bucket")
		}
	})
}

func TestListBuckets(t *testing.T) {
	handler, router := setupTestHandler()

	// Create some buckets
	_, _ = handler.storage.CreateBucket("bucket-1")
	_, _ = handler.storage.CreateBucket("bucket-2")
	_, _ = handler.storage.CreateBucket("bucket-3")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result struct {
		XMLName xml.Name `xml:"ListAllMyBucketsResult"`
		Buckets struct {
			Bucket []struct {
				Name string `xml:"Name"`
			} `xml:"Bucket"`
		} `xml:"Buckets"`
	}

	err := xml.Unmarshal(w.Body.Bytes(), &result)
	if err != nil {
		t.Fatalf("Failed to parse XML response: %v", err)
	}

	if len(result.Buckets.Bucket) != 3 {
		t.Errorf("Expected 3 buckets, got %d", len(result.Buckets.Bucket))
	}
}

func TestPutGetObject(t *testing.T) {
	handler, router := setupTestHandler()

	bucket := "test-bucket"
	key := "test-object.txt"
	content := []byte("Hello, S3pit!")

	// Create bucket first
	_, _ = handler.storage.CreateBucket(bucket)

	t.Run("PutObject", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/"+bucket+"/"+key, bytes.NewReader(content))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check ETag header
		if w.Header().Get("ETag") == "" {
			t.Error("Expected ETag header")
		}
	})

	t.Run("GetObject", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/"+bucket+"/"+key, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		if !bytes.Equal(w.Body.Bytes(), content) {
			t.Errorf("Content mismatch. Got %s, want %s", w.Body.Bytes(), content)
		}

		if w.Header().Get("Content-Type") != "text/plain" {
			t.Errorf("Expected Content-Type text/plain, got %s", w.Header().Get("Content-Type"))
		}
	})

	t.Run("HeadObject", func(t *testing.T) {
		req := httptest.NewRequest("HEAD", "/"+bucket+"/"+key, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		if w.Header().Get("Content-Length") != "13" {
			t.Errorf("Expected Content-Length 13, got %s", w.Header().Get("Content-Length"))
		}
	})
}

func TestDeleteObject(t *testing.T) {
	handler, router := setupTestHandler()

	bucket := "test-bucket"
	key := "delete-me.txt"

	// Setup
	_, _ = handler.storage.CreateBucket(bucket)
	_, _ = handler.storage.PutObject(bucket, key, bytes.NewReader([]byte("delete")), 6, "text/plain")

	// Delete object
	req := httptest.NewRequest("DELETE", "/"+bucket+"/"+key, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	// Verify deletion
	req = httptest.NewRequest("GET", "/"+bucket+"/"+key, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d for deleted object, got %d", http.StatusNotFound, w.Code)
	}
}

func TestListObjectsV2(t *testing.T) {
	handler, router := setupTestHandler()

	bucket := "test-bucket"
	_, _ = handler.storage.CreateBucket(bucket)

	// Create test objects
	objects := []string{
		"dir1/file1.txt",
		"dir1/file2.txt",
		"dir2/file3.txt",
		"file4.txt",
	}

	for _, key := range objects {
		_, _ = handler.storage.PutObject(bucket, key, bytes.NewReader([]byte("test")), 4, "text/plain")
	}

	t.Run("ListWithPrefix", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/"+bucket+"?list-type=2&prefix=dir1/", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var result struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
		}

		err := xml.Unmarshal(w.Body.Bytes(), &result)
		if err != nil {
			t.Fatalf("Failed to parse XML response: %v", err)
		}

		if len(result.Contents) != 2 {
			t.Errorf("Expected 2 objects with prefix 'dir1/', got %d", len(result.Contents))
		}
	})

	t.Run("ListWithDelimiter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/"+bucket+"?list-type=2&delimiter=/", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var result struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
			CommonPrefixes []struct {
				Prefix string `xml:"Prefix"`
			} `xml:"CommonPrefixes"`
		}

		err := xml.Unmarshal(w.Body.Bytes(), &result)
		if err != nil {
			t.Fatalf("Failed to parse XML response: %v", err)
		}

		if len(result.CommonPrefixes) != 2 {
			t.Errorf("Expected 2 common prefixes, got %d", len(result.CommonPrefixes))
		}

		if len(result.Contents) != 1 {
			t.Errorf("Expected 1 root object, got %d", len(result.Contents))
		}
	})
}

func TestCopyObject(t *testing.T) {
	handler, router := setupTestHandler()

	srcBucket := "src-bucket"
	dstBucket := "dst-bucket"
	srcKey := "source.txt"
	dstKey := "destination.txt"
	content := []byte("Copy me!")

	// Setup
	_, _ = handler.storage.CreateBucket(srcBucket)
	_, _ = handler.storage.CreateBucket(dstBucket)
	_, _ = handler.storage.PutObject(srcBucket, srcKey, bytes.NewReader(content), int64(len(content)), "text/plain")

	// Copy object
	req := httptest.NewRequest("PUT", "/"+dstBucket+"/"+dstKey, nil)
	req.Header.Set("x-amz-copy-source", "/"+srcBucket+"/"+srcKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify copy
	req = httptest.NewRequest("GET", "/"+dstBucket+"/"+dstKey, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if !bytes.Equal(w.Body.Bytes(), content) {
		t.Errorf("Copied content mismatch. Got %s, want %s", w.Body.Bytes(), content)
	}
}

func TestMultipartUpload(t *testing.T) {
	handler, router := setupTestHandler()

	bucket := "test-bucket"
	key := "multipart-object.txt"
	_, _ = handler.storage.CreateBucket(bucket)

	var uploadID string

	t.Run("InitiateMultipartUpload", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/"+bucket+"/"+key+"?uploads", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var result struct {
			XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
			UploadId string   `xml:"UploadId"`
		}

		err := xml.Unmarshal(w.Body.Bytes(), &result)
		if err != nil {
			t.Fatalf("Failed to parse XML response: %v", err)
		}

		uploadID = result.UploadId
		if uploadID == "" {
			t.Error("Expected upload ID")
		}
	})

	var etags []string

	t.Run("UploadParts", func(t *testing.T) {
		parts := []struct {
			number  int
			content string
		}{
			{1, "Part 1 "},
			{2, "Part 2 "},
			{3, "Part 3"},
		}

		for _, part := range parts {
			req := httptest.NewRequest("PUT",
				"/"+bucket+"/"+key+"?partNumber="+strings.TrimSpace(fmt.Sprintf("%d", part.number))+"&uploadId="+uploadID,
				bytes.NewReader([]byte(part.content)))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d for part %d, got %d", http.StatusOK, part.number, w.Code)
			}

			etag := w.Header().Get("ETag")
			if etag == "" {
				t.Errorf("Expected ETag for part %d", part.number)
			}
			etags = append(etags, etag)
		}
	})

	t.Run("CompleteMultipartUpload", func(t *testing.T) {
		// Build completion XML
		type CompletedPart struct {
			PartNumber int    `xml:"PartNumber"`
			ETag       string `xml:"ETag"`
		}

		type CompleteMultipartUpload struct {
			XMLName xml.Name        `xml:"CompleteMultipartUpload"`
			Parts   []CompletedPart `xml:"Part"`
		}

		complete := CompleteMultipartUpload{
			Parts: []CompletedPart{
				{1, etags[0]},
				{2, etags[1]},
				{3, etags[2]},
			},
		}

		body, _ := xml.Marshal(complete)

		req := httptest.NewRequest("POST", "/"+bucket+"/"+key+"?uploadId="+uploadID,
			bytes.NewReader(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Verify the assembled object
		req = httptest.NewRequest("GET", "/"+bucket+"/"+key, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		expected := "Part 1 Part 2 Part 3"
		if w.Body.String() != expected {
			t.Errorf("Multipart content mismatch. Got %s, want %s", w.Body.String(), expected)
		}
	})
}

func TestErrorResponses(t *testing.T) {
	handler, router := setupTestHandler()

	t.Run("BucketNotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/nonexistent-bucket", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}

		var errResp struct {
			XMLName xml.Name `xml:"Error"`
			Code    string   `xml:"Code"`
			Message string   `xml:"Message"`
		}
		err := xml.Unmarshal(w.Body.Bytes(), &errResp)
		if err != nil {
			t.Fatalf("Failed to parse error XML: %v", err)
		}

		if errResp.Code != "NoSuchBucket" {
			t.Errorf("Expected error code NoSuchBucket, got %s", errResp.Code)
		}
	})

	t.Run("ObjectNotFound", func(t *testing.T) {
		_, _ = handler.storage.CreateBucket("test-bucket")

		req := httptest.NewRequest("GET", "/test-bucket/nonexistent.txt", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
		}

		var errResp struct {
			XMLName xml.Name `xml:"Error"`
			Code    string   `xml:"Code"`
			Message string   `xml:"Message"`
		}
		err := xml.Unmarshal(w.Body.Bytes(), &errResp)
		if err != nil {
			t.Fatalf("Failed to parse error XML: %v", err)
		}

		if errResp.Code != "NoSuchKey" {
			t.Errorf("Expected error code NoSuchKey, got %s", errResp.Code)
		}
	})
}

func TestImplicitBucketCreation(t *testing.T) {
	handler, router := setupTestHandler()

	t.Run("PutObjectCreatesImplicitBucket", func(t *testing.T) {
		bucket := "implicit-bucket"
		key := "test.txt"
		content := []byte("implicit bucket test")

		// Put object without creating bucket first
		req := httptest.NewRequest("PUT", "/"+bucket+"/"+key, bytes.NewReader(content))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check for implicit bucket creation header
		if w.Header().Get("x-s3pit-bucket-created") != "true" {
			t.Error("Expected x-s3pit-bucket-created header for implicit bucket creation")
		}

		// Verify bucket exists
		buckets, _ := handler.storage.ListBuckets()
		found := false
		for _, b := range buckets {
			if b.Name == bucket {
				found = true
				break
			}
		}
		if !found {
			t.Error("Implicit bucket was not created")
		}

		// Verify object exists
		req = httptest.NewRequest("GET", "/"+bucket+"/"+key, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		if !bytes.Equal(w.Body.Bytes(), content) {
			t.Error("Object content mismatch after implicit bucket creation")
		}
	})

	t.Run("MultipartUploadCreatesImplicitBucket", func(t *testing.T) {
		bucket := "implicit-multipart-bucket"
		key := "multipart.txt"

		// Initiate multipart upload without creating bucket first
		req := httptest.NewRequest("POST", "/"+bucket+"/"+key+"?uploads", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check for implicit bucket creation header
		if w.Header().Get("x-s3pit-bucket-created") != "true" {
			t.Error("Expected x-s3pit-bucket-created header for implicit bucket creation")
		}

		// Verify bucket exists
		buckets, _ := handler.storage.ListBuckets()
		found := false
		for _, b := range buckets {
			if b.Name == bucket {
				found = true
				break
			}
		}
		if !found {
			t.Error("Implicit bucket was not created for multipart upload")
		}
	})
}
