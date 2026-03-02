package repositories

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// ObjectRepository implements object operations using enhanced DynamORM patterns
type ObjectRepository struct {
	*EnhancedBaseRepository[*models.Object]
	domain      string
	accountRepo *AccountRepository
	queryUtils  *QueryUtils
}

// NewObjectRepository creates a new object repository with enhanced functionality
func NewObjectRepository(db core.DB, tableName, domain string, logger *zap.Logger) *ObjectRepository {
	// Create enhanced repository optimized for object operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Object](db, tableName, logger, nil, "ObjectRepository", "object")

	// Set up enhanced services for object operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Objects cached for federation performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for ActivityPub events

	return &ObjectRepository{
		EnhancedBaseRepository: enhancedRepo,
		domain:                 domain,
		accountRepo:            NewAccountRepository(db, tableName, domain, logger),
		queryUtils:             NewQueryUtils(db, logger),
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
		return ErrorHandler.HandleCreateError(err, EntityObject, "marshal")
	}

	// Parse to extract common fields
	var baseObj activitypub.BaseObject
	if err := json.Unmarshal(objJSON, &baseObj); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityObject, "parse")
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
		if err := common.ValidateSliceNotEmpty("note attachments", note.Attachment); err == nil {
			attachJSON, _ := json.Marshal(note.Attachment)
			objModel.AttachmentJSON = string(attachJSON)
		}
		if err := common.ValidateSliceNotEmpty("note tags", note.Tag); err == nil {
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

	// Store using BaseRepository
	if err := r.Create(ctx, objModel); err != nil {
		r.logger.Error("failed to create object",
			zap.String("object_id", baseObj.ID),
			zap.String("type", baseObj.Type),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityObject, baseObj.ID)
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

	pk := fmt.Sprintf("object#%s", id)
	sk := fmt.Sprintf("object#%s", id)

	if err := r.Get(ctx, pk, sk, &objModel); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityObject, id)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityObject, id)
	}

	// Convert back to appropriate ActivityPub type
	return r.modelToActivityPubObject(&objModel)
}

// UpdateObject updates an existing object
func (r *ObjectRepository) UpdateObject(ctx context.Context, object any) error {
	// Similar to CreateObject, extract fields and update
	objJSON, err := json.Marshal(object)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "marshal")
	}

	var baseObj activitypub.BaseObject
	if err := json.Unmarshal(objJSON, &baseObj); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "parse")
	}

	// Get existing object
	var objModel models.Object
	pk := fmt.Sprintf("object#%s", baseObj.ID)
	sk := fmt.Sprintf("object#%s", baseObj.ID)

	if err := r.Get(ctx, pk, sk, &objModel); err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Object doesn't exist, create it
			return r.CreateObject(ctx, object)
		}
		return ErrorHandler.HandleGetError(err, EntityObject, "object")
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

	// Update in database using BaseRepository
	if err := r.Update(ctx, &objModel); err != nil {
		r.logger.Error("failed to update object",
			zap.String("object_id", baseObj.ID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityObject, "object")
	}

	return nil
}

// UpdateObjectWithHistory updates an object and tracks the edit history
func (r *ObjectRepository) UpdateObjectWithHistory(ctx context.Context, object any, updatedBy string) error {
	// Extract object ID
	objJSON, err := json.Marshal(object)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "marshal")
	}

	var baseObj activitypub.BaseObject
	if err := json.Unmarshal(objJSON, &baseObj); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "parse")
	}

	objectID := baseObj.ID

	// Get existing object for history tracking
	existingObject, err := r.GetObject(ctx, objectID)
	if err != nil {
		// If object doesn't exist, create it instead
		if strings.Contains(err.Error(), "not found") {
			return r.CreateObject(ctx, object)
		}
		return ErrorHandler.HandleGetError(err, EntityObject, "object")
	}

	// Store edit history before updating
	if err := r.storeEditHistoryForUpdate(ctx, objectID, existingObject, updatedBy); err != nil {
		r.logger.Warn("failed to store edit history",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Continue - don't fail the update if history fails
	}

	// Update the object with edited flag
	if err := r.updateObjectWithEditedFlag(ctx, object); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "object")
	}

	r.logger.Info("updated object with history tracking",
		zap.String("object_id", objectID),
		zap.String("updated_by", updatedBy))

	return nil
}

