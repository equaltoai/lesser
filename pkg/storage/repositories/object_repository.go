package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ObjectRepository implements object operations using DynamORM
type ObjectRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewObjectRepository creates a new object repository
func NewObjectRepository(db core.DB, tableName string, logger *zap.Logger) *ObjectRepository {
	return &ObjectRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
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

	var objects []models.Object
	err := query.All(&objects)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan objects: %w", err)
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