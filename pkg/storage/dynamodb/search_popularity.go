package dynamodb

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// PopularitySearchStrategy performs search based on follower count and engagement
type PopularitySearchStrategy struct {
	service *SearchService
}

func (s *PopularitySearchStrategy) Name() string {
	return "popularity_search"
}

func (s *PopularitySearchStrategy) Search(ctx context.Context, query string, options SearchOptions) ([]*SearchResult, error) {
	searchTerm := strings.ToLower(strings.TrimSpace(query))

	// Determine follower count bucket to search
	// GSI4PK format: ACTOR_RANK#<bucket> where bucket is 1-100, 100-1k, 1k-10k, etc.
	buckets := []string{"10k+", "1k-10k", "100-1k", "1-100"}

	results := make([]*SearchResult, 0)

	for _, bucket := range buckets {
		// Query GSI4 for popular actors in this bucket
		gsi4pk := fmt.Sprintf("ACTOR_RANK#%s", bucket)

		// Build expression with optional filter for search term
		builder := expression.NewBuilder().
			WithKeyCondition(
				expression.Key("GSI4PK").Equal(expression.Value(gsi4pk)),
			)

		// Add filter for username or display name containing the search term
		if searchTerm != "" {
			builder = builder.WithFilter(
				expression.Or(
					expression.Contains(expression.Name("Username"), searchTerm),
					expression.Contains(expression.Name("DisplayName"), searchTerm),
				),
			)
		}

		expr, err := builder.Build()
		if err != nil {
			s.service.logger.Warn("failed to build expression", zap.Error(err))
			continue
		}

		queryInput := &dynamodb.QueryInput{
			TableName:                 aws.String(s.service.tableName),
			IndexName:                 aws.String("GSI4"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Limit:                     safeInt32(options.Limit),
			ScanIndexForward:          aws.Bool(false), // Sort by follower count descending
		}

		if expr.Filter() != nil {
			queryInput.FilterExpression = expr.Filter()
		}

		result, err := s.service.dynamo.Query(ctx, queryInput)
		if err != nil {
			s.service.logger.Warn("popularity search query failed",
				zap.String("bucket", bucket),
				zap.Error(err))
			continue
		}

		for _, item := range result.Items {
			var record ActorRecord
			if err := attributevalue.UnmarshalMap(item, &record); err != nil {
				continue
			}

			if record.Actor != nil {
				// Extract counts from the record
				followerCount := 0
				if fc, ok := item["FollowerCount"]; ok {
					if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
						followerCount, _ = strconv.Atoi(fcNum.Value)
					}
				}

				followingCount := 0
				if fc, ok := item["FollowingCount"]; ok {
					if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
						followingCount, _ = strconv.Atoi(fcNum.Value)
					}
				}

				statusCount := 0
				if sc, ok := item["StatusCount"]; ok {
					if scNum, ok := sc.(*types.AttributeValueMemberN); ok {
						statusCount, _ = strconv.Atoi(scNum.Value)
					}
				}

				// Calculate score based on follower count and activity
				score := s.calculatePopularityScoreWithCounts(record.Actor, searchTerm, followerCount, followingCount, statusCount)

				// Apply time decay for inactive accounts
				if record.UpdatedAt.Before(time.Now().AddDate(0, -3, 0)) {
					// Account inactive for 3+ months, reduce score
					monthsInactive := time.Since(record.UpdatedAt).Hours() / 24 / 30
					decay := math.Exp(-0.1 * monthsInactive)
					score *= decay
				}

				searchResult := &SearchResult{
					Actor:         record.Actor,
					Score:         score,
					MatchedFields: []string{"popularity"},
				}

				// Add highlights if term matches
				if searchTerm != "" {
					searchResult.Highlights = s.generateHighlights(record.Actor, searchTerm)
					if len(searchResult.Highlights) > 0 {
						searchResult.MatchedFields = append(searchResult.MatchedFields, "content")
					}
				}

				results = append(results, searchResult)
			}
		}

		// If we have enough results from higher buckets, stop searching lower ones
		if len(results) >= options.Limit {
			break
		}
	}

	// Sort by score and apply limit
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > options.Limit {
		results = results[:options.Limit]
	}

	return results, nil
}

func (s *PopularitySearchStrategy) calculatePopularityScoreWithCounts(actor *activitypub.Actor, searchTerm string, followerCount, followingCount, statusCount int) float64 {
	// Base score from follower count (logarithmic scale)
	followerScore := 0.0

	if followerCount > 0 {
		followerScore = math.Log10(float64(followerCount)+1) / 6.0 // Normalize to ~0-1 range
	}

	// Engagement score from post count and following ratio
	engagementScore := 0.0
	if statusCount > 0 {
		// Active posters get a boost
		engagementScore += math.Min(float64(statusCount)/1000.0, 0.2)
	}

	// Following/follower ratio penalty (avoid spam accounts)
	if followerCount > 0 && followingCount > 0 {
		ratio := float64(followingCount) / float64(followerCount)
		if ratio > 10 {
			// Likely spam account, heavy penalty
			engagementScore -= 0.3
		} else if ratio > 3 {
			// Moderate penalty
			engagementScore -= 0.1
		}
	}

	// Exact match bonus
	exactMatchBonus := 0.0
	if searchTerm != "" {
		usernameLower := strings.ToLower(actor.PreferredUsername)
		displayNameLower := strings.ToLower(actor.Name)

		if usernameLower == searchTerm {
			exactMatchBonus = 0.3
		} else if displayNameLower == searchTerm {
			exactMatchBonus = 0.2
		} else if strings.Contains(usernameLower, searchTerm) {
			exactMatchBonus = 0.1
		} else if strings.Contains(displayNameLower, searchTerm) {
			exactMatchBonus = 0.05
		}
	}

	// Combine scores (popularity search prioritizes follower count)
	totalScore := (followerScore * 0.7) + (engagementScore * 0.2) + (exactMatchBonus * 0.1)

	// Ensure score is in 0-1 range
	return math.Max(0, math.Min(1, totalScore))
}

func (s *PopularitySearchStrategy) generateHighlights(actor *activitypub.Actor, searchTerm string) map[string]string {
	highlights := make(map[string]string)

	// Check username
	if strings.Contains(strings.ToLower(actor.PreferredUsername), searchTerm) {
		idx := strings.Index(strings.ToLower(actor.PreferredUsername), searchTerm)
		if idx >= 0 {
			highlighted := actor.PreferredUsername[:idx] + "<em>" +
				actor.PreferredUsername[idx:idx+len(searchTerm)] + "</em>" +
				actor.PreferredUsername[idx+len(searchTerm):]
			highlights["username"] = highlighted
		}
	}

	// Check display name
	if strings.Contains(strings.ToLower(actor.Name), searchTerm) {
		idx := strings.Index(strings.ToLower(actor.Name), searchTerm)
		if idx >= 0 {
			highlighted := actor.Name[:idx] + "<em>" +
				actor.Name[idx:idx+len(searchTerm)] + "</em>" +
				actor.Name[idx+len(searchTerm):]
			highlights["display_name"] = highlighted
		}
	}

	return highlights
}
