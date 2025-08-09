package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ObjectRepository implements object operations using DynamORM
type ObjectRepository struct {
	db        core.DB
	tableName string
	domain    string
	logger    *zap.Logger
	accountRepo *AccountRepository
}

// NewObjectRepository creates a new object repository
func NewObjectRepository(db core.DB, tableName, domain string, logger *zap.Logger) *ObjectRepository {
	return &ObjectRepository{
		db:        db,
		tableName: tableName,
		domain:    domain,
		logger:    logger,
		accountRepo: NewAccountRepository(db, tableName, domain, logger),
	}
}

// getDomainURL returns the full domain URL
func (r *ObjectRepository) getDomainURL() string {
	return fmt.Sprintf("https://%s", r.domain)
}

// CreateObject stores a generic ActivityPub object
func (r *ObjectRepository) CreateObject(ctx context.Context, object any) error {
	// Convert the object to ActivityPub base object to extract common fields
	objJSON, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	// Parse to extract common fields
	var baseObj activitypub.BaseObject
	if err := json.Unmarshal(objJSON, &baseObj); err != nil {
		return fmt.Errorf("failed to parse base object: %w", err)
	}

	// Also try to extract Note-specific fields if it's a Note
	var note activitypub.Note
	isNote := false
	if baseObj.Type == activitypub.NoteType {
		if err := json.Unmarshal(objJSON, &note); err == nil {
			isNote = true
		}
	}

	// Determine the actor ID
	actorID := ""
	if isNote && note.AttributedTo != "" {
		actorID = note.AttributedTo
	}

	// Create the model
	objModel := models.NewObject(baseObj.ID, baseObj.Type, actorID)

	// Set common fields
	if isNote {
		objModel.Content = note.Content
		objModel.AttributedTo = note.AttributedTo
		objModel.To = note.To
		objModel.CC = note.CC
		if note.InReplyTo != "" {
			objModel.InReplyTo = &note.InReplyTo
		}
		objModel.Sensitive = note.Sensitive

		// Store complex fields as JSON
		if len(note.Attachment) > 0 {
			attachJSON, _ := json.Marshal(note.Attachment)
			objModel.AttachmentJSON = string(attachJSON)
		}
		if len(note.Tag) > 0 {
			tagJSON, _ := json.Marshal(note.Tag)
			objModel.TagJSON = string(tagJSON)
		}
	}

	// Set timestamps
	if baseObj.Published != nil {
		objModel.Published = *baseObj.Published
	}
	if baseObj.Updated != nil {
		objModel.Updated = *baseObj.Updated
	}

	// Store context as JSON
	if baseObj.Context != nil {
		contextJSON, _ := json.Marshal(baseObj.Context)
		objModel.ContextJSON = string(contextJSON)
	}

	// Update GSI keys
	objModel.UpdateGSIKeys()

	// Store in DynamoDB
	if err := r.db.WithContext(ctx).Model(objModel).Create(); err != nil {
		r.logger.Error("failed to create object",
			zap.String("object_id", baseObj.ID),
			zap.String("type", baseObj.Type),
			zap.Error(err))
		return fmt.Errorf("failed to create object: %w", err)
	}

	r.logger.Info("stored object",
		zap.String("object_id", baseObj.ID),
		zap.String("type", baseObj.Type),
		zap.String("actor", actorID))

	return nil
}

// GetObject retrieves an object by ID
func (r *ObjectRepository) GetObject(ctx context.Context, id string) (any, error) {
	var objModel models.Object

	query := r.db.WithContext(ctx).Model(&objModel).
		Where("PK", "=", fmt.Sprintf("object#%s", id)).
		Where("SK", "=", fmt.Sprintf("object#%s", id))

	if err := query.First(&objModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("object not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	// Convert back to appropriate ActivityPub type
	return r.modelToActivityPubObject(&objModel)
}

// UpdateObject updates an existing object
func (r *ObjectRepository) UpdateObject(ctx context.Context, object any) error {
	// Similar to CreateObject, extract fields and update
	objJSON, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	var baseObj activitypub.BaseObject
	if err := json.Unmarshal(objJSON, &baseObj); err != nil {
		return fmt.Errorf("failed to parse base object: %w", err)
	}

	// Get existing object
	var objModel models.Object
	query := r.db.WithContext(ctx).Model(&objModel).
		Where("PK", "=", fmt.Sprintf("object#%s", baseObj.ID)).
		Where("SK", "=", fmt.Sprintf("object#%s", baseObj.ID))

	if err := query.First(&objModel); err != nil {
		if errors.IsNotFound(err) {
			// Object doesn't exist, create it
			return r.CreateObject(ctx, object)
		}
		return fmt.Errorf("failed to get object for update: %w", err)
	}

	// Update fields
	objModel.Updated = time.Now()
	if baseObj.Type == activitypub.NoteType {
		var note activitypub.Note
		if err := json.Unmarshal(objJSON, &note); err == nil {
			objModel.Content = note.Content
			objModel.Sensitive = note.Sensitive
			// Update other fields as needed
		}
	}

	// Update in database
	if err := r.db.WithContext(ctx).Model(&objModel).
		Where("PK", "=", objModel.PK).
		Where("SK", "=", objModel.SK).
		Update(); err != nil {
		r.logger.Error("failed to update object",
			zap.String("object_id", baseObj.ID),
			zap.Error(err))
		return fmt.Errorf("failed to update object: %w", err)
	}

	return nil
}

// DeleteObject deletes an object by ID
func (r *ObjectRepository) DeleteObject(ctx context.Context, objectID string) error {
	objModel := &models.Object{
		PK: fmt.Sprintf("object#%s", objectID),
		SK: fmt.Sprintf("object#%s", objectID),
	}

	query := r.db.WithContext(ctx).Model(objModel).
		Where("PK", "=", objModel.PK).
		Where("SK", "=", objModel.SK)

	if err := query.Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("object not found for deletion",
				zap.String("object_id", objectID))
			return nil
		}
		r.logger.Error("failed to delete object",
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to delete object: %w", err)
	}

	r.logger.Info("deleted object",
		zap.String("object_id", objectID))

	return nil
}

// GetObjectsByActor retrieves objects created by a specific actor
func (r *ObjectRepository) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("actor#%s", actorID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var objects []models.Object
	err := query.All(&objects)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan objects: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(objects) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = objects[limit-1].SK
		objects = objects[:limit] // Trim to requested limit
	}

	// Convert to ActivityPub objects
	result := make([]any, 0, len(objects))
	for _, objModel := range objects {
		apObj, err := r.modelToActivityPubObject(&objModel)
		if err != nil {
			r.logger.Warn("failed to convert object",
				zap.String("object_id", objModel.ID),
				zap.Error(err))
			continue
		}
		result = append(result, apObj)
	}

	return result, nextCursor, nil
}

