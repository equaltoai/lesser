package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Federation graph constants
const (
	federationNodePrefix    = "FEDERATION_NODE#"
	federationEdgePrefix    = "FEDERATION_EDGE#"
	instanceMetadataPrefix  = "INSTANCE_META#"
	federationClusterPrefix = "FEDERATION_CLUSTER#"
	connectionPrefix        = "CONNECTION#"
)

// GetFederationNodes retrieves federation nodes up to a certain depth
func (s *dynamoDBStorage) GetFederationNodes(ctx context.Context, depth int) ([]*storage.FederationNode, error) {
	// Use GSI1 for active federation instances
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "FEDERATION_ACTIVE"},
		},
		Limit:            aws.Int32(100),  // Limit to 100 nodes initially
		ScanIndexForward: aws.Bool(false), // Most recently seen first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query federation nodes: %w", err)
	}

	nodes := make([]*storage.FederationNode, 0, len(result.Items))
	for _, item := range result.Items {
		var node storage.FederationNode
		if err := attributevalue.UnmarshalMap(item, &node); err != nil {
			continue
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

// GetFederationNodesByHealth retrieves federation nodes filtered by health status
func (s *dynamoDBStorage) GetFederationNodesByHealth(ctx context.Context, health string) ([]*storage.FederationNode, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk AND begins_with(GSI1SK, :health)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: "FEDERATION_GRAPH#NODES"},
			":health": &types.AttributeValueMemberS{Value: health + "#"},
		},
		Limit: aws.Int32(100),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query federation nodes by health: %w", err)
	}

	// Extract domains from the health index items
	domains := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if domain, ok := item["Domain"]; ok {
			if domainStr, ok := domain.(*types.AttributeValueMemberS); ok {
				domains = append(domains, domainStr.Value)
			}
		}
	}

	// Batch get the actual node items
	if len(domains) == 0 {
		return []*storage.FederationNode{}, nil
	}

	nodes := make([]*storage.FederationNode, 0, len(domains))
	for _, domain := range domains {
		getInput := &dynamodb.GetItemInput{
			TableName: aws.String(s.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: federationNodePrefix + domain},
				"SK": &types.AttributeValueMemberS{Value: "NODE"},
			},
		}

		result, err := s.client.GetItem(ctx, getInput)
		if err != nil {
			continue
		}

		if result.Item != nil {
			var node storage.FederationNode
			if err := attributevalue.UnmarshalMap(result.Item, &node); err == nil {
				nodes = append(nodes, &node)
			}
		}
	}

	return nodes, nil
}

// GetFederationEdges retrieves edges between specified domains
func (s *dynamoDBStorage) GetFederationEdges(ctx context.Context, domains []string) ([]*storage.FederationEdge, error) {
	if len(domains) == 0 {
		return []*storage.FederationEdge{}, nil
	}

	// Build batch get items for all possible domain pairs
	var keys []map[string]types.AttributeValue
	for i, source := range domains {
		for j, target := range domains {
			if i != j {
				keys = append(keys, map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: federationEdgePrefix + source},
					"SK": &types.AttributeValueMemberS{Value: target},
				})
			}
		}
	}

	// Batch get in chunks of 100
	edges := make([]*storage.FederationEdge, 0)
	for i := 0; i < len(keys); i += 100 {
		end := i + 100
		if end > len(keys) {
			end = len(keys)
		}

		input := &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				s.tableName: {
					Keys: keys[i:end],
				},
			},
		}

		result, err := s.client.BatchGetItem(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to batch get federation edges: %w", err)
		}

		for _, item := range result.Responses[s.tableName] {
			var edge storage.FederationEdge
			if err := attributevalue.UnmarshalMap(item, &edge); err != nil {
				continue
			}
			edges = append(edges, &edge)
		}
	}

	return edges, nil
}

