package graph

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/equaltoai/lesser/pkg/services/hashtags"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/severance"
	"github.com/equaltoai/lesser/pkg/services/threads"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

const (
	// allOperationsValue represents all operations for a service
	allOperationsValue = "All"
)

// generateID generates a unique ID for objects
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// notificationMatchesTypes checks if a notification matches one of the provided types
func notificationMatchesTypes(notification *model.Notification, types []string) bool {
	if len(types) == 0 {
		return true
	}

	for _, t := range types {
		if notification.Type == t {
			return true
		}
	}

	return false
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

func buildActivityFromAnnounce(actorUsername, objectID string, announce *storage.Announce) *activitypub.Activity {
	if announce == nil {
		return nil
	}

	published := announce.Published
	if published.IsZero() {
		published = time.Now()
	}

	publishedCopy := published
	to := append([]string(nil), announce.To...)
	cc := append([]string(nil), announce.CC...)

	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:        announce.ID,
			Type:      activitypub.AnnounceType,
			Published: &publishedCopy,
			To:        to,
			CC:        cc,
		},
		Actor:  actorUsername,
		Object: objectID,
	}
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
func (r *queryResolver) buildAndSortDrivers(drivers []*cost.Driver) []*cost.Driver {
	// Sort by cost percentage
	sort.Slice(drivers, func(i, j int) bool {
		return drivers[i].PercentageOfTotal > drivers[j].PercentageOfTotal
	})

	// Return top 5 drivers
	if len(drivers) > 5 {
		return drivers[:5]
	}
	return drivers
}

// createReadWriteDrivers creates standard DynamoDB read/write cost drivers
func (r *queryResolver) createReadWriteDrivers(totalReads, totalWrites int64, totalCost float64) []*cost.Driver {
	drivers := []*cost.Driver{}

	if totalReads > 0 && totalCost > 0 {
		readCost := float64(totalReads) * 0.00025 // Approximate DynamoDB read cost
		readPercentage := (readCost / totalCost) * 100
		readCostMicro := int64(readCost * 1_000_000)
		drivers = append(drivers, &cost.Driver{
			Service:           "DynamoDB",
			Operation:         "Read",
			CostMicroCents:    readCostMicro,
			PercentageOfTotal: readPercentage,
			OperationCount:    totalReads,
			AverageCost:       readCostMicro / totalReads,
			Trend:             "STABLE", // Will be calculated by enrichDriversWithTrends
		})
	}

	if totalWrites > 0 && totalCost > 0 {
		writeCost := float64(totalWrites) * 0.00125 // Approximate DynamoDB write cost
		writePercentage := (writeCost / totalCost) * 100
		writeCostMicro := int64(writeCost * 1_000_000)
		drivers = append(drivers, &cost.Driver{
			Service:           "DynamoDB",
			Operation:         "Write",
			CostMicroCents:    writeCostMicro,
			PercentageOfTotal: writePercentage,
			OperationCount:    totalWrites,
			AverageCost:       writeCostMicro / totalWrites,
			Trend:             "STABLE", // Will be calculated by enrichDriversWithTrends
		})
	}

	return r.buildAndSortDrivers(drivers)
}

// enrichDriversWithTrends calculates and sets trend for each driver based on historical data
func (r *queryResolver) enrichDriversWithTrends(ctx context.Context, drivers []*cost.Driver, currentPeriodStart, currentPeriodEnd time.Time) []*cost.Driver {
	if len(drivers) == 0 {
		return drivers
	}

	// Get tracking repository
	storage := r.Registry.GetStorage()
	if storage == nil {
		r.Logger.Warn("storage not available for trend calculation")
		return drivers
	}

	costRepo := storage.Cost()
	if costRepo == nil {
		r.Logger.Warn("cost repository not available for trend calculation")
		return drivers
	}

	// Calculate previous period (same duration as current period)
	periodDuration := currentPeriodEnd.Sub(currentPeriodStart)
	previousPeriodEnd := currentPeriodStart
	previousPeriodStart := previousPeriodEnd.Add(-periodDuration)

	// For each driver, calculate trend
	for _, driver := range drivers {
		// Get current period cost (already have it)
		currentCost := float64(driver.CostMicroCents) / 1_000_000.0

		// Get previous period cost for the same service/operation
		previousCost := r.getPreviousPeriodCost(ctx, costRepo, driver.Service, driver.Operation, previousPeriodStart, previousPeriodEnd)

		// Calculate trend
		driver.Trend = r.calculateTrend(currentCost, previousCost)
	}

	return drivers
}

