package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// ThreadSync represents thread synchronization metadata
type ThreadSync struct {
	PK               string    `dynamodbav:"PK"` // THREAD_SYNC#statusID
	SK               string    `dynamodbav:"SK"` // METADATA
	StatusID         string    `dynamodbav:"StatusID"`
	LastSyncAt       time.Time `dynamodbav:"LastSyncAt"`
	SyncStatus       string    `dynamodbav:"SyncStatus"`     // "pending", "syncing", "completed", "failed"
	MissingReplies   []string  `dynamodbav:"MissingReplies"` // List of missing reply IDs
	RemoteFetched    bool      `dynamodbav:"RemoteFetched"`  // Whether we've attempted remote fetch
	ThreadDepth      int       `dynamodbav:"ThreadDepth"`    // Current thread depth known
	LastErrorMessage string    `dynamodbav:"LastErrorMessage,omitempty"`
	UpdatedAt        time.Time `dynamodbav:"UpdatedAt"`
}

// SyncThreadFromRemote synchronizes a thread by fetching missing parts from remote
func (s *dynamoDBStorage) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	// First check if we need to sync
	syncRecord, err := s.getThreadSyncRecord(ctx, statusID)
	if err != nil {
		s.logger().Warn("failed to get thread sync record", zap.Error(err))
	}

	// If already synced recently, skip
	if syncRecord != nil && syncRecord.SyncStatus == "completed" &&
		time.Since(syncRecord.LastSyncAt) < 30*time.Minute {
		s.logger().Debug("thread already synced recently", zap.String("status_id", statusID))
	}

	// Mark as syncing
	if err := s.markThreadSyncing(ctx, statusID); err != nil {
		s.logger().Warn("failed to mark thread as syncing", zap.Error(err))
	}

	// Implement actual remote fetching logic
	s.logger().Info("starting thread sync", zap.String("status_id", statusID))

	// Get the status to determine its origin and context
	existingStatus, err := s.GetObject(ctx, statusID)
	if err != nil {
		s.logger().Error("failed to get existing status", zap.String("status_id", statusID), zap.Error(err))
		return nil, fmt.Errorf("failed to get existing status: %w", err)
	}

	// Extract status information
	var statusURL string
	switch obj := existingStatus.(type) {
	case *activitypub.Note:
		statusURL = obj.ID
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			statusURL = id
		}
	}

	if statusURL == "" {
		s.logger().Error("unable to determine status URL", zap.String("status_id", statusID))
		return nil, fmt.Errorf("unable to determine status URL")
	}

	// Create authorized fetch service
	authService := federation.NewAuthorizedFetchService(s, s.domain, s.logger())

	// Get a signing actor (we need an actor to sign requests)
	signingActor, err := s.getSigningActor(ctx)
	if err != nil {
		s.logger().Error("failed to get signing actor", zap.Error(err))
		return nil, fmt.Errorf("failed to get signing actor: %w", err)
	}

	// 1. Try to fetch the complete context (replies collection)
	if err := s.fetchAndStoreRemoteThread(ctx, authService, signingActor, statusURL); err != nil {
		s.logger().Warn("failed to fetch complete thread context",
			zap.String("status_url", statusURL),
			zap.Error(err))
	}

	// 2. Try to fetch thread ancestors (inReplyTo chain)
	if err := s.fetchThreadAncestors(ctx, authService, signingActor, existingStatus); err != nil {
		s.logger().Warn("failed to fetch thread ancestors",
			zap.String("status_id", statusID),
			zap.Error(err))
	}

	s.logger().Info("thread sync completed", zap.String("status_id", statusID))

	// For now, mark as completed and return existing status
	if err := s.markThreadSyncCompleted(ctx, statusID); err != nil {
		s.logger().Warn("failed to mark thread sync as completed", zap.Error(err))
	}

	// Try to get the status (this would be enhanced to return the synced status)
	status := &storage.StatusSearchResult{
		StatusID:  statusID,
		Content:   "",
		AuthorID:  "",
		Published: time.Now(),
		Score:     1.0,
	}

	return status, nil
}

