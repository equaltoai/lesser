package dynamodb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// RecordHashtagUsage records when a hashtag is used in a status
func (s *dynamoDBStorage) RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error {
	// Create a usage record
	usage := map[string]any{
		"PK":       fmt.Sprintf("HASHTAG_USAGE#%s", hashtag),
		"SK":       fmt.Sprintf("STATUS#%s", statusID),
		"AuthorID": authorID,
		"Hashtag":  hashtag,
		"StatusID": statusID,
		"UsedAt":   time.Now().Format(time.RFC3339),
		"TTL":      time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 day TTL
	}

	item, err := attributevalue.MarshalMap(usage)
	if err != nil {
		return fmt.Errorf("failed to marshal hashtag usage: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to record hashtag usage: %w", err)
	}

	// Also update the trending index
	return s.updateHashtagTrendScore(ctx, hashtag)
}

// RecordStatusEngagement records engagement on a status (like, boost, reply)
func (s *dynamoDBStorage) RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error {
	// Create an engagement record
	engagement := map[string]any{
		"PK":             fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID),
		"SK":             fmt.Sprintf("%s#%s#%s", engagementType, time.Now().Format("20060102150405"), userID),
		"StatusID":       statusID,
		"EngagementType": engagementType,
		"UserID":         userID,
		"EngagedAt":      time.Now().Format(time.RFC3339),
		"TTL":            time.Now().Add(7 * 24 * time.Hour).Unix(),
	}

	item, err := attributevalue.MarshalMap(engagement)
	if err != nil {
		return fmt.Errorf("failed to marshal status engagement: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to record status engagement: %w", err)
	}

	// Update status trend score
	return s.updateStatusTrendScore(ctx, statusID)
}

// RecordLinkShare records when a link is shared in a status
func (s *dynamoDBStorage) RecordLinkShare(ctx context.Context, url string, statusID string, authorID string) error {
	// Create a link share record
	share := map[string]any{
		"PK":       fmt.Sprintf("LINK_SHARE#%s", url),
		"SK":       fmt.Sprintf("STATUS#%s", statusID),
		"URL":      url,
		"StatusID": statusID,
		"AuthorID": authorID,
		"SharedAt": time.Now().Format(time.RFC3339),
		"TTL":      time.Now().Add(7 * 24 * time.Hour).Unix(),
	}

	item, err := attributevalue.MarshalMap(share)
	if err != nil {
		return fmt.Errorf("failed to marshal link share: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to record link share: %w", err)
	}

	// Update link trend score
	return s.updateLinkTrendScore(ctx, url)
}

// GetTrendingHashtags returns the top trending hashtags since the given time
func (s *dynamoDBStorage) GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error) {
	// Query the trending index for hashtags
	timeBucket := time.Now().Format("2006-01-02")
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI8"), // Trending index
		KeyConditionExpression: aws.String("GSI8PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket)},
		},
		ScanIndexForward: aws.Bool(false), // Sort by score descending
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// For now, return empty results if GSI doesn't exist
		s.logger().Warn("failed to query trending hashtags", zap.Error(err))
		return []*storage.TrendingHashtag{}, nil
	}

	trends := make([]*storage.TrendingHashtag, 0, len(result.Items))
	for _, item := range result.Items {
		trend := &storage.TrendingHashtag{}

		// Extract fields manually to handle the data properly
		if name, ok := item["Name"]; ok {
			if nameStr, ok := name.(*types.AttributeValueMemberS); ok {
				trend.Name = nameStr.Value
			}
		}
		if url, ok := item["URL"]; ok {
			if urlStr, ok := url.(*types.AttributeValueMemberS); ok {
				trend.URL = urlStr.Value
			}
		}
		if usage, ok := item["UsageCount"]; ok {
			if usageNum, ok := usage.(*types.AttributeValueMemberN); ok {
				if val, err := strconv.ParseInt(usageNum.Value, 10, 64); err == nil {
					trend.UsageCount = val
				}
			}
		}
		if users, ok := item["UniqueUsers"]; ok {
			if usersNum, ok := users.(*types.AttributeValueMemberN); ok {
				if val, err := strconv.ParseInt(usersNum.Value, 10, 64); err == nil {
					trend.UniqueUsers = val
				}
			}
		}
		if lastUsed, ok := item["LastUsed"]; ok {
			if lastStr, ok := lastUsed.(*types.AttributeValueMemberS); ok {
				if t, err := time.Parse(time.RFC3339, lastStr.Value); err == nil {
					trend.LastUsed = t
				}
			}
		}
		if firstSeen, ok := item["FirstSeen"]; ok {
			if firstStr, ok := firstSeen.(*types.AttributeValueMemberS); ok {
				if t, err := time.Parse(time.RFC3339, firstStr.Value); err == nil {
					trend.FirstSeen = t
				}
			}
		}

		trends = append(trends, trend)
	}

	return trends, nil
}