// CountObjectReplies counts the number of replies to an object
func (r *ObjectRepository) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	// Query objects that have InReplyTo set to this objectID
	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi2-index"). // Assuming GSI2 is used for reply relationships
		Where("GSI2PK", "=", fmt.Sprintf("reply#%s", objectID))

	var objects []models.Object
	if err := query.All(&objects); err != nil {
		r.logger.Error("failed to count object replies",
			zap.String("object_id", objectID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count object replies: %w", err)
	}

	count := len(objects)
	r.logger.Debug("counted object replies",
		zap.String("object_id", objectID),
		zap.Int("count", count))

	return count, nil
}

// TombstoneObject marks an object as deleted by creating a tombstone
func (r *ObjectRepository) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	// First verify the object exists
	existingObj, err := r.GetObject(ctx, objectID)
	if err != nil {
		return fmt.Errorf("object not found for tombstoning: %w", err)
	}

	// Get the object ID from the result
	var objID string
	if objMap, ok := existingObj.(map[string]any); ok {
		if id, ok := objMap["id"].(string); ok {
			objID = id
		}
	} else if note, ok := existingObj.(*activitypub.Note); ok {
		objID = note.ID
	}

	if objID == "" {
		return fmt.Errorf("could not extract object ID")
	}

	// Create a tombstone object
	tombstone := models.NewObject(objectID, "Tombstone", deletedBy)
	tombstone.Content = fmt.Sprintf("Object %s was deleted", objectID)
	tombstone.Published = time.Now()
	tombstone.Updated = time.Now()

	// Set tombstone-specific fields
	tombstone.AttributedTo = deletedBy

	// Update GSI keys
	tombstone.UpdateGSIKeys()

	// Delete the original object and create tombstone in a transaction-like manner
	// First delete the original
	if err := r.DeleteObject(ctx, objectID); err != nil {
		r.logger.Error("failed to delete original object for tombstoning",
			zap.String("object_id", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to delete original object: %w", err)
	}

	// Then create the tombstone
	if err := r.db.WithContext(ctx).Model(tombstone).Create(); err != nil {
		r.logger.Error("failed to create tombstone",
			zap.String("object_id", objectID),
			zap.String("deleted_by", deletedBy),
			zap.Error(err))
		return fmt.Errorf("failed to create tombstone: %w", err)
	}

	r.logger.Info("tombstoned object",
		zap.String("object_id", objectID),
		zap.String("deleted_by", deletedBy))

	return nil
}

// modelToActivityPubObject converts a model to the appropriate ActivityPub object
func (r *ObjectRepository) modelToActivityPubObject(objModel *models.Object) (any, error) {
	switch objModel.Type {
	case activitypub.NoteType:
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        objModel.ID,
				Type:      objModel.Type,
				Published: &objModel.Published,
				Updated:   &objModel.Updated,
				To:        objModel.To,
				CC:        objModel.CC,
				Sensitive: objModel.Sensitive,
			},
			Content:      objModel.Content,
			AttributedTo: objModel.AttributedTo,
		}

		// Set InReplyTo if present
		if objModel.InReplyTo != nil {
			note.InReplyTo = *objModel.InReplyTo
		}

		// Parse complex fields from JSON
		if objModel.AttachmentJSON != "" {
			_ = json.Unmarshal([]byte(objModel.AttachmentJSON), &note.Attachment)
		}
		if objModel.TagJSON != "" {
			_ = json.Unmarshal([]byte(objModel.TagJSON), &note.Tag)
		}
		if objModel.ContextJSON != "" {
			_ = json.Unmarshal([]byte(objModel.ContextJSON), &note.Context)
		}

		return note, nil

	default:
		// Return as generic map for other types
		result := map[string]any{
			"id":           objModel.ID,
			"type":         objModel.Type,
			"attributedTo": objModel.AttributedTo,
			"content":      objModel.Content,
			"published":    objModel.Published,
			"updated":      objModel.Updated,
		}

		if objModel.To != nil {
			result["to"] = objModel.To
		}
		if objModel.CC != nil {
			result["cc"] = objModel.CC
		}

		return result, nil
	}
}

// CreateUpdateHistory creates a new update history entry for an object
func (r *ObjectRepository) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	// Convert storage.UpdateHistory to models.UpdateHistory
	// Convert PreviousState map to JSON string
	previousStateJSON := ""
	if history.PreviousState != nil {
		jsonBytes, err := json.Marshal(history.PreviousState)
		if err != nil {
			r.logger.Error("failed to marshal previous state", zap.Error(err))
			return fmt.Errorf("failed to marshal previous state: %w", err)
		}
		previousStateJSON = string(jsonBytes)
	}

	updateHistory := &models.UpdateHistory{
		ObjectID:      history.ObjectID,
		Version:       history.Version,
		UpdatedAt:     history.UpdatedAt,
		UpdatedBy:     history.UpdatedBy,
		PreviousState: previousStateJSON,
		Summary:       history.Summary,
		CreatedAt:     time.Now(),
	}

	// Update the key fields
	updateHistory.UpdateKeys()

	// Create the update history record
	err := r.db.WithContext(ctx).Model(updateHistory).Create()
	if err != nil {
		r.logger.Error("failed to create update history",
			zap.String("object_id", history.ObjectID),
			zap.Int("version", history.Version),
			zap.Error(err))
		return fmt.Errorf("failed to create update history: %w", err)
	}

	r.logger.Info("update history created",
		zap.String("object_id", history.ObjectID),
		zap.Int("version", history.Version))

	return nil
}