// getPreviousPeriodCost retrieves the cost for a specific service/operation in the previous period
func (r *queryResolver) getPreviousPeriodCost(ctx context.Context, costRepo *repositories.TrackingRepository, service, operation string, start, end time.Time) float64 {
	// Get cost records for the previous period
	costRecords, err := costRepo.GetCostsByDateRange(ctx, start, end)
	if err != nil {
		r.Logger.Warn("failed to get previous period costs for trend calculation",
			zap.String("service", service),
			zap.String("operation", operation),
			zap.Error(err))
		return 0.0
	}

	var totalCost float64
	for _, record := range costRecords {
		// Match service (if not "All")
		if service != allOperationsValue && record.ServiceName != service {
			continue
		}

		// Match operation (if not "All")
		if operation != allOperationsValue && record.OperationType != operation {
			continue
		}

		// Special handling for DynamoDB operations
		if service == "DynamoDB" {
			if operation == "Read" && (record.OperationType == "Query" || record.OperationType == "GetItem" || record.OperationType == "Scan") {
				totalCost += record.EstimatedCostDollars
			} else if operation == "Write" && (record.OperationType == "PutItem" || record.OperationType == "UpdateItem" || record.OperationType == "DeleteItem") {
				totalCost += record.EstimatedCostDollars
			}
		} else {
			totalCost += record.EstimatedCostDollars
		}
	}

	return totalCost
}

// calculateTrend determines trend classification based on cost comparison
func (r *queryResolver) calculateTrend(currentCost, previousCost float64) string {
	// If no previous data, return stable
	if previousCost == 0 {
		return "STABLE"
	}

	// Calculate percentage change
	changePercent := ((currentCost - previousCost) / previousCost) * 100

	// Classify trend based on thresholds
	if changePercent > 10 {
		return "INCREASING"
	} else if changePercent < -10 {
		return "DECREASING"
	}

	return "STABLE"
}

