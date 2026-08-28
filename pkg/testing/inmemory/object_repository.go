// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

const quoteTypeDisabled = "disabled"

// ObjectRepository is a thread-safe in-memory implementation of interfaces.ObjectRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type ObjectRepository struct {
	mu sync.RWMutex

	// Core object storage
	objects map[string]*objectEntry // keyed by object ID

	// Objects by actor index
	objectsByActor map[string][]string // actorID -> []objectID

	// Reply relationships
	repliesByParent map[string][]string // parentID -> []replyID
	replyCounts     map[string]int      // objectID -> reply count

	// Tombstones
	tombstones       map[string]*models.Tombstone // objectID -> tombstone
	tombstonesByType map[string][]string          // formerType -> []objectID

	// Update history
	updateHistory map[string][]*storage.UpdateHistory // objectID -> []history

	// Collections
	collections map[string]map[string]*storage.CollectionItem // collectionID -> itemID -> item

	// Quote relationships
	quotes          map[string]*storage.QuoteRelationship // quoteID -> quote
	quotesByTarget  map[string][]string                   // targetNoteID -> []quoteID
	quotesByQuoter  map[string][]string                   // quoterNoteID -> []quoteID
	quotesByActor   map[string]map[string]string          // actorID -> noteID -> quoteID
	quoteTypes      map[string]string                     // statusID -> quoteType
	withdrawnStatus map[string]bool                       // statusID -> withdrawn

	// Thread sync
	threadSynced   map[string]bool                   // statusID -> synced
	missingReplies map[string][]string               // statusID -> []missingReplyID
	threadContexts map[string]*storage.ThreadContext // statusID -> context
}

// objectEntry stores an object with its metadata
type objectEntry struct {
	object     any
	objectType string
	actorID    string
	inReplyTo  string
	createdAt  time.Time
	updatedAt  time.Time
}

// NewObjectRepository creates a new in-memory object repository
func NewObjectRepository() *ObjectRepository {
	return &ObjectRepository{
		objects:          make(map[string]*objectEntry),
		objectsByActor:   make(map[string][]string),
		repliesByParent:  make(map[string][]string),
		replyCounts:      make(map[string]int),
		tombstones:       make(map[string]*models.Tombstone),
		tombstonesByType: make(map[string][]string),
		updateHistory:    make(map[string][]*storage.UpdateHistory),
		collections:      make(map[string]map[string]*storage.CollectionItem),
		quotes:           make(map[string]*storage.QuoteRelationship),
		quotesByTarget:   make(map[string][]string),
		quotesByQuoter:   make(map[string][]string),
		quotesByActor:    make(map[string]map[string]string),
		quoteTypes:       make(map[string]string),
		withdrawnStatus:  make(map[string]bool),
		threadSynced:     make(map[string]bool),
		missingReplies:   make(map[string][]string),
		threadContexts:   make(map[string]*storage.ThreadContext),
	}
}

// ===== Core Object Operations =====