// GetTrendingStatuses returns the top trending statuses since the given time
func (s *dynamoDBStorage) GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error) {
	timeBucket := time.Now().Format("2006-01-02")
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI8"),
		KeyConditionExpression: aws.String("GSI8PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket)},
		},
		ScanIndexForward: aws.Bool(false), // Sort by score descending
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// For now, return empty results if GSI doesn't exist
		s.logger().Warn("failed to query trending statuses", zap.Error(err))
		return []*storage.TrendingStatus{}, nil
	}

	trends := make([]*storage.TrendingStatus, 0, len(result.Items))
	for _, item := range result.Items {
		trend := &storage.TrendingStatus{}

		// Extract fields manually to handle the data properly
		if id, ok := item["ID"]; ok {
			if idStr, ok := id.(*types.AttributeValueMemberS); ok {
				trend.ID = idStr.Value
			}
		}
		if url, ok := item["URL"]; ok {
			if urlStr, ok := url.(*types.AttributeValueMemberS); ok {
				trend.URL = urlStr.Value
			}
		}
		if authorID, ok := item["AuthorID"]; ok {
			if authorStr, ok := authorID.(*types.AttributeValueMemberS); ok {
				trend.AuthorID = authorStr.Value
			}
		}
		if content, ok := item["Content"]; ok {
			if contentStr, ok := content.(*types.AttributeValueMemberS); ok {
				trend.Content = contentStr.Value
			}
		}
		if engagements, ok := item["Engagements"]; ok {
			if engNum, ok := engagements.(*types.AttributeValueMemberN); ok {
				if val, err := strconv.ParseInt(engNum.Value, 10, 64); err == nil {
					trend.Engagements = val
				}
			}
		}
		if publishedAt, ok := item["PublishedAt"]; ok {
			if pubStr, ok := publishedAt.(*types.AttributeValueMemberS); ok {
				if t, err := time.Parse(time.RFC3339, pubStr.Value); err == nil {
					trend.PublishedAt = t
				}
			}
		}

		trends = append(trends, trend)
	}

	return trends, nil
}

// GetTrendingLinks returns the top trending links since the given time
func (s *dynamoDBStorage) GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error) {
	timeBucket := time.Now().Format("2006-01-02")
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		IndexName:              aws.String("GSI8"),
		KeyConditionExpression: aws.String("GSI8PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket)},
		},
		ScanIndexForward: aws.Bool(false), // Sort by score descending
		Limit:            safeInt32(limit),
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// For now, return empty results if GSI doesn't exist
		return []*storage.TrendingLink{}, nil
	}

	trends := make([]*storage.TrendingLink, 0, len(result.Items))
	for _, item := range result.Items {
		trend := &storage.TrendingLink{}

		// Extract fields manually
		if url, ok := item["URL"]; ok {
			if urlStr, ok := url.(*types.AttributeValueMemberS); ok {
				trend.URL = urlStr.Value
			}
		}
		if title, ok := item["Title"]; ok {
			if titleStr, ok := title.(*types.AttributeValueMemberS); ok {
				trend.Title = titleStr.Value
			}
		}
		if desc, ok := item["Description"]; ok {
			if descStr, ok := desc.(*types.AttributeValueMemberS); ok {
				trend.Description = descStr.Value
			}
		}
		if linkType, ok := item["Type"]; ok {
			if typeStr, ok := linkType.(*types.AttributeValueMemberS); ok {
				trend.Type = typeStr.Value
			}
		}
		if author, ok := item["AuthorName"]; ok {
			if authorStr, ok := author.(*types.AttributeValueMemberS); ok {
				trend.AuthorName = authorStr.Value
			}
		}
		if image, ok := item["Image"]; ok {
			if imgStr, ok := image.(*types.AttributeValueMemberS); ok {
				trend.Image = imgStr.Value
			}
		}
		if shares, ok := item["ShareCount"]; ok {
			if shareNum, ok := shares.(*types.AttributeValueMemberN); ok {
				if val, err := strconv.ParseInt(shareNum.Value, 10, 64); err == nil {
					trend.ShareCount = val
				}
			}
		}

		trends = append(trends, trend)
	}

	return trends, nil
}