// generateAIExplanation creates an AI-powered explanation of the object
func (r *queryResolver) generateAIExplanation(ctx context.Context, aiSvc *ai.Service, objectID string, modelObject *model.Object, obj any) *model.ObjectExplanation {
	if result, err := aiSvc.GetAnalysis(ctx, &ai.GetAnalysisQuery{ObjectID: objectID}); err == nil && result.Analysis != nil {
		analysis := result.Analysis
		explanation := &model.ObjectExplanation{
			Object:          modelObject,
			StorageLocation: fmt.Sprintf("DynamoDB Table: main, PK: object#%s, SK: object#%s", objectID, objectID),
			SizeBytes:       r.calculateObjectSize(obj),
			StorageCost:     r.estimateStorageCost(obj),
			AccessPattern:   []*model.AccessLog{},
		}
		explanation.AccessPattern = append(explanation.AccessPattern, &model.AccessLog{
			Timestamp: model.Time(analysis.AnalyzedAt),
			Operation: "AI_Analysis",
			Cost:      5,
		})
		return explanation
	}

	if _, err := aiSvc.QueueForAnalysis(ctx, &ai.QueueAnalysisCommand{
		ObjectID:   objectID,
		ObjectType: string(modelObject.Type),
		Force:      false,
	}); err != nil {
		r.Logger.Warn("failed to queue object for AI analysis",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	return r.generateFallbackExplanation(objectID, modelObject, obj)
}

// generateFallbackExplanation creates a structural analysis when AI is unavailable
func (r *queryResolver) generateFallbackExplanation(objectID string, modelObject *model.Object, obj any) *model.ObjectExplanation {
	return &model.ObjectExplanation{
		Object:          modelObject,
		StorageLocation: fmt.Sprintf("DynamoDB Table: main, PK: object#%s, SK: object#%s", objectID, objectID),
		SizeBytes:       r.calculateObjectSize(obj),
		StorageCost:     r.estimateStorageCost(obj),
		AccessPattern:   []*model.AccessLog{},
	}
}

// enrichWithStorageAnalysis adds storage cost and access pattern information
func (r *queryResolver) enrichWithStorageAnalysis(ctx context.Context, explanation *model.ObjectExplanation, objectID string) {
	if costRepo := r.Registry.GetStorage().Cost(); costRepo != nil {
		if activityCost, err := costRepo.GetActivityCost(ctx, objectID); err == nil && activityCost != nil {
			explanation.StorageCost = float64(activityCost.TotalCostMicroCents) / 1_000_000.0
			explanation.AccessPattern = append(explanation.AccessPattern, &model.AccessLog{
				Timestamp: model.Time(activityCost.Timestamp),
				Operation: "GetItem",
				Cost:      int(activityCost.ReadCapacityUnits),
			})
		}
	}

	if err := common.ValidateSliceNotEmpty("access_pattern", explanation.AccessPattern); err != nil {
		explanation.AccessPattern = []*model.AccessLog{
			{
				Timestamp: model.Time(time.Now().Add(-time.Hour)),
				Operation: "GetItem",
				Cost:      1,
			},
			{
				Timestamp: model.Time(time.Now().Add(-30 * time.Minute)),
				Operation: "Query",
				Cost:      2,
			},
		}
	}
}

// calculateObjectSize estimates the storage size of an object in bytes
func (r *queryResolver) calculateObjectSize(obj any) int {
	switch o := obj.(type) {
	case *activitypub.Note:
		size := len(o.ID) + len(o.Content) + len(o.Type)
		if o.Summary != "" {
			size += len(o.Summary)
		}
		return size + 100
	case *activitypub.Article:
		size := len(o.ID) + len(o.Content) + len(o.Type)
		if o.Name != "" {
			size += len(o.Name)
		}
		return size + 100
	case map[string]interface{}:
		if jsonBytes, err := json.Marshal(obj); err == nil {
			return len(jsonBytes) + 100
		}
		return 500
	default:
		return 300
	}
}

// estimateStorageCost calculates the estimated monthly storage cost
func (r *queryResolver) estimateStorageCost(obj any) float64 {
	sizeBytes := float64(r.calculateObjectSize(obj))
	sizeGB := sizeBytes / (1024 * 1024 * 1024)
	return sizeGB * 0.25
}

func (r *Resolver) convertThreadContextResultToModel(_ context.Context, result *threads.ThreadContextResult) *model.ThreadContext {
	if result == nil {
		return nil
	}

	// Convert the root note to a GraphQL Object
	var rootNoteObj *model.Object
	if result.RootNote != nil {
		rootNoteObj = &model.Object{
			ID:        result.RootNote.ID,
			Type:      model.ObjectTypeNote,
			Content:   result.RootNote.Content,
			CreatedAt: model.Time(*result.RootNote.Published),
		}
	}

	syncStatus := model.SyncStatus("NONE")
	switch result.SyncStatus {
	case threads.SyncStatusComplete:
		syncStatus = model.SyncStatus("COMPLETE")
	case threads.SyncStatusPartial:
		syncStatus = model.SyncStatus("PARTIAL")
	case threads.SyncStatusFailed:
		syncStatus = model.SyncStatus("FAILED")
	case threads.SyncStatusSyncing:
		syncStatus = model.SyncStatus("SYNCING")
	}

	return &model.ThreadContext{
		RootNote:         rootNoteObj,
		ReplyCount:       result.ReplyCount,
		ParticipantCount: result.ParticipantCount,
		MissingPosts:     result.MissingCount,
		LastActivity:     model.Time(result.LastActivity),
		SyncStatus:       syncStatus,
	}
}

// ====================================================================
// HASHTAG HELPERS
// ====================================================================

// convertHashtagToModel converts service Hashtag to GraphQL model.Hashtag
// This is THE converter used by all resolvers - consistency is critical
func (r *Resolver) convertHashtagToModel(ctx context.Context, h *hashtags.Hashtag, viewerID string) *model.Hashtag {
	if h == nil {
		return nil
	}

	domain := r.getDomain()
	url := r.buildHashtagURL(h, domain)
	settings := r.convertHashtagNotificationSettingsFromService(ctx, h, viewerID)
	relatedHashtags := r.convertRelatedHashtags(h.Related, domain)

	result := &model.Hashtag{
		Name:                 h.Name,
		URL:                  url,
		DisplayName:          "#" + h.Name,
		PostCount:            h.PostCount,
		FollowerCount:        h.FollowerCount,
		TrendingScore:        h.TrendingScore,
		IsFollowing:          h.IsFollowing,
		NotificationSettings: settings,
		RelatedHashtags:      relatedHashtags,
	}

	if h.FollowedAt != nil {
		t := model.Time(*h.FollowedAt)
		result.FollowedAt = &t
	}

	return result
}

// getDomain extracts the domain from the registry config
func (r *Resolver) getDomain() string {
	domain := "localhost"
	if r.Registry != nil && r.Registry.GetConfig() != nil && r.Registry.GetConfig().BaseURL != "" {
		baseURL := r.Registry.GetConfig().BaseURL
		if strings.HasPrefix(baseURL, "https://") {
			domain = strings.TrimPrefix(baseURL, "https://")
		} else if strings.HasPrefix(baseURL, "http://") {
			domain = strings.TrimPrefix(baseURL, "http://")
		} else {
			domain = baseURL
		}
		domain = strings.TrimSuffix(domain, "/")
	}
	return domain
}

// buildHashtagURL builds the URL for a hashtag
func (r *Resolver) buildHashtagURL(h *hashtags.Hashtag, domain string) string {
	if h.URL != "" {
		return h.URL
	}
	if h.Name != "" {
		return fmt.Sprintf("https://%s/tags/%s", domain, h.Name)
	}
	return ""
}

// convertHashtagNotificationSettingsFromService converts service notification settings
func (r *Resolver) convertHashtagNotificationSettingsFromService(ctx context.Context, h *hashtags.Hashtag, viewerID string) *model.HashtagNotificationSettings {
	if h.NotificationSettings != nil {
		return r.convertStorageNotificationSettings(h.NotificationSettings)
	}
	if viewerID != "" {
		return r.fetchHashtagNotificationSettings(ctx, h.Name, viewerID)
	}
	return nil
}

// convertStorageNotificationSettings converts storage notification settings to GraphQL model
func (r *Resolver) convertStorageNotificationSettings(settings *storage.HashtagNotificationSettings) *model.HashtagNotificationSettings {
	level := model.NotificationLevelAll
	levelStr := strings.ToLower(settings.Level)
	if levelStr == common.RelationshipFollowing || levelStr == "mutuals" {
		level = model.NotificationLevelFollowing
	}

	result := &model.HashtagNotificationSettings{
		Level:   level,
		Muted:   settings.Muted,
		Filters: r.convertNotificationFilters(settings.Filters),
	}

	if settings.MutedUntil != nil && !settings.MutedUntil.IsZero() {
		t := model.Time(*settings.MutedUntil)
		result.MutedUntil = &t
	}

	return result
}

// convertNotificationFilters converts storage filters to GraphQL model
func (r *Resolver) convertNotificationFilters(filters []*storage.NotificationFilter) []*model.NotificationFilter {
	if len(filters) == 0 {
		return []*model.NotificationFilter{}
	}

	result := make([]*model.NotificationFilter, 0, len(filters))
	for _, filter := range filters {
		if filter != nil {
			result = append(result, &model.NotificationFilter{
				Type:  strings.Join(filter.Types, ","),
				Value: strings.Join(filter.ExcludeTypes, ","),
			})
		}
	}
	return result
}

// convertRelatedHashtags converts related hashtag names to GraphQL models
func (r *Resolver) convertRelatedHashtags(related []string, domain string) []*model.Hashtag {
	if len(related) == 0 {
		return nil
	}

	relatedHashtags := make([]*model.Hashtag, 0, len(related))
	for _, relTag := range related {
		if relTag != "" {
			relatedHashtags = append(relatedHashtags, &model.Hashtag{
				Name:        relTag,
				URL:         fmt.Sprintf("https://%s/tags/%s", domain, relTag),
				DisplayName: "#" + relTag,
			})
		}
	}
	return relatedHashtags
}

// fetchHashtagNotificationSettings retrieves notification settings from storage
func (r *Resolver) fetchHashtagNotificationSettings(ctx context.Context, hashtag, userID string) *model.HashtagNotificationSettings {
	defaultSettings := &model.HashtagNotificationSettings{
		Level:   model.NotificationLevelNone,
		Muted:   false,
		Filters: []*model.NotificationFilter{},
	}

	if hashtag == "" || userID == "" {
		return defaultSettings
	}

	if r.Storage == nil {
		return defaultSettings
	}

	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		return defaultSettings
	}

	settings, err := hashtagRepo.GetHashtagNotificationSettings(ctx, userID, hashtag)
	if err != nil || settings == nil {
		return defaultSettings
	}

	return r.convertStorageNotificationSettings(settings)
}

// isFollowingHashtag checks if the user is following a hashtag
//
//nolint:unused // Used by tests and future features
func (r *Resolver) isFollowingHashtag(ctx context.Context, userID, hashtag string) bool {
	if userID == "" || hashtag == "" {
		return false
	}

	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		return false
	}

	following, err := hashtagRepo.IsFollowingHashtag(ctx, userID, hashtag)
	if err != nil {
		return false
	}

	return following
}

