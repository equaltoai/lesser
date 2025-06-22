package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// dnsCacheRecord represents the DynamoDB record for DNS cache entries
type dnsCacheRecord struct {
	PK         string    `dynamodbav:"PK"` // DNSCACHE#hostname
	SK         string    `dynamodbav:"SK"` // ENTRY
	Hostname   string    `dynamodbav:"Hostname"`
	IPs        []string  `dynamodbav:"IPs"`
	ResolvedAt time.Time `dynamodbav:"ResolvedAt"`
	TTL        int       `dynamodbav:"TTL"`       // seconds
	ExpiresAt  time.Time `dynamodbav:"ExpiresAt"` // For DynamoDB TTL
}

// GetDNSCache retrieves a cached DNS lookup result
func (d *dynamoDBStorage) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DNSCACHE#%s", hostname)},
			"SK": &types.AttributeValueMemberS{Value: "ENTRY"},
		},
	}

	result, err := d.client.GetItem(ctx, input)
	if err != nil {
		common.Logger().Error("failed to get DNS cache entry",
			zap.String("hostname", hostname),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get DNS cache entry: %w", err)
	}

	if result.Item == nil {
		return nil, nil // Not found
	}

	var record dnsCacheRecord
	if err := attributevalue.UnmarshalMap(result.Item, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DNS cache entry: %w", err)
	}

	// Check if the entry has expired
	if time.Now().After(record.ExpiresAt) {
		// Entry has expired, return nil
		return nil, nil
	}

	// Convert to storage model
	entry := &storage.DNSCacheEntry{
		Hostname:   record.Hostname,
		IPs:        record.IPs,
		ResolvedAt: record.ResolvedAt,
		TTL:        record.TTL,
	}

	return entry, nil
}

// SetDNSCache stores a DNS lookup result in the cache
func (d *dynamoDBStorage) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	if entry == nil {
		return fmt.Errorf("DNS cache entry cannot be nil")
	}

	// Calculate expiration time for DynamoDB TTL
	expiresAt := time.Now().Add(time.Duration(entry.TTL) * time.Second)

	record := dnsCacheRecord{
		PK:         fmt.Sprintf("DNSCACHE#%s", entry.Hostname),
		SK:         "ENTRY",
		Hostname:   entry.Hostname,
		IPs:        entry.IPs,
		ResolvedAt: entry.ResolvedAt,
		TTL:        entry.TTL,
		ExpiresAt:  expiresAt,
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal DNS cache entry: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	}

	if _, err := d.client.PutItem(ctx, input); err != nil {
		common.Logger().Error("failed to set DNS cache entry",
			zap.String("hostname", entry.Hostname),
			zap.Error(err))
		return fmt.Errorf("failed to set DNS cache entry: %w", err)
	}

	common.Logger().Debug("DNS cache entry stored",
		zap.String("hostname", entry.Hostname),
		zap.Int("ip_count", len(entry.IPs)),
		zap.Int("ttl_seconds", entry.TTL))

	return nil
}