// GetUpdateHistory retrieves update history for an object
func (r *ObjectRepository) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	// Validate limit
	if limit <= 0 || limit > 100 {
		limit = 10 // default
	}

	// Build the query - query by PK and SK prefix
	query := r.db.WithContext(ctx).Model(&models.UpdateHistory{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#HISTORY", objectID)).
		OrderBy("SK", "DESC"). // Newest version first
		Limit(limit)

	var histories []models.UpdateHistory
	err := query.All(&histories)

	if err != nil {
		r.logger.Error("failed to query update history",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query update history: %w", err)
	}

	// Convert to storage.UpdateHistory
	result := make([]*storage.UpdateHistory, len(histories))
	for i, h := range histories {
		// Convert PreviousState JSON string back to map
		var previousState map[string]interface{}
		if h.PreviousState != "" {
			if err := json.Unmarshal([]byte(h.PreviousState), &previousState); err != nil {
				r.logger.Warn("failed to unmarshal previous state",
					zap.String("object_id", h.ObjectID),
					zap.Int("version", h.Version),
					zap.Error(err))
				// Continue without previous state rather than failing
			}
		}

		result[i] = &storage.UpdateHistory{
			ObjectID:      h.ObjectID,
			Version:       h.Version,
			UpdatedAt:     h.UpdatedAt,
			UpdatedBy:     h.UpdatedBy,
			PreviousState: previousState,
			Summary:       h.Summary,
		}
	}

	return result, nil
}

// ===== CollectionItem Methods =====

// AddToCollection adds an item to a collection
func (r *ObjectRepository) AddToCollection(ctx context.Context, collection string, item *storage.CollectionItem) error {
	collectionItem := models.NewCollectionItem(collection, item.ItemID, item.ItemType, item.AddedBy)
	collectionItem.Position = item.Position

	if err := r.db.WithContext(ctx).Model(collectionItem).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Info("item already in collection",
				zap.String("collection", collection),
				zap.String("item_id", item.ItemID))
			return nil // Not an error to add something already in collection
		}
		r.logger.Error("failed to add to collection",
			zap.String("collection", collection),
			zap.String("item_id", item.ItemID),
			zap.Error(err))
		return fmt.Errorf("failed to add to collection: %w", err)
	}

	r.logger.Info("added item to collection",
		zap.String("collection", collection),
		zap.String("item_id", item.ItemID))

	return nil
}

// RemoveFromCollection removes an item from a collection
func (r *ObjectRepository) RemoveFromCollection(ctx context.Context, collection, itemID string) error {
	collectionItem := &models.CollectionItem{
		PK: fmt.Sprintf("COLLECTION#%s", collection),
		SK: fmt.Sprintf("ITEM#%s", itemID),
	}

	if err := r.db.WithContext(ctx).Model(collectionItem).
		Where("PK", "=", collectionItem.PK).
		Where("SK", "=", collectionItem.SK).
		Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("item not in collection",
				zap.String("collection", collection),
				zap.String("item_id", itemID))
			return nil
		}
		r.logger.Error("failed to remove from collection",
			zap.String("collection", collection),
			zap.String("item_id", itemID),
			zap.Error(err))
		return fmt.Errorf("failed to remove from collection: %w", err)
	}

	r.logger.Info("removed item from collection",
		zap.String("collection", collection),
		zap.String("item_id", itemID))

	return nil
}