// isHashtagMuted checks if the user has muted a hashtag
//
//nolint:unused // Used by tests and future features
func (r *Resolver) isHashtagMuted(ctx context.Context, userID, hashtag string) bool {
	if userID == "" || hashtag == "" {
		return false
	}

	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		return false
	}

	muted, err := hashtagRepo.IsHashtagMuted(ctx, userID, hashtag)
	if err != nil {
		return false
	}

	return muted
}

// convertSeveredRelationshipToModel converts service SeveredRelationship to GraphQL model
func (r *Resolver) convertSeveredRelationshipToModel(_ context.Context, sev *severance.SeveredRelationship) *model.SeveredRelationship {
	if sev == nil {
		return nil
	}

	// Convert reason to GraphQL enum
	var reason model.SeveranceReason
	switch sev.Reason {
	case models.SeveranceReasonDomainBlock:
		reason = model.SeveranceReasonDomainBlock
	case models.SeveranceReasonInstanceDown:
		reason = model.SeveranceReasonInstanceDown
	case models.SeveranceReasonDefederation:
		reason = model.SeveranceReasonDefederation
	case models.SeveranceReasonPolicyViolation:
		reason = model.SeveranceReasonPolicyViolation
	default:
		reason = model.SeveranceReasonOther
	}

	result := &model.SeveredRelationship{
		ID:                sev.ID,
		LocalInstance:     sev.LocalInstance,
		RemoteInstance:    sev.RemoteInstance,
		Reason:            reason,
		AffectedFollowers: sev.AffectedFollowers,
		AffectedFollowing: sev.AffectedFollowing,
		Timestamp:         model.Time(sev.DetectedAt),
		Reversible:        sev.Reversible,
	}

	// Add optional details
	if sev.Details != "" || sev.AdminNotes != "" {
		description := sev.Details
		if sev.AdminNotes != "" {
			if description != "" {
				description += "\n"
			}
			description += sev.AdminNotes
		}
		result.Details = &model.SeveranceDetails{
			Description:  description,
			Metadata:     []string{},
			AutoDetected: sev.AutoDetected,
		}
		if sev.AdminNotes != "" {
			result.Details.AdminNotes = &sev.AdminNotes
		}
	}

	return result
}

