package dynamodb

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// GetFollowerCountBucket returns the appropriate bucket for a given follower count
func GetFollowerCountBucket(count int) string {
	switch {
	case count >= 10000:
		return "10k+"
	case count >= 1000:
		return "1k-10k"
	case count >= 100:
		return "100-1k"
	default:
		return "1-100"
	}
}

// FormatFollowerCountForGSI formats the follower count for GSI4SK to ensure proper sorting
// Returns a zero-padded string (10 digits) followed by username
func FormatFollowerCountForGSI(count int, username string) string {
	// Pad to 10 digits for proper sorting within bucket
	paddedCount := fmt.Sprintf("%010d", count)
	return fmt.Sprintf("%s#%s", paddedCount, username)
}

// ParseFollowerCountFromGSI extracts the follower count from a GSI4SK value
func ParseFollowerCountFromGSI(gsi4sk string) (int, error) {
	parts := strings.Split(gsi4sk, "#")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid GSI4SK format: %s", gsi4sk)
	}

	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse follower count: %w", err)
	}

	return count, nil
}

// ExtractActorCounts extracts follower, following, and status counts from DynamoDB item attributes
// This will be used until we have proper count tracking in the Actor struct
type ActorCounts struct {
	FollowerCount  int
	FollowingCount int
	StatusCount    int
}

// GetActorCountsFromItem extracts counts from a DynamoDB item
// GetActorCountsFromItem extracts follower, following, and status counts from a DynamoDB item
func GetActorCountsFromItem(item map[string]interface{}) ActorCounts {
	counts := ActorCounts{
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
	}

	// Extract FollowerCount
	if fc, ok := item["FollowerCount"]; ok {
		if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
			counts.FollowerCount, _ = strconv.Atoi(fcNum.Value)
		}
	}

	// Extract FollowingCount
	if fc, ok := item["FollowingCount"]; ok {
		if fcNum, ok := fc.(*types.AttributeValueMemberN); ok {
			counts.FollowingCount, _ = strconv.Atoi(fcNum.Value)
		}
	}

	// Extract StatusCount
	if sc, ok := item["StatusCount"]; ok {
		if scNum, ok := sc.(*types.AttributeValueMemberN); ok {
			counts.StatusCount, _ = strconv.Atoi(scNum.Value)
		}
	}

	return counts
}
