// Package transcoding provides CloudFront signed URL generation
package transcoding

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 - SHA1 required by CloudFront signed URLs
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// CloudFrontService handles CloudFront signed URL generation
type CloudFrontService struct {
	privateKey *rsa.PrivateKey
	logger     *zap.Logger
	domain     string
	keyPairID  string
	defaultTTL time.Duration
}

// CloudFrontConfig holds configuration for CloudFront service
type CloudFrontConfig struct {
	Domain        string        // CloudFront distribution domain
	KeyPairID     string        // CloudFront key pair ID
	PrivateKeyPEM string        // Private key in PEM format
	DefaultTTL    time.Duration // Default URL expiration time
}

var (
	// ErrInvalidPrivateKey is returned when the private key is invalid
	ErrInvalidPrivateKey = errors.New("invalid CloudFront private key")
	// ErrSigningFailed is returned when URL signing fails
	ErrSigningFailed = errors.New("failed to sign URL")
)

// NewCloudFrontService creates a new CloudFront service
func NewCloudFrontService(config CloudFrontConfig, logger *zap.Logger) (*CloudFrontService, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Parse private key
	privateKey, err := parsePrivateKey(config.PrivateKeyPEM)
	if err != nil {
		return nil, errors.Join(ErrInvalidPrivateKey, err)
	}

	// Set default TTL if not provided
	defaultTTL := config.DefaultTTL
	if defaultTTL == 0 {
		defaultTTL = 24 * time.Hour // Default 24 hours
	}

	return &CloudFrontService{
		privateKey: privateKey,
		logger:     logger,
		domain:     config.Domain,
		keyPairID:  config.KeyPairID,
		defaultTTL: defaultTTL,
	}, nil
}

// SignURL generates a signed CloudFront URL with custom expiration
func (c *CloudFrontService) SignURL(resourcePath string, expireTime time.Time) (string, error) {
	// Build full URL
	resourceURL := fmt.Sprintf("https://%s/%s", c.domain, resourcePath)

	c.logger.Debug("signing URL",
		zap.String("url", resourceURL),
		zap.Time("expires_at", expireTime))

	// Create policy statement
	policy := cloudFrontPolicy{
		Statement: []policyStatement{
			{
				Resource: resourceURL,
				Condition: policyCondition{
					DateLessThan: awsEpochTime{Time: expireTime},
				},
			},
		},
	}

	// Sign the policy
	signedURL, err := c.signPolicy(resourceURL, &policy)
	if err != nil {
		c.logger.Error("failed to sign URL",
			zap.String("url", resourceURL),
			zap.Error(err))
		return "", errors.Join(ErrSigningFailed, err)
	}

	return signedURL, nil
}

// SignURLWithDefaultTTL generates a signed URL with the default TTL
func (c *CloudFrontService) SignURLWithDefaultTTL(resourcePath string) (string, error) {
	expireTime := time.Now().Add(c.defaultTTL)
	return c.SignURL(resourcePath, expireTime)
}

// SignStreamingURL generates a signed URL for streaming content
func (c *CloudFrontService) SignStreamingURL(mediaID, format string, quality *string, ttl time.Duration) (string, error) {
	// Determine the resource path based on format
	var resourcePath string

	switch format {
	case "hls":
		if quality != nil && *quality != "" {
			// Specific quality variant
			resourcePath = fmt.Sprintf("%s/hls/%s.m3u8", mediaID, *quality)
		} else {
			// Master playlist
			resourcePath = fmt.Sprintf("%s/hls/master.m3u8", mediaID)
		}
	case "dash":
		resourcePath = fmt.Sprintf("%s/dash/manifest.mpd", mediaID)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}

	// Use provided TTL or default
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	expireTime := time.Now().Add(ttl)

	return c.SignURL(resourcePath, expireTime)
}

