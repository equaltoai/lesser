package security

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/patrickmn/go-cache"
)

// IPAuthorizationConfig holds configuration for IP authorization
type IPAuthorizationConfig struct {
	AllowedIPList string
	AllowedIPs    []string
}

// IsAuthorizedIP checks if the given IP address is authorized based on the configuration
func IsAuthorizedIP(sourceIP string, config IPAuthorizationConfig) bool {
	// If AllowedIPList is provided, parse it
	if config.AllowedIPList != "" {
		allowedIPs := parseIPList(config.AllowedIPList)
		return checkIPInList(sourceIP, allowedIPs)
	}

	// Otherwise use the AllowedIPs slice
	return checkIPInList(sourceIP, config.AllowedIPs)
}

// IsAuthorizedIPSimple checks if the source IP is in the provided allowed IP list
// This is a convenience function for simple use cases
func IsAuthorizedIPSimple(sourceIP string, allowedIPList string) bool {
	if allowedIPList == "" {
		return false
	}

	allowedIPs := parseIPList(allowedIPList)
	return checkIPInList(sourceIP, allowedIPs)
}

// checkIPInList checks if the source IP is in the list of allowed IPs
func checkIPInList(sourceIP string, allowedIPs []string) bool {
	// Normalize the source IP (remove port if present)
	sourceIP = stripPort(strings.TrimSpace(sourceIP))

	for _, allowedIP := range allowedIPs {
		// Normalize the allowed IP as well
		allowedIP = stripPort(strings.TrimSpace(allowedIP))

		if allowedIP == sourceIP {
			return true
		}
	}
	return false
}

// parseIPList parses a comma-separated list of IP addresses
func parseIPList(ipList string) []string {
	if ipList == "" {
		return []string{}
	}

	parts := strings.Split(ipList, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// SSMIPAuthorizer handles IP authorization using AWS SSM parameters with caching
type SSMIPAuthorizer struct {
	ssmClient *ssm.Client
	cache     *cache.Cache
	cacheTTL  time.Duration
}

// SSMIPAuthorizerConfig configures the SSM IP authorizer
type SSMIPAuthorizerConfig struct {
	// CacheTTL is the duration to cache IP lists. Defaults to 15 minutes.
	CacheTTL time.Duration
}

// NewSSMIPAuthorizer creates a new SSM IP authorizer with default AWS config
func NewSSMIPAuthorizer(ctx context.Context) (*SSMIPAuthorizer, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return NewSSMIPAuthorizerWithConfig(ssm.NewFromConfig(cfg), SSMIPAuthorizerConfig{}), nil
}

// NewSSMIPAuthorizerWithClient creates a new SSM IP authorizer with a provided SSM client
func NewSSMIPAuthorizerWithClient(ssmClient *ssm.Client) *SSMIPAuthorizer {
	return NewSSMIPAuthorizerWithConfig(ssmClient, SSMIPAuthorizerConfig{})
}

// NewSSMIPAuthorizerWithConfig creates a new SSM IP authorizer with a provided SSM client and config
func NewSSMIPAuthorizerWithConfig(ssmClient *ssm.Client, config SSMIPAuthorizerConfig) *SSMIPAuthorizer {
	cacheTTL := config.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 15 * time.Minute // Default to 15 minutes
	}

	// Create cache with TTL and cleanup interval
	cleanupInterval := cacheTTL / 2
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	return &SSMIPAuthorizer{
		ssmClient: ssmClient,
		cache:     cache.New(cacheTTL, cleanupInterval),
		cacheTTL:  cacheTTL,
	}
}

// IsAuthorizedIP checks if the source IP is in the allowed list retrieved from SSM with caching
func (s *SSMIPAuthorizer) IsAuthorizedIP(ctx context.Context, sourceIP string, ssmParameterName string) (bool, error) {
	if ssmParameterName == "" {
		return false, fmt.Errorf("SSM parameter name must be provided")
	}

	// Try to get from cache first
	cacheKey := fmt.Sprintf("ssm:ip-list:%s", ssmParameterName)
	if cached, found := s.cache.Get(cacheKey); found {
		allowedIPs, ok := cached.([]string)
		if ok {
			// Use cached IP list
			return checkIPInList(sourceIP, allowedIPs), nil
		}
	}

	// Not in cache, fetch from SSM
	result, err := s.ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(ssmParameterName),
	})
	if err != nil {
		// Don't expose parameter names in errors
		return false, fmt.Errorf("failed to retrieve IP authorization configuration: %w", err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return false, fmt.Errorf("IP authorization configuration is invalid")
	}

	// Parse the comma-separated list of IPs
	allowedIPs := parseIPList(*result.Parameter.Value)

	// Cache the parsed IP list
	s.cache.Set(cacheKey, allowedIPs, s.cacheTTL)

	// Check if the source IP is in the allowed list
	return checkIPInList(sourceIP, allowedIPs), nil
}