// Helper methods for updating trend scores

func (s *dynamoDBStorage) updateHashtagTrendScore(ctx context.Context, hashtag string) error {
	// Query recent usage (last 24 hours)
	since := time.Now().Add(-24 * time.Hour)

	// Count unique users and total usage
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("HASHTAG_USAGE#%s", hashtag)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return fmt.Errorf("failed to query hashtag usage: %w", err)
	}

	// Count unique users
	uniqueUsers := make(map[string]bool)
	usageCount := int64(0)

	for _, item := range result.Items {
		// Extract AuthorID
		if authorAttr, ok := item["AuthorID"]; ok {
			if author, ok := authorAttr.(*types.AttributeValueMemberS); ok {
				uniqueUsers[author.Value] = true
			}
		}

		// Check if usage is recent
		if usedAtAttr, ok := item["UsedAt"]; ok {
			if usedAtStr, ok := usedAtAttr.(*types.AttributeValueMemberS); ok {
				if usedAt, err := time.Parse(time.RFC3339, usedAtStr.Value); err == nil {
					if usedAt.After(since) {
						usageCount++
					}
				}
			}
		}
	}

	// Calculate trend score using time-decay algorithm
	now := time.Now()
	ageFactor := 1.0 // New hashtags get full score
	diversityFactor := float64(len(uniqueUsers)) / float64(usageCount+1)
	score := float64(usageCount) * ageFactor * (1 + diversityFactor)

	// Update trending index entry
	timeBucket := now.Format("2006-01-02")
	paddedScore := fmt.Sprintf("%010.0f", score*1000) // Pad for proper sorting

	trendItem := map[string]any{
		"PK":          fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket),
		"SK":          fmt.Sprintf("SCORE#%s#%s", paddedScore, hashtag),
		"GSI8PK":      fmt.Sprintf("TREND_TYPE#HASHTAG#%s", timeBucket), // For GSI8
		"GSI8SK":      fmt.Sprintf("SCORE#%s#%s", paddedScore, hashtag),
		"Name":        hashtag,
		"URL":         fmt.Sprintf("https://%s/tags/%s", s.domain, hashtag),
		"UsageCount":  usageCount,
		"UniqueUsers": int64(len(uniqueUsers)),
		"LastUsed":    now.Format(time.RFC3339),
		"FirstSeen":   now.Format(time.RFC3339), // Would be updated if exists
		"TrendScore":  score,
		"UpdatedAt":   now.Format(time.RFC3339),
		"TTL":         now.Add(7 * 24 * time.Hour).Unix(),
	}

	trendItemAV, err := attributevalue.MarshalMap(trendItem)
	if err != nil {
		return fmt.Errorf("failed to marshal trend item: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      trendItemAV,
	})

	return err
}

