package api

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/auth"
	"github.com/wozozo/s3pit/pkg/storage"
	"github.com/wozozo/s3pit/pkg/tenant"
)

type Handler struct {
	storage       storage.Storage
	auth          auth.Handler
	tenantManager *tenant.Manager
	config        *config.Config
}

func NewHandler(storage storage.Storage, auth auth.Handler, tenantManager *tenant.Manager, config *config.Config) *Handler {
	return &Handler{
		storage:       storage,
		auth:          auth,
		tenantManager: tenantManager,
		config:        config,
	}
}

// getStorage returns the appropriate storage for the current request
// If using TenantAwareStorage, it returns the tenant-specific storage instance
func (h *Handler) getStorage(c *gin.Context) storage.Storage {
	// If using tenant-aware storage, get the tenant-specific storage
	if tenantStorage, ok := h.storage.(*storage.TenantAwareStorage); ok {
		// Get access key from context (set by auth middleware)
		if accessKey, exists := c.Get("accessKey"); exists {
			if key, ok := accessKey.(string); ok && key != "" {
				// Get the tenant-specific storage (thread-safe)
				storage, err := tenantStorage.GetStorageForTenant(key)
				if err != nil {
					// Log error but return the base storage as fallback
					log.Printf("[STORAGE ERROR] Failed to get tenant storage for key %s: %v", key, err)
					// In production, you might want to handle this differently
					return h.storage
				}
				// Debug: Log which storage is being used
				if c.Request.URL.Path != "/health" && strings.Contains(c.Request.URL.Path, "eight-articles") {
					log.Printf("[STORAGE DEBUG] Using tenant storage for accessKey: %s", key)
				}
				return storage
			}
		} else {
			// No access key in context
			if strings.Contains(c.Request.URL.Path, "eight-articles") {
				log.Printf("[STORAGE WARNING] No accessKey in context for path: %s", c.Request.URL.Path)
			}
		}
	}
	return h.storage
}

type ListBucketsResponse struct {
	XMLName xml.Name `xml:"ListAllMyBucketsResult"`
	Xmlns   string   `xml:"xmlns,attr"`
	Owner   struct {
		ID          string `xml:"ID"`
		DisplayName string `xml:"DisplayName"`
	} `xml:"Owner"`
	Buckets struct {
		Bucket []BucketInfo `xml:"Bucket"`
	} `xml:"Buckets"`
}

type BucketInfo struct {
	Name         string    `xml:"Name"`
	CreationDate time.Time `xml:"CreationDate"`
}

type ListObjectsV2Response struct {
	XMLName               xml.Name       `xml:"ListBucketResult"`
	Xmlns                 string         `xml:"xmlns,attr"`
	Name                  string         `xml:"Name"`
	Prefix                string         `xml:"Prefix,omitempty"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys"`
	KeyCount              int            `xml:"KeyCount"`
	IsTruncated           bool           `xml:"IsTruncated"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
	Contents              []ObjectInfo   `xml:"Contents"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
}

type ObjectInfo struct {
	Key          string    `xml:"Key"`
	LastModified time.Time `xml:"LastModified"`
	ETag         string    `xml:"ETag"`
	Size         int64     `xml:"Size"`
	StorageClass string    `xml:"StorageClass"`
}

type CommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

type DeleteRequest struct {
	XMLName xml.Name       `xml:"Delete"`
	Objects []DeleteObject `xml:"Object"`
	Quiet   bool           `xml:"Quiet"`
}

type DeleteObject struct {
	Key string `xml:"Key"`
}

type DeleteResponse struct {
	XMLName xml.Name        `xml:"DeleteResult"`
	Deleted []DeletedObject `xml:"Deleted"`
	Error   []DeleteError   `xml:"Error"`
}

type DeletedObject struct {
	Key string `xml:"Key"`
}

type DeleteError struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func (h *Handler) ListBuckets(c *gin.Context) {
	buckets, err := h.getStorage(c).ListBuckets()
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	response := ListBucketsResponse{
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
	}
	response.Owner.ID = "s3pit"
	response.Owner.DisplayName = "S3pit User"

	for _, bucket := range buckets {
		response.Buckets.Bucket = append(response.Buckets.Bucket, BucketInfo{
			Name:         bucket.Name,
			CreationDate: bucket.CreationDate,
		})
	}

	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, response)
}

