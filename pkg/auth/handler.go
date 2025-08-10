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
)

type Handler interface {
	Authenticate(r *http.Request) (string, error)
}

type AuthMode string

const (
	ModeSigV4 AuthMode = "sigv4"
)

type handler struct {
	mode            AuthMode
	accessKeyID     string
	secretAccessKey string
	currentRequest  *http.Request // Used for signature calculation
}

func NewHandler(mode string, accessKeyID, secretAccessKey string) (Handler, error) {
	authMode := AuthMode(mode)

	switch authMode {
	case ModeSigV4:
		// valid mode
	default:
		return nil, autherrors.WrapAuthError("mode validation", fmt.Errorf("%s: %w", mode, autherrors.ErrInvalidAuthMode))
	}

	return &handler{
		mode:            authMode,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
	}, nil
}

func (h *handler) Authenticate(r *http.Request) (string, error) {
	switch h.mode {
	case ModeSigV4:
		return h.authenticateSigV4(r)

	default:
		return "", autherrors.ErrAuthModeNotConfigured
	}
}


func (h *handler) authenticateSigV4(r *http.Request) (string, error) {
	// Store current request for use in helper methods
	h.currentRequest = r

	// Check for query string authentication (presigned URL)
	if r.URL.Query().Get("X-Amz-Algorithm") != "" {
		return h.authenticateSigV4Query(r)
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

	var accessKey, signature, signedHeaders string
	var credentialScope []string

	for _, part := range parts[1:] {
		part = strings.TrimSuffix(part, ",")

		if strings.HasPrefix(part, "Credential=") {
			credParts := strings.Split(strings.TrimPrefix(part, "Credential="), "/")
			if len(credParts) >= 4 {
				accessKey = credParts[0]
				credentialScope = credParts[1:]
			}
		} else if strings.HasPrefix(part, "SignedHeaders=") {
			signedHeaders = strings.TrimPrefix(part, "SignedHeaders=")
		} else if strings.HasPrefix(part, "Signature=") {
			signature = strings.TrimPrefix(part, "Signature=")
		}
	}

	if accessKey == "" || signature == "" || signedHeaders == "" || len(credentialScope) == 0 {
		return "", autherrors.ErrIncompleteAuthHeader
	}

	if accessKey != h.accessKeyID {
		return "", autherrors.ErrInvalidAccessKey
	}

	// Verify the signature
	canonicalRequest := h.buildCanonicalRequest(r, signedHeaders)
	stringToSign := h.buildStringToSign(canonicalRequest, credentialScope)
	calculatedSig := h.calculateSignature(credentialScope[0], credentialScope[1], stringToSign)

	if calculatedSig != signature {
		return "", autherrors.ErrSignatureMismatch
	}

	return accessKey, nil
}

func (h *handler) authenticateSigV4Query(r *http.Request) (string, error) {
	// Store current request for use in helper methods
	h.currentRequest = r

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

	accessKey := credParts[0]
	if accessKey != h.accessKeyID {
		return "", autherrors.ErrInvalidAccessKey
	}

	// Check expiration
	expires := query.Get("X-Amz-Expires")
	date := query.Get("X-Amz-Date")
	if expires != "" && date != "" {
		// Parse date and check if URL has expired
		t, err := time.Parse("20060102T150405Z", date)
		if err != nil {
			return "", autherrors.ErrInvalidDateFormat
		}

		var expiresInt int
		if _, err := fmt.Sscanf(expires, "%d", &expiresInt); err != nil {
			return "", autherrors.ErrInvalidExpiresFormat
		}

		if time.Now().After(t.Add(time.Duration(expiresInt) * time.Second)) {
			return "", autherrors.ErrPresignedURLExpired
		}
	}

	// Verify signature
	signedHeaders := query.Get("X-Amz-SignedHeaders")
	signature := query.Get("X-Amz-Signature")

	if signedHeaders == "" || signature == "" {
		return "", autherrors.ErrMissingSignedHeaders
	}

	// Build canonical request for query auth
	canonicalRequest := h.buildCanonicalRequestForQuery(r, signedHeaders)
	credentialScope := credParts[1:]
	stringToSign := h.buildStringToSign(canonicalRequest, credentialScope)
	calculatedSig := h.calculateSignature(credentialScope[0], credentialScope[1], stringToSign)

	if calculatedSig != signature {
		return "", autherrors.ErrSignatureMismatch
	}

	return accessKey, nil
}

func (h *handler) buildCanonicalRequest(r *http.Request, signedHeaders string) string {
	// HTTP Method
	method := r.Method

	// Canonical URI
	uri := r.URL.Path
	if uri == "" {
		uri = "/"
	}

	// Canonical Query String
	queryKeys := make([]string, 0, len(r.URL.Query()))
	for k := range r.URL.Query() {
		// Skip signature for query auth
		if k != "X-Amz-Signature" {
			queryKeys = append(queryKeys, k)
		}
	}
	sort.Strings(queryKeys)

	var queryPairs []string
	for _, k := range queryKeys {
		for _, v := range r.URL.Query()[k] {
			queryPairs = append(queryPairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	canonicalQuery := strings.Join(queryPairs, "&")

	// Canonical Headers
	headerNames := strings.Split(signedHeaders, ";")
	var canonicalHeaders strings.Builder
	for _, name := range headerNames {
		canonicalHeaders.WriteString(strings.ToLower(name))
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(r.Header.Get(name)))
		canonicalHeaders.WriteString("\n")
	}

	// Hashed Payload
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		// Read body and compute hash
		var bodyBytes []byte
		if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
		h := sha256.Sum256(bodyBytes)
		payloadHash = hex.EncodeToString(h[:])
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		method, uri, canonicalQuery, canonicalHeaders.String(), signedHeaders, payloadHash)
}

func (h *handler) buildCanonicalRequestForQuery(r *http.Request, signedHeaders string) string {
	// For presigned URLs, the canonical query string excludes the signature
	method := r.Method
	uri := r.URL.Path
	if uri == "" {
		uri = "/"
	}

	// Build query string without signature
	queryKeys := make([]string, 0, len(r.URL.Query()))
	for k := range r.URL.Query() {
		if k != "X-Amz-Signature" {
			queryKeys = append(queryKeys, k)
		}
	}
	sort.Strings(queryKeys)

	var queryPairs []string
	for _, k := range queryKeys {
		for _, v := range r.URL.Query()[k] {
			queryPairs = append(queryPairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	canonicalQuery := strings.Join(queryPairs, "&")

	// Canonical Headers (usually just host for presigned URLs)
	headerNames := strings.Split(signedHeaders, ";")
	var canonicalHeaders strings.Builder
	for _, name := range headerNames {
		canonicalHeaders.WriteString(strings.ToLower(name))
		canonicalHeaders.WriteString(":")
		if strings.ToLower(name) == "host" {
			canonicalHeaders.WriteString(r.Host)
		} else {
			canonicalHeaders.WriteString(strings.TrimSpace(r.Header.Get(name)))
		}
		canonicalHeaders.WriteString("\n")
	}

	// For presigned URLs, payload is always UNSIGNED-PAYLOAD
	payloadHash := "UNSIGNED-PAYLOAD"

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		method, uri, canonicalQuery, canonicalHeaders.String(), signedHeaders, payloadHash)
}

func (h *handler) buildStringToSign(canonicalRequest string, credentialScope []string) string {
	// Get current time or time from request
	var timeStr string
	if amzDate := h.currentRequest.Header.Get("X-Amz-Date"); amzDate != "" {
		timeStr = amzDate
	} else if amzDate := h.currentRequest.URL.Query().Get("X-Amz-Date"); amzDate != "" {
		timeStr = amzDate
	} else {
		timeStr = time.Now().UTC().Format("20060102T150405Z")
	}

	// Hash the canonical request
	hash := sha256.Sum256([]byte(canonicalRequest))
	hashedCanonicalRequest := hex.EncodeToString(hash[:])

	// Build credential scope string
	credentialScopeStr := strings.Join(credentialScope, "/")

	return fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		timeStr, credentialScopeStr, hashedCanonicalRequest)
}

func (h *handler) calculateSignature(dateStamp, region, stringToSign string) string {
	kDate := hmacSHA256([]byte("AWS4"+h.secretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	signature := hmacSHA256(kSigning, []byte(stringToSign))
	return hex.EncodeToString(signature)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
