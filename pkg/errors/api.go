package errors

import (
	"errors"
)

// MapStorageErrorToS3 maps storage errors to S3 error codes and messages
func MapStorageErrorToS3(err error) (code string, message string) {
	// Check for specific storage errors
	switch {
	case errors.Is(err, ErrBucketNotFound):
		return "NoSuchBucket", "The specified bucket does not exist"
	case errors.Is(err, ErrBucketNotEmpty):
		return "BucketNotEmpty", "The bucket you tried to delete is not empty"
	case errors.Is(err, ErrBucketExists):
		return "BucketAlreadyExists", "The requested bucket name is not available"
	case errors.Is(err, ErrObjectNotFound):
		return "NoSuchKey", "The specified key does not exist"
	case errors.Is(err, ErrUploadNotFound):
		return "NoSuchUpload", "The specified upload does not exist"
	case errors.Is(err, ErrInvalidBucketName),
		errors.Is(err, ErrBucketNameEmpty),
		errors.Is(err, ErrBucketNameTooLong),
		errors.Is(err, ErrBucketNameInvalidChar),
		errors.Is(err, ErrBucketNameInvalidFormat):
		return "InvalidBucketName", "The specified bucket is not valid"
	case errors.Is(err, ErrInvalidObjectKey),
		errors.Is(err, ErrObjectKeyEmpty),
		errors.Is(err, ErrObjectKeyTooLong),
		errors.Is(err, ErrObjectKeyNullBytes):
		return "InvalidObjectName", "The specified key is not valid"
	case errors.Is(err, ErrPartNotFound):
		return "InvalidPart", "One or more of the specified parts could not be found"
	default:
		// Default to internal error for unknown errors
		return "InternalError", err.Error()
	}
}

// MapAuthErrorToS3 maps authentication errors to S3 error codes and messages
func MapAuthErrorToS3(err error) (code string, message string) {
	switch {
	case errors.Is(err, ErrMissingAuthHeader):
		return "MissingSecurityHeader", "Request is missing a required authentication header"
	case errors.Is(err, ErrInvalidAccessKey),
		errors.Is(err, ErrAccessKeyNotFound):
		return "InvalidAccessKeyId", "The AWS Access Key Id you provided does not exist in our records"
	case errors.Is(err, ErrSignatureMismatch):
		return "SignatureDoesNotMatch", "The request signature we calculated does not match the signature you provided"
	case errors.Is(err, ErrPresignedURLExpired):
		return "AccessDenied", "Request has expired"
	case errors.Is(err, ErrInvalidAuthHeader),
		errors.Is(err, ErrInvalidAuthFormat),
		errors.Is(err, ErrIncompleteAuthHeader),
		errors.Is(err, ErrUnsupportedAuthVersion),
		errors.Is(err, ErrInvalidAlgorithm),
		errors.Is(err, ErrInvalidCredential),
		errors.Is(err, ErrInvalidDateFormat),
		errors.Is(err, ErrInvalidExpiresFormat):
		return "InvalidRequest", "The request is not valid"
	default:
		return "AccessDenied", "Access Denied"
	}
}