func (s *dynamoDBStorage) updateStatusTrendScore(ctx context.Context, statusID string) error {
	// First, get the status details
	statusObj, err := s.GetObject(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	// Extract status details
	var authorID, content, url string
	var publishedAt time.Time

	if note, ok := statusObj.(map[string]any); ok {
		if actor, ok := note["attributedTo"].(string); ok {
			authorID = actor
		}
		if c, ok := note["content"].(string); ok {
			content = c
			if len(content) > 500 {
				content = content[:500] + "..."
			}
		}
		if u, ok := note["url"].(string); ok {
			url = u
		}
		if p, ok := note["published"].(string); ok {
			publishedAt, _ = time.Parse(time.RFC3339, p)
		}
	}

	// Count recent engagements
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS_ENGAGEMENT#%s", statusID)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return fmt.Errorf("failed to query status engagement: %w", err)
	}

	// Count engagement types
	engagementCounts := map[string]int{
		"like":  0,
		"boost": 0,
		"reply": 0,
	}
	uniqueEngagers := make(map[string]bool)

	for _, item := range result.Items {
		if typeAttr, ok := item["EngagementType"]; ok {
			if engType, ok := typeAttr.(*types.AttributeValueMemberS); ok {
				engagementCounts[engType.Value]++
			}
		}
		if userAttr, ok := item["UserID"]; ok {
			if user, ok := userAttr.(*types.AttributeValueMemberS); ok {
				uniqueEngagers[user.Value] = true
			}
		}
	}

	// Calculate engagement score
	totalEngagements := int64(engagementCounts["like"] + engagementCounts["boost"]*2 + engagementCounts["reply"]*3)

	// Get author trust score (simplified - would query trust service)
	authorTrustScore := 1.0

	// Calculate trend score
	now := time.Now()
	age := now.Sub(publishedAt)
	ageFactor := 1.0 / (1 + age.Hours()/2) // 2-hour half-life
	diversityFactor := float64(len(uniqueEngagers)) / float64(totalEngagements+1)
	score := float64(totalEngagements) * ageFactor * (1 + diversityFactor) * authorTrustScore

	// Update trending index entry
	timeBucket := now.Format("2006-01-02")
	paddedScore := fmt.Sprintf("%010.0f", score*1000)

	trendItem := map[string]any{
		"PK":          fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket),
		"SK":          fmt.Sprintf("SCORE#%s#%s", paddedScore, statusID),
		"GSI8PK":      fmt.Sprintf("TREND_TYPE#STATUS#%s", timeBucket),
		"GSI8SK":      fmt.Sprintf("SCORE#%s#%s", paddedScore, statusID),
		"ID":          statusID,
		"URL":         url,
		"AuthorID":    authorID,
		"Content":     content,
		"Engagements": totalEngagements,
		"PublishedAt": publishedAt.Format(time.RFC3339),
		"TrendScore":  score,
		"UpdatedAt":   now.Format(time.RFC3339),
		"TTL":         now.Add(7 * 24 * time.Hour).Unix(),
	}

	trendItemAV, err := attributevalue.MarshalMap(trendItem)
	if err != nil {
		return fmt.Errorf("failed to marshal trend item: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      trendItemAV,
	})

	return err
}

func (s *dynamoDBStorage) updateLinkTrendScore(ctx context.Context, url string) error {
	// Query recent link shares
	queryInput := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("LINK_SHARE#%s", url)},
		},
	}

	result, err := s.client.Query(ctx, queryInput)
	if err != nil {
		return fmt.Errorf("failed to query link shares: %w", err)
	}

	// Count unique sharers
	uniqueSharers := make(map[string]bool)
	shareCount := int64(0)
	since := time.Now().Add(-24 * time.Hour)

	for _, item := range result.Items {
		// Extract AuthorID
		if authorAttr, ok := item["AuthorID"]; ok {
			if author, ok := authorAttr.(*types.AttributeValueMemberS); ok {
				uniqueSharers[author.Value] = true
			}
		}

		// Check if share is recent
		if sharedAtAttr, ok := item["SharedAt"]; ok {
			if sharedAtStr, ok := sharedAtAttr.(*types.AttributeValueMemberS); ok {
				if sharedAt, err := time.Parse(time.RFC3339, sharedAtStr.Value); err == nil {
					if sharedAt.After(since) {
						shareCount++
					}
				}
			}
		}
	}

	// Extract basic link metadata
	title := extractDomainFromURL(url)
	description := ""
	image := ""
	linkType := "link"

	// Determine link type based on URL patterns
	lowerURL := strings.ToLower(url)
	if strings.Contains(lowerURL, "youtube.com") || strings.Contains(lowerURL, "youtu.be") {
		linkType = "video"
		title = "YouTube Video"
	} else if strings.Contains(lowerURL, ".jpg") || strings.Contains(lowerURL, ".png") ||
		strings.Contains(lowerURL, ".gif") || strings.Contains(lowerURL, ".webp") {
		linkType = "photo"
		title = "Image"
		image = url
	}

	// Calculate trend score
	now := time.Now()
	diversityFactor := float64(len(uniqueSharers)) / float64(shareCount+1)
	score := float64(shareCount) * (1 + diversityFactor)

	// Update trending index entry
	timeBucket := now.Format("2006-01-02")
	paddedScore := fmt.Sprintf("%010.0f", score*1000)

	trendItem := map[string]any{
		"PK":          fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket),
		"SK":          fmt.Sprintf("SCORE#%s#%s", paddedScore, url),
		"GSI8PK":      fmt.Sprintf("TREND_TYPE#LINK#%s", timeBucket),
		"GSI8SK":      fmt.Sprintf("SCORE#%s#%s", paddedScore, url),
		"URL":         url,
		"Title":       title,
		"Description": description,
		"Type":        linkType,
		"AuthorName":  "", // Could extract from first sharer
		"Image":       image,
		"ShareCount":  shareCount,
		"TrendScore":  score,
		"UpdatedAt":   now.Format(time.RFC3339),
		"TTL":         now.Add(7 * 24 * time.Hour).Unix(),
	}

	trendItemAV, err := attributevalue.MarshalMap(trendItem)
	if err != nil {
		return fmt.Errorf("failed to marshal trend item: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      trendItemAV,
	})

	return err
}