// CreateObject stores a generic ActivityPub object
func (r *ObjectRepository) CreateObject(_ context.Context, object any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract object ID and metadata from the object
	objectID, objectType, actorID, inReplyTo := extractObjectMetadata(object)
	if objectID == "" {
		return fmt.Errorf("object ID is required")
	}

	if _, exists := r.objects[objectID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	entry := &objectEntry{
		object:     object,
		objectType: objectType,
		actorID:    actorID,
		inReplyTo:  inReplyTo,
		createdAt:  now,
		updatedAt:  now,
	}

	r.objects[objectID] = entry

	// Index by actor
	if actorID != "" {
		r.objectsByActor[actorID] = append(r.objectsByActor[actorID], objectID)
	}

	// Index as reply if applicable
	if inReplyTo != "" {
		r.repliesByParent[inReplyTo] = append(r.repliesByParent[inReplyTo], objectID)
	}

	return nil
}

// GetObject retrieves an object by ID
func (r *ObjectRepository) GetObject(_ context.Context, id string) (any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.objects[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return entry.object, nil
}

// UpdateObject updates an existing object
func (r *ObjectRepository) UpdateObject(_ context.Context, object any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	objectID, objectType, actorID, inReplyTo := extractObjectMetadata(object)
	if objectID == "" {
		return fmt.Errorf("object ID is required")
	}

	entry, exists := r.objects[objectID]
	if !exists {
		now := time.Now()
		objectType, actorID, inReplyTo := "", "", ""
		_, objectType, actorID, inReplyTo = extractObjectMetadata(object)
		r.objects[objectID] = &objectEntry{
			object:     object,
			objectType: objectType,
			actorID:    actorID,
			inReplyTo:  inReplyTo,
			createdAt:  now,
			updatedAt:  now,
		}
		if actorID != "" {
			r.objectsByActor[actorID] = append(r.objectsByActor[actorID], objectID)
		}
		if inReplyTo != "" {
			r.repliesByParent[inReplyTo] = append(r.repliesByParent[inReplyTo], objectID)
		}
		return nil
	}

	entry.object = object
	entry.objectType = objectType
	entry.actorID = actorID
	entry.inReplyTo = inReplyTo
	entry.updatedAt = time.Now()

	return nil
}

// UpdateObjectWithHistory updates an object and tracks the edit history
func (r *ObjectRepository) UpdateObjectWithHistory(_ context.Context, object any, updatedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	objectID, objectType, actorID, inReplyTo := extractObjectMetadata(object)
	if objectID == "" {
		return fmt.Errorf("object ID is required")
	}

	entry, exists := r.objects[objectID]
	if !exists {
		now := time.Now()
		r.objects[objectID] = &objectEntry{
			object:     object,
			objectType: objectType,
			actorID:    actorID,
			inReplyTo:  inReplyTo,
			createdAt:  now,
			updatedAt:  now,
		}
		if actorID != "" {
			r.objectsByActor[actorID] = append(r.objectsByActor[actorID], objectID)
		}
		if inReplyTo != "" {
			r.repliesByParent[inReplyTo] = append(r.repliesByParent[inReplyTo], objectID)
		}
		return nil
	}

	// Store previous state in history
	version := len(r.updateHistory[objectID]) + 1
	history := &storage.UpdateHistory{
		ObjectID:      objectID,
		Version:       version,
		UpdatedAt:     time.Now(),
		UpdatedBy:     updatedBy,
		PreviousState: map[string]any{"object": entry.object},
		Summary:       "Update",
	}
	r.updateHistory[objectID] = append(r.updateHistory[objectID], history)

	// Update the object
	entry.object = object
	entry.objectType = objectType
	entry.actorID = actorID
	entry.inReplyTo = inReplyTo
	entry.updatedAt = time.Now()

	return nil
}

// DeleteObject deletes an object by ID
func (r *ObjectRepository) DeleteObject(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.objects[objectID]
	if !exists {
		return nil // Idempotent delete
	}

	// Remove from actor index
	if entry.actorID != "" {
		r.objectsByActor[entry.actorID] = removeString(r.objectsByActor[entry.actorID], objectID)
	}

	// Remove from reply index
	if entry.inReplyTo != "" {
		r.repliesByParent[entry.inReplyTo] = removeString(r.repliesByParent[entry.inReplyTo], objectID)
	}

	delete(r.objects, objectID)
	return nil
}

// GetObjectsByActor retrieves objects created by a specific actor
func (r *ObjectRepository) GetObjectsByActor(_ context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	objectIDs := r.objectsByActor[actorID]
	results, nextCursor := r.paginateObjects(objectIDs, limit, cursor)
	return results, nextCursor, nil
}

// ===== Status Operations =====

// GetStatus retrieves a status by ID (alias for GetObject)
func (r *ObjectRepository) GetStatus(ctx context.Context, statusID string) (any, error) {
	return r.GetObject(ctx, statusID)
}

// GetUserStatusCount counts the number of statuses by a user
func (r *ObjectRepository) GetUserStatusCount(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.objectsByActor[userID]), nil
}

// GetStatusReplyCount counts replies to a specific status
func (r *ObjectRepository) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	return r.CountReplies(ctx, statusID)
}

// ===== Reply Operations =====

// CountObjectReplies counts the number of replies to an object
func (r *ObjectRepository) CountObjectReplies(_ context.Context, objectID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.repliesByParent[objectID]), nil
}

