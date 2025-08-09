package dynamorm

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// Model is a sample implementation of a DynamORM model
// This serves as a template for implementing specific models
type Model struct {
	// Primary keys using standard naming
	PK string `dynamorm:"pk" json:"pk"` // entity_type#{id}
	SK string `dynamorm:"sk" json:"sk"` // entity_type#{id}

	// Standard fields from BaseModel
	BaseModel

	// Example GSI attributes
	Type      string `dynamorm:"index:type-index,pk" json:"type"`
	Timestamp string `dynamorm:"index:type-index,sk" json:"timestamp"`

	// Business data
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

// ModelRepository is a sample implementation of a DynamORM repository
// This serves as a template for implementing specific repositories
type ModelRepository struct {
	BaseRepository
}

// NewModelRepository creates a new ModelRepository
func NewModelRepository(db core.DB, tableName string) *ModelRepository {
	return &ModelRepository{
		BaseRepository: *NewBaseRepository(db, tableName),
	}
}

// Create creates a new model
func (r *ModelRepository) Create(_ context.Context, model *Model) error {
	// Set timestamps
	now := time.Now()
	model.CreatedAt = now
	model.UpdatedAt = now

	// Set primary keys if not already set
	if model.PK == "" {
		model.PK = fmt.Sprintf("model#%s", model.ID)
	}
	if model.SK == "" {
		model.SK = fmt.Sprintf("model#%s", model.ID)
	}

	// Create the model using the query builder pattern
	err := r.GetDB().Model(model).Create()
	if err != nil {
		return MapErrorWithContext(err, "failed to create model")
	}

	return nil
}

// Get gets a model by ID
func (r *ModelRepository) Get(_ context.Context, id string) (*Model, error) {
	model := &Model{}

	err := r.GetDB().Model(model).
		Where("PK", "=", fmt.Sprintf("model#%s", id)).
		Where("SK", "=", fmt.Sprintf("model#%s", id)).
		First(model)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to get model")
	}

	return model, nil
}

// Update updates a model
func (r *ModelRepository) Update(_ context.Context, model *Model) error {
	// Update timestamp
	model.UpdatedAt = time.Now()

	// Set primary keys if not already set
	if model.PK == "" {
		model.PK = fmt.Sprintf("model#%s", model.ID)
	}
	if model.SK == "" {
		model.SK = fmt.Sprintf("model#%s", model.ID)
	}

	// Update the model
	err := r.GetDB().Model(model).Update()
	if err != nil {
		return MapErrorWithContext(err, "failed to update model")
	}

	return nil
}

// Delete deletes a model
func (r *ModelRepository) Delete(_ context.Context, id string) error {
	model := &Model{
		PK: fmt.Sprintf("model#%s", id),
		SK: fmt.Sprintf("model#%s", id),
	}

	err := r.GetDB().Model(model).Delete()
	if err != nil {
		return MapErrorWithContext(err, "failed to delete model")
	}

	return nil
}

// List lists models by type
func (r *ModelRepository) List(_ context.Context, modelType string, limit int) ([]*Model, error) {
	var models []*Model

	err := r.GetDB().Model(&Model{}).
		Index("type-index").
		Where("Type", "=", modelType).
		OrderBy("Timestamp", "DESC").
		Limit(limit).
		All(&models)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list models")
	}

	return models, nil
}