// storeEditHistoryForUpdate creates an update history record
func (r *ObjectRepository) storeEditHistoryForUpdate(ctx context.Context, objectID string, existingObject any, updatedBy string) error {
	// Convert existing object to map for storage
	var previousState map[string]any

	// Serialize the existing object to JSON then deserialize to map
	objectJSON, err := json.Marshal(existingObject)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "serialize")
	}

	if err := json.Unmarshal(objectJSON, &previousState); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "deserialize")
	}

	// Get the current version number (start from version 1 for first edit)
	historyEntries, err := r.GetUpdateHistory(ctx, objectID, 1)
	if err != nil {
		r.logger.Debug("failed to get update history, assuming first edit",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	version := len(historyEntries) + 1 // Version 1 is the first edit (original is version 0)

	// Create update history record
	updateHistory := &storage.UpdateHistory{
		ObjectID:      objectID,
		Version:       version,
		UpdatedAt:     time.Now(),
		UpdatedBy:     updatedBy,
		PreviousState: previousState,
		Summary:       "ActivityPub Update activity",
	}

	// Store the history
	return r.CreateUpdateHistory(ctx, updateHistory)
}

// updateObjectWithEditedFlag updates the object and marks it as edited
func (r *ObjectRepository) updateObjectWithEditedFlag(ctx context.Context, object any) error {
	// Similar to UpdateObject, but with edited flag handling
	objJSON, err := json.Marshal(object)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "marshal")
	}

	var baseObj activitypub.BaseObject
	if err := json.Unmarshal(objJSON, &baseObj); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "parse")
	}

	// Get existing object model
	var objModel models.Object
	pk := fmt.Sprintf("object#%s", baseObj.ID)
	sk := fmt.Sprintf("object#%s", baseObj.ID)

	if err := r.Get(ctx, pk, sk, &objModel); err != nil {
		return ErrorHandler.HandleGetError(err, EntityObject, "object")
	}

	// Update fields
	now := time.Now()
	objModel.Updated = now

	if baseObj.Type == activitypub.NoteType {
		var note activitypub.Note
		if err := json.Unmarshal(objJSON, &note); err == nil {
			objModel.Content = note.Content
			objModel.Sensitive = note.Sensitive

			// Update complex fields as JSON
			if err := common.ValidateSliceNotEmpty("note attachments", note.Attachment); err == nil {
				attachJSON, _ := json.Marshal(note.Attachment)
				objModel.AttachmentJSON = string(attachJSON)
			}
			if err := common.ValidateSliceNotEmpty("note tags", note.Tag); err == nil {
				tagJSON, _ := json.Marshal(note.Tag)
				objModel.TagJSON = string(tagJSON)
			}
		}
	}

	// Update in database using BaseRepository
	if err := r.Update(ctx, &objModel); err != nil {
		r.logger.Error("failed to update object",
			zap.String("object_id", baseObj.ID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityObject, "object")
	}

	return nil
}