// GetInstanceMetadata retrieves metadata for a specific instance
func (s *dynamoDBStorage) GetInstanceMetadata(ctx context.Context, domain string) (*storage.InstanceMetadata, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: instanceMetadataPrefix + domain},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance metadata: %w", err)
	}

	if result.Item == nil {
		return nil, storage.ErrNotFound
	}

	var metadata storage.InstanceMetadata
	if err := attributevalue.UnmarshalMap(result.Item, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instance metadata: %w", err)
	}

	return &metadata, nil
}

// GetStrongestConnectionsByType retrieves the strongest federation connections by connection type
func (s *dynamoDBStorage) GetStrongestConnectionsByType(ctx context.Context, connectionType string, limit int) ([]*storage.FederationEdge, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("FEDERATION_EDGES#%s", connectionType)},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Highest volume first
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query strongest connections: %w", err)
	}

	// Extract edge information from volume index items
	edges := make([]*storage.FederationEdge, 0, len(result.Items))
	for _, item := range result.Items {
		if source, ok := item["SourceDomain"]; ok {
			if sourceStr, ok := source.(*types.AttributeValueMemberS); ok {
				if target, ok := item["TargetDomain"]; ok {
					if targetStr, ok := target.(*types.AttributeValueMemberS); ok {
						// Get the actual edge item
						edgeInput := &dynamodb.GetItemInput{
							TableName: aws.String(s.tableName),
							Key: map[string]types.AttributeValue{
								"PK": &types.AttributeValueMemberS{Value: federationEdgePrefix + sourceStr.Value},
								"SK": &types.AttributeValueMemberS{Value: targetStr.Value},
							},
						}

						edgeResult, err := s.client.GetItem(ctx, edgeInput)
						if err == nil && edgeResult.Item != nil {
							var edge storage.FederationEdge
							if err := attributevalue.UnmarshalMap(edgeResult.Item, &edge); err == nil {
								edges = append(edges, &edge)
							}
						}
					}
				}
			}
		}
	}

	return edges, nil
}

// CalculateFederationClusters calculates instance clusters based on connections
func (s *dynamoDBStorage) CalculateFederationClusters(ctx context.Context) ([]*storage.InstanceCluster, error) {
	// This is a complex operation that would typically be done in a batch job
	// For now, return pre-calculated clusters stored in DynamoDB

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: federationClusterPrefix + "CLUSTERS"},
		},
		Limit: aws.Int32(50), // Limit to 50 clusters
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query federation clusters: %w", err)
	}

	clusters := make([]*storage.InstanceCluster, 0, len(result.Items))
	for _, item := range result.Items {
		var cluster storage.InstanceCluster
		if err := attributevalue.UnmarshalMap(item, &cluster); err != nil {
			continue
		}
		clusters = append(clusters, &cluster)
	}

	return clusters, nil
}

// GetInstanceConnections retrieves connections for a specific instance
func (s *dynamoDBStorage) GetInstanceConnections(ctx context.Context, domain string, connectionType string) ([]*storage.InstanceConnection, error) {
	// Query using GSI2 for instance connections
	pkValue := fmt.Sprintf("INSTANCE#%s#CONNECTIONS", domain)
	if connectionType != "" {
		pkValue = fmt.Sprintf("INSTANCE#%s#CONNECTIONS#%s", domain, connectionType)
	}

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pkValue},
		},
		Limit: aws.Int32(100),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query instance connections: %w", err)
	}

	connections := make([]*storage.InstanceConnection, 0, len(result.Items))
	for _, item := range result.Items {
		var conn storage.InstanceConnection
		if err := attributevalue.UnmarshalMap(item, &conn); err != nil {
			continue
		}
		connections = append(connections, &conn)
	}

	return connections, nil
}

