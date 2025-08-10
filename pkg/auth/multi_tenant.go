package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	autherrors "github.com/wozozo/s3pit/pkg/errors"
	"github.com/wozozo/s3pit/pkg/tenant"
)

// MultiTenantHandler implements multi-tenant authentication
type MultiTenantHandler struct {
	mode          AuthMode
	tenantManager *tenant.Manager
}

// NewMultiTenantHandler creates a new multi-tenant authentication handler
func NewMultiTenantHandler(mode string, tenantManager *tenant.Manager) (Handler, error) {
	authMode := AuthMode(mode)

	switch authMode {
	case ModeSigV4:
		// valid mode
	default:
		return nil, autherrors.WrapAuthError("mode validation", fmt.Errorf("%s: %w", mode, autherrors.ErrInvalidAuthMode))
	}

	return &MultiTenantHandler{
		mode:          authMode,
		tenantManager: tenantManager,
	}, nil
}

func (h *MultiTenantHandler) Authenticate(r *http.Request) (string, error) {
	switch h.mode {
	case ModeSigV4:
		return h.authenticateSigV4(r)

	default:
		return "", autherrors.ErrAuthModeNotConfigured
	}
}

func (h *MultiTenantHandler) extractAccessKey(r *http.Request) string {
	// Try to extract from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256") {
			parts := strings.Split(authHeader, " ")
			for _, part := range parts[1:] {
				if strings.HasPrefix(part, "Credential=") {
					credParts := strings.Split(strings.TrimPrefix(part, "Credential="), "/")
					if len(credParts) > 0 {
						return strings.TrimSuffix(credParts[0], ",")
					}
				}
			}
		} else if strings.HasPrefix(authHeader, "AWS ") {
			parts := strings.Split(strings.TrimPrefix(authHeader, "AWS "), ":")
			if len(parts) == 2 {
				return parts[0]
			}
		}
	}

	// Try to extract from query parameters (presigned URLs)
	if r.URL.Query().Get("X-Amz-Credential") != "" {
		credParts := strings.Split(r.URL.Query().Get("X-Amz-Credential"), "/")
		if len(credParts) > 0 {
			return credParts[0]
		}
	}

	return ""
}

func (h *MultiTenantHandler) authenticateSigV4(r *http.Request) (string, error) {
	// Extract access key
	accessKey := h.extractAccessKey(r)
	if accessKey == "" {
		return "", autherrors.ErrMissingAccessKey
	}

	// Get secret key from tenant manager
	var secretKey string
	if h.tenantManager != nil {
		if t, exists := h.tenantManager.GetTenant(accessKey); exists {
			secretKey = t.SecretAccessKey
		} else {
			return "", autherrors.WrapCredentialError(accessKey, autherrors.ErrAccessKeyNotFound)
		}
	} else {
		return "", autherrors.ErrNoTenantManager
	}

	// Check for query string authentication (presigned URL)
	if r.URL.Query().Get("X-Amz-Algorithm") != "" {
		return h.authenticateSigV4Query(r, accessKey, secretKey)
	}

	// Standard header authentication
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", autherrors.ErrMissingAuthHeader
	}

	if !strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256") {
		return "", autherrors.ErrUnsupportedAuthVersion
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) < 2 {
		return "", autherrors.ErrInvalidAuthFormat
	}

	var signature, signedHeaders string
	var credentialScope []string

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "Credential=") {
			credential := strings.TrimPrefix(part, "Credential=")
			credential = strings.TrimSuffix(credential, ",")
			credParts := strings.Split(credential, "/")
			if len(credParts) >= 5 {
				credentialScope = credParts[1:5] // date/region/service/aws4_request
			}
		} else if strings.HasPrefix(part, "SignedHeaders=") {
			signedHeaders = strings.TrimPrefix(part, "SignedHeaders=")
			signedHeaders = strings.TrimSuffix(signedHeaders, ",")
		} else if strings.HasPrefix(part, "Signature=") {
			signature = strings.TrimPrefix(part, "Signature=")
		}
	}

	if len(credentialScope) < 4 || signature == "" || signedHeaders == "" {
		return "", autherrors.ErrIncompleteAuthHeader
	}

	// Calculate expected signature
	expectedSig, err := h.calculateSignature(r, accessKey, secretKey, credentialScope, signedHeaders)
	if err != nil {
		return "", autherrors.WrapSignatureError(err)
	}

	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return "", autherrors.ErrSignatureMismatch
	}

	return accessKey, nil
}

func (h *MultiTenantHandler) authenticateSigV4Query(r *http.Request, accessKey, secretKey string) (string, error) {
	query := r.URL.Query()

	// Extract required parameters
	algorithm := query.Get("X-Amz-Algorithm")
	if algorithm != "AWS4-HMAC-SHA256" {
		return "", autherrors.ErrInvalidAlgorithm
	}

	credential := query.Get("X-Amz-Credential")
	if credential == "" {
		return "", autherrors.ErrMissingCredential
	}

	credParts := strings.Split(credential, "/")
	if len(credParts) < 5 {
		return "", autherrors.ErrInvalidCredential
	}

	date := query.Get("X-Amz-Date")
	if date == "" {
		return "", autherrors.ErrMissingDate
	}

	signedHeaders := query.Get("X-Amz-SignedHeaders")
	if signedHeaders == "" {
		return "", autherrors.ErrMissingSignedHeaders
	}

	signature := query.Get("X-Amz-Signature")
	if signature == "" {
		return "", autherrors.ErrMissingSignature
	}

	expires := query.Get("X-Amz-Expires")
	if expires != "" {
		// Check if URL has expired
		signTime, err := time.Parse("20060102T150405Z", date)
		if err != nil {
			return "", autherrors.WrapAuthError("date parsing", autherrors.ErrInvalidDateFormat)
		}

		expiresInt := 0
		_, _ = fmt.Sscanf(expires, "%d", &expiresInt)
		if time.Since(signTime) > time.Duration(expiresInt)*time.Second {
			return "", autherrors.ErrPresignedURLExpired
		}
	}

	// Calculate expected signature for presigned URL
	credentialScope := credParts[1:5] // date/region/service/aws4_request
	expectedSig, err := h.calculatePresignedSignature(r, accessKey, secretKey, credentialScope, signedHeaders)
	if err != nil {
		return "", autherrors.WrapSignatureError(err)
	}

	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return "", autherrors.ErrSignatureMismatch
	}

	return accessKey, nil
}