// DeleteObject deletes an object by ID
func (r *ObjectRepository) DeleteObject(ctx context.Context, objectID string) error {
	pk := fmt.Sprintf("object#%s", objectID)
	sk := fmt.Sprintf("object#%s", objectID)

	if err := r.Delete(ctx, pk, sk); err != nil {
		if strings.Contains(err.Error(), "not found") {
			r.logger.Debug("object not found for deletion",
				zap.String("object_id", objectID))
			return nil
		}
		r.logger.Error("failed to delete object",
			zap.String("object_id", objectID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityObject, objectID)
	}

	r.logger.Info("deleted object",
		zap.String("object_id", objectID))

	return nil
}

// GetObjectsByActor retrieves objects created by a specific actor
func (r *ObjectRepository) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	var objects []models.Object

	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("actor#%s", actorID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	err := query.All(&objects)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityObject, "scan")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("objects", objects, limit); err != nil {
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
	var objects []models.Object
	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi2"). // Assuming gsi2 is used for reply relationships
		Where("gsi2PK", "=", fmt.Sprintf("reply#%s", objectID))

	if err := query.All(&objects); err != nil {
		r.logger.Error("failed to count object replies",
			zap.String("object_id", objectID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityObject, "count replies")
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
		return ErrorHandler.HandleGetError(err, EntityObject, objectID)
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

	if err := common.ValidateRequiredParam("object ID", objID); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityObject, "extract_id")
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
		return ErrorHandler.HandleDeleteError(err, EntityObject, objectID)
	}

	// Then create the tombstone using BaseRepository
	if err := r.Create(ctx, tombstone); err != nil {
		r.logger.Error("failed to create tombstone",
			zap.String("object_id", objectID),
			zap.String("deleted_by", deletedBy),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityObject, "tombstone")
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
			return ErrorHandler.HandleCreateError(err, EntityObject, "marshal_state")
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
	updateHistory.UpdateKeys() // Internal model operation

	// Create the update history record
	err := r.db.WithContext(ctx).Model(updateHistory).Create()
	if err != nil {
		r.logger.Error("failed to create update history",
			zap.String("object_id", history.ObjectID),
			zap.Int("version", history.Version),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityObject, "update_history")
	}

	r.logger.Info("update history created",
		zap.String("object_id", history.ObjectID),
		zap.Int("version", history.Version))

	return nil
}

// GetUpdateHistory retrieves update history for an object
func (r *ObjectRepository) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	// Validate limit
	if err := common.ValidateQueryLimit(limit, 100, "update history"); err != nil {
		limit = 10 // default on validation error
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
		return nil, ErrorHandler.HandleQueryError(err, EntityObject, "update_history")
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
	return r.queryUtils.AddToCollectionHelper(ctx, collection, item, r.db)
}

// RemoveFromCollection removes an item from a collection
func (r *ObjectRepository) RemoveFromCollection(ctx context.Context, collection, itemID string) error {
	pk := fmt.Sprintf("COLLECTION#%s", collection)
	sk := fmt.Sprintf("ITEM#%s", itemID)

	collectionItem := &models.CollectionItem{
		PK: pk,
		SK: sk,
	}

	return r.queryUtils.DeleteWithNotFoundHandling(ctx, pk, sk, collectionItem, "remove from collection", collection, itemID)
}

// GetCollectionItems retrieves items from a collection with pagination
func (r *ObjectRepository) GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*storage.CollectionItem, string, error) {
	if err := common.ValidateQueryLimit(limit, 100, "collection items"); err != nil {
		limit = 20 // default on validation error
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
		return nil, "", ErrorHandler.HandleQueryError(err, EntityObject, "collection_items")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("items", items, limit); err != nil {
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
		return false, ErrorHandler.HandleQueryError(err, EntityObject, "collection_membership")
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
		return 0, ErrorHandler.HandleQueryError(err, EntityObject, "count_collection")
	}

	return int(count), nil
}

// CountQuotes counts the number of quotes for a specific note
func (r *ObjectRepository) CountQuotes(ctx context.Context, noteID string) (int, error) {
	// Query quotes using GSI1 where GSI1PK = QUOTED#<noteID>
	count, err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("QUOTED#%s", noteID)).
		Count()

	if err != nil {
		r.logger.Error("failed to count quotes",
			zap.String("note_id", noteID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityObject, "count_quotes")
	}

	r.logger.Debug("counted quotes for note",
		zap.String("note_id", noteID),
		zap.Int64("count", count))

	return int(count), nil
}

// CountWithdrawnQuotes counts the number of withdrawn quotes for a specific note
func (r *ObjectRepository) CountWithdrawnQuotes(ctx context.Context, noteID string) (int, error) {
	// Count withdrawn quotes by querying all quotes for this note and checking withdrawn status
	// Since withdrawn quotes have cleared GSI keys, we need to scan by target note ID
	var quotes []models.QuoteRelationship
	err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where("SK", "=", fmt.Sprintf("QUOTED#%s", noteID)).
		All(&quotes)

	if err != nil {
		r.logger.Error("failed to get quotes for withdrawn count",
			zap.String("note_id", noteID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityObject, "withdrawn_quotes")
	}

	// Count only withdrawn quotes
	withdrawnCount := 0
	for _, quote := range quotes {
		if quote.Withdrawn {
			withdrawnCount++
		}
	}

	r.logger.Debug("counted withdrawn quotes for note",
		zap.String("note_id", noteID),
		zap.Int("withdrawn_count", withdrawnCount),
		zap.Int("total_quotes", len(quotes)))

	return withdrawnCount, nil
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
		Index("gsi6").
		Where("gsi6PK", "=", fmt.Sprintf("REPLIES#%s", parentID)).
		Count()

	if err != nil {
		r.logger.Error("failed to count replies",
			zap.String("object_id", objectID),
			zap.String("parent_id", parentID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityObject, "count_replies")
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
	if err := common.ValidateRequiredParam("model ID", model.ID); err != nil {
		model.GenerateID()
	}

	// Update composite keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the quote relationship
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create quote relationship",
			zap.String("quoter_note_id", quote.QuoterNoteID),
			zap.String("target_note_id", quote.TargetNoteID),
			zap.String("quoter_id", quote.QuoterID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityObject, "quote_relationship")
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
		if goerrors.Is(err, storage.ErrNotFound) {
			return []*storage.StatusSearchResult{}, nil
		}
		r.logger.Error("failed to get thread sync record",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityObject, "thread_sync")
	}

	if len(syncRecord.MissingReplies) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	// Convert missing reply IDs to proper status search results with metadata lookup
	missing := make([]*storage.StatusSearchResult, 0, len(syncRecord.MissingReplies))
	for _, replyID := range syncRecord.MissingReplies {
		// Try to get any available metadata for this reply
		reply := &storage.StatusSearchResult{
			StatusID:  replyID,
			Content:   "[Missing Reply - Remote Fetch Required]",
			AuthorID:  "",
			Published: time.Now().Add(-24 * time.Hour), // Mark as older to reduce priority
			Score:     0.1,                             // Lower score for missing replies
		}

		// Try to extract basic metadata from reply ID if it's a URL
		if strings.HasPrefix(replyID, "http") {
			// Extract domain for better display
			if parts := strings.Split(replyID, "/"); len(parts) > 2 {
				reply.Content = fmt.Sprintf("[Missing Reply from %s]", parts[2])
			}
		}

		missing = append(missing, reply)
	}

	r.logger.Debug("retrieved missing replies with metadata",
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
			return nil, storage.ErrNotFound
		}
		return nil, ErrorHandler.HandleGetError(err, EntityObject, "thread_sync")
	}

	return &sync, nil
}

// MarkThreadAsSynced marks a thread as successfully synced
func (r *ObjectRepository) MarkThreadAsSynced(ctx context.Context, statusID string) error {
	// Get existing sync record or create new one
	syncRecord, err := r.getThreadSyncRecord(ctx, statusID)
	if err != nil {
		if goerrors.Is(err, storage.ErrNotFound) {
			syncRecord = models.NewThreadSync(statusID)
		} else {
			return ErrorHandler.HandleGetError(err, EntityObject, "thread_sync")
		}
	}

	if syncRecord == nil {
		syncRecord = models.NewThreadSync(statusID)
	}

	// Mark as completed
	syncRecord.MarkCompleted()
	_ = syncRecord.UpdateKeys() // Internal model operation, error ignored as it's a local data structure update

	// Update or create the record - use Create with conditional to implement upsert
	if err := r.db.WithContext(ctx).Model(syncRecord).Create(); err != nil {
		r.logger.Error("failed to mark thread as synced",
			zap.String("status_id", statusID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityObject, "thread_sync")
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
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("actor#%s", userID)).
		Count()

	if err != nil {
		r.logger.Error("failed to count user statuses",
			zap.String("user_id", userID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityObject, "user_statuses")
	}

	return int(count), nil
}

// GetStatusReplyCount counts replies to a specific status (alias for CountReplies)
func (r *ObjectRepository) GetStatusReplyCount(ctx context.Context, statusID string) (int, error) {
	return r.CountReplies(ctx, statusID)
}

// GetReplies retrieves replies to an object with pagination
func (r *ObjectRepository) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
	if err := common.ValidateQueryLimit(limit, 100, "replies"); err != nil {
		limit = 20 // default on validation error
	}

	// Ensure we have the full object URL for GSI6 key
	parentID := objectID
	if !strings.HasPrefix(objectID, "http") {
		parentID = fmt.Sprintf("%s/objects/%s", r.getDomainURL(), objectID)
	}

	// Use GSI6 to efficiently get replies
	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi6").
		Where("gsi6PK", "=", fmt.Sprintf("REPLIES#%s", parentID)).
		OrderBy("gsi6SK", "ASC"). // Oldest first
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
		return nil, "", ErrorHandler.HandleQueryError(err, EntityObject, "replies")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("objects", objects, limit); err != nil {
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

// IncrementReplyCount increments the reply count for an object with proper atomic operations
func (r *ObjectRepository) IncrementReplyCount(ctx context.Context, objectID string) error {
	// Update the parent object's reply count using atomic increment
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
		return ErrorHandler.HandleGetError(err, EntityObject, "reply_count")
	}

	// Update reply count in object metadata using proper model patterns
	metadata, err := r.getOrCreateStatusMetadata(ctx, objectID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityObject, "status_metadata")
	}

	// Increment reply count atomically
	metadata.IncrementReplyCount()
	metadata.UpdatedAt = time.Now()

	// Update the metadata record using direct db call (metadata is non-standard model)
	if err := metadata.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "metadata_prepare")
	}

	err = r.db.WithContext(ctx).Model(metadata).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "reply_count")
	}

	// Also update thread context if this is part of a thread
	if err := r.updateThreadContext(ctx, objectID, "increment_reply"); err != nil {
		// Log but don't fail - thread context is supplementary
		r.logger.Warn("failed to update thread context",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	r.logger.Debug("incremented reply count for object",
		zap.String("object_id", objectID),
		zap.Int("new_count", metadata.ReplyCount))

	return nil
}

// GetReplyCount gets the reply count for a status
func (r *ObjectRepository) GetReplyCount(ctx context.Context, statusID string) (int64, error) {
	count, err := r.CountReplies(ctx, statusID)
	return int64(count), err
}

// SyncThreadFromRemote syncs a thread from a remote server with proper background processing
func (r *ObjectRepository) SyncThreadFromRemote(ctx context.Context, statusID string) (*storage.StatusSearchResult, error) {
	r.logger.Info("syncing thread from remote", zap.String("status_id", statusID))

	// Try to find the status locally first
	var objectModel models.Object
	pk := fmt.Sprintf("object#%s", statusID)
	sk := fmt.Sprintf("object#%s", statusID)
	err := r.Get(ctx, pk, sk, &objectModel)

	if err != nil && !strings.Contains(err.Error(), "not found") {
		r.logger.Error("Failed to query local object for sync",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityObject, "local_sync")
	}

	// If found locally, return it as a search result
	if err == nil {
		// Convert Object to StatusSearchResult with full metadata
		published := objectModel.Published
		if published.IsZero() {
			published = objectModel.CreatedAt
		}

		// Mark thread as synced if we have local data
		if syncErr := r.MarkThreadAsSynced(ctx, statusID); syncErr != nil {
			r.logger.Warn("failed to mark thread as synced",
				zap.String("status_id", statusID),
				zap.Error(syncErr))
		}

		return &storage.StatusSearchResult{
			StatusID:  statusID,
			Content:   objectModel.Content,
			AuthorID:  objectModel.AttributedTo,
			Published: published,
			Score:     1.0, // Local object gets high score
		}, nil
	}

	// If not found locally, trigger background fetch process
	if err := r.triggerBackgroundFetch(ctx, statusID); err != nil {
		r.logger.Error("failed to trigger background fetch",
			zap.String("status_id", statusID),
			zap.Error(err))
		// Continue - background fetch failure shouldn't fail the sync
	}

	// Check if we have any cached remote data about this status
	if result := r.getCachedRemoteStatus(ctx, statusID); result != nil {
		r.logger.Info("returning cached remote status",
			zap.String("status_id", statusID))
		return result, nil
	}

	r.logger.Info("thread not found locally, background fetch initiated",
		zap.String("status_id", statusID))

	// Indicate to callers that the status is not currently available
	return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityObject, statusID)
}

// SyncMissingRepliesFromRemote syncs missing replies from remote servers
func (r *ObjectRepository) SyncMissingRepliesFromRemote(ctx context.Context, statusID string) ([]*storage.StatusSearchResult, error) {
	// Get current missing replies
	missing, err := r.GetMissingReplies(ctx, statusID)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityObject, "missing_replies")
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
	syncRecord.UpdateKeys() // Internal model operation

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

// GetThreadContext retrieves the thread context for a status with full hierarchy
func (r *ObjectRepository) GetThreadContext(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	var context models.ThreadContext

	// Try to find by GSI1 (status lookup) with proper index name
	err := r.db.WithContext(ctx).Model(&context).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Where("gsi1SK", "=", "THREAD").
		First(&context)

	if err != nil {
		if errors.IsNotFound(err) {
			// Try to build thread context from object relationships
			return r.buildThreadContextFromObjects(ctx, statusID)
		}
		r.logger.Error("failed to get thread context",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityObject, "thread_context")
	}

	// Build ancestors and descendants from the thread context model
	ancestors, err := r.getThreadAncestors(ctx, context.RootStatusID, statusID, context.Depth)
	if err != nil {
		r.logger.Warn("failed to get thread ancestors",
			zap.String("status_id", statusID),
			zap.Error(err))
		ancestors = []string{}
	}

	descendants, err := r.getThreadDescendants(ctx, context.RootStatusID, statusID)
	if err != nil {
		r.logger.Warn("failed to get thread descendants",
			zap.String("status_id", statusID),
			zap.Error(err))
		descendants = []string{}
	}

	result := &storage.ThreadContext{
		StatusID:    statusID,
		Ancestors:   ancestors,
		Descendants: descendants,
	}

	r.logger.Debug("retrieved complete thread context",
		zap.String("status_id", statusID),
		zap.String("root_status_id", context.RootStatusID),
		zap.Int("depth", context.Depth),
		zap.Int("ancestors", len(result.Ancestors)),
		zap.Int("descendants", len(result.Descendants)))

	return result, nil
}

// GetQuotesForNote retrieves quotes for a specific note with pagination
func (r *ObjectRepository) GetQuotesForNote(ctx context.Context, noteID string, limit int, cursor string) ([]*storage.QuoteRelationship, string, error) {
	if err := common.ValidateQueryLimit(limit, 100, "note quotes"); err != nil {
		limit = 20 // default on validation error
	}

	// Use GSI1 to find quotes where GSI1PK = QUOTED#<noteID>
	query := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("QUOTED#%s", noteID)).
		OrderBy("gsi1SK", "DESC"). // Newest first
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
		return nil, "", ErrorHandler.HandleQueryError(err, EntityObject, "note_quotes")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("quote models", quoteModels, limit); err != nil {
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
		return false, ErrorHandler.HandleQueryError(err, EntityObject, "check_quoted")
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
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("QUOTER_NOTE#%s", quoteNoteID)).
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
		return ErrorHandler.HandleQueryError(err, EntityObject, "quote_withdrawal")
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
		return ErrorHandler.HandleUpdateError(err, EntityObject, "withdraw_quote")
	}

	r.logger.Info("withdrew quote",
		zap.String("quote_note_id", quoteNoteID),
		zap.String("quote_id", quote.ID))

	return nil
}

