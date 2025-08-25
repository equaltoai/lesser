package graph

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"go.uber.org/zap"
)

// generateID generates a unique ID for objects
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}

// floatPtr returns a pointer to a float64

// intPtr returns a pointer to an int

// boolPtr returns a pointer to a bool

// deriveVisibility determines the visibility level based on To and CC fields

// convertMentions extracts mentions from tags

// convertTags filters tags to exclude mentions

// convertAttachments converts attachment slice to pointer slice

// getTimeOrNow returns the time or current time if nil

// getUsernameFromContext extracts username from authentication context
func getUsernameFromContext(ctx context.Context) string {
	// Extract claims from context
	if claims, ok := ctx.Value(common.ContextKeyClaims).(*auth.Claims); ok && claims != nil {
		return claims.Username
	}
	return ""
}

// GetUserID extracts user ID from authentication context
func GetUserID(ctx context.Context) string {
	// Try to get claims from context
	if claims, ok := ctx.Value(common.ContextKeyClaims).(*auth.Claims); ok && claims != nil {
		return claims.Username // In this system, username is used as user ID
	}
	return ""
}

// convertToGraphQLObject converts storage objects to GraphQL model objects

// validateNoteInput validates the input for creating a note

// extractDomainFromActorID extracts the domain from an actor ID

// generateUniqueID generates a unique ID for objects

// determineAudience determines the To field based on visibility

// determineCCAudience determines the CC field based on visibility

// getSensitive safely extracts the sensitive flag

// getSpoilerText safely extracts the spoiler text

// buildTags builds tag array from hashtags and mentions

// buildAttachments builds attachment objects from media IDs

// shouldFederate determines if an activity should be federated based on visibility

// convertToGraphQLObject converts an ActivityPub object to GraphQL Object type

// getObjectActorID extracts the actor ID from an object

// determineModerationCategory categorizes the moderation reason

// Helper methods for getting object interaction counts

// calculateMissingPosts calculates the number of missing posts in a thread

// calculateAverageEngagement calculates average engagement for posts in the thread

// executeSocialAction executes a social action (like, share) and returns the ActivityPub activity
func (r *mutationResolver) executeSocialAction(
	ctx context.Context,
	objectID string,
	actionType string,
	actionName string,
	serviceCall func(context.Context, string, string) error,
) (*activitypub.Activity, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Execute the service call
	err = serviceCall(ctx, objectID, username)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Failed to %s object", actionName),
			zap.String("user", username),
			zap.String("object", objectID),
			zap.Error(err))
		return nil, ErrSocialActionFailedWithContext(actionName, err)
	}

	// Track costs
	r.trackDynamoOperation(ctx, "write", 1)

	// Return activity
	now := time.Now()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        generateID(),
			Type:      actionType,
			Published: &now,
		},
		Actor:  username,
		Object: objectID,
	}, nil
}

// executeSocialUndo executes a social undo action (unlike, unshare) and returns success
func (r *mutationResolver) executeSocialUndo(
	ctx context.Context,
	objectID string,
	actionName string,
	serviceCall func(context.Context, string, string) error,
) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Execute the service call
	err = serviceCall(ctx, objectID, username)
	if err != nil {
		r.Logger.Error(fmt.Sprintf("Failed to %s object", actionName),
			zap.String("user", username),
			zap.String("object", objectID),
			zap.Error(err))
		return false, ErrSocialUndoFailedWithContext(actionName, err)
	}

	// Track costs
	r.trackDynamoOperation(ctx, "write", 1)

	return true, nil
}

// executeListMembershipOperation executes a list membership operation (add/remove accounts)
func (r *mutationResolver) executeListMembershipOperation(
	ctx context.Context,
	listID string,
	accountIDs []string,
	actionName string,
	serviceCall func(ctx context.Context, listID, accountID, username string) (*lists.MembershipResult, error),
) (*model.List, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Process each account individually
	var lastResult *lists.MembershipResult
	for _, accountID := range accountIDs {
		result, err := serviceCall(ctx, listID, accountID, username)
		if err != nil {
			r.Logger.Error(fmt.Sprintf("Failed to %s account to list", actionName),
				zap.String("user", username),
				zap.String("list", listID),
				zap.String("account", accountID),
				zap.Error(err))
			// Continue with other accounts even if one fails
			continue
		}
		lastResult = result
	}

	if lastResult == nil {
		return nil, ErrListMembershipFailedWithAction(actionName)
	}

	// Get the updated list
	list, err := r.Registry.Lists().GetList(ctx, &lists.GetListQuery{
		ListID:   listID,
		ViewerID: username,
	})
	if err != nil {
		return nil, ErrGetUpdatedListFailedWithContext(err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", int64(len(accountIDs)))
	return r.convertListToGraphQL(ctx, list), nil
}

// buildAndSortDrivers creates, sorts, and limits cost drivers
func (r *queryResolver) buildAndSortDrivers(drivers []*model.Driver) []*model.Driver {
	// Sort by cost percentage
	sort.Slice(drivers, func(i, j int) bool {
		return drivers[i].PercentOfTotal > drivers[j].PercentOfTotal
	})

	// Return top 5 drivers
	if len(drivers) > 5 {
		return drivers[:5]
	}
	return drivers
}

// createReadWriteDrivers creates standard DynamoDB read/write cost drivers
func (r *queryResolver) createReadWriteDrivers(totalReads, totalWrites int64, totalCost float64) []*model.Driver {
	drivers := []*model.Driver{}

	if totalReads > 0 && totalCost > 0 {
		readCost := float64(totalReads) * 0.00025 // Approximate DynamoDB read cost
		readPercentage := (readCost / totalCost) * 100
		drivers = append(drivers, &model.Driver{
			Type:           "DynamoDB Reads",
			Cost:           readCost,
			PercentOfTotal: readPercentage,
			Trend:          model.TrendStable,
		})
	}

	if totalWrites > 0 && totalCost > 0 {
		writeCost := float64(totalWrites) * 0.00125 // Approximate DynamoDB write cost
		writePercentage := (writeCost / totalCost) * 100
		drivers = append(drivers, &model.Driver{
			Type:           "DynamoDB Writes",
			Cost:           writeCost,
			PercentOfTotal: writePercentage,
			Trend:          model.TrendStable,
		})
	}

	return r.buildAndSortDrivers(drivers)
}