// UpdateFederationNode updates or creates a federation node
func (s *dynamoDBStorage) UpdateFederationNode(ctx context.Context, node *storage.FederationNode) error {
	if node.Domain == "" {
		return fmt.Errorf("domain is required")
	}

	// Marshal the node
	item, err := attributevalue.MarshalMap(node)
	if err != nil {
		return fmt.Errorf("failed to marshal federation node: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: federationNodePrefix + node.Domain}
	item["SK"] = &types.AttributeValueMemberS{Value: "NODE"}

	// Add GSI1 attributes for active federation tracking
	item["GSI1PK"] = &types.AttributeValueMemberS{Value: "FEDERATION_ACTIVE"}
	item["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", node.LastSeen.Unix(), node.Domain)}

	// Add GSI3 attributes for domain lookups
	item["GSI3PK"] = &types.AttributeValueMemberS{Value: "DOMAIN#" + node.Domain}
	item["GSI3SK"] = &types.AttributeValueMemberS{Value: "FEDERATION_NODE"}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update federation node: %w", err)
	}

	// Also create a health index item for efficient health-based queries
	healthItem := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: federationNodePrefix + node.Domain},
		"SK":        &types.AttributeValueMemberS{Value: "HEALTH_INDEX"},
		"GSI1PK":    &types.AttributeValueMemberS{Value: "FEDERATION_GRAPH#NODES"},
		"GSI1SK":    &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", node.Health, node.Domain)},
		"Domain":    &types.AttributeValueMemberS{Value: node.Domain},
		"Health":    &types.AttributeValueMemberS{Value: node.Health},
		"UpdatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	healthInput := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      healthItem,
	}

	if _, err := s.client.PutItem(ctx, healthInput); err != nil {
		return fmt.Errorf("failed to update federation node health index: %w", err)
	}

	return nil
}

// UpdateFederationEdge updates or creates a federation edge
func (s *dynamoDBStorage) UpdateFederationEdge(ctx context.Context, edge *storage.FederationEdge) error {
	if edge.SourceDomain == "" || edge.TargetDomain == "" {
		return fmt.Errorf("source and target domains are required")
	}

	// Marshal the edge
	item, err := attributevalue.MarshalMap(edge)
	if err != nil {
		return fmt.Errorf("failed to marshal federation edge: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: federationEdgePrefix + edge.SourceDomain}
	item["SK"] = &types.AttributeValueMemberS{Value: edge.TargetDomain}

	// Add GSI2 attributes for connection queries
	item["GSI2PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s#CONNECTIONS#%s", edge.SourceDomain, edge.ConnectionType)}
	item["GSI2SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%d#%s", edge.LastActivity.Unix(), edge.TargetDomain)}

	// Update timestamp
	item["UpdatedAt"] = &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update federation edge: %w", err)
	}

	// Also create a volume index item for efficient volume-based queries
	totalVolume := edge.VolumeIn + edge.VolumeOut
	paddedVolume := fmt.Sprintf("%020d", totalVolume)

	volumeItem := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: federationEdgePrefix + edge.SourceDomain},
		"SK":             &types.AttributeValueMemberS{Value: "VOLUME#" + edge.ConnectionType + "#" + edge.TargetDomain},
		"GSI2PK":         &types.AttributeValueMemberS{Value: fmt.Sprintf("FEDERATION_EDGES#%s", edge.ConnectionType)},
		"GSI2SK":         &types.AttributeValueMemberS{Value: fmt.Sprintf("VOLUME#%s#%s#%s", paddedVolume, edge.SourceDomain, edge.TargetDomain)},
		"SourceDomain":   &types.AttributeValueMemberS{Value: edge.SourceDomain},
		"TargetDomain":   &types.AttributeValueMemberS{Value: edge.TargetDomain},
		"ConnectionType": &types.AttributeValueMemberS{Value: edge.ConnectionType},
		"TotalVolume":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", totalVolume)},
		"UpdatedAt":      &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	volumeInput := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      volumeItem,
	}

	if _, err := s.client.PutItem(ctx, volumeInput); err != nil {
		return fmt.Errorf("failed to update federation edge volume index: %w", err)
	}

	return nil
}