// extractDomainFromURL extracts the domain name from a URL for use as a title
func extractDomainFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	domain := parsedURL.Hostname()
	// Remove www. prefix if present
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

// DeleteOldHashtagTrends deletes hashtag trend records older than the specified time
func (s *dynamoDBStorage) DeleteOldHashtagTrends(ctx context.Context, before time.Time) error {
	// Query for old hashtag trends
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("begins_with(PK, :pk_prefix) AND CreatedAt < :before"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk_prefix": &types.AttributeValueMemberS{Value: "HASHTAG_TREND#"},
			":before":    &types.AttributeValueMemberS{Value: before.Format(time.RFC3339)},
		},
		ProjectionExpression: aws.String("PK, SK"),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan for old hashtag trends: %w", err)
	}

	// Delete items in batches
	for len(result.Items) > 0 {
		writeRequests := make([]types.WriteRequest, 0, min(25, len(result.Items)))

		for i := 0; i < min(25, len(result.Items)); i++ {
			item := result.Items[i]
			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"PK": item["PK"],
						"SK": item["SK"],
					},
				},
			})
		}

		// Batch delete
		_, err := s.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				*s.getTableName(): writeRequests,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to batch delete old hashtag trends: %w", err)
		}

		// Remove processed items
		result.Items = result.Items[len(writeRequests):]
	}

	return nil
}

// DeleteOldLinkTrends deletes link trend records older than the specified time
func (s *dynamoDBStorage) DeleteOldLinkTrends(ctx context.Context, before time.Time) error {
	// Query for old link trends
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("begins_with(PK, :pk_prefix) AND CreatedAt < :before"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk_prefix": &types.AttributeValueMemberS{Value: "LINK_TREND#"},
			":before":    &types.AttributeValueMemberS{Value: before.Format(time.RFC3339)},
		},
		ProjectionExpression: aws.String("PK, SK"),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan for old link trends: %w", err)
	}

	// Delete items in batches
	for len(result.Items) > 0 {
		writeRequests := make([]types.WriteRequest, 0, min(25, len(result.Items)))

		for i := 0; i < min(25, len(result.Items)); i++ {
			item := result.Items[i]
			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"PK": item["PK"],
						"SK": item["SK"],
					},
				},
			})
		}

		// Batch delete
		_, err := s.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				*s.getTableName(): writeRequests,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to batch delete old link trends: %w", err)
		}

		// Remove processed items
		result.Items = result.Items[len(writeRequests):]
	}

	return nil
}

// DeleteOldStatusTrends deletes status trend records older than the specified time
func (s *dynamoDBStorage) DeleteOldStatusTrends(ctx context.Context, before time.Time) error {
	// Query for old status trends
	input := &dynamodb.ScanInput{
		TableName:        s.getTableName(),
		FilterExpression: aws.String("begins_with(PK, :pk_prefix) AND CreatedAt < :before"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk_prefix": &types.AttributeValueMemberS{Value: "STATUS_TREND#"},
			":before":    &types.AttributeValueMemberS{Value: before.Format(time.RFC3339)},
		},
		ProjectionExpression: aws.String("PK, SK"),
	}

	result, err := s.client.Scan(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan for old status trends: %w", err)
	}

	// Delete items in batches
	for len(result.Items) > 0 {
		writeRequests := make([]types.WriteRequest, 0, min(25, len(result.Items)))

		for i := 0; i < min(25, len(result.Items)); i++ {
			item := result.Items[i]
			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"PK": item["PK"],
						"SK": item["SK"],
					},
				},
			})
		}

		// Batch delete
		_, err := s.client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				*s.getTableName(): writeRequests,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to batch delete old status trends: %w", err)
		}

		// Remove processed items
		result.Items = result.Items[len(writeRequests):]
	}

	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