// GetCollectionItems retrieves items from a collection with pagination
func (r *ObjectRepository) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.CollectionItem{}).
		Where("PK", "=", fmt.Sprintf("COLLECTION#%s", collection)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var items []models.CollectionItem
	err := query.All(&items)
	if err != nil {
		r.logger.Error("failed to get collection items",
			zap.String("collection", collection),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get collection items: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(items) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = items[limit-1].SK
		items = items[:limit] // Trim to requested limit
	}

	// Convert to storage.CollectionItem slice
	result := make([]*storage.CollectionItem, len(items))
	for i, item := range items {
		result[i] = &storage.CollectionItem{
			CollectionID: item.Collection,
			ItemID:       item.ItemID,
			ItemType:     item.ItemType,
			AddedBy:      item.AddedBy,
			AddedAt:      item.AddedAt,
			Position:     item.Position,
		}
	}

	return result, nextCursor, nil
}

// IsInCollection checks if an item is in a collection
func (r *ObjectRepository) IsInCollection(ctx context.Context, collection, itemID string) (bool, error) {
	var item models.CollectionItem

	err := r.db.WithContext(ctx).Model(&item).
		Where("PK", "=", fmt.Sprintf("COLLECTION#%s", collection)).
		Where("SK", "=", fmt.Sprintf("ITEM#%s", itemID)).
		First(&item)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check collection membership",
			zap.String("collection", collection),
			zap.String("item_id", itemID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check collection membership: %w", err)
	}

	return true, nil
}

// CountCollectionItems returns the count of items in a collection
func (r *ObjectRepository) CountCollectionItems(ctx context.Context, collection string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.CollectionItem{}).
		Where("PK", "=", fmt.Sprintf("COLLECTION#%s", collection)).
		Count()

	if err != nil {
		r.logger.Error("failed to count collection items",
			zap.String("collection", collection),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count collection items: %w", err)
	}

	return int(count), nil
}

// CountQuotes counts the number of quotes for a specific note
func (r *ObjectRepository) CountQuotes(ctx context.Context, noteID string) (int, error) {
	// Query quotes using GSI1 where GSI1PK = QUOTED#<noteID>
	count, err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("QUOTED#%s", noteID)).
		Count()

	if err != nil {
		r.logger.Error("failed to count quotes",
			zap.String("note_id", noteID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count quotes: %w", err)
	}

	r.logger.Debug("counted quotes for note",
		zap.String("note_id", noteID),
		zap.Int64("count", count))

	return int(count), nil
}

// CountReplies counts the number of replies to an object using GSI6
func (r *ObjectRepository) CountReplies(ctx context.Context, objectID string) (int, error) {
	// Ensure we have the full object URL for GSI6 key
	parentID := objectID
	if !strings.HasPrefix(objectID, "http") {
		parentID = fmt.Sprintf("%s/objects/%s", r.getDomainURL(), objectID)
	}

	// Use GSI6 to efficiently count replies
	count, err := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi6-index").
		Where("GSI6PK", "=", fmt.Sprintf("REPLIES#%s", parentID)).
		Count()

	if err != nil {
		r.logger.Error("failed to count replies",
			zap.String("object_id", objectID),
			zap.String("parent_id", parentID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count replies: %w", err)
	}

	r.logger.Debug("counted replies for object",
		zap.String("object_id", objectID),
		zap.String("parent_id", parentID),
		zap.Int64("count", count))

	return int(count), nil
}

// CreateQuoteRelationship creates a new quote relationship between notes
func (r *ObjectRepository) CreateQuoteRelationship(ctx context.Context, quote *storage.QuoteRelationship) error {
	// Convert storage.QuoteRelationship to models.QuoteRelationship
	model := &models.QuoteRelationship{
		ID:             quote.ID,
		QuoterNoteID:   quote.QuoterNoteID,
		TargetNoteID:   quote.TargetNoteID,
		QuoterID:       quote.QuoterID,
		TargetAuthorID: quote.TargetAuthorID,
		Timestamp:      quote.Timestamp,
		Withdrawn:      quote.Withdrawn,
		WithdrawnAt:    quote.WithdrawnAt,
	}

	// Generate ID if not provided
	if model.ID == "" {
		model.GenerateID()
	}

	// Update composite keys
	model.UpdateKeys()

	// Create the quote relationship
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create quote relationship",
			zap.String("quoter_note_id", quote.QuoterNoteID),
			zap.String("target_note_id", quote.TargetNoteID),
			zap.String("quoter_id", quote.QuoterID),
			zap.Error(err))
		return fmt.Errorf("failed to create quote relationship: %w", err)
	}

	r.logger.Debug("created quote relationship",
		zap.String("id", model.ID),
		zap.String("quoter_note_id", quote.QuoterNoteID),
		zap.String("target_note_id", quote.TargetNoteID),
		zap.String("quoter_id", quote.QuoterID))

	return nil
}

// GetMissingReplies returns a list of known missing replies in a thread
func (r *ObjectRepository) GetMissingReplies(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Get the thread sync record
	syncRecord, err := r.getThreadSyncRecord(ctx, statusID)
	if err != nil {
		r.logger.Error("failed to get thread sync record",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get thread sync record: %w", err)
	}

	if syncRecord == nil || len(syncRecord.MissingReplies) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	// Convert missing reply IDs to status search results
	missing := make([]*storage.StatusSearchResult, 0, len(syncRecord.MissingReplies))
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

	r.logger.Debug("retrieved missing replies",
		zap.String("status_id", statusID),
		zap.Int("count", len(missing)))

	return missing, nil
}

// getThreadSyncRecord retrieves the thread sync record for a status
func (r *ObjectRepository) getThreadSyncRecord(ctx context.Context, statusID string) (*models.ThreadSync, error) {
	var sync models.ThreadSync

	err := r.db.WithContext(ctx).Model(&sync).
		Where("PK", "=", fmt.Sprintf("THREAD_SYNC#%s", statusID)).
		Where("SK", "=", "METADATA").
		First(&sync)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get thread sync record: %w", err)
	}

	return &sync, nil
}

// MarkThreadAsSynced marks a thread as successfully synced
func (r *ObjectRepository) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	// Get existing sync record or create new one
	syncRecord, err := r.getThreadSyncRecord(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get thread sync record: %w", err)
	}

	if syncRecord == nil {
		// Create new sync record
		syncRecord = models.NewThreadSync(statusID)
	}

	// Mark as completed
	syncRecord.MarkCompleted()
	syncRecord.UpdateKeys()

	// Update or create the record - use Create with conditional to implement upsert
	if err := r.db.WithContext(ctx).Model(syncRecord).Create(); err != nil {
		r.logger.Error("failed to mark thread as synced",
			zap.String("status_id", statusID),
			zap.Error(err))
		return fmt.Errorf("failed to mark thread as synced: %w", err)
	}

	r.logger.Info("marked thread as synced",
		zap.String("status_id", statusID))

	return nil
}

// Additional status methods that might be missing

// GetStatus retrieves a status by ID (alias for GetObject)
func (r *ObjectRepository) GetStatus(ctx context.Context, statusID string) (any, error) {
	return r.GetObject(ctx, statusID)
}

// GetUserStatusCount counts the number of statuses by a user
func (r *ObjectRepository) GetUserStatusCount(ctx context.Context, userID string) (int, error) {
	// Use GSI1 to query by actor
	count, err := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("actor#%s", userID)).
		Count()

	if err != nil {
		r.logger.Error("failed to count user statuses",
			zap.String("user_id", userID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count user statuses: %w", err)
	}

	return int(count), nil
}

// GetStatusReplyCount counts replies to a specific status (alias for CountReplies)
func (r *ObjectRepository) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	return r.CountReplies(ctx, statusID)
}

// GetReplies retrieves replies to an object with pagination
func (r *ObjectRepository) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Ensure we have the full object URL for GSI6 key
	parentID := objectID
	if !strings.HasPrefix(objectID, "http") {
		parentID = fmt.Sprintf("%s/objects/%s", r.getDomainURL(), objectID)
	}

	// Use GSI6 to efficiently get replies
	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi6-index").
		Where("GSI6PK", "=", fmt.Sprintf("REPLIES#%s", parentID)).
		OrderBy("GSI6SK", "ASC"). // Oldest first
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var objects []models.Object
	err := query.All(&objects)
	if err != nil {
		r.logger.Error("failed to get replies",
			zap.String("object_id", objectID),
			zap.String("parent_id", parentID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get replies: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(objects) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = objects[limit-1].GSI6SK
		objects = objects[:limit] // Trim to requested limit
	}

	// Convert to ActivityPub objects
	result := make([]any, 0, len(objects))
	for _, objModel := range objects {
		apObj, err := r.modelToActivityPubObject(&objModel)
		if err != nil {
			r.logger.Warn("failed to convert reply object",
				zap.String("object_id", objModel.ID),
				zap.Error(err))
			continue
		}
		result = append(result, apObj)
	}

	r.logger.Debug("retrieved replies",
		zap.String("object_id", objectID),
		zap.Int("count", len(result)))

	return result, nextCursor, nil
}

// IncrementReplyCount increments the reply count for an object
func (r *ObjectRepository) IncrementReplyCount(ctx context.Context, objectID string) error {
	// This would typically be done atomically with DynamoDB's ADD operation
	// For now, we'll track this in the object itself or in a separate counter

	// Update the parent object's reply count (if it exists)
	var objModel models.Object
	err := r.db.WithContext(ctx).Model(&objModel).
		Where("PK", "=", fmt.Sprintf("object#%s", objectID)).
		Where("SK", "=", fmt.Sprintf("object#%s", objectID)).
		First(&objModel)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object doesn't exist, nothing to increment
			return nil
		}
		return fmt.Errorf("failed to get object for reply count increment: %w", err)
	}

	// Get the status to update reply count
	var status models.Status
	err = r.db.WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", fmt.Sprintf("status#%s", objectID)).
		Where("SK", "=", fmt.Sprintf("status#%s", objectID)).
		First(&status)
	if err != nil {
		if errors.IsNotFound(err) {
			// Status doesn't exist, nothing to increment
			return nil
		}
		return fmt.Errorf("failed to get status for reply count increment: %w", err)
	}

	// Increment the reply count
	status.ReplyCount++
	if err := status.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare status update: %w", err)
	}

	err = r.db.WithContext(ctx).Model(&status).Update()
	if err != nil {
		return fmt.Errorf("failed to update reply count: %w", err)
	}

	r.logger.Debug("incremented reply count for object",
		zap.String("object_id", objectID),
		zap.Int("new_count", status.ReplyCount))

	return nil
}