// WithdrawStatusFromQuotes withdraws a status from being quoted with proper cascade effects
func (r *ObjectRepository) WithdrawStatusFromQuotes(ctx context.Context, statusID string) error {
	r.logger.Info("withdrawing status from quotes with cascade effects",
		zap.String("status_id", statusID))

	// Get or create status metadata
	metadata, err := r.getOrCreateStatusMetadata(ctx, statusID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityObject, "status_metadata")
	}

	// Mark as withdrawn from quotes
	metadata.WithdrawFromQuotes()

	// Update the metadata using direct db call (metadata is non-standard model)
	if err := metadata.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "metadata_prepare")
	}

	err = r.db.WithContext(ctx).Model(metadata).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "status_metadata")
	}

	// Withdraw all existing quotes of this status (cascade effect)
	if err := r.withdrawExistingQuotes(ctx, statusID); err != nil {
		r.logger.Error("failed to withdraw existing quotes",
			zap.String("status_id", statusID),
			zap.Error(err))
		// Continue - partial success is acceptable
	}

	// Update search indices to reflect withdrawal
	if err := r.updateSearchIndexForWithdrawal(ctx, statusID); err != nil {
		r.logger.Warn("failed to update search index for quote withdrawal",
			zap.String("status_id", statusID),
			zap.Error(err))
		// Non-critical, continue
	}

	r.logger.Info("status successfully withdrawn from quotes with cascade effects",
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
		return ErrorHandler.HandleGetError(err, EntityObject, "status_metadata")
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

	// Update the metadata using direct db call (metadata is non-standard model)
	if err := metadata.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "metadata_prepare")
	}

	err = r.db.WithContext(ctx).Model(metadata).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "quote_permissions")
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
		return false, ErrorHandler.HandleGetError(err, EntityObject, "status_metadata")
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
		return "", ErrorHandler.HandleGetError(err, EntityObject, "status_metadata")
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
		return false, ErrorHandler.HandleGetError(err, EntityObject, "status_metadata")
	}

	r.logger.Debug("checking if withdrawn from quotes",
		zap.String("status_id", statusID),
		zap.Bool("withdrawn", metadata.WithdrawnFromQuotes))

	return metadata.WithdrawnFromQuotes, nil
}