// convertAffectedRelationshipToModel converts service AffectedRelationship to GraphQL model
func (r *Resolver) convertAffectedRelationshipToModel(ctx context.Context, aff *severance.AffectedRelationship) *model.AffectedRelationship {
	if aff == nil {
		return nil
	}

	// Try to fetch the full actor from storage
	var actor *activitypub.Actor
	if r.Registry != nil {
		storage := r.Registry.GetStorage()
		if storage != nil {
			actorRepo := storage.Actor()
			if actorRepo != nil && aff.ActorID != "" {
				fetchedActor, err := actorRepo.GetActor(ctx, aff.ActorID)
				if err == nil {
					actor = fetchedActor
				} else {
					// If we can't fetch the actor, construct a minimal one from available fields
					r.Logger.Debug("failed to fetch actor for affected relationship, using minimal actor",
						zap.String("actor_id", aff.ActorID),
						zap.Error(err))
					actor = r.constructMinimalActor(aff.ActorID, aff.ActorHandle, aff.ActorDomain)
				}
			}
		}
	}

	// If we still don't have an actor, construct a minimal one
	if actor == nil {
		actor = r.constructMinimalActor(aff.ActorID, aff.ActorHandle, aff.ActorDomain)
	}

	result := &model.AffectedRelationship{
		Actor:            actor,
		RelationshipType: aff.RelationshipType,
		EstablishedAt:    model.Time(aff.EstablishedAt),
	}

	if aff.LastInteraction != nil {
		t := model.Time(*aff.LastInteraction)
		result.LastInteraction = &t
	}

	return result
}

// constructMinimalActor creates a minimal Actor from available fields
func (r *Resolver) constructMinimalActor(actorID, actorHandle, actorDomain string) *activitypub.Actor {
	// Parse handle to extract username if needed
	username := actorID
	if username == "" && actorHandle != "" {
		// Handle is typically @user@domain.com, extract user part
		username = strings.TrimPrefix(actorHandle, "@")
		if idx := strings.Index(username, "@"); idx != -1 {
			username = username[:idx]
		}
	}

	// Construct a minimal actor with available information
	actorURL := fmt.Sprintf("https://%s/@%s", actorDomain, username)
	if actorDomain == "" {
		actorDomain = "unknown.domain"
		actorURL = fmt.Sprintf("https://%s/users/%s", actorDomain, username)
	}

	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorURL,
			Type: activitypub.PersonType,
		},
		PreferredUsername: username,
		Name:              actorHandle,
		Inbox:             actorURL + "/inbox",
		Outbox:            actorURL + "/outbox",
	}
}