// SignBatchURLs signs multiple URLs at once (for preloading)
func (c *CloudFrontService) SignBatchURLs(resourcePaths []string, ttl time.Duration) (map[string]string, error) {
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	expireTime := time.Now().Add(ttl)

	signedURLs := make(map[string]string, len(resourcePaths))
	errors := []error{}

	for _, path := range resourcePaths {
		signedURL, err := c.SignURL(path, expireTime)
		if err != nil {
			c.logger.Warn("failed to sign URL in batch",
				zap.String("path", path),
				zap.Error(err))
			errors = append(errors, err)
			continue
		}
		signedURLs[path] = signedURL
	}

	if len(errors) > 0 && len(signedURLs) == 0 {
		return nil, fmt.Errorf("failed to sign any URLs: %v", errors)
	}

	return signedURLs, nil
}

// GetExpirationTime returns the expiration time for a given TTL
func (c *CloudFrontService) GetExpirationTime(ttl time.Duration) time.Time {
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	return time.Now().Add(ttl)
}

// ValidateConfig validates the CloudFront configuration
func ValidateConfig(config CloudFrontConfig) error {
	if config.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if config.KeyPairID == "" {
		return fmt.Errorf("key pair ID is required")
	}
	if config.PrivateKeyPEM == "" {
		return fmt.Errorf("private key is required")
	}

	// Try to parse the private key
	_, err := parsePrivateKey(config.PrivateKeyPEM)
	if err != nil {
		return errors.Join(fmt.Errorf("invalid private key"), err)
	}

	return nil
}

// signPolicy signs a CloudFront policy and returns the signed URL
func (c *CloudFrontService) signPolicy(resourceURL string, policy *cloudFrontPolicy) (string, error) {
	// Marshal policy to JSON
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}

	// Sign the policy
	hash := sha1.New() // #nosec G401 - SHA1 required by CloudFront signed URLs
	hash.Write(policyJSON)
	digest := hash.Sum(nil)

	signature, err := rsa.SignPKCS1v15(nil, c.privateKey, crypto.SHA1, digest)
	if err != nil {
		return "", err
	}

	// Encode policy and signature for URL
	encodedPolicy := makeURLSafe(base64.StdEncoding.EncodeToString(policyJSON))
	encodedSignature := makeURLSafe(base64.StdEncoding.EncodeToString(signature))

	// Build signed URL
	separator := "?"
	if strings.Contains(resourceURL, "?") {
		separator = "&"
	}

	signedURL := fmt.Sprintf("%s%sPolicy=%s&Signature=%s&Key-Pair-Id=%s",
		resourceURL, separator, encodedPolicy, encodedSignature, c.keyPairID)

	return signedURL, nil
}

// makeURLSafe makes a base64-encoded string URL-safe
func makeURLSafe(encoded string) string {
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "=", "_")
	encoded = strings.ReplaceAll(encoded, "/", "~")
	return url.QueryEscape(encoded)
}

// parsePrivateKey parses a PEM-encoded private key
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try parsing as PKCS1
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return privateKey, nil
	}

	// Try parsing as PKCS8
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key")
	}

	return rsaKey, nil
}

// cloudFrontPolicy represents a CloudFront policy statement
type cloudFrontPolicy struct {
	Statement []policyStatement `json:"Statement"`
}

// policyStatement represents a single policy statement
type policyStatement struct {
	Resource  string          `json:"Resource"`
	Condition policyCondition `json:"Condition"`
}

// policyCondition represents policy conditions
type policyCondition struct {
	DateLessThan awsEpochTime `json:"DateLessThan"`
}

// awsEpochTime represents AWS epoch time format
type awsEpochTime struct {
	Time time.Time
}

// MarshalJSON marshals awsEpochTime to JSON
func (t awsEpochTime) MarshalJSON() ([]byte, error) {
	epoch := t.Time.Unix()
	return json.Marshal(map[string]int64{"AWS:EpochTime": epoch})
}