func (h *Handler) HeadBucket(c *gin.Context) {
	bucket := c.Param("bucket")

	exists, err := h.getStorage(c).BucketExists(bucket)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	if !exists {
		h.sendError(c, "NoSuchBucket", "The specified bucket does not exist", http.StatusNotFound)
		return
	}

	c.Status(http.StatusOK)
}

func (h *Handler) CreateBucket(c *gin.Context) {
	bucket := c.Param("bucket")

	created, err := h.getStorage(c).CreateBucket(bucket)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	if created {
		c.Header("x-s3pit-bucket-created", "true")
	}

	c.Status(http.StatusOK)
}

func (h *Handler) DeleteBucket(c *gin.Context) {
	bucket := c.Param("bucket")

	err := h.getStorage(c).DeleteBucket(bucket)
	if err != nil {
		if err == storage.ErrBucketNotEmpty {
			h.sendError(c, "BucketNotEmpty", "The bucket you tried to delete is not empty", http.StatusConflict)
			return
		}
		if err == storage.ErrBucketNotFound {
			h.sendError(c, "NoSuchBucket", "The specified bucket does not exist", http.StatusNotFound)
			return
		}
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) ListObjectsV2(c *gin.Context) {
	bucket := c.Param("bucket")

	exists, err := h.getStorage(c).BucketExists(bucket)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	if !exists {
		h.sendError(c, "NoSuchBucket", "The specified bucket does not exist", http.StatusNotFound)
		return
	}

	prefix := c.Query("prefix")
	delimiter := c.Query("delimiter")
	maxKeys := 1000
	if mk := c.Query("max-keys"); mk != "" {
		if parsed, err := strconv.Atoi(mk); err == nil && parsed > 0 {
			maxKeys = parsed
		}
	}
	continuationToken := c.Query("continuation-token")

	objects, commonPrefixes, nextToken, err := h.getStorage(c).ListObjects(bucket, prefix, delimiter, maxKeys, continuationToken)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	response := ListObjectsV2Response{
		Xmlns:                 "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:                  bucket,
		Prefix:                prefix,
		Delimiter:             delimiter,
		MaxKeys:               maxKeys,
		KeyCount:              len(objects),
		IsTruncated:           nextToken != "",
		ContinuationToken:     continuationToken,
		NextContinuationToken: nextToken,
	}

	for _, obj := range objects {
		response.Contents = append(response.Contents, ObjectInfo{
			Key:          obj.Key,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			Size:         obj.Size,
			StorageClass: "STANDARD",
		})
	}

	for _, prefix := range commonPrefixes {
		response.CommonPrefixes = append(response.CommonPrefixes, CommonPrefix{
			Prefix: prefix,
		})
	}

	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, response)
}

func (h *Handler) HeadObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	meta, err := h.getStorage(c).GetObjectMetadata(bucket, key)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			h.sendError(c, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
			return
		}
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	c.Header("Content-Type", meta.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", meta.Size))
	c.Header("ETag", meta.ETag)
	c.Header("Last-Modified", meta.LastModified.Format(http.TimeFormat))
	c.Status(http.StatusOK)
}

func (h *Handler) GetObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	reader, meta, err := h.getStorage(c).GetObject(bucket, key)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			h.sendError(c, "NoSuchKey", "The specified key does not exist", http.StatusNotFound)
			return
		}
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	c.Header("Content-Type", meta.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", meta.Size))
	c.Header("ETag", meta.ETag)
	c.Header("Last-Modified", meta.LastModified.Format(http.TimeFormat))

	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
}