// ClearCache clears the IP list cache
func (s *SSMIPAuthorizer) ClearCache() {
	s.cache.Flush()
}

// GetCacheStats returns basic cache statistics
func (s *SSMIPAuthorizer) GetCacheStats() (items int, expired int) {
	itemCount := s.cache.ItemCount()
	// Note: go-cache doesn't expose expired count directly
	return itemCount, 0
}

// BuildVPCNATGatewayParameterName builds the SSM parameter name for VPC NAT gateway lists
// Example: pt-partner-paytheory-prod-gochallenge-vpc-nat-gateway-list
// The component parameter specifies the service-specific part of the parameter name
func BuildVPCNATGatewayParameterName(partner, stage, component string) string {
	return fmt.Sprintf("pt-partner-%s-%s-%s", partner, stage, component)
}

// IPAuthorizationService provides a generic interface for IP authorization
type IPAuthorizationService struct {
	authorizer       *SSMIPAuthorizer
	ssmParameterName string
}

// NewIPAuthorizationService creates a new IP authorization service
func NewIPAuthorizationService(ssmClient *ssm.Client, ssmParameterName string) *IPAuthorizationService {
	return &IPAuthorizationService{
		authorizer:       NewSSMIPAuthorizerWithClient(ssmClient),
		ssmParameterName: ssmParameterName,
	}
}

// NewIPAuthorizationServiceFromEnv creates a new IP authorization service using environment variables
// It requires PARTNER and STAGE env vars, and the component name must be provided
func NewIPAuthorizationServiceFromEnv(ctx context.Context, component string) (*IPAuthorizationService, error) {
	if component == "" {
		return nil, fmt.Errorf("component name must be provided")
	}

	partner := os.Getenv("PARTNER")
	stage := os.Getenv("STAGE")

	if partner == "" || stage == "" {
		return nil, fmt.Errorf("PARTNER and STAGE environment variables must be set")
	}

	// Build the SSM parameter name
	ssmParameterName := BuildVPCNATGatewayParameterName(partner, stage, component)

	// Create SSM client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	ssmClient := ssm.NewFromConfig(cfg)

	return NewIPAuthorizationService(ssmClient, ssmParameterName), nil
}

// IsAuthorizedIP checks if the given IP is authorized
func (s *IPAuthorizationService) IsAuthorizedIP(ctx context.Context, sourceIP string) (bool, error) {
	if sourceIP == "" {
		return false, fmt.Errorf("source IP cannot be empty")
	}

	return s.authorizer.IsAuthorizedIP(ctx, sourceIP, s.ssmParameterName)
}

// CheckIPAuthorization is a standalone helper function for one-off IP authorization checks
// This is useful when you don't want to create a service instance
func CheckIPAuthorization(ctx context.Context, sourceIP string, ssmClient *ssm.Client, ssmParameterName string) (bool, error) {
	if sourceIP == "" {
		return false, fmt.Errorf("source IP cannot be empty")
	}

	if ssmParameterName == "" {
		return false, fmt.Errorf("SSM parameter name must be provided")
	}

	// Create a cached authorizer
	authorizer := NewSSMIPAuthorizerWithClient(ssmClient)

	// Check if the IP is authorized
	return authorizer.IsAuthorizedIP(ctx, sourceIP, ssmParameterName)
}