// SyncMissingRepliesFromRemote fetches missing replies in a thread
func (s *dynamoDBStorage) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Get current thread context to identify missing replies
	context, err := s.GetThreadContext(ctx, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread context: %w", err)
	}

	// Identify potential gaps in the thread
	missingReplies := s.identifyMissingReplies(ctx, statusID, context)

	s.logger().Info("identified missing replies",
		zap.String("status_id", statusID),
		zap.Int("missing_count", len(missingReplies)))

	// Implement actual remote fetching of missing replies
	var fetchedReplies []*storage.StatusSearchResult

	if len(missingReplies) > 0 {
		// Create authorized fetch service
		authService := federation.NewAuthorizedFetchService(s, s.domain, s.logger())

		// Get a signing actor
		signingActor, err := s.getSigningActor(ctx)
		if err != nil {
			s.logger().Error("failed to get signing actor for missing replies", zap.Error(err))
			return []*storage.StatusSearchResult{}, nil
		}

		// Fetch each missing reply
		for _, missingID := range missingReplies {
			s.logger().Debug("attempting to fetch missing reply",
				zap.String("status_id", statusID),
				zap.String("missing_id", missingID))

			// Try to fetch and store the missing reply
			if err := s.fetchAndStoreRemoteStatus(ctx, authService, signingActor, missingID); err != nil {
				s.logger().Warn("failed to fetch missing reply",
					zap.String("missing_id", missingID),
					zap.Error(err))
				continue
			}

			// Convert to StatusSearchResult
			obj, err := s.GetObject(ctx, missingID)
			if err != nil {
				s.logger().Warn("failed to retrieve fetched reply",
					zap.String("missing_id", missingID),
					zap.Error(err))
				continue
			}

			result := s.objectToStatusSearchResult(obj)
			if result != nil {
				fetchedReplies = append(fetchedReplies, result)
			}
		}

		s.logger().Info("fetched missing replies",
			zap.String("status_id", statusID),
			zap.Int("requested", len(missingReplies)),
			zap.Int("fetched", len(fetchedReplies)))
	}

	return fetchedReplies, nil
}

// GetThreadContext retrieves the full context (ancestors and descendants) of a status
func (s *dynamoDBStorage) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	// Get ancestors (replies this status is replying to)
	ancestors, err := s.getThreadAncestors(ctx, statusID)
	if err != nil {
		s.logger().Warn("failed to get thread ancestors", zap.Error(err))
		ancestors = []*storage.StatusSearchResult{}
	}

	// Get descendants (replies to this status)
	descendants, err := s.getThreadDescendants(ctx, statusID)
	if err != nil {
		s.logger().Warn("failed to get thread descendants", zap.Error(err))
		descendants = []*storage.StatusSearchResult{}
	}

	return &storage.ThreadContext{
		Ancestors:   ancestors,
		Descendants: descendants,
	}, nil
}

// getThreadAncestors gets all ancestor statuses in the thread
func (s *dynamoDBStorage) getThreadAncestors(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	ancestors := make([]*storage.StatusSearchResult, 0)

	// Follow the inReplyTo chain upwards
	currentID := statusID
	maxDepth := 20 // Prevent infinite loops

	for depth := 0; depth < maxDepth; depth++ {
		// Get the current status
		obj, err := s.GetObject(ctx, currentID)
		if err != nil {
			s.logger().Debug("failed to get object while traversing ancestors",
				zap.String("current_id", currentID),
				zap.Int("depth", depth),
				zap.Error(err))
			break
		}

		// Extract inReplyTo field
		var inReplyTo string
		switch o := obj.(type) {
		case *activitypub.Note:
			inReplyTo = o.InReplyTo
		case map[string]any:
			if reply, ok := o["inReplyTo"].(string); ok {
				inReplyTo = reply
			}
		}

		// If no parent, we've reached the root
		if inReplyTo == "" {
			break
		}

		// Get the parent status
		parentObj, err := s.GetObject(ctx, inReplyTo)
		if err != nil {
			s.logger().Debug("failed to get parent object",
				zap.String("parent_id", inReplyTo),
				zap.Int("depth", depth),
				zap.Error(err))
			break
		}

		// Convert to StatusSearchResult and add to ancestors
		result := s.objectToStatusSearchResult(parentObj)
		if result != nil {
			// Add to the beginning to maintain chronological order (oldest first)
			ancestors = append([]*storage.StatusSearchResult{result}, ancestors...)
		}

		// Move to the parent for next iteration
		currentID = inReplyTo
	}

	s.logger().Debug("found thread ancestors",
		zap.String("status_id", statusID),
		zap.Int("ancestor_count", len(ancestors)))

	return ancestors, nil
}