func (h *MultiTenantHandler) calculateSignature(r *http.Request, accessKey, secretKey string, credentialScope []string, signedHeaders string) (string, error) {
	// Step 1: Create canonical request
	canonicalRequest := h.createCanonicalRequest(r, signedHeaders)

	// Step 2: Create string to sign
	dateStamp := credentialScope[0]
	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		// Fallback to Date header
		if dateHeader := r.Header.Get("Date"); dateHeader != "" {
			t, err := time.Parse(time.RFC1123, dateHeader)
			if err == nil {
				amzDate = t.Format("20060102T150405Z")
			}
		}
	}

	credentialScopeStr := strings.Join(credentialScope, "/")
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%x",
		amzDate,
		credentialScopeStr,
		sha256.Sum256([]byte(canonicalRequest)))

	// Step 3: Calculate signature
	signingKey := h.getSigningKey(secretKey, dateStamp, credentialScope[1], credentialScope[2])
	signature := hmacSHA256Multi(signingKey, []byte(stringToSign))

	return hex.EncodeToString(signature), nil
}

func (h *MultiTenantHandler) calculatePresignedSignature(r *http.Request, accessKey, secretKey string, credentialScope []string, signedHeaders string) (string, error) {
	// For presigned URLs, create a modified request without the signature
	modifiedURL := *r.URL
	q := modifiedURL.Query()
	q.Del("X-Amz-Signature")
	modifiedURL.RawQuery = q.Encode()

	modifiedReq := *r
	modifiedReq.URL = &modifiedURL

	// Create canonical request for presigned URL
	canonicalRequest := h.createCanonicalRequestForPresigned(&modifiedReq, signedHeaders)

	// Create string to sign
	dateStamp := credentialScope[0]
	amzDate := r.URL.Query().Get("X-Amz-Date")
	credentialScopeStr := strings.Join(credentialScope, "/")

	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%x",
		amzDate,
		credentialScopeStr,
		sha256.Sum256([]byte(canonicalRequest)))

	// Calculate signature
	signingKey := h.getSigningKey(secretKey, dateStamp, credentialScope[1], credentialScope[2])
	signature := hmacSHA256Multi(signingKey, []byte(stringToSign))

	return hex.EncodeToString(signature), nil
}

func (h *MultiTenantHandler) createCanonicalRequest(r *http.Request, signedHeaders string) string {
	// Get canonical URI
	canonicalURI := r.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// Get canonical query string
	canonicalQueryString := h.getCanonicalQueryString(r.URL.Query())

	// Get canonical headers
	headers := strings.Split(signedHeaders, ";")
	canonicalHeaders := ""
	for _, header := range headers {
		value := r.Header.Get(header)
		if header == "host" {
			value = r.Host
		}
		canonicalHeaders += fmt.Sprintf("%s:%s\n", header, strings.TrimSpace(value))
	}

	// Get hashed payload
	hashedPayload := r.Header.Get("X-Amz-Content-Sha256")
	if hashedPayload == "" {
		// Calculate payload hash
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		hash := sha256.Sum256(body)
		hashedPayload = hex.EncodeToString(hash[:])
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		r.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedPayload)
}

func (h *MultiTenantHandler) createCanonicalRequestForPresigned(r *http.Request, signedHeaders string) string {
	// Get canonical URI
	canonicalURI := r.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// Get canonical query string (includes all X-Amz-* parameters except Signature)
	canonicalQueryString := h.getCanonicalQueryString(r.URL.Query())

	// Get canonical headers
	headers := strings.Split(signedHeaders, ";")
	canonicalHeaders := ""
	for _, header := range headers {
		value := r.Header.Get(header)
		if header == "host" {
			value = r.Host
		}
		canonicalHeaders += fmt.Sprintf("%s:%s\n", header, strings.TrimSpace(value))
	}

	// For presigned URLs, payload is always "UNSIGNED-PAYLOAD"
	hashedPayload := "UNSIGNED-PAYLOAD"

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		r.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedPayload)
}

func (h *MultiTenantHandler) getCanonicalQueryString(values url.Values) string {
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		v := values.Get(k)
		pairs = append(pairs, fmt.Sprintf("%s=%s",
			url.QueryEscape(k),
			url.QueryEscape(v)))
	}

	return strings.Join(pairs, "&")
}

func (h *MultiTenantHandler) getSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256Multi([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256Multi(kDate, []byte(region))
	kService := hmacSHA256Multi(kRegion, []byte(service))
	kSigning := hmacSHA256Multi(kService, []byte("aws4_request"))
	return kSigning
}

func hmacSHA256Multi(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
