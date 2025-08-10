package api

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// S3ErrorCode represents standard S3 error codes
type S3ErrorCode string

const (
	// Common errors
	ErrAccessDenied                  S3ErrorCode = "AccessDenied"
	ErrBadDigest                     S3ErrorCode = "BadDigest"
	ErrBucketAlreadyExists           S3ErrorCode = "BucketAlreadyExists"
	ErrBucketAlreadyOwnedByYou       S3ErrorCode = "BucketAlreadyOwnedByYou"
	ErrBucketNotEmpty                S3ErrorCode = "BucketNotEmpty"
	ErrIncompleteBody                S3ErrorCode = "IncompleteBody"
	ErrInternalError                 S3ErrorCode = "InternalError"
	ErrInvalidAccessKeyId            S3ErrorCode = "InvalidAccessKeyId"
	ErrInvalidArgument               S3ErrorCode = "InvalidArgument"
	ErrInvalidBucketName             S3ErrorCode = "InvalidBucketName"
	ErrInvalidDigest                 S3ErrorCode = "InvalidDigest"
	ErrInvalidObjectName             S3ErrorCode = "InvalidObjectName"
	ErrInvalidPart                   S3ErrorCode = "InvalidPart"
	ErrInvalidPartNumber             S3ErrorCode = "InvalidPartNumber"
	ErrInvalidPartOrder              S3ErrorCode = "InvalidPartOrder"
	ErrInvalidRequest                S3ErrorCode = "InvalidRequest"
	ErrInvalidStorageClass           S3ErrorCode = "InvalidStorageClass"
	ErrInvalidTargetBucketForLogging S3ErrorCode = "InvalidTargetBucketForLogging"
	ErrMalformedXML                  S3ErrorCode = "MalformedXML"
	ErrMethodNotAllowed              S3ErrorCode = "MethodNotAllowed"
	ErrMissingContentLength          S3ErrorCode = "MissingContentLength"
	ErrMissingSecurityHeader         S3ErrorCode = "MissingSecurityHeader"
	ErrNoSuchBucket                  S3ErrorCode = "NoSuchBucket"
	ErrNoSuchBucketPolicy            S3ErrorCode = "NoSuchBucketPolicy"
	ErrNoSuchCORSConfiguration       S3ErrorCode = "NoSuchCORSConfiguration"
	ErrNoSuchKey                     S3ErrorCode = "NoSuchKey"
	ErrNoSuchUpload                  S3ErrorCode = "NoSuchUpload"
	ErrNotImplemented                S3ErrorCode = "NotImplemented"
	ErrPreconditionFailed            S3ErrorCode = "PreconditionFailed"
	ErrRequestTimeout                S3ErrorCode = "RequestTimeout"
	ErrSignatureDoesNotMatch         S3ErrorCode = "SignatureDoesNotMatch"
	ErrTooManyBuckets                S3ErrorCode = "TooManyBuckets"
)

// S3Error represents a complete S3 error with all necessary details
type S3Error struct {
	Code       S3ErrorCode
	Message    string
	Resource   string
	RequestID  string
	StatusCode int
}

// Error implements the error interface
func (e S3Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ErrorResponse is the XML structure for S3 error responses
type S3ErrorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId"`
	HostID    string   `xml:"HostId,omitempty"`
}

// errorCodeToHTTPStatus maps S3 error codes to HTTP status codes
var errorCodeToHTTPStatus = map[S3ErrorCode]int{
	ErrAccessDenied:                  http.StatusForbidden,
	ErrBadDigest:                     http.StatusBadRequest,
	ErrBucketAlreadyExists:           http.StatusConflict,
	ErrBucketAlreadyOwnedByYou:       http.StatusConflict,
	ErrBucketNotEmpty:                http.StatusConflict,
	ErrIncompleteBody:                http.StatusBadRequest,
	ErrInternalError:                 http.StatusInternalServerError,
	ErrInvalidAccessKeyId:            http.StatusForbidden,
	ErrInvalidArgument:               http.StatusBadRequest,
	ErrInvalidBucketName:             http.StatusBadRequest,
	ErrInvalidDigest:                 http.StatusBadRequest,
	ErrInvalidObjectName:             http.StatusBadRequest,
	ErrInvalidPart:                   http.StatusBadRequest,
	ErrInvalidPartNumber:             http.StatusBadRequest,
	ErrInvalidPartOrder:              http.StatusBadRequest,
	ErrInvalidRequest:                http.StatusBadRequest,
	ErrInvalidStorageClass:           http.StatusBadRequest,
	ErrInvalidTargetBucketForLogging: http.StatusBadRequest,
	ErrMalformedXML:                  http.StatusBadRequest,
	ErrMethodNotAllowed:              http.StatusMethodNotAllowed,
	ErrMissingContentLength:          http.StatusBadRequest,
	ErrMissingSecurityHeader:         http.StatusBadRequest,
	ErrNoSuchBucket:                  http.StatusNotFound,
	ErrNoSuchBucketPolicy:            http.StatusNotFound,
	ErrNoSuchCORSConfiguration:       http.StatusNotFound,
	ErrNoSuchKey:                     http.StatusNotFound,
	ErrNoSuchUpload:                  http.StatusNotFound,
	ErrNotImplemented:                http.StatusNotImplemented,
	ErrPreconditionFailed:            http.StatusPreconditionFailed,
	ErrRequestTimeout:                http.StatusRequestTimeout,
	ErrSignatureDoesNotMatch:         http.StatusForbidden,
	ErrTooManyBuckets:                http.StatusBadRequest,
}

// sendS3Error sends a properly formatted S3 error response
func (h *Handler) sendS3Error(c *gin.Context, err S3Error) {
	if err.RequestID == "" {
		err.RequestID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	if err.Resource == "" {
		err.Resource = c.Request.URL.Path
	}

	statusCode := err.StatusCode
	if statusCode == 0 {
		if code, ok := errorCodeToHTTPStatus[err.Code]; ok {
			statusCode = code
		} else {
			statusCode = http.StatusInternalServerError
		}
	}

	c.Header("Content-Type", "application/xml")
	c.XML(statusCode, S3ErrorResponse{
		Code:      string(err.Code),
		Message:   err.Message,
		Resource:  err.Resource,
		RequestID: err.RequestID,
		HostID:    "s3pit",
	})
}

// sendError is a convenience method for sending errors (backward compatibility)
func (h *Handler) sendError(c *gin.Context, code string, message string, statusCode int) {
	h.sendS3Error(c, S3Error{
		Code:       S3ErrorCode(code),
		Message:    message,
		StatusCode: statusCode,
	})
}