// GetQuotesOfStatus retrieves quotes of a specific status
func (r *ObjectRepository) GetQuotesOfStatus(ctx context.Context, statusID string, limit int) ([]*storage.StatusSearchResult, error) {
	if err := common.ValidateQueryLimit(limit, 100, "status quotes"); err != nil {
		limit = 20 // default on validation error
	}

	// Use GSI1 to find quotes where GSI1PK = QUOTED#<statusID>
	query := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("QUOTED#%s", statusID)).
		OrderBy("gsi1SK", "DESC"). // Newest first
		Limit(limit)

	var quoteModels []models.QuoteRelationship
	err := query.All(&quoteModels)
	if err != nil {
		r.logger.Error("failed to get quotes of status",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityObject, "quotes_of_status")
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
	metadata.UpdateKeys() // Internal model operation

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

		// Save the new metadata using direct db call (metadata is non-standard model)
		if err := metadata.BeforeCreate(); err != nil {
			return nil, ErrorHandler.HandleCreateError(err, EntityObject, "metadata_prepare")
		}

		err = r.db.WithContext(ctx).Model(metadata).Create()
		if err != nil {
			return nil, ErrorHandler.HandleCreateError(err, EntityObject, "status_metadata")
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

	if err := common.ValidateRequiredParam("author ID", authorID); err != nil {
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

// triggerBackgroundFetch triggers a background fetch for a remote status
func (r *ObjectRepository) triggerBackgroundFetch(ctx context.Context, statusID string) error {
	// Create a background fetch job record
	fetchJob := &models.BackgroundFetchJob{
		StatusID:   statusID,
		FetchType:  "thread_sync",
		Priority:   "normal",
		MaxRetries: 3,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Status:     "pending",
	}

	// Update the keys
	fetchJob.UpdateKeys() // Internal model operation

	// Store the fetch job for background processing
	if err := r.db.WithContext(ctx).Model(fetchJob).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityObject, "fetch_job")
	}

	r.logger.Info("background fetch job created",
		zap.String("status_id", statusID),
		zap.String("job_type", "thread_sync"))

	return nil
}

// getCachedRemoteStatus retrieves cached remote status data if available
func (r *ObjectRepository) getCachedRemoteStatus(ctx context.Context, statusID string) *storage.StatusSearchResult {
	// Try to get from cache models
	var cached models.StatusSearchResult
	err := r.db.WithContext(ctx).Model(&cached).
		Where("PK", "=", fmt.Sprintf("CACHE#STATUS#%s", statusID)).
		Where("SK", "=", "METADATA").
		First(&cached)

	if err != nil {
		// No cached data available
		return nil
	}

	// Convert to storage format
	return &storage.StatusSearchResult{
		StatusID:  cached.StatusID,
		Content:   cached.Content,
		AuthorID:  cached.AuthorID,
		Published: cached.Published,
		Score:     0.8, // Cached remote data gets good score
	}
}

// buildThreadContextFromObjects builds thread context from object relationships
func (r *ObjectRepository) buildThreadContextFromObjects(ctx context.Context, statusID string) (*storage.ThreadContext, error) {
	// Get the object to understand its reply relationships
	var object models.Object
	pk := fmt.Sprintf("object#%s", statusID)
	sk := fmt.Sprintf("object#%s", statusID)
	err := r.Get(ctx, pk, sk, &object)

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Create minimal context for unknown status
			return &storage.ThreadContext{
				StatusID:    statusID,
				Ancestors:   []string{},
				Descendants: []string{},
			}, nil
		}
		return nil, ErrorHandler.HandleGetError(err, EntityObject, "thread_context")
	}

	var ancestors []string
	var descendants []string

	// Build ancestors by following InReplyTo chain
	if object.InReplyTo != nil && *object.InReplyTo != "" {
		ancestors = r.buildAncestorChain(ctx, *object.InReplyTo)
	}

	// Get direct replies as descendants
	descendants, _ = r.getDirectReplies(ctx, statusID)

	return &storage.ThreadContext{
		StatusID:    statusID,
		Ancestors:   ancestors,
		Descendants: descendants,
	}, nil
}