// GetReplyCount gets the reply count for a status
func (r *ObjectRepository) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	count, err := r.CountReplies(ctx, statusID)
	return int64(count), err
}

// SyncThreadFromRemote syncs a thread from a remote server
func (r *ObjectRepository) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	r.logger.Info("syncing thread from remote", zap.String("status_id", statusID))

	// Try to find the status locally first
	var objectModel models.Object
	err := r.db.WithContext(ctx).Model(&models.Object{}).
		Where("PK", "=", fmt.Sprintf("object#%s", statusID)).
		Where("SK", "=", fmt.Sprintf("object#%s", statusID)).
		First(&objectModel)

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("Failed to query local object for sync",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query local object: %w", err)
	}

	// If found locally, return it as a search result
	if err == nil {
		// Convert Object to StatusSearchResult
		published := objectModel.Published
		if published.IsZero() {
			published = objectModel.CreatedAt
		}

		return &storage.StatusSearchResult{
			StatusID:  statusID,
			Content:   objectModel.Content,
			AuthorID:  objectModel.AttributedTo,
			Published: published,
			Score:     1.0, // Local object gets high score
		}, nil
	}

	// If not found locally, this would normally trigger remote fetching
	// For now, return nil to indicate the thread couldn't be synced
	r.logger.Info("thread not found locally, remote fetching not yet implemented",
		zap.String("status_id", statusID))

	// Return nil instead of error to indicate "not found but not an error"
	// This allows callers to handle the case gracefully
	return nil, nil
}

// SyncMissingRepliesFromRemote syncs missing replies from remote servers
func (r *ObjectRepository) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Get current missing replies
	missing, err := r.GetMissingReplies(ctx, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get missing replies: %w", err)
	}

	r.logger.Info("syncing missing replies from remote",
		zap.String("status_id", statusID),
		zap.Int("missing_count", len(missing)))

	// Implement basic remote reply sync tracking
	syncRecord := &models.ReplySyncRecord{
		StatusID:    statusID,
		SyncAttempt: time.Now(),
		SyncResult:  "partial", // Mark as partial sync
	}
	syncRecord.UpdateKeys()

	// Store sync attempt record
	if err := syncRecord.BeforeCreate(); err == nil {
		if createErr := r.db.WithContext(ctx).Model(syncRecord).Create(); createErr != nil {
			// Log but don't fail the operation - sync record is just for tracking
			r.logger.Warn("failed to create reply sync record",
				zap.String("statusID", statusID),
				zap.Error(createErr))
		}
	}

	// Filter missing replies that are too old to fetch (> 30 days)
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	fetchableReplies := make([]*storage.StatusSearchResult, 0)

	for _, reply := range missing {
		// Check if reply is recent enough to attempt fetching
		if reply.Published.After(cutoff) {
			fetchableReplies = append(fetchableReplies, reply)
		}
	}

	r.logger.Info("filtered fetchable replies",
		zap.String("status_id", statusID),
		zap.Int("total_missing", len(missing)),
		zap.Int("fetchable", len(fetchableReplies)))

	// Return only fetchable replies for further processing by federation layer
	return fetchableReplies, nil
}