// CountReplies counts the number of replies to an object using GSI6
func (r *ObjectRepository) CountReplies(_ context.Context, objectID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check explicit reply count first
	if count, exists := r.replyCounts[objectID]; exists {
		return count, nil
	}

	return len(r.repliesByParent[objectID]), nil
}

// GetReplies retrieves replies to an object with pagination
func (r *ObjectRepository) GetReplies(_ context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	replyIDs := r.repliesByParent[objectID]
	results, nextCursor := r.paginateObjects(replyIDs, limit, cursor)
	return results, nextCursor, nil
}

func (r *ObjectRepository) paginateObjects(objectIDs []string, limit int, cursor string) ([]any, string) {
	if len(objectIDs) == 0 {
		return []any{}, ""
	}

	if limit <= 0 {
		limit = 20
	}

	startIdx := 0
	if cursor != "" {
		for i, id := range objectIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	results := make([]any, 0, limit)
	for i := startIdx; i < len(objectIDs) && len(results) < limit; i++ {
		if entry, exists := r.objects[objectIDs[i]]; exists {
			results = append(results, entry.object)
		}
	}

	nextCursor := ""
	if startIdx+limit < len(objectIDs) {
		nextCursor = objectIDs[startIdx+limit-1]
	}

	return results, nextCursor
}

// IncrementReplyCount increments the reply count for an object
func (r *ObjectRepository) IncrementReplyCount(_ context.Context, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.replyCounts[objectID]++
	return nil
}

// GetReplyCount gets the reply count for a status
func (r *ObjectRepository) GetReplyCount(_ context.Context, statusID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if count, exists := r.replyCounts[statusID]; exists {
		return int64(count), nil
	}

	return int64(len(r.repliesByParent[statusID])), nil
}

// ===== Tombstone Operations =====

// TombstoneObject marks an object as deleted by creating a tombstone
func (r *ObjectRepository) TombstoneObject(_ context.Context, objectID string, deletedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.objects[objectID]
	if !exists {
		return storage.ErrNotFound
	}

	// Create tombstone
	tombstone := &models.Tombstone{
		ID:         objectID,
		FormerType: entry.objectType,
		DeletedBy:  deletedBy,
		Deleted:    time.Now(),
	}

	r.tombstones[objectID] = tombstone
	r.tombstonesByType[entry.objectType] = append(r.tombstonesByType[entry.objectType], objectID)

	// Remove the original object
	if entry.actorID != "" {
		r.objectsByActor[entry.actorID] = removeString(r.objectsByActor[entry.actorID], objectID)
	}
	if entry.inReplyTo != "" {
		r.repliesByParent[entry.inReplyTo] = removeString(r.repliesByParent[entry.inReplyTo], objectID)
	}
	delete(r.objects, objectID)

	return nil
}

// CreateTombstone creates a tombstone for a deleted object
func (r *ObjectRepository) CreateTombstone(_ context.Context, tombstone *models.Tombstone) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tombstone == nil || tombstone.ID == "" {
		return fmt.Errorf("tombstone ID is required")
	}

	r.tombstones[tombstone.ID] = tombstone
	r.tombstonesByType[tombstone.FormerType] = append(r.tombstonesByType[tombstone.FormerType], tombstone.ID)

	return nil
}

