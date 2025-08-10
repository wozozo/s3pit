package errors

import (
	"errors"
	"fmt"
)

// Authentication errors
var (
	// Auth mode errors
	ErrInvalidAuthMode       = errors.New("invalid auth mode")
	ErrAuthModeNotConfigured = errors.New("authentication mode not configured")

	// Authorization header errors
	ErrMissingAuthHeader      = errors.New("missing authorization header")
	ErrInvalidAuthHeader      = errors.New("invalid authorization header")
	ErrInvalidAuthFormat      = errors.New("invalid authorization header format")
	ErrIncompleteAuthHeader   = errors.New("incomplete authorization header")
	ErrUnsupportedAuthVersion = errors.New("only AWS Signature Version 4 is supported")

	// Credential errors
	ErrMissingAccessKey  = errors.New("missing access key")
	ErrInvalidAccessKey  = errors.New("invalid access key")
	ErrAccessKeyNotFound = errors.New("access key not found")
	ErrMissingCredential = errors.New("missing credential")
	ErrInvalidCredential = errors.New("invalid credential format")

	// Signature errors
	ErrInvalidAlgorithm     = errors.New("invalid algorithm")
	ErrMissingSignature     = errors.New("missing signature")
	ErrSignatureMismatch    = errors.New("signature mismatch")
	ErrMissingSignedHeaders = errors.New("missing signed headers")

	// Date/time errors
	ErrMissingDate          = errors.New("missing date")
	ErrInvalidDateFormat    = errors.New("invalid date format")
	ErrInvalidExpiresFormat = errors.New("invalid expires format")
	ErrPresignedURLExpired  = errors.New("presigned URL has expired")

	// Tenant errors
	ErrNoTenantManager = errors.New("no tenant manager configured")
)

// WrapAuthError wraps an authentication error with context
func WrapAuthError(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("auth %s: %w", context, err)
}

// WrapCredentialError wraps a credential error with access key context
func WrapCredentialError(accessKey string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("credential %s: %w", accessKey, err)
}

// WrapSignatureError wraps a signature error with additional context
func WrapSignatureError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("signature validation: %w", err)
}

// IsAuthenticationError checks if an error is an authentication error
func IsAuthenticationError(err error) bool {
	return errors.Is(err, ErrMissingAuthHeader) ||
		errors.Is(err, ErrInvalidAuthHeader) ||
		errors.Is(err, ErrInvalidAuthFormat) ||
		errors.Is(err, ErrIncompleteAuthHeader) ||
		errors.Is(err, ErrUnsupportedAuthVersion) ||
		errors.Is(err, ErrMissingAccessKey) ||
		errors.Is(err, ErrInvalidAccessKey) ||
		errors.Is(err, ErrAccessKeyNotFound) ||
		errors.Is(err, ErrSignatureMismatch)
}

// IsExpiredError checks if an error is due to expiration
func IsExpiredError(err error) bool {
	return errors.Is(err, ErrPresignedURLExpired)
}
