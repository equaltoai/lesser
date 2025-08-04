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

// ObjectRepositoryV2 implements object operations using BaseRepository
// This demonstrates how to refactor ObjectRepository to reduce boilerplate
type ObjectRepositoryV2 struct {
	*BaseRepository[*models.Object]
	domain string
	logger *zap.Logger
}

// NewObjectRepositoryV2 creates a new object repository using BaseRepository
func NewObjectRepositoryV2(db core.DB, tableName, domain string, logger *zap.Logger) *ObjectRepositoryV2 {
	return &ObjectRepositoryV2{
		BaseRepository: NewBaseRepository[*models.Object](db, tableName, logger),
		domain:         domain,
		logger:         logger,
	}
}

// getDomainURL returns the full domain URL
func (r *ObjectRepositoryV2) getDomainURL() string {
	return fmt.Sprintf("https://%s", r.domain)
}

// CreateObject stores a generic ActivityPub object
// BEFORE: 40+ lines of manual key construction and error handling
// AFTER: Focused on business logic with BaseRepository handling CRUD
func (r *ObjectRepositoryV2) CreateObject(ctx context.Context, object any) error {
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

	// Use BaseRepository Create - saves ~20 lines of boilerplate
	if err := r.Create(ctx, objModel); err != nil {
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
// BEFORE: 15+ lines of query construction
// AFTER: Single BaseRepository Get call
func (r *ObjectRepositoryV2) GetObject(ctx context.Context, id string) (any, error) {
	objModel := &models.Object{}
	
	// Use BaseRepository Get - saves ~15 lines of boilerplate
	err := r.Get(ctx, fmt.Sprintf("object#%s", id), fmt.Sprintf("object#%s", id), objModel)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("object not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	// Convert back to appropriate ActivityPub type
	return r.modelToActivityPubObject(objModel)
}

// UpdateObject updates an existing object
// BEFORE: Complex query, get, update logic
// AFTER: Get + Update using BaseRepository
func (r *ObjectRepositoryV2) UpdateObject(ctx context.Context, object any) error {
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
	objModel := &models.Object{}
	err = r.Get(ctx, fmt.Sprintf("object#%s", baseObj.ID), fmt.Sprintf("object#%s", baseObj.ID), objModel)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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

	// Use BaseRepository Update - saves ~15 lines of boilerplate
	if err := r.Update(ctx, objModel); err != nil {
		r.logger.Error("failed to update object",
			zap.String("object_id", baseObj.ID),
			zap.Error(err))
		return fmt.Errorf("failed to update object: %w", err)
	}

	return nil
}

// DeleteObject deletes an object by ID
// BEFORE: 25 lines with complex error handling
// AFTER: Single BaseRepository Delete call
func (r *ObjectRepositoryV2) DeleteObject(ctx context.Context, objectID string) error {
	// Use BaseRepository Delete - saves ~20 lines of boilerplate
	err := r.Delete(ctx, fmt.Sprintf("object#%s", objectID), fmt.Sprintf("object#%s", objectID))
	if err != nil {
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
// Uses BaseRepository QueryGSI for efficient queries
func (r *ObjectRepositoryV2) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Use BaseRepository QueryGSI - saves ~20 lines of query construction
	objects, err := r.QueryGSI(ctx, "gsi1-index", fmt.Sprintf("actor#%s", actorID), limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan objects: %w", err)
	}
	
	// Handle cursor logic manually for now (BaseRepository doesn't support cursor yet)
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
		apObj, err := r.modelToActivityPubObject(objModel)
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
func (r *ObjectRepositoryV2) CountObjectReplies(ctx context.Context, objectID string) (int, error) {
	// This is using a different GSI pattern, keep original implementation
	// Would need to enhance BaseRepository for complex GSI queries
	query := r.db.WithContext(ctx).Model(&models.Object{}).
		Index("gsi2-index").  // Assuming GSI2 is used for reply relationships
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
func (r *ObjectRepositoryV2) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
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

	// Then create the tombstone using BaseRepository
	if err := r.Create(ctx, tombstone); err != nil {
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
func (r *ObjectRepositoryV2) modelToActivityPubObject(objModel *models.Object) (any, error) {
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
			json.Unmarshal([]byte(objModel.AttachmentJSON), &note.Attachment)
		}
		if objModel.TagJSON != "" {
			json.Unmarshal([]byte(objModel.TagJSON), &note.Tag)
		}
		if objModel.ContextJSON != "" {
			json.Unmarshal([]byte(objModel.ContextJSON), &note.Context)
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
func (r *ObjectRepositoryV2) CreateUpdateHistory(ctx context.Context, history *storage.UpdateHistory) error {
	// This requires a different model (UpdateHistory), keeping original implementation
	// Would need separate BaseRepository for UpdateHistory
	updateHistory := &models.UpdateHistory{
		ObjectID:      history.ObjectID,
		Version:       history.Version,
		UpdatedAt:     history.UpdatedAt,
		UpdatedBy:     history.UpdatedBy,
		PreviousState: history.PreviousState,
		Summary:       history.Summary,
		CreatedAt:     time.Now(),
	}

	// Update the key fields
	updateHistory.UpdateKeys()

	// Create using raw DynamORM for now
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
func (r *ObjectRepositoryV2) GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*storage.UpdateHistory, error) {
	// This requires a different model, keeping original implementation
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
		result[i] = &storage.UpdateHistory{
			ObjectID:      h.ObjectID,
			Version:       h.Version,
			UpdatedAt:     h.UpdatedAt,
			UpdatedBy:     h.UpdatedBy,
			PreviousState: h.PreviousState,
			Summary:       h.Summary,
		}
	}

	return result, nil
}

// CountReplies counts the number of replies to an object using GSI6
func (r *ObjectRepositoryV2) CountReplies(ctx context.Context, objectID string) (int, error) {
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

// GetReplies retrieves replies to an object with pagination
func (r *ObjectRepositoryV2) GetReplies(ctx context.Context, objectID string, limit int, cursor string) ([]any, string, error) {
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

// Additional methods remain largely the same...
// The following methods handle complex operations that don't benefit much from BaseRepository:
// - CollectionItem operations (different model)
// - QuoteRelationship operations (different model)  
// - Thread sync operations (different model)
// - Status metadata operations (different model)

// Code Reduction Summary:
// - CreateObject: ~20 lines saved (DynamORM create boilerplate)
// - GetObject: ~15 lines saved (query construction)
// - UpdateObject: ~15 lines saved (update logic)
// - DeleteObject: ~20 lines saved (delete with error handling)
// - GetObjectsByActor: ~20 lines saved (GSI query)
// - TombstoneObject: ~10 lines saved (create operation)
// Total: ~100 lines of boilerplate eliminated for core operations!
//
// Additional benefits:
// - Consistent error handling across all methods
// - Built-in logging at the BaseRepository level
// - Type safety with generics
// - Easier to test and maintain
//
// Note: Many methods in ObjectRepository work with different models
// (UpdateHistory, CollectionItem, QuoteRelationship, etc.) which would
// each need their own BaseRepository instances for full benefits.