// GetThreadContext retrieves the thread context for a status
func (r *ObjectRepository) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	var context models.ThreadContext

	// Try to find by GSI1 (status lookup)
	err := r.db.WithContext(ctx).Model(&context).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Where("GSI1SK", "=", "THREAD").
		First(&context)

	if err != nil {
		if errors.IsNotFound(err) {
			// No thread context found, could be a root post
			r.logger.Debug("no thread context found",
				zap.String("status_id", statusID))
			return nil, nil
		}
		r.logger.Error("failed to get thread context",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get thread context: %w", err)
	}

	// Convert to storage.ThreadContext - note that this has a different structure
	// For now, return an empty context since the interface has changed
	result := &storage.ThreadContext{
		StatusID:    statusID,
		Ancestors:   []string{},
		Descendants: []string{},
	}

	r.logger.Debug("retrieved thread context",
		zap.String("status_id", statusID),
		zap.Int("ancestors", len(result.Ancestors)),
		zap.Int("descendants", len(result.Descendants)))

	return result, nil
}

// GetQuotesForNote retrieves quotes for a specific note with pagination
func (r *ObjectRepository) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Use GSI1 to find quotes where GSI1PK = QUOTED#<noteID>
	query := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("QUOTED#%s", noteID)).
		OrderBy("GSI1SK", "DESC"). // Newest first
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var quoteModels []models.QuoteRelationship
	err := query.All(&quoteModels)
	if err != nil {
		r.logger.Error("failed to get quotes for note",
			zap.String("note_id", noteID),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get quotes for note: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(quoteModels) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = quoteModels[limit-1].GSI1SK
		quoteModels = quoteModels[:limit] // Trim to requested limit
	}

	// Convert to storage.QuoteRelationship
	quotes := make([]*storage.QuoteRelationship, len(quoteModels))
	for i, model := range quoteModels {
		quotes[i] = &storage.QuoteRelationship{
			ID:             model.ID,
			QuoterNoteID:   model.QuoterNoteID,
			TargetNoteID:   model.TargetNoteID,
			QuoterID:       model.QuoterID,
			TargetAuthorID: model.TargetAuthorID,
			Timestamp:      model.Timestamp,
			Withdrawn:      model.Withdrawn,
			WithdrawnAt:    model.WithdrawnAt,
		}
	}

	r.logger.Debug("retrieved quotes for note",
		zap.String("note_id", noteID),
		zap.Int("count", len(quotes)))

	return quotes, nextCursor, nil
}

// IsQuoted checks if a note is quoted by a specific actor
func (r *ObjectRepository) IsQuoted(ctx context.Context, actorID, noteID string) (bool, error) {
	var quote models.QuoteRelationship

	err := r.db.WithContext(ctx).Model(&quote).
		Where("PK", "=", fmt.Sprintf("QUOTE#%s", actorID)).
		Where("SK", "=", fmt.Sprintf("TARGET#%s", noteID)).
		First(&quote)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check if quoted",
			zap.String("actor_id", actorID),
			zap.String("note_id", noteID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check if quoted: %w", err)
	}

	// Check if not withdrawn
	return !quote.Withdrawn, nil
}

// WithdrawQuote withdraws a quote by marking it as withdrawn
func (r *ObjectRepository) WithdrawQuote(ctx context.Context, quoteNoteID string) error {
	// Find the quote relationship by quoter note ID
	var quote models.QuoteRelationship

	// We need to find the quote by the quoter note ID, which could be in GSI2
	err := r.db.WithContext(ctx).Model(&quote).
		Index("GSI2").
		Where("GSI2PK", "=", fmt.Sprintf("QUOTER_NOTE#%s", quoteNoteID)).
		First(&quote)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("quote not found for withdrawal",
				zap.String("quote_note_id", quoteNoteID))
			return nil
		}
		r.logger.Error("failed to find quote for withdrawal",
			zap.String("quote_note_id", quoteNoteID),
			zap.Error(err))
		return fmt.Errorf("failed to find quote for withdrawal: %w", err)
	}

	// Mark as withdrawn
	now := time.Now()
	quote.Withdrawn = true
	quote.WithdrawnAt = &now

	// Update the quote
	if err := r.db.WithContext(ctx).Model(&quote).
		Where("PK", "=", quote.PK).
		Where("SK", "=", quote.SK).
		Update(); err != nil {
		r.logger.Error("failed to withdraw quote",
			zap.String("quote_note_id", quoteNoteID),
			zap.String("quote_id", quote.ID),
			zap.Error(err))
		return fmt.Errorf("failed to withdraw quote: %w", err)
	}

	r.logger.Info("withdrew quote",
		zap.String("quote_note_id", quoteNoteID),
		zap.String("quote_id", quote.ID))

	return nil
}

// WithdrawStatusFromQuotes withdraws a status from being quoted
func (r *ObjectRepository) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	// This would mark the status as not quotable
	// For now, just log the action
	r.logger.Info("withdrawing status from quotes",
		zap.String("status_id", statusID))

	// Get or create status metadata
	metadata, err := r.getOrCreateStatusMetadata(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status metadata: %w", err)
	}

	// Mark as withdrawn from quotes
	metadata.WithdrawFromQuotes()

	// Update the metadata
	if err := metadata.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare metadata update: %w", err)
	}

	err = r.db.WithContext(ctx).Model(metadata).Update()
	if err != nil {
		return fmt.Errorf("failed to update status metadata: %w", err)
	}

	r.logger.Info("status withdrawn from quotes",
		zap.String("status_id", statusID))

	return nil
}