// GetTombstone retrieves a tombstone by object ID
func (r *ObjectRepository) GetTombstone(_ context.Context, objectID string) (*models.Tombstone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tombstone, exists := r.tombstones[objectID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return tombstone, nil
}

// IsTombstoned checks if an object has been tombstoned (deleted)
func (r *ObjectRepository) IsTombstoned(_ context.Context, objectID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.tombstones[objectID]
	return exists, nil
}

// GetTombstonesByActor retrieves all tombstones created by a specific actor
func (r *ObjectRepository) GetTombstonesByActor(_ context.Context, actorID string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.Tombstone
	var nextCursor string
	foundCursor := cursor == ""

	for id, tombstone := range r.tombstones {
		if tombstone.DeletedBy != actorID {
			continue
		}

		if !foundCursor {
			if id == cursor {
				foundCursor = true
			}
			continue
		}

		results = append(results, tombstone)
		if len(results) >= limit {
			nextCursor = id
			break
		}
	}

	return results, nextCursor, nil
}

// GetTombstonesByType retrieves tombstones by their former type
func (r *ObjectRepository) GetTombstonesByType(_ context.Context, formerType string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	objectIDs := r.tombstonesByType[formerType]
	if len(objectIDs) == 0 {
		return []*models.Tombstone{}, "", nil
	}

	startIdx := 0
	if cursor != "" {
		for i, id := range objectIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*models.Tombstone
	var nextCursor string

	for i := startIdx; i < len(objectIDs) && len(results) < limit; i++ {
		if tombstone, exists := r.tombstones[objectIDs[i]]; exists {
			results = append(results, tombstone)
		}
	}

	if startIdx+limit < len(objectIDs) {
		nextCursor = objectIDs[startIdx+limit-1]
	}

	return results, nextCursor, nil
}

// CleanupExpiredTombstones removes tombstones that have exceeded their TTL
func (r *ObjectRepository) CleanupExpiredTombstones(_ context.Context, batchSize int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cleaned := 0
	now := time.Now()

	for id, tombstone := range r.tombstones {
		if cleaned >= batchSize {
			break
		}

		// Check if TTL has expired (default 30 days)
		if tombstone.TTL > 0 && now.Unix() > tombstone.TTL {
			delete(r.tombstones, id)
			r.tombstonesByType[tombstone.FormerType] = removeString(r.tombstonesByType[tombstone.FormerType], id)
			cleaned++
		}
	}

	return cleaned, nil
}

// ReplaceObjectWithTombstone atomically replaces an object with a tombstone
func (r *ObjectRepository) ReplaceObjectWithTombstone(_ context.Context, objectID, formerType, deletedBy, attributedTo string, isPublic bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Delete the original object if it exists
	if entry, exists := r.objects[objectID]; exists {
		if entry.actorID != "" {
			r.objectsByActor[entry.actorID] = removeString(r.objectsByActor[entry.actorID], objectID)
		}
		if entry.inReplyTo != "" {
			r.repliesByParent[entry.inReplyTo] = removeString(r.repliesByParent[entry.inReplyTo], objectID)
		}
		delete(r.objects, objectID)
	}

	// Create tombstone
	tombstone := &models.Tombstone{
		ID:           objectID,
		FormerType:   formerType,
		DeletedBy:    deletedBy,
		AttributedTo: attributedTo,
		IsPublic:     isPublic,
		Deleted:      time.Now(),
	}

	r.tombstones[objectID] = tombstone
	r.tombstonesByType[formerType] = append(r.tombstonesByType[formerType], objectID)

	return nil
}

// ===== Update History Operations =====

// CreateUpdateHistory creates a new update history entry for an object
func (r *ObjectRepository) CreateUpdateHistory(_ context.Context, history *storage.UpdateHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if history == nil || history.ObjectID == "" {
		return fmt.Errorf("history object ID is required")
	}

	r.updateHistory[history.ObjectID] = append(r.updateHistory[history.ObjectID], history)
	return nil
}

// GetUpdateHistory retrieves update history for an object
func (r *ObjectRepository) GetUpdateHistory(_ context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	history := r.updateHistory[objectID]
	if len(history) == 0 {
		return []*storage.UpdateHistory{}, nil
	}

	if err := common.ValidateQueryLimit(limit, 100, "update history"); err != nil {
		limit = 10
	}
	if limit > len(history) {
		limit = len(history)
	}

	// Return most recent first, limited
	resultCap := len(history)
	if resultCap > 100 {
		resultCap = 100
	}
	result := make([]*storage.UpdateHistory, 0, resultCap)
	for i := len(history) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, history[i])
	}

	return result, nil
}

// GetObjectHistory retrieves the version history of an object
func (r *ObjectRepository) GetObjectHistory(ctx context.Context, objectID string) ([]*storage.UpdateHistory, error) {
	return r.GetUpdateHistory(ctx, objectID, 100)
}

// ===== Collection Operations =====