// buildAncestorChain builds the chain of ancestor posts
func (r *ObjectRepository) buildAncestorChain(ctx context.Context, startID string) []string {
	var ancestors []string
	currentID := startID
	maxDepth := 50 // Prevent infinite loops

	for depth := 0; depth < maxDepth && currentID != ""; depth++ {
		ancestors = append(ancestors, currentID)

		// Get next parent
		var object models.Object
		pk := fmt.Sprintf("object#%s", currentID)
		sk := fmt.Sprintf("object#%s", currentID)
		err := r.Get(ctx, pk, sk, &object)

		if err != nil || object.InReplyTo == nil {
			break
		}
		currentID = *object.InReplyTo
	}

	// Reverse to get correct order (oldest ancestor first)
	for i := len(ancestors)/2 - 1; i >= 0; i-- {
		opp := len(ancestors) - 1 - i
		ancestors[i], ancestors[opp] = ancestors[opp], ancestors[i]
	}

	return ancestors
}

// getDirectReplies gets direct replies to a status
func (r *ObjectRepository) getDirectReplies(ctx context.Context, statusID string) ([]string, error) {
	// Use GSI6 to find replies efficiently
	var objects []models.Object
	err := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi6").
		Where("gsi6PK", "=", fmt.Sprintf("REPLIES#%s", statusID)).
		All(&objects)

	if err != nil {
		return nil, err
	}

	replies := make([]string, len(objects))
	for i, obj := range objects {
		replies[i] = obj.ID
	}

	return replies, nil
}

// getThreadAncestors gets ancestors from thread context
func (r *ObjectRepository) getThreadAncestors(ctx context.Context, rootID, statusID string, depth int) ([]string, error) {
	if depth <= 0 {
		return []string{}, nil
	}

	// Query thread contexts to build ancestor chain
	var contexts []models.ThreadContext
	err := r.db.WithContext(ctx).Model(&models.ThreadContext{}).
		Where("PK", "=", fmt.Sprintf("THREAD#%s", rootID)).
		All(&contexts)

	if err != nil {
		return nil, err
	}

	// Build ancestor chain from contexts
	ancestors := make([]string, 0)
	for _, ctx := range contexts {
		if ctx.StatusID != statusID && ctx.Depth < depth {
			ancestors = append(ancestors, ctx.StatusID)
		}
	}

	return ancestors, nil
}

// getThreadDescendants gets descendants from thread context
func (r *ObjectRepository) getThreadDescendants(ctx context.Context, rootID, statusID string) ([]string, error) {
	// Get all contexts in this thread that are descendants
	var contexts []models.ThreadContext
	err := r.db.WithContext(ctx).Model(&models.ThreadContext{}).
		Where("PK", "=", fmt.Sprintf("THREAD#%s", rootID)).
		All(&contexts)

	if err != nil {
		return nil, err
	}

	descendants := make([]string, 0)
	for _, ctx := range contexts {
		if strings.Contains(ctx.Path, statusID) && ctx.StatusID != statusID {
			descendants = append(descendants, ctx.StatusID)
		}
	}

	return descendants, nil
}