// UpdateQuotePermissions updates the quote permissions for a status
func (r *ObjectRepository) UpdateQuotePermissions(ctx context.Context, statusID string, permissions *storage.QuotePermissions) error {
	r.logger.Info("updating quote permissions",
		zap.String("status_id", statusID))

	// Get or create status metadata
	metadata, err := r.getOrCreateStatusMetadata(ctx, statusID)
	if err != nil {
		return fmt.Errorf("failed to get status metadata: %w", err)
	}

	// Update quote permissions
	metadata.AllowQuotes = permissions.AllowPublic || permissions.AllowFollowers || permissions.AllowMentioned

	// Set quote type based on permissions
	if permissions.AllowPublic {
		metadata.QuoteType = models.VisibilityPublic
	} else if permissions.AllowFollowers {
		metadata.QuoteType = "followers"
	} else if permissions.AllowMentioned {
		metadata.QuoteType = "mentioned"
	} else {
		metadata.QuoteType = VisibilityDisabled
		metadata.AllowQuotes = false
	}

	// Serialize and store permissions as JSON
	permissionsJSON := fmt.Sprintf(`{"allow_public":%t,"allow_followers":%t,"allow_mentioned":%t,"block_list":[]}`,
		permissions.AllowPublic, permissions.AllowFollowers, permissions.AllowMentioned)
	metadata.QuotePermissions = permissionsJSON

	// Update the metadata
	if err := metadata.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare metadata update: %w", err)
	}

	err = r.db.WithContext(ctx).Model(metadata).Update()
	if err != nil {
		return fmt.Errorf("failed to update quote permissions: %w", err)
	}

	r.logger.Info("updated quote permissions",
		zap.String("status_id", statusID),
		zap.String("quote_type", metadata.QuoteType))

	return nil
}

// IsQuoteAllowed checks if a quote is allowed for a status by a quoter
func (r *ObjectRepository) IsQuoteAllowed(ctx context.Context, statusID, quoterID string) (bool, error) {
	// Get status metadata to check quote permissions
	metadata, err := r.getStatusMetadata(ctx, statusID)
	if err != nil {
		if errors.IsNotFound(err) {
			// No metadata means default to restrictive (quotes disabled)
			r.logger.Debug("no metadata found, defaulting to disabled quotes",
				zap.String("status_id", statusID),
				zap.String("quoter_id", quoterID))
			return false, nil
		}
		return false, fmt.Errorf("failed to get status metadata: %w", err)
	}

	// Check if quotes are allowed at all
	if !metadata.IsQuotable() {
		r.logger.Debug("quotes not allowed for status",
			zap.String("status_id", statusID),
			zap.String("quoter_id", quoterID),
			zap.String("quote_type", metadata.QuoteType))
		return false, nil
	}

	// If public quotes are allowed, allow all quotes
	if metadata.IsPubliclyQuotable() {
		return true, nil
	}

	// Handle permission-based quote types
	switch metadata.QuoteType {
	case models.VisibilityPublic:
		return true, nil
	case "followers":
		// Check if quoter follows the original author
		return r.checkFollowerPermission(ctx, statusID, quoterID)
	case "mentioned":
		// Check if quoter is mentioned in the original status
		return r.checkMentionPermission(ctx, statusID, quoterID)
	case "disabled":
		return false, nil
	default:
		// Unknown type, default to restrictive
		r.logger.Debug("unknown quote type, defaulting to disabled",
			zap.String("status_id", statusID),
			zap.String("quoter_id", quoterID),
			zap.String("quote_type", metadata.QuoteType))
		return false, nil
	}
}

// GetQuoteType returns the quote type for a status
func (r *ObjectRepository) GetQuoteType(ctx context.Context, statusID string) (string, error) {
	// Get status metadata to check quote type
	metadata, err := r.getStatusMetadata(ctx, statusID)
	if err != nil {
		if errors.IsNotFound(err) {
			// No metadata means default to disabled (restrictive)
			r.logger.Debug("no metadata found for status, defaulting to disabled quotes",
				zap.String("status_id", statusID))
			return "disabled", nil
		}
		return "", fmt.Errorf("failed to get status metadata: %w", err)
	}

	r.logger.Debug("getting quote type",
		zap.String("status_id", statusID),
		zap.String("quote_type", metadata.QuoteType))

	return metadata.QuoteType, nil
}

// IsWithdrawnFromQuotes checks if a status is withdrawn from quotes
func (r *ObjectRepository) IsWithdrawnFromQuotes(ctx context.Context, statusID string) (bool, error) {
	// Get status metadata to check withdrawal status
	metadata, err := r.getStatusMetadata(ctx, statusID)
	if err != nil {
		if errors.IsNotFound(err) {
			// No metadata means not withdrawn
			return false, nil
		}
		return false, fmt.Errorf("failed to get status metadata: %w", err)
	}

	r.logger.Debug("checking if withdrawn from quotes",
		zap.String("status_id", statusID),
		zap.Bool("withdrawn", metadata.WithdrawnFromQuotes))

	return metadata.WithdrawnFromQuotes, nil
}

// GetQuotesOfStatus retrieves quotes of a specific status
func (r *ObjectRepository) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Use GSI1 to find quotes where GSI1PK = QUOTED#<statusID>
	query := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("QUOTED#%s", statusID)).
		OrderBy("GSI1SK", "DESC"). // Newest first
		Limit(limit)

	var quoteModels []models.QuoteRelationship
	err := query.All(&quoteModels)
	if err != nil {
		r.logger.Error("failed to get quotes of status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get quotes of status: %w", err)
	}

	// Convert to status search results
	quotes := make([]*storage.StatusSearchResult, 0, len(quoteModels))
	for _, model := range quoteModels {
		if !model.Withdrawn { // Only include non-withdrawn quotes
			quote := &storage.StatusSearchResult{
				StatusID:  model.QuoterNoteID,
				Content:   "", // Would need to fetch the actual content
				AuthorID:  model.QuoterID,
				Published: model.Timestamp,
				Score:     1.0,
			}
			quotes = append(quotes, quote)
		}
	}

	r.logger.Debug("retrieved quotes of status",
		zap.String("status_id", statusID),
		zap.Int("count", len(quotes)))

	return quotes, nil
}

// Helper methods for status metadata