// AddToCollection adds an item to a collection
func (r *ObjectRepository) AddToCollection(_ context.Context, collection string, item *storage.CollectionItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if item == nil || item.ItemID == "" {
		return fmt.Errorf("collection item ID is required")
	}

	if r.collections[collection] == nil {
		r.collections[collection] = make(map[string]*storage.CollectionItem)
	}

	r.collections[collection][item.ItemID] = item
	return nil
}

// RemoveFromCollection removes an item from a collection
func (r *ObjectRepository) RemoveFromCollection(_ context.Context, collection, itemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.collections[collection] != nil {
		delete(r.collections[collection], itemID)
	}
	return nil
}

// GetCollectionItems retrieves items from a collection with pagination
func (r *ObjectRepository) GetCollectionItems(_ context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := r.collections[collection]
	if len(items) == 0 {
		return []*storage.CollectionItem{}, "", nil
	}

	var results []*storage.CollectionItem
	var nextCursor string
	foundCursor := cursor == ""

	for id, item := range items {
		if !foundCursor {
			if id == cursor {
				foundCursor = true
			}
			continue
		}

		results = append(results, item)
		if len(results) >= limit {
			nextCursor = id
			break
		}
	}

	return results, nextCursor, nil
}

// IsInCollection checks if an item is in a collection
func (r *ObjectRepository) IsInCollection(_ context.Context, collection, itemID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.collections[collection] == nil {
		return false, nil
	}

	_, exists := r.collections[collection][itemID]
	return exists, nil
}

// CountCollectionItems returns the count of items in a collection
func (r *ObjectRepository) CountCollectionItems(_ context.Context, collection string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.collections[collection]), nil
}

// ===== Quote Operations =====

// CountQuotes counts the number of quotes for a specific note
func (r *ObjectRepository) CountQuotes(_ context.Context, noteID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, quoteID := range r.quotesByTarget[noteID] {
		if quote, exists := r.quotes[quoteID]; exists && !quote.Withdrawn {
			count++
		}
	}
	return count, nil
}

// CreateQuoteRelationship creates a new quote relationship between notes
func (r *ObjectRepository) CreateQuoteRelationship(_ context.Context, quote *storage.QuoteRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if quote == nil || quote.ID == "" {
		return fmt.Errorf("quote ID is required")
	}

	r.quotes[quote.ID] = quote
	r.quotesByTarget[quote.TargetNoteID] = append(r.quotesByTarget[quote.TargetNoteID], quote.ID)
	r.quotesByQuoter[quote.QuoterNoteID] = append(r.quotesByQuoter[quote.QuoterNoteID], quote.ID)

	// Index by actor
	if r.quotesByActor[quote.QuoterID] == nil {
		r.quotesByActor[quote.QuoterID] = make(map[string]string)
	}
	r.quotesByActor[quote.QuoterID][quote.TargetNoteID] = quote.ID

	return nil
}

// GetQuotesForNote retrieves quotes for a specific note with pagination
func (r *ObjectRepository) GetQuotesForNote(_ context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	quoteIDs := r.quotesByTarget[noteID]
	if len(quoteIDs) == 0 {
		return []*storage.QuoteRelationship{}, "", nil
	}

	startIdx := 0
	if cursor != "" {
		for i, id := range quoteIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*storage.QuoteRelationship
	var nextCursor string

	for i := startIdx; i < len(quoteIDs) && len(results) < limit; i++ {
		if quote, exists := r.quotes[quoteIDs[i]]; exists && !quote.Withdrawn {
			results = append(results, quote)
		}
	}

	if startIdx+limit < len(quoteIDs) {
		nextCursor = quoteIDs[startIdx+limit-1]
	}

	return results, nextCursor, nil
}

// IsQuoted checks if a note is quoted by a specific actor
func (r *ObjectRepository) IsQuoted(_ context.Context, actorID, noteID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.quotesByActor[actorID] == nil {
		return false, nil
	}

	quoteID, exists := r.quotesByActor[actorID][noteID]
	if !exists {
		return false, nil
	}

	quote, exists := r.quotes[quoteID]
	if !exists {
		return false, nil
	}

	return !quote.Withdrawn, nil
}

// WithdrawQuote withdraws a quote by marking it as withdrawn
func (r *ObjectRepository) WithdrawQuote(_ context.Context, quoteNoteID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find quote by quoter note ID
	for _, quoteID := range r.quotesByQuoter[quoteNoteID] {
		if quote, exists := r.quotes[quoteID]; exists {
			now := time.Now()
			quote.Withdrawn = true
			quote.WithdrawnAt = &now
		}
	}

	return nil
}

// WithdrawStatusFromQuotes withdraws a status from being quoted with proper cascade effects
func (r *ObjectRepository) WithdrawStatusFromQuotes(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.withdrawnStatus[statusID] = true

	// Withdraw all existing quotes of this status
	for _, quoteID := range r.quotesByTarget[statusID] {
		if quote, exists := r.quotes[quoteID]; exists {
			now := time.Now()
			quote.Withdrawn = true
			quote.WithdrawnAt = &now
		}
	}

	return nil
}

// UpdateQuotePermissions updates the quote permissions for a status
func (r *ObjectRepository) UpdateQuotePermissions(_ context.Context, statusID string, permissions *storage.QuotePermissions) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if permissions.AllowPublic {
		r.quoteTypes[statusID] = "public"
	} else if permissions.AllowFollowers {
		r.quoteTypes[statusID] = "followers"
	} else if permissions.AllowMentioned {
		r.quoteTypes[statusID] = "mentioned"
	} else {
		r.quoteTypes[statusID] = quoteTypeDisabled
	}

	return nil
}