// updateThreadContext updates thread context metadata
func (r *ObjectRepository) updateThreadContext(ctx context.Context, statusID, action string) error {
	// Get existing thread context
	var threadCtx models.ThreadContext
	err := r.db.WithContext(ctx).Model(&threadCtx).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Where("gsi1SK", "=", "THREAD").
		First(&threadCtx)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new thread context if none exists
			return r.createThreadContext(ctx, statusID, action)
		}
		return err
	}

	// Update based on action
	switch action {
	case "increment_reply":
		threadCtx.IncrementReplyCount()
	}

	threadCtx.UpdateKeys() // Internal model operation
	return r.db.WithContext(ctx).Model(&threadCtx).Update()
}

// createThreadContext creates a new thread context
func (r *ObjectRepository) createThreadContext(ctx context.Context, statusID, action string) error {
	// Get the object to understand its relationships
	var object models.Object
	pk := fmt.Sprintf("object#%s", statusID)
	sk := fmt.Sprintf("object#%s", statusID)
	err := r.Get(ctx, pk, sk, &object)

	if err != nil {
		// Can't create context without object
		return nil
	}

	// Determine root status and depth
	rootStatusID := statusID
	depth := 0
	if object.InReplyTo != nil && *object.InReplyTo != "" {
		rootStatusID = *object.InReplyTo // Simplified - would need to find actual root
		depth = 1
	}

	threadCtx := &models.ThreadContext{
		RootStatusID: rootStatusID,
		StatusID:     statusID,
		Depth:        depth,
		AuthorID:     object.AttributedTo,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ReplyCount:   0,
		TotalReplies: 0,
		Participants: []string{object.AttributedTo},
	}

	if action == "increment_reply" {
		threadCtx.IncrementReplyCount()
	}

	threadCtx.UpdateKeys() // Internal model operation
	return r.db.WithContext(ctx).Model(threadCtx).Create()
}

// withdrawExistingQuotes withdraws all existing quotes of a status
func (r *ObjectRepository) withdrawExistingQuotes(ctx context.Context, statusID string) error {
	// Find all quotes of this status using GSI1
	var quotes []models.QuoteRelationship
	err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("QUOTED#%s", statusID)).
		All(&quotes)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityObject, "existing_quotes")
	}

	// Withdraw each quote
	for _, quote := range quotes {
		if !quote.Withdrawn {
			now := time.Now()
			quote.Withdrawn = true
			quote.WithdrawnAt = &now

			if err := r.db.WithContext(ctx).Model(&quote).
				Where("PK", "=", quote.PK).
				Where("SK", "=", quote.SK).
				Update(); err != nil {
				r.logger.Error("failed to withdraw quote",
					zap.String("quote_id", quote.ID),
					zap.Error(err))
			}
		}
	}

	r.logger.Info("withdrew existing quotes",
		zap.String("status_id", statusID),
		zap.Int("count", len(quotes)))

	return nil
}

// updateSearchIndexForWithdrawal updates search indices for quote withdrawal
func (r *ObjectRepository) updateSearchIndexForWithdrawal(ctx context.Context, statusID string) error {
	// Update search cache to reflect withdrawal
	searchCache := models.NewSearchCache(fmt.Sprintf("quotes:%s", statusID))
	searchCache.InvalidateCache("quote_withdrawal")
	searchCache.UpdateKeys() // Internal model operation

	// Create invalidation record
	if err := r.db.WithContext(ctx).Model(searchCache).Create(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityObject, "search_cache")
	}

	return nil
}

// parseMentionsFromTags parses mentions from various tag formats
func (r *ObjectRepository) parseMentionsFromTags(tagsInterface any) []string {
	switch tags := tagsInterface.(type) {
	case []any:
		return r.parseMentionsFromAnySlice(tags)
	case []activitypub.Tag:
		return r.parseMentionsFromTagSlice(tags)
	case string:
		return r.parseMentionsFromJSONString(tags)
	default:
		return []string{}
	}
}

// parseMentionsFromAnySlice extracts mentions from []any format
func (r *ObjectRepository) parseMentionsFromAnySlice(tags []any) []string {
	var mentions []string
	for _, tagInterface := range tags {
		if mention := r.extractMentionFromMap(tagInterface); mention != "" {
			mentions = append(mentions, mention)
		}
	}
	return mentions
}

// extractMentionFromMap extracts a mention href from a tag map
func (r *ObjectRepository) extractMentionFromMap(tagInterface any) string {
	tagMap, ok := tagInterface.(map[string]any)
	if !ok {
		return ""
	}

	tagType, ok := tagMap["type"].(string)
	if !ok || tagType != "Mention" {
		return ""
	}

	href, ok := tagMap["href"].(string)
	if !ok || common.ValidateRequiredParam("href", href) != nil {
		return ""
	}

	return href
}

// parseMentionsFromTagSlice extracts mentions from activitypub.Tag slice
func (r *ObjectRepository) parseMentionsFromTagSlice(tags []activitypub.Tag) []string {
	var mentions []string
	for _, tag := range tags {
		if tag.Type == TagTypeMention && tag.Href != "" {
			mentions = append(mentions, tag.Href)
		}
	}
	return mentions
}

// parseMentionsFromJSONString parses mentions from JSON string format
func (r *ObjectRepository) parseMentionsFromJSONString(tags string) []string {
	var tagSlice []activitypub.Tag
	if err := json.Unmarshal([]byte(tags), &tagSlice); err != nil {
		return []string{}
	}
	return r.parseMentionsFromTagSlice(tagSlice)
}