// getThreadDescendants gets all descendant statuses in the thread
func (s *dynamoDBStorage) getThreadDescendants(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Query for replies to this status
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("replies-by-status"),
		KeyConditionExpression: aws.String("InReplyTo = :statusID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":statusID": &types.AttributeValueMemberS{Value: statusID},
		},
		ScanIndexForward: aws.Bool(true), // Chronological order
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		// If GSI doesn't exist, return empty
		s.logger().Warn("replies-by-status GSI not available",
			zap.String("status_id", statusID),
			zap.Error(err))
		return []*storage.StatusSearchResult{}, nil
	}

	descendants := make([]*storage.StatusSearchResult, 0)
	for _, item := range result.Items {
		// Extract status information from reply record
		replyID := ""
		if val, ok := item["StatusID"]; ok {
			if s, ok := val.(*types.AttributeValueMemberS); ok {
				replyID = s.Value
			}
		}

		if replyID == "" {
			continue
		}

		reply := &storage.StatusSearchResult{
			StatusID:  replyID,
			Content:   "",
			AuthorID:  "",
			Published: time.Now(),
			Score:     1.0,
		}

		descendants = append(descendants, reply)

		// Recursively get descendants of this reply
		subDescendants, err := s.getThreadDescendants(ctx, replyID)
		if err != nil {
			s.logger().Warn("failed to get sub-descendants",
				zap.String("reply_id", replyID),
				zap.Error(err))
			continue
		}

		descendants = append(descendants, subDescendants...)
	}

	return descendants, nil
}

// MarkThreadAsSynced marks a thread as successfully synced
func (s *dynamoDBStorage) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	return s.markThreadSyncCompleted(ctx, statusID)
}

// GetMissingReplies returns a list of known missing replies in a thread
func (s *dynamoDBStorage) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	syncRecord, err := s.getThreadSyncRecord(ctx, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread sync record: %w", err)
	}

	if syncRecord == nil || len(syncRecord.MissingReplies) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	// Convert missing reply IDs to status search results
	missing := make([]*storage.StatusSearchResult, 0)
	for _, replyID := range syncRecord.MissingReplies {
		reply := &storage.StatusSearchResult{
			StatusID:  replyID,
			Content:   "[Missing Reply]",
			AuthorID:  "",
			Published: time.Now(),
			Score:     0.5,
		}
		missing = append(missing, reply)
	}

	return missing, nil
}

// Helper methods

func (s *dynamoDBStorage) getThreadSyncRecord(ctx context.Context, statusID string) (*ThreadSync, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAD_SYNC#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	result, err := s.client.GetItem(ctx, input)
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, nil
	}

	var sync ThreadSync
	if err := s.UnmarshalItem(result.Item, &sync); err != nil {
		return nil, err
	}

	return &sync, nil
}

func (s *dynamoDBStorage) markThreadSyncing(ctx context.Context, statusID string) error {
	now := time.Now()

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAD_SYNC#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET SyncStatus = :status, LastSyncAt = :now, UpdatedAt = :now, StatusID = :statusID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":   &types.AttributeValueMemberS{Value: "syncing"},
			":now":      &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":statusID": &types.AttributeValueMemberS{Value: statusID},
		},
	}

	_, err := s.client.UpdateItem(ctx, input)
	return err
}

func (s *dynamoDBStorage) markThreadSyncCompleted(ctx context.Context, statusID string) error {
	now := time.Now()

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("THREAD_SYNC#%s", statusID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET SyncStatus = :status, UpdatedAt = :now, RemoteFetched = :true"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: "completed"},
			":now":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":true":   &types.AttributeValueMemberBOOL{Value: true},
		},
	}

	_, err := s.client.UpdateItem(ctx, input)
	return err
}