// IsQuoteAllowed checks if a quote is allowed for a status by a quoter
func (r *ObjectRepository) IsQuoteAllowed(_ context.Context, statusID, _ string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if withdrawn
	if r.withdrawnStatus[statusID] {
		return false, nil
	}

	quoteType := r.quoteTypes[statusID]
	switch quoteType {
	case "public":
		return true, nil
	case quoteTypeDisabled:
		return false, nil
	case "followers", "mentioned":
		// Simplified - would need relationship check in real implementation
		return false, nil
	default:
		// Default to disabled.
		return false, nil
	}
}

// GetQuoteType returns the quote type for a status
func (r *ObjectRepository) GetQuoteType(_ context.Context, statusID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	quoteType, exists := r.quoteTypes[statusID]
	if !exists {
		return models.VisibilityPublic, nil
	}
	return quoteType, nil
}

// GetQuoteTypes returns quote controls for a request-local batch.
func (r *ObjectRepository) GetQuoteTypes(_ context.Context, statusIDs []string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	quoteTypes := make(map[string]string, len(statusIDs))
	for _, statusID := range statusIDs {
		quoteType, ok := r.quoteTypes[statusID]
		if !ok {
			quoteType = models.VisibilityPublic
		}
		quoteTypes[statusID] = quoteType
	}
	return quoteTypes, nil
}

// IsWithdrawnFromQuotes checks if a status is withdrawn from quotes
func (r *ObjectRepository) IsWithdrawnFromQuotes(_ context.Context, statusID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.withdrawnStatus[statusID], nil
}

// GetQuotesOfStatus retrieves quotes of a specific status
func (r *ObjectRepository) GetQuotesOfStatus(_ context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	quoteIDs := r.quotesByTarget[statusID]
	var results []*storage.StatusSearchResult

	for _, quoteID := range quoteIDs {
		if len(results) >= limit {
			break
		}
		if quote, exists := r.quotes[quoteID]; exists && !quote.Withdrawn {
			results = append(results, &storage.StatusSearchResult{
				StatusID:  quote.QuoterNoteID,
				AuthorID:  quote.QuoterID,
				Published: quote.Timestamp,
				Score:     1.0,
			})
		}
	}

	return results, nil
}

// ===== Thread Operations =====

// GetMissingReplies returns a list of known missing replies in a thread
func (r *ObjectRepository) GetMissingReplies(_ context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	missingIDs := r.missingReplies[statusID]
	if len(missingIDs) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	var results []*storage.StatusSearchResult
	for _, replyID := range missingIDs {
		results = append(results, &storage.StatusSearchResult{
			StatusID:  replyID,
			Content:   "[Missing Reply]",
			Published: time.Now().Add(-24 * time.Hour),
			Score:     0.1,
		})
	}

	return results, nil
}