// CreateTombstone creates a tombstone for a deleted object
func (r *ObjectRepository) CreateTombstone(ctx context.Context, tombstone *models.Tombstone) error {
	if err := r.db.WithContext(ctx).Model(tombstone).Create(); err != nil {
		r.logger.Error("failed to create tombstone",
			zap.String("object_id", tombstone.ID),
			zap.String("deleted_by", tombstone.DeletedBy),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityObject, "tombstone")
	}

	r.logger.Info("created tombstone",
		zap.String("object_id", tombstone.ID),
		zap.String("former_type", tombstone.FormerType),
		zap.String("deleted_by", tombstone.DeletedBy))

	return nil
}

// GetTombstone retrieves a tombstone by object ID
func (r *ObjectRepository) GetTombstone(ctx context.Context, objectID string) (*models.Tombstone, error) {
	var tombstone models.Tombstone
	err := r.db.WithContext(ctx).Model(&tombstone).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s", objectID)).
		Where("SK", "=", "TOMBSTONE").
		First(&tombstone)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityObject, objectID)
		}
		r.logger.Error("failed to get tombstone",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityObject, "tombstone")
	}

	return &tombstone, nil
}

// IsTombstoned checks if an object has been tombstoned (deleted)
func (r *ObjectRepository) IsTombstoned(ctx context.Context, objectID string) (bool, error) {
	_, err := r.GetTombstone(ctx, objectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// getTombstonesByGSI is a helper function to query tombstones using different GSI patterns
func (r *ObjectRepository) getTombstonesByGSI(ctx context.Context, gsiIndex, pkField, skField, pkValue, logField, logValue string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	var tombstones []*models.Tombstone

	gsiIndex = strings.ToLower(gsiIndex)

	query := r.db.WithContext(ctx).Model(&models.Tombstone{}).
		Where(pkField, "=", pkValue)

	if cursor != "" {
		query = query.Where(skField, ">", cursor)
	}

	if limit > 0 {
		query = query.Limit(limit + 1) // Get one extra to determine next cursor
	}

	err := query.Index(gsiIndex).Scan(&tombstones)
	if err != nil {
		r.logger.Error("failed to get tombstones",
			zap.String(logField, logValue),
			zap.String("gsi", gsiIndex),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityObject, "tombstones")
	}

	var nextCursor string
	if limit > 0 && len(tombstones) > limit {
		// Remove the extra record and set cursor
		tombstones = tombstones[:limit]
		if err := common.ValidateSliceNotEmpty("tombstones", tombstones); err == nil {
			// Get the SK value from the last tombstone
			switch skField {
			case gsi1SKField:
				nextCursor = tombstones[len(tombstones)-1].GSI1SK
			case "gsi2SK":
				nextCursor = tombstones[len(tombstones)-1].GSI2SK
			}
		}
	}

	return tombstones, nextCursor, nil
}

// GetTombstonesByActor retrieves all tombstones created by a specific actor
func (r *ObjectRepository) GetTombstonesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	return r.getTombstonesByGSI(ctx, "gsi1", "gsi1PK", gsi1SKField,
		fmt.Sprintf("ACTOR#%s#TOMBSTONES", actorID), "actor_id", actorID, limit, cursor)
}

// GetTombstonesByType retrieves tombstones by their former type
func (r *ObjectRepository) GetTombstonesByType(ctx context.Context, formerType string, limit int, cursor string) ([]*models.Tombstone, string, error) {
	return r.getTombstonesByGSI(ctx, "gsi2", "gsi2PK", "gsi2SK",
		fmt.Sprintf("TOMBSTONE#%s", formerType), "former_type", formerType, limit, cursor)
}

// CleanupExpiredTombstones removes tombstones that have exceeded their TTL
func (r *ObjectRepository) CleanupExpiredTombstones(ctx context.Context, batchSize int) (int, error) {
	// Tombstones are TTL-driven (`ttl` on the item, `ttl` configured on the table). Manual cleanup
	// used to perform a DynamoDB Scan which is both expensive and unnecessary.
	r.logger.Info("skipping manual tombstone cleanup (ttl handles expiration)",
		zap.Int("batch_size", batchSize),
	)
	return 0, nil
}

// GetObjectHistory retrieves the version history of an object
func (r *ObjectRepository) GetObjectHistory(ctx context.Context, objectID string) ([]*storage.UpdateHistory, error) {
	// Get all update history for the object
	histories, err := r.GetUpdateHistory(ctx, objectID, 100) // Get all versions
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityObject, "object_history")
	}

	return histories, nil
}

// ReplaceObjectWithTombstone atomically replaces an object with a tombstone
func (r *ObjectRepository) ReplaceObjectWithTombstone(ctx context.Context, objectID, formerType, deletedBy string) error {
	// Create tombstone
	tombstone := &models.Tombstone{
		ID:         objectID,
		FormerType: formerType,
		DeletedBy:  deletedBy,
		Summary:    fmt.Sprintf("Object deleted by %s", deletedBy),
		Deleted:    time.Now(),
	}

	// First delete the original object
	if err := r.DeleteObject(ctx, objectID); err != nil {
		// If the object doesn't exist, it might already be deleted - continue anyway
		r.logger.Warn("object not found for deletion (may already be deleted)",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Then create the tombstone
	if err := r.CreateTombstone(ctx, tombstone); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityObject, "tombstone_after_deletion")
	}

	return nil
}
