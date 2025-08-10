package storage

import (
	storageerrors "github.com/wozozo/s3pit/pkg/errors"
	"encoding/json"
	"fmt"
	"strings"
)

// BucketPolicy represents an S3 bucket policy
type BucketPolicy struct {
	Version   string            `json:"Version"`
	Statement []PolicyStatement `json:"Statement"`
}

// PolicyStatement represents a single statement in a bucket policy
type PolicyStatement struct {
	Sid       string      `json:"Sid,omitempty"`
	Effect    string      `json:"Effect"`
	Principal interface{} `json:"Principal"`
	Action    interface{} `json:"Action"`
	Resource  interface{} `json:"Resource"`
}

// IsPublicReadable checks if the bucket policy allows public read access
func (bp *BucketPolicy) IsPublicReadable(bucket, key string) bool {
	if bp == nil {
		return false
	}

	for _, stmt := range bp.Statement {
		// Check if this statement allows public access
		if stmt.Effect != "Allow" {
			continue
		}

		// Check if principal is public ("*" or {"AWS": "*"})
		if !isPublicPrincipal(stmt.Principal) {
			continue
		}

		// Check if action allows GetObject
		if !allowsGetObject(stmt.Action) {
			continue
		}

		// Check if resource matches
		if matchesResource(stmt.Resource, bucket, key) {
			return true
		}
	}

	return false
}

func isPublicPrincipal(principal interface{}) bool {
	switch p := principal.(type) {
	case string:
		return p == "*"
	case map[string]interface{}:
		if aws, ok := p["AWS"]; ok {
			switch aws := aws.(type) {
			case string:
				return aws == "*"
			case []interface{}:
				for _, v := range aws {
					if str, ok := v.(string); ok && str == "*" {
						return true
					}
				}
			}
		}
	}
	return false
}

func allowsGetObject(action interface{}) bool {
	switch a := action.(type) {
	case string:
		return a == "s3:GetObject" || a == "s3:*"
	case []interface{}:
		for _, v := range a {
			if str, ok := v.(string); ok {
				if str == "s3:GetObject" || str == "s3:*" {
					return true
				}
			}
		}
	}
	return false
}

func matchesResource(resource interface{}, bucket, key string) bool {
	resourceStr := fmt.Sprintf("arn:aws:s3:::%s/%s", bucket, key)

	switch r := resource.(type) {
	case string:
		return matchesResourcePattern(r, resourceStr)
	case []interface{}:
		for _, v := range r {
			if str, ok := v.(string); ok {
				if matchesResourcePattern(str, resourceStr) {
					return true
				}
			}
		}
	}
	return false
}

func matchesResourcePattern(pattern, resource string) bool {
	// Simple wildcard matching
	// e.g., "arn:aws:s3:::my-bucket/*" matches any object in my-bucket
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(resource, prefix+"/")
	}
	return pattern == resource
}

// DefaultPublicReadPolicy creates a default public read policy for a bucket
func DefaultPublicReadPolicy(bucket string) *BucketPolicy {
	return &BucketPolicy{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Sid:       "PublicReadGetObject",
				Effect:    "Allow",
				Principal: "*",
				Action:    "s3:GetObject",
				Resource:  fmt.Sprintf("arn:aws:s3:::%s/*", bucket),
			},
		},
	}
}

// ParseBucketPolicy parses a JSON bucket policy
func ParseBucketPolicy(data []byte) (*BucketPolicy, error) {
	var policy BucketPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, storageerrors.WrapStorageError("parse bucket policy", err)
	}
	return &policy, nil
}