// MarkThreadAsSynced marks a thread as successfully synced
func (r *ObjectRepository) MarkThreadAsSynced(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.threadSynced[statusID] = true
	return nil
}

// SyncThreadFromRemote syncs a thread from a remote server
func (r *ObjectRepository) SyncThreadFromRemote(_ context.Context, statusID string) (*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if we have the object locally
	entry, exists := r.objects[statusID]
	if exists {
		objectID, _, actorID, _ := extractObjectMetadata(entry.object)
		return &storage.StatusSearchResult{
			StatusID:  objectID,
			AuthorID:  actorID,
			Published: entry.createdAt,
			Score:     1.0,
		}, nil
	}

	return nil, storage.ErrNotFound
}

// SyncMissingRepliesFromRemote syncs missing replies from remote servers
func (r *ObjectRepository) SyncMissingRepliesFromRemote(_ context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	missingIDs := r.missingReplies[statusID]
	if len(missingIDs) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	// Filter to recent missing replies (within 30 days)
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	var results []*storage.StatusSearchResult

	for _, replyID := range missingIDs {
		results = append(results, &storage.StatusSearchResult{
			StatusID:  replyID,
			Content:   "[Missing Reply - Remote Fetch Required]",
			Published: cutoff.Add(time.Hour), // Mark as recent enough to fetch
			Score:     0.5,
		})
	}

	return results, nil
}

// GetThreadContext retrieves the thread context for a status with full hierarchy
func (r *ObjectRepository) GetThreadContext(_ context.Context, statusID string) (*storage.ThreadContext, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if we have a cached context
	if ctx, exists := r.threadContexts[statusID]; exists {
		return ctx, nil
	}

	// Build context from object relationships
	entry, exists := r.objects[statusID]
	if !exists {
		return &storage.ThreadContext{
			StatusID:    statusID,
			Ancestors:   []string{},
			Descendants: []string{},
		}, nil
	}

	// Build ancestors by following inReplyTo chain
	var ancestors []string
	currentID := entry.inReplyTo
	for currentID != "" {
		ancestors = append([]string{currentID}, ancestors...) // Prepend to get oldest first
		if parentEntry, exists := r.objects[currentID]; exists {
			currentID = parentEntry.inReplyTo
		} else {
			break
		}
	}

	// Get direct descendants (replies)
	descendants := r.repliesByParent[statusID]

	return &storage.ThreadContext{
		StatusID:    statusID,
		Ancestors:   ancestors,
		Descendants: descendants,
	}, nil
}

// ===== Helper Functions =====

// extractObjectMetadata extracts common metadata from an object
func extractObjectMetadata(object any) (id, objectType, actorID, inReplyTo string) {
	if object == nil {
		return "", "", "", ""
	}

	// Try to extract from map
	if objMap, ok := object.(map[string]any); ok {
		if v, ok := objMap["id"].(string); ok {
			id = v
		}
		if v, ok := objMap["type"].(string); ok {
			objectType = v
		}
		if v, ok := objMap["attributedTo"].(string); ok {
			actorID = v
		}
		if v, ok := objMap["inReplyTo"].(string); ok {
			inReplyTo = v
		}
		return
	}

	var payload struct {
		ID           string `json:"id"`
		Type         string `json:"type"`
		AttributedTo string `json:"attributedTo"`
		InReplyTo    string `json:"inReplyTo"`
	}

	raw, err := json.Marshal(object)
	if err != nil {
		return "", "", "", ""
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", ""
	}

	return payload.ID, payload.Type, payload.AttributedTo, payload.InReplyTo
}

// removeString removes a string from a slice
func removeString(slice []string, item string) []string {
	for i, s := range slice {
		if s == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// SetMissingReplies sets missing replies for a status (test helper)
func (r *ObjectRepository) SetMissingReplies(statusID string, replyIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.missingReplies[statusID] = replyIDs
}

// SetThreadContext sets thread context for a status (test helper)
func (r *ObjectRepository) SetThreadContext(statusID string, ctx *storage.ThreadContext) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.threadContexts[statusID] = ctx
}

// Ensure ObjectRepository implements interfaces.ObjectRepository
var _ interfaces.ObjectRepository = (*ObjectRepository)(nil)
