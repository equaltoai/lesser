package theorydb

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// Model is a sample implementation of a DynamORM model
// This serves as a template for implementing specific models
type Model struct {
	// Primary keys using standard naming
	PK string `theorydb:"pk" json:"pk"` // entity_type#{id}
	SK string `theorydb:"sk" json:"sk"` // entity_type#{id}

	// Standard fields from BaseModel
	BaseModel

	// Example GSI attributes
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// Business data
	ID          string `json:"id"`
	Type        string `theorydb:"attr:type" json:"type"`
	Timestamp   string `theorydb:"attr:timestamp" json:"timestamp"`
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

	// Set timestamp if not already set
	if model.Timestamp == "" {
		model.Timestamp = now.Format(time.RFC3339)
	}

	// Set primary keys if not already set
	if err := common.ValidateRequiredParam("PK", model.PK); err != nil {
		model.PK = fmt.Sprintf("model#%s", model.ID)
	}
	if err := common.ValidateRequiredParam("SK", model.SK); err != nil {
		model.SK = fmt.Sprintf("model#%s", model.ID)
	}

	// Set GSI1 keys for type-based listing
	model.GSI1PK = fmt.Sprintf("MODEL_TYPE#%s", model.Type)
	model.GSI1SK = model.Timestamp

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
	if err := common.ValidateRequiredParam("PK", model.PK); err != nil {
		model.PK = fmt.Sprintf("model#%s", model.ID)
	}
	if err := common.ValidateRequiredParam("SK", model.SK); err != nil {
		model.SK = fmt.Sprintf("model#%s", model.ID)
	}

	// Keep GSI1 keys in sync
	model.GSI1PK = fmt.Sprintf("MODEL_TYPE#%s", model.Type)
	model.GSI1SK = model.Timestamp

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
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("MODEL_TYPE#%s", modelType)).
		OrderBy("gsi1SK", "DESC").
		Limit(limit).
		All(&models)

	if err != nil {
		return nil, MapErrorWithContext(err, "failed to list models")
	}

	return models, nil
}