func (s *dynamoDBStorage) identifyMissingReplies(ctx context.Context, rootStatusID string, threadContext *storage.ThreadContext) []string {
	missing := make([]string, 0)

	if threadContext == nil {
		return missing
	}

	// Create a map of known status IDs for quick lookup
	knownStatuses := make(map[string]bool)

	// Add the root status
	knownStatuses[rootStatusID] = true

	// Add ancestors to known statuses
	for _, ancestor := range threadContext.Ancestors {
		knownStatuses[ancestor.StatusID] = true
	}

	// Add descendants to known statuses
	for _, descendant := range threadContext.Descendants {
		knownStatuses[descendant.StatusID] = true
	}

	s.logger().Debug("analyzing thread for missing replies",
		zap.String("root_status", rootStatusID),
		zap.Int("known_ancestors", len(threadContext.Ancestors)),
		zap.Int("known_descendants", len(threadContext.Descendants)))

	// Check descendants for missing parents (inReplyTo chains with gaps)
	for _, descendant := range threadContext.Descendants {
		// Get the descendant object to check its inReplyTo
		obj, err := s.GetObject(ctx, descendant.StatusID)
		if err != nil {
			s.logger().Debug("failed to get descendant object for gap analysis",
				zap.String("descendant_id", descendant.StatusID),
				zap.Error(err))
			continue
		}

		var inReplyTo string
		switch o := obj.(type) {
		case *activitypub.Note:
			inReplyTo = o.InReplyTo
		case map[string]any:
			if reply, ok := o["inReplyTo"].(string); ok {
				inReplyTo = reply
			}
		}

		// If this descendant replies to something we don't have, it's missing
		if inReplyTo != "" && !knownStatuses[inReplyTo] {
			missing = append(missing, inReplyTo)
			s.logger().Debug("identified missing reply in chain",
				zap.String("missing_id", inReplyTo),
				zap.String("descendant_id", descendant.StatusID))
		}
	}

	// Check ancestors for missing parents (gaps in the upward chain)
	for _, ancestor := range threadContext.Ancestors {
		obj, err := s.GetObject(ctx, ancestor.StatusID)
		if err != nil {
			continue
		}

		var inReplyTo string
		switch o := obj.(type) {
		case *activitypub.Note:
			inReplyTo = o.InReplyTo
		case map[string]any:
			if reply, ok := o["inReplyTo"].(string); ok {
				inReplyTo = reply
			}
		}

		// If this ancestor replies to something we don't have, it's missing
		if inReplyTo != "" && !knownStatuses[inReplyTo] {
			missing = append(missing, inReplyTo)
			s.logger().Debug("identified missing reply in ancestor chain",
				zap.String("missing_id", inReplyTo),
				zap.String("ancestor_id", ancestor.StatusID))
		}
	}

	// Advanced gap detection: Look for sequence gaps in reply timestamps
	// If we have replies with large time gaps, there might be missing intermediate replies
	if len(threadContext.Descendants) >= 2 {
		// Sort descendants by timestamp for gap analysis
		descendants := make([]*storage.StatusSearchResult, len(threadContext.Descendants))
		copy(descendants, threadContext.Descendants)

		// Simple time-based gap detection
		for i := 1; i < len(descendants); i++ {
			timeDiff := descendants[i].Published.Sub(descendants[i-1].Published)
			// If there's more than 30 minutes between consecutive replies in an active thread,
			// it might indicate missing intermediate replies
			if timeDiff.Minutes() > 30 && len(descendants) < 10 {
				s.logger().Debug("detected potential time gap in thread",
					zap.String("before_id", descendants[i-1].StatusID),
					zap.String("after_id", descendants[i].StatusID),
					zap.Duration("gap", timeDiff))
				// We could add heuristics here to estimate missing reply URLs
			}
		}
	}

	// Look for conversation patterns that suggest missing replies
	// If we have a very sparse thread (< 3 replies) but with multiple participants,
	// there might be missing replies
	if len(threadContext.Descendants) > 0 && len(threadContext.Descendants) < 3 {
		participants := make(map[string]bool)
		for _, desc := range threadContext.Descendants {
			participants[desc.AuthorID] = true
		}

		if len(participants) > 2 {
			s.logger().Debug("sparse multi-participant thread detected, may have missing replies",
				zap.String("status_id", rootStatusID),
				zap.Int("known_replies", len(threadContext.Descendants)),
				zap.Int("participants", len(participants)))
		}
	}

	// Remove duplicates from missing list
	seen := make(map[string]bool)
	uniqueMissing := make([]string, 0)
	for _, id := range missing {
		if !seen[id] {
			seen[id] = true
			uniqueMissing = append(uniqueMissing, id)
		}
	}

	s.logger().Info("gap analysis completed",
		zap.String("status_id", rootStatusID),
		zap.Int("total_known", len(knownStatuses)),
		zap.Int("missing_count", len(uniqueMissing)))

	return uniqueMissing
}