// getStatusMetadata retrieves status metadata for a given status ID
func (r *ObjectRepository) getStatusMetadata(ctx context.Context, statusID string) (*models.StatusMetadata, error) {
	var metadata models.StatusMetadata
	metadata.StatusID = statusID
	metadata.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.StatusMetadata{}).
		Where("PK", "=", metadata.PK).
		Where("SK", "=", metadata.SK).
		First(&metadata)

	if err != nil {
		return nil, err
	}

	return &metadata, nil
}

// getOrCreateStatusMetadata gets existing metadata or creates new with defaults
func (r *ObjectRepository) getOrCreateStatusMetadata(ctx context.Context, statusID string) (*models.StatusMetadata, error) {
	// Try to get existing metadata first
	metadata, err := r.getStatusMetadata(ctx, statusID)
	if err == nil {
		return metadata, nil
	}

	// If not found, create new metadata with defaults
	if errors.IsNotFound(err) {
		metadata = models.NewStatusMetadata(statusID)

		// Save the new metadata
		if err := metadata.BeforeCreate(); err != nil {
			return nil, fmt.Errorf("failed to prepare metadata creation: %w", err)
		}

		err = r.db.WithContext(ctx).Model(metadata).Create()
		if err != nil {
			return nil, fmt.Errorf("failed to create status metadata: %w", err)
		}

		return metadata, nil
	}

	// Other error occurred
	return nil, err
}

// checkFollowerPermission checks if quoter follows the original status author
func (r *ObjectRepository) checkFollowerPermission(ctx context.Context, statusID, quoterID string) (bool, error) {
	// Get the original status to find the author
	status, err := r.GetObject(ctx, statusID)
	if err != nil {
		r.logger.Error("failed to get status for follower check",
			zap.String("status_id", statusID),
			zap.String("quoter_id", quoterID),
			zap.Error(err))
		return false, nil // Default to deny on error
	}

	// Extract author ID from the status
	var authorID string
	if statusMap, ok := status.(map[string]any); ok {
		if attr, ok := statusMap["attributedTo"].(string); ok {
			authorID = attr
		}
	} else if note, ok := status.(*activitypub.Note); ok {
		authorID = note.AttributedTo
	}

	if authorID == "" {
		r.logger.Error("could not extract author from status",
			zap.String("status_id", statusID),
			zap.String("quoter_id", quoterID))
		return false, nil // Default to deny if we can't find author
	}

	// Check if quoter follows the author
	isFollowing, err := r.accountRepo.IsFollowing(ctx, quoterID, authorID)
	if err != nil {
		r.logger.Error("failed to check following relationship",
			zap.String("status_id", statusID),
			zap.String("quoter_id", quoterID),
			zap.String("author_id", authorID),
			zap.Error(err))
		return false, nil // Default to deny on error
	}

	r.logger.Debug("checked follower quote permission",
		zap.String("status_id", statusID),
		zap.String("quoter_id", quoterID),
		zap.String("author_id", authorID),
		zap.Bool("is_following", isFollowing),
		zap.Bool("allowed", isFollowing))

	return isFollowing, nil
}

// checkMentionPermission checks if quoter is mentioned in the original status
func (r *ObjectRepository) checkMentionPermission(ctx context.Context, statusID, quoterID string) (bool, error) {
	// Get the original status to check mentions
	status, err := r.GetObject(ctx, statusID)
	if err != nil {
		r.logger.Error("failed to get status for mention check",
			zap.String("status_id", statusID),
			zap.String("quoter_id", quoterID),
			zap.Error(err))
		return false, nil // Default to deny on error
	}

	// Parse mentions from the status
	mentions := r.extractMentions(status)
	
	// Check if quoter is in the mentions
	for _, mention := range mentions {
		if mention == quoterID {
			r.logger.Debug("quoter found in mentions, allowing quote",
				zap.String("status_id", statusID),
				zap.String("quoter_id", quoterID))
			return true, nil
		}
	}

	r.logger.Debug("quoter not found in mentions, denying quote",
		zap.String("status_id", statusID),
		zap.String("quoter_id", quoterID),
		zap.Strings("mentions", mentions))

	return false, nil
}

// extractMentions extracts mentioned user IDs from a status object
func (r *ObjectRepository) extractMentions(status any) []string {
	var mentions []string

	// Handle different status types
	if statusMap, ok := status.(map[string]any); ok {
		// Try to get tags from the map
		if tagsInterface, ok := statusMap["tag"]; ok {
			mentions = r.parseMentionsFromTags(tagsInterface)
		}
	} else if note, ok := status.(*activitypub.Note); ok {
		// Extract mentions from ActivityPub Note tags
		for _, tag := range note.Tag {
			if tag.Type == TagTypeMention && tag.Href != "" {
				mentions = append(mentions, tag.Href)
			}
		}
	}

	return mentions
}

// parseMentionsFromTags parses mentions from various tag formats
func (r *ObjectRepository) parseMentionsFromTags(tagsInterface any) []string {
	var mentions []string

	// Handle different tag formats
	switch tags := tagsInterface.(type) {
	case []any:
		for _, tagInterface := range tags {
			if tagMap, ok := tagInterface.(map[string]any); ok {
				if tagType, ok := tagMap["type"].(string); ok && tagType == "Mention" {
					if href, ok := tagMap["href"].(string); ok && href != "" {
						mentions = append(mentions, href)
					}
				}
			}
		}
	case []activitypub.Tag:
		for _, tag := range tags {
			if tag.Type == TagTypeMention && tag.Href != "" {
				mentions = append(mentions, tag.Href)
			}
		}
	case string:
		// Handle JSON string format - try to unmarshal
		var tagSlice []activitypub.Tag
		if err := json.Unmarshal([]byte(tags), &tagSlice); err == nil {
			for _, tag := range tagSlice {
				if tag.Type == TagTypeMention && tag.Href != "" {
					mentions = append(mentions, tag.Href)
				}
			}
		}
	}

	return mentions
}