func (h *Handler) PutObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	// Debug logging
	if bucket == "eight-articles" {
		if accessKey, exists := c.Get("accessKey"); exists {
			log.Printf("[PUTOBJECT DEBUG] bucket: %s, key: %s, accessKey: %v", bucket, key, accessKey)
		}
	}

	if h.config.AutoCreateBucket {
		created, err := h.getStorage(c).CreateBucket(bucket)
		if err != nil {
			h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}
		if created {
			c.Header("x-s3pit-bucket-created", "true")
		}
	} else {
		exists, err := h.getStorage(c).BucketExists(bucket)
		if err != nil {
			h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			h.sendError(c, "NoSuchBucket", "The specified bucket does not exist", http.StatusNotFound)
			return
		}
	}

	contentType := c.GetHeader("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	etag, err := h.getStorage(c).PutObject(bucket, key, c.Request.Body, c.Request.ContentLength, contentType)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	c.Header("ETag", etag)
	c.Status(http.StatusOK)
}

func (h *Handler) DeleteObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	err := h.getStorage(c).DeleteObject(bucket, key)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			c.Status(http.StatusNoContent)
			return
		}
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteObjects(c *gin.Context) {
	bucket := c.Param("bucket")

	var req DeleteRequest
	if err := c.ShouldBindXML(&req); err != nil {
		h.sendError(c, "MalformedXML", "The XML you provided was not well-formed", http.StatusBadRequest)
		return
	}

	response := DeleteResponse{}

	for _, obj := range req.Objects {
		err := h.getStorage(c).DeleteObject(bucket, obj.Key)
		if err != nil {
			if err != storage.ErrObjectNotFound {
				response.Error = append(response.Error, DeleteError{
					Key:     obj.Key,
					Code:    "InternalError",
					Message: err.Error(),
				})
			}
		} else if !req.Quiet {
			response.Deleted = append(response.Deleted, DeletedObject(obj))
		}
	}

	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, response)
}

func (h *Handler) HandlePostObject(c *gin.Context) {
	if c.Query("uploads") != "" {
		h.InitiateMultipartUpload(c)
		return
	}

	if uploadId := c.Query("uploadId"); uploadId != "" {
		if partNumber := c.Query("partNumber"); partNumber != "" {
			h.UploadPart(c)
			return
		}
		h.CompleteMultipartUpload(c)
		return
	}

	if c.Query("delete") != "" {
		h.DeleteObjects(c)
		return
	}

	h.sendError(c, "NotImplemented", "This operation is not implemented", http.StatusNotImplemented)
}

func (h *Handler) CopyObject(c *gin.Context) {
	destBucket := c.Param("bucket")
	destKey := strings.TrimPrefix(c.Param("key"), "/")

	copySource := c.GetHeader("x-amz-copy-source")
	if copySource == "" {
		h.sendError(c, "InvalidRequest", "x-amz-copy-source header is required", http.StatusBadRequest)
		return
	}

	// Parse source bucket and key from x-amz-copy-source
	// Format: /bucket/key or bucket/key
	copySource = strings.TrimPrefix(copySource, "/")
	parts := strings.SplitN(copySource, "/", 2)
	if len(parts) != 2 {
		h.sendError(c, "InvalidRequest", "Invalid x-amz-copy-source format", http.StatusBadRequest)
		return
	}

	sourceBucket := parts[0]
	sourceKey := parts[1]

	// Check if source object exists
	sourceMeta, err := h.getStorage(c).GetObjectMetadata(sourceBucket, sourceKey)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			h.sendError(c, "NoSuchKey", "The specified source key does not exist", http.StatusNotFound)
			return
		}
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto-create destination bucket if enabled
	if h.config.AutoCreateBucket {
		created, err := h.getStorage(c).CreateBucket(destBucket)
		if err != nil {
			h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}
		if created {
			c.Header("x-s3pit-bucket-created", "true")
		}
	} else {
		exists, err := h.getStorage(c).BucketExists(destBucket)
		if err != nil {
			h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			h.sendError(c, "NoSuchBucket", "The specified bucket does not exist", http.StatusNotFound)
			return
		}
	}

	// Get source object data
	reader, _, err := h.getStorage(c).GetObject(sourceBucket, sourceKey)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Put to destination
	etag, err := h.getStorage(c).PutObject(destBucket, destKey, reader, sourceMeta.Size, sourceMeta.ContentType)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	// Return CopyObjectResult XML
	type CopyObjectResult struct {
		XMLName      xml.Name  `xml:"CopyObjectResult"`
		LastModified time.Time `xml:"LastModified"`
		ETag         string    `xml:"ETag"`
	}

	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, CopyObjectResult{
		LastModified: time.Now(),
		ETag:         etag,
	})
}