// UpdateInstanceMetadata updates instance metadata
func (s *dynamoDBStorage) UpdateInstanceMetadata(ctx context.Context, metadata *storage.InstanceMetadata) error {
	if metadata.Domain == "" {
		return fmt.Errorf("domain is required")
	}

	// Set last updated time
	metadata.LastUpdated = time.Now()

	// Marshal the metadata
	item, err := attributevalue.MarshalMap(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal instance metadata: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: instanceMetadataPrefix + metadata.Domain}
	item["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to update instance metadata: %w", err)
	}

	return nil
}

// GetRecentInstanceConnections retrieves connections for an instance within a time window
func (s *dynamoDBStorage) GetRecentInstanceConnections(ctx context.Context, domain string, since time.Duration) ([]*storage.InstanceConnection, error) {
	cutoffTime := time.Now().Add(-since)

	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk AND GSI2SK > :cutoff"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s#CONNECTIONS", domain)},
			":cutoff": &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", cutoffTime.Unix())},
		},
		Limit: aws.Int32(1000),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent connections: %w", err)
	}

	connections := make([]*storage.InstanceConnection, 0, len(result.Items))
	for _, item := range result.Items {
		var conn storage.InstanceConnection
		if err := attributevalue.UnmarshalMap(item, &conn); err != nil {
			continue
		}
		if conn.LastActivity.After(cutoffTime) {
			connections = append(connections, &conn)
		}
	}

	return connections, nil
}

// StoreFederationTimeSeries stores time-series federation metrics
func (s *dynamoDBStorage) StoreFederationTimeSeries(ctx context.Context, data *storage.FederationTimeSeries) error {
	if data.Domain == "" || data.Period == "" {
		return fmt.Errorf("domain and period are required")
	}

	// Set TTL based on period
	switch data.Period {
	case "hourly":
		data.TTL = time.Now().Add(7 * 24 * time.Hour).Unix() // Keep hourly data for 7 days
	case "daily":
		data.TTL = time.Now().Add(30 * 24 * time.Hour).Unix() // Keep daily data for 30 days
	case "weekly":
		data.TTL = time.Now().Add(365 * 24 * time.Hour).Unix() // Keep weekly data for 1 year
	}

	// Marshal the data
	item, err := attributevalue.MarshalMap(data)
	if err != nil {
		return fmt.Errorf("failed to marshal time series data: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("TIMESERIES#%s#%s", data.Domain, data.Period)}
	item["SK"] = &types.AttributeValueMemberS{Value: data.Timestamp.Format(time.RFC3339)}

	// Add GSI for period-based queries
	item["GSI1PK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("TIMESERIES#%s", data.Period)}
	item["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%s#%s", data.Timestamp.Format(time.RFC3339), data.Domain)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to store time series data: %w", err)
	}

	return nil
}

// StoreInstanceCluster stores a calculated federation cluster
func (s *dynamoDBStorage) StoreInstanceCluster(ctx context.Context, cluster *storage.InstanceCluster) error {
	if cluster.ClusterID == "" {
		return fmt.Errorf("cluster ID is required")
	}

	cluster.Size = len(cluster.Instances)
	cluster.UpdatedAt = time.Now()

	// Marshal the cluster
	item, err := attributevalue.MarshalMap(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{Value: federationClusterPrefix + "CLUSTERS"}
	item["SK"] = &types.AttributeValueMemberS{Value: cluster.ClusterID}

	// Add GSI for size-based queries
	item["GSI1PK"] = &types.AttributeValueMemberS{Value: "CLUSTERS_BY_SIZE"}
	item["GSI1SK"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%05d#%s", cluster.Size, cluster.ClusterID)}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	}

	if _, err := s.client.PutItem(ctx, input); err != nil {
		return fmt.Errorf("failed to store cluster: %w", err)
	}

	return nil
}
