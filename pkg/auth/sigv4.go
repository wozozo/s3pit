package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SigV4Signer provides methods for generating AWS Signature Version 4 signatures
type SigV4Signer struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	Service         string
}

// NewSigV4Signer creates a new SigV4 signer
func NewSigV4Signer(accessKeyID, secretAccessKey, region string) *SigV4Signer {
	return &SigV4Signer{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Region:          region,
		Service:         "s3",
	}
}

// PresignedURLOptions contains options for generating presigned URLs
type PresignedURLOptions struct {
	Method      string
	Bucket      string
	Key         string
	Expires     int               // seconds
	ContentType string            // optional, for PUT requests
	Headers     map[string]string // additional headers to sign
}

// GeneratePresignedURL generates a presigned URL with SigV4 signature
func (s *SigV4Signer) GeneratePresignedURL(host string, opts PresignedURLOptions) (string, error) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	// Build credential scope
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, s.Region, s.Service)
	credential := fmt.Sprintf("%s/%s", s.AccessKeyID, credentialScope)

	// Build the canonical URI
	canonicalURI := fmt.Sprintf("/%s/%s", opts.Bucket, opts.Key)
	if opts.Key == "" {
		canonicalURI = fmt.Sprintf("/%s", opts.Bucket)
	}

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	queryParams.Set("X-Amz-Credential", credential)
	queryParams.Set("X-Amz-Date", amzDate)
	queryParams.Set("X-Amz-Expires", fmt.Sprintf("%d", opts.Expires))
	queryParams.Set("X-Amz-SignedHeaders", "host")

	// Build canonical query string (must be sorted)
	canonicalQueryString := s.buildCanonicalQueryString(queryParams)

	// Build canonical headers
	canonicalHeaders := fmt.Sprintf("host:%s\n", host)
	signedHeaders := "host"

	// Add additional headers if provided
	if len(opts.Headers) > 0 {
		var headerNames []string
		for name := range opts.Headers {
			headerNames = append(headerNames, strings.ToLower(name))
		}
		sort.Strings(headerNames)

		for _, name := range headerNames {
			canonicalHeaders += fmt.Sprintf("%s:%s\n", name, strings.TrimSpace(opts.Headers[name]))
		}
		signedHeaders = strings.Join(append([]string{"host"}, headerNames...), ";")
		queryParams.Set("X-Amz-SignedHeaders", signedHeaders)
		canonicalQueryString = s.buildCanonicalQueryString(queryParams)
	}

	// Build canonical request
	hashedPayload := "UNSIGNED-PAYLOAD"
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		opts.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedPayload)

	// Build string to sign
	hashedCanonicalRequest := s.hashSHA256([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		hashedCanonicalRequest)

	// Calculate signature
	signature := s.calculateSignature(dateStamp, stringToSign)

	// Add signature to query parameters
	queryParams.Set("X-Amz-Signature", signature)

	// Build final URL
	finalURL := fmt.Sprintf("http://%s%s?%s", host, canonicalURI, queryParams.Encode())
	if strings.HasPrefix(host, "https://") {
		finalURL = fmt.Sprintf("https://%s%s?%s", strings.TrimPrefix(host, "https://"), canonicalURI, queryParams.Encode())
	}

	return finalURL, nil
}

// buildCanonicalQueryString builds a canonical query string for SigV4
func (s *SigV4Signer) buildCanonicalQueryString(values url.Values) string {
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		for _, v := range values[k] {
			pairs = append(pairs, fmt.Sprintf("%s=%s",
				url.QueryEscape(k),
				url.QueryEscape(v)))
		}
	}

	return strings.Join(pairs, "&")
}

// calculateSignature calculates the SigV4 signature
func (s *SigV4Signer) calculateSignature(dateStamp, stringToSign string) string {
	kDate := s.hmacSHA256([]byte("AWS4"+s.SecretAccessKey), []byte(dateStamp))
	kRegion := s.hmacSHA256(kDate, []byte(s.Region))
	kService := s.hmacSHA256(kRegion, []byte(s.Service))
	kSigning := s.hmacSHA256(kService, []byte("aws4_request"))

	signature := s.hmacSHA256(kSigning, []byte(stringToSign))
	return hex.EncodeToString(signature)
}

// hmacSHA256 computes HMAC-SHA256
func (s *SigV4Signer) hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// hashSHA256 computes SHA256 hash
func (s *SigV4Signer) hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ValidatePresignedURL validates a presigned URL signature
func (s *SigV4Signer) ValidatePresignedURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	query := parsedURL.Query()

	// Check algorithm
	if query.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		return fmt.Errorf("invalid algorithm")
	}

	// Check expiration
	amzDate := query.Get("X-Amz-Date")
	expires := query.Get("X-Amz-Expires")
	if amzDate != "" && expires != "" {
		t, err := time.Parse("20060102T150405Z", amzDate)
		if err != nil {
			return fmt.Errorf("invalid date format")
		}

		var expiresInt int
		if _, err := fmt.Sscanf(expires, "%d", &expiresInt); err != nil {
			return fmt.Errorf("invalid expires format")
		}

		if time.Now().After(t.Add(time.Duration(expiresInt) * time.Second)) {
			return fmt.Errorf("presigned URL has expired")
		}
	}

	// Extract signature from query
	providedSignature := query.Get("X-Amz-Signature")

	// Remove signature from query for recalculation
	query.Del("X-Amz-Signature")

	// Rebuild canonical query string
	canonicalQueryString := s.buildCanonicalQueryString(query)

	// Extract credential and build scope
	credential := query.Get("X-Amz-Credential")
	credParts := strings.Split(credential, "/")
	if len(credParts) < 5 {
		return fmt.Errorf("invalid credential format")
	}

	dateStamp := credParts[1]
	credentialScope := strings.Join(credParts[1:], "/")

	// Build canonical request
	canonicalURI := parsedURL.Path
	signedHeaders := query.Get("X-Amz-SignedHeaders")
	canonicalHeaders := fmt.Sprintf("host:%s\n", parsedURL.Host)

	canonicalRequest := fmt.Sprintf("GET\n%s\n%s\n%s\n%s\nUNSIGNED-PAYLOAD",
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders)

	// Build string to sign
	hashedCanonicalRequest := s.hashSHA256([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		hashedCanonicalRequest)

	// Calculate expected signature
	expectedSignature := s.calculateSignature(dateStamp, stringToSign)

	if expectedSignature != providedSignature {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}