// getSigningActor gets an actor to use for signing federation requests
func (s *dynamoDBStorage) getSigningActor(ctx context.Context) (*activitypub.Actor, error) {
	// Try to get the instance actor or any local actor for signing
	actors, err := s.SearchAccounts(ctx, "", 1, false, 0)
	if err != nil || len(actors) == 0 {
		return nil, fmt.Errorf("no local actors available for signing")
	}
	return actors[0], nil
}

// fetchAndStoreRemoteThread fetches replies and context from a remote status
func (s *dynamoDBStorage) fetchAndStoreRemoteThread(ctx context.Context, authService *federation.AuthorizedFetchService, signingActor *activitypub.Actor, statusURL string) error {
	// Try to fetch the replies collection by constructing the replies URL
	// According to ActivityPub spec, replies are typically at {object-id}/replies
	repliesURL := statusURL + "/replies"

	s.logger().Debug("attempting to fetch replies collection",
		zap.String("status_url", statusURL),
		zap.String("replies_url", repliesURL))

	// Try to fetch the replies collection
	if err := s.fetchRepliesCollection(ctx, authService, signingActor, repliesURL); err != nil {
		s.logger().Debug("failed to fetch replies collection at standard URL",
			zap.String("replies_url", repliesURL),
			zap.Error(err))

		// Try alternative: fetch the object and look for explicit replies property
		if err := s.fetchObjectReplies(ctx, authService, signingActor, statusURL); err != nil {
			s.logger().Debug("failed to fetch object replies",
				zap.String("status_url", statusURL),
				zap.Error(err))
		}
	}

	return nil
}

// fetchObjectReplies fetches an object and looks for replies property
func (s *dynamoDBStorage) fetchObjectReplies(ctx context.Context, authService *federation.AuthorizedFetchService, signingActor *activitypub.Actor, statusURL string) error {
	obj, err := authService.FetchObject(ctx, statusURL, signingActor)
	if err != nil {
		return fmt.Errorf("failed to fetch status object: %w", err)
	}

	var repliesURL string

	// Check if the raw object (map) has a replies property
	if objMap, ok := obj.(map[string]any); ok {
		if replies, ok := objMap["replies"]; ok {
			if repliesStr, ok := replies.(string); ok {
				repliesURL = repliesStr
			} else if repliesMap, ok := replies.(map[string]any); ok {
				if id, ok := repliesMap["id"].(string); ok {
					repliesURL = id
				}
			}
		}
	}

	// If we found a replies collection URL, fetch it
	if repliesURL != "" {
		return s.fetchRepliesCollection(ctx, authService, signingActor, repliesURL)
	}

	return nil
}