func (h *Handler) InitiateMultipartUpload(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	// Auto-create bucket if enabled
	if h.config.AutoCreateBucket {
		created, err := h.getStorage(c).CreateBucket(bucket)
		if err != nil {
			h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}
		if created {
			c.Header("x-s3pit-bucket-created", "true")
		}
	} else {
		exists, err := h.getStorage(c).BucketExists(bucket)
		if err != nil {
			h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			h.sendError(c, "NoSuchBucket", "The specified bucket does not exist", http.StatusNotFound)
			return
		}
	}

	// Initialize multipart upload in storage
	uploadId, err := h.getStorage(c).InitiateMultipartUpload(bucket, key)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	type InitiateMultipartUploadResult struct {
		XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
		Xmlns    string   `xml:"xmlns,attr"`
		Bucket   string   `xml:"Bucket"`
		Key      string   `xml:"Key"`
		UploadId string   `xml:"UploadId"`
	}

	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, InitiateMultipartUploadResult{
		Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Bucket:   bucket,
		Key:      key,
		UploadId: uploadId,
	})
}

func (h *Handler) UploadPart(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")
	uploadId := c.Query("uploadId")
	partNumberStr := c.Query("partNumber")

	if uploadId == "" {
		h.sendError(c, "InvalidRequest", "uploadId is required", http.StatusBadRequest)
		return
	}

	partNumber, err := strconv.Atoi(partNumberStr)
	if err != nil || partNumber < 1 || partNumber > 10000 {
		h.sendError(c, "InvalidPartNumber", "Part number must be between 1 and 10000", http.StatusBadRequest)
		return
	}

	contentLength := c.Request.ContentLength
	if contentLength <= 0 {
		h.sendError(c, "MissingContentLength", "Content-Length header is required", http.StatusBadRequest)
		return
	}

	etag, err := h.getStorage(c).UploadPart(bucket, key, uploadId, partNumber, c.Request.Body, contentLength)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	c.Header("ETag", etag)
	c.Status(http.StatusOK)
}

func (h *Handler) CompleteMultipartUpload(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")
	uploadId := c.Query("uploadId")

	if uploadId == "" {
		h.sendError(c, "InvalidRequest", "uploadId is required", http.StatusBadRequest)
		return
	}

	type Part struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}

	type CompleteMultipartUploadRequest struct {
		XMLName xml.Name `xml:"CompleteMultipartUpload"`
		Parts   []Part   `xml:"Part"`
	}

	var req CompleteMultipartUploadRequest
	if err := c.ShouldBindXML(&req); err != nil {
		h.sendError(c, "MalformedXML", "The XML you provided was not well-formed", http.StatusBadRequest)
		return
	}

	// Convert to storage format
	var parts []storage.CompletedPart
	for _, p := range req.Parts {
		parts = append(parts, storage.CompletedPart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	etag, err := h.getStorage(c).CompleteMultipartUpload(bucket, key, uploadId, parts)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	type CompleteMultipartUploadResult struct {
		XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
		Xmlns    string   `xml:"xmlns,attr"`
		Location string   `xml:"Location"`
		Bucket   string   `xml:"Bucket"`
		Key      string   `xml:"Key"`
		ETag     string   `xml:"ETag"`
	}

	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, CompleteMultipartUploadResult{
		Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Location: fmt.Sprintf("http://%s/%s/%s", c.Request.Host, bucket, key),
		Bucket:   bucket,
		Key:      key,
		ETag:     etag,
	})
}

func (h *Handler) AbortMultipartUpload(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")
	uploadId := c.Query("uploadId")

	if uploadId == "" {
		h.sendError(c, "InvalidRequest", "uploadId is required", http.StatusBadRequest)
		return
	}

	err := h.getStorage(c).AbortMultipartUpload(bucket, key, uploadId)
	if err != nil {
		h.sendError(c, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusNoContent)
}