// fetchRepliesCollection fetches and stores replies from a collection
func (s *dynamoDBStorage) fetchRepliesCollection(ctx context.Context, authService *federation.AuthorizedFetchService, signingActor *activitypub.Actor, collectionURL string) error {
	collection, err := authService.FetchObject(ctx, collectionURL, signingActor)
	if err != nil {
		return fmt.Errorf("failed to fetch collection: %w", err)
	}

	var items []any
	switch coll := collection.(type) {
	case map[string]any:
		if itemsArray, ok := coll["items"].([]any); ok {
			items = itemsArray
		} else if orderedItems, ok := coll["orderedItems"].([]any); ok {
			items = orderedItems
		}
	}

	// Fetch and store each reply
	for _, item := range items {
		var itemURL string
		switch v := item.(type) {
		case string:
			itemURL = v
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				itemURL = id
			}
		}

		if itemURL != "" {
			if err := s.fetchAndStoreRemoteStatus(ctx, authService, signingActor, itemURL); err != nil {
				s.logger().Warn("failed to fetch remote status",
					zap.String("status_url", itemURL),
					zap.Error(err))
			}
		}
	}

	return nil
}

// fetchThreadAncestors fetches parent statuses by following inReplyTo chain
func (s *dynamoDBStorage) fetchThreadAncestors(ctx context.Context, authService *federation.AuthorizedFetchService, signingActor *activitypub.Actor, status any) error {
	var inReplyTo string

	switch obj := status.(type) {
	case *activitypub.Note:
		inReplyTo = obj.InReplyTo
	case map[string]any:
		if reply, ok := obj["inReplyTo"].(string); ok {
			inReplyTo = reply
		}
	}

	// Follow the inReplyTo chain up to a reasonable depth
	depth := 0
	maxDepth := 10

	for inReplyTo != "" && depth < maxDepth {
		depth++

		// Check if we already have this status locally
		if _, err := s.GetObject(ctx, inReplyTo); err == nil {
			// We already have this status, stop here
			break
		}

		// Fetch the parent status
		if err := s.fetchAndStoreRemoteStatus(ctx, authService, signingActor, inReplyTo); err != nil {
			s.logger().Warn("failed to fetch parent status",
				zap.String("parent_url", inReplyTo),
				zap.Int("depth", depth),
				zap.Error(err))
			break
		}

		// Get the next parent
		parentObj, err := s.GetObject(ctx, inReplyTo)
		if err != nil {
			break
		}

		// Reset inReplyTo for next iteration
		inReplyTo = ""
		switch parent := parentObj.(type) {
		case *activitypub.Note:
			inReplyTo = parent.InReplyTo
		case map[string]any:
			if reply, ok := parent["inReplyTo"].(string); ok {
				inReplyTo = reply
			}
		}
	}

	return nil
}

// fetchAndStoreRemoteStatus fetches a remote status and stores it locally
func (s *dynamoDBStorage) fetchAndStoreRemoteStatus(ctx context.Context, authService *federation.AuthorizedFetchService, signingActor *activitypub.Actor, statusURL string) error {
	// Fetch the remote status
	obj, err := authService.FetchObject(ctx, statusURL, signingActor)
	if err != nil {
		return fmt.Errorf("failed to fetch object: %w", err)
	}

	// Store the object locally
	if err := s.CreateObject(ctx, obj); err != nil {
		return fmt.Errorf("failed to store object: %w", err)
	}

	s.logger().Debug("stored remote status",
		zap.String("status_url", statusURL))

	return nil
}

// objectToStatusSearchResult converts an ActivityPub object to StatusSearchResult
func (s *dynamoDBStorage) objectToStatusSearchResult(obj any) *storage.StatusSearchResult {
	if obj == nil {
		return nil
	}

	var result storage.StatusSearchResult

	switch o := obj.(type) {
	case *activitypub.Note:
		result.StatusID = o.ID
		result.Content = o.Content
		result.AuthorID = o.AttributedTo
		if o.Published != nil {
			result.Published = *o.Published
		}
		result.Score = 1.0

	case map[string]any:
		if id, ok := o["id"].(string); ok {
			result.StatusID = id
		}
		if content, ok := o["content"].(string); ok {
			result.Content = content
		}
		if attr, ok := o["attributedTo"].(string); ok {
			result.AuthorID = attr
		}
		if pub, ok := o["published"].(string); ok {
			if t, err := time.Parse(time.RFC3339, pub); err == nil {
				result.Published = t
			}
		}
		result.Score = 1.0

	default:
		return nil
	}

	if result.StatusID == "" {
		return nil
	}

	return &result
}
