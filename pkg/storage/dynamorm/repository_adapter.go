package dynamorm

import (
	"context"
	"reflect"
	"strings"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// GenericRepository provides a generic repository implementation
// that can be used to adapt DynamORM to existing repository interfaces
type GenericRepository struct {
	DB         core.DB
	TableName  string
	EntityType string
}

// NewGenericRepository creates a new GenericRepository
func NewGenericRepository(db core.DB, tableName, entityType string) *GenericRepository {
	return &GenericRepository{
		DB:         db,
		TableName:  tableName,
		EntityType: entityType,
	}
}

// Create creates a new entity
func (r *GenericRepository) Create(ctx context.Context, entity interface{}) error {
	// Set primary keys if the entity implements KeySetter
	if keySetter, ok := entity.(KeySetter); ok {
		keySetter.SetKeys()
	}

	// Create the entity
	err := r.DB.Model(entity).Create()
	if err != nil {
		return MapRepositoryError(err, "Create", r.EntityType, getEntityID(entity))
	}

	return nil
}

// Get retrieves an entity by ID
func (r *GenericRepository) Get(ctx context.Context, id string, entity interface{}) error {
	// Generate keys for the entity type
	pk, sk := GenerateSimpleKeys(r.EntityType, id)

	// Query the entity
	err := r.DB.Model(entity).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(entity)

	if err != nil {
		return MapRepositoryError(err, "Get", r.EntityType, id)
	}

	return nil
}

// Update updates an entity
func (r *GenericRepository) Update(ctx context.Context, entity interface{}) error {
	// Set primary keys if the entity implements KeySetter
	if keySetter, ok := entity.(KeySetter); ok {
		keySetter.SetKeys()
	}

	// Update the entity
	err := r.DB.Model(entity).Update()
	if err != nil {
		return MapRepositoryError(err, "Update", r.EntityType, getEntityID(entity))
	}

	return nil
}

// Delete deletes an entity by ID
func (r *GenericRepository) Delete(ctx context.Context, id string, entityPtr interface{}) error {
	// Generate keys for the entity type
	pk, sk := GenerateSimpleKeys(r.EntityType, id)

	// Create a new instance of the entity type with just the keys
	entityValue := reflect.ValueOf(entityPtr).Elem()
	entityType := entityValue.Type()
	entity := reflect.New(entityType).Interface()

	// Set the keys on the entity
	err := r.DB.Model(entity).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		return MapRepositoryError(err, "Delete", r.EntityType, id)
	}

	return nil
}

// List lists entities with optional filtering
func (r *GenericRepository) List(ctx context.Context, filter map[string]interface{}, entities interface{}) error {
	// Start building the query
	query := r.DB.Model(reflect.New(reflect.TypeOf(entities).Elem().Elem().Elem()).Interface())

	for key, value := range filter {
		// Handle special case for GSI queries
		if strings.HasPrefix(key, "index:") {
			indexName := strings.TrimPrefix(key, "index:")
			query = query.Index(indexName)
			continue
		}

		// Handle operators in key (e.g., "CreatedAt:>")
		parts := strings.Split(key, ":")
		fieldName := parts[0]
		operator := "="
		if len(parts) > 1 {
			operator = parts[1]
		}

		query = query.Where(fieldName, operator, value)
	}

	// Execute the query
	err := query.All(entities)
	if err != nil {
		return MapRepositoryError(err, "List", r.EntityType, "")
	}

	return nil
}

// BatchGet retrieves multiple entities by their IDs
func (r *GenericRepository) BatchGet(ctx context.Context, ids []string, entities interface{}) error {
	// Since BatchGet might not be directly available in the core.DB interface,
	// we'll implement it using individual Get operations

	// Get the slice value to append results to
	sliceValue := reflect.ValueOf(entities).Elem()
	elementType := sliceValue.Type().Elem()

	// Process each ID
	for _, id := range ids {
		// Create a new instance of the entity
		entityPtr := reflect.New(elementType.Elem()).Interface()

		// Get the entity
		err := r.Get(ctx, id, entityPtr)
		if err != nil {
			// Skip not found errors in batch operations
			if IsNotFound(err) {
				continue
			}
			return MapRepositoryError(err, "BatchGet", r.EntityType, id)
		}

		// Append to the result slice
		sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(entityPtr)))
	}

	return nil
}

// KeySetter is an interface for entities that can set their own keys
type KeySetter interface {
	SetKeys()
}

// getEntityID attempts to extract an ID from an entity for error reporting
func getEntityID(entity interface{}) string {
	// Try to get ID field
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// Check for common ID fields
	for _, fieldName := range []string{"ID", "Id", "PK"} {
		field := val.FieldByName(fieldName)
		if field.IsValid() && field.Kind() == reflect.String {
			return field.String()
		}
	}

	return "unknown"
}

// ModelConverter is an interface for converting between model types
type ModelConverter interface {
	// ToStorage converts a DynamORM model to a storage model
	ToStorage() interface{}

	// FromStorage converts a storage model to a DynamORM model
	FromStorage(interface{}) error
}

// RepositoryAdapterConfig provides configuration for repository adapters
type RepositoryAdapterConfig struct {
	DB              core.DB
	TableName       string
	EntityType      string
	OriginalRepo    interface{}
	ConversionFuncs map[string]interface{}
}

// NewRepositoryAdapterConfig creates a new RepositoryAdapterConfig
func NewRepositoryAdapterConfig(db core.DB, tableName, entityType string) *RepositoryAdapterConfig {
	return &RepositoryAdapterConfig{
		DB:              db,
		TableName:       tableName,
		EntityType:      entityType,
		ConversionFuncs: make(map[string]interface{}),
	}
}

// WithOriginalRepo sets the original repository
func (c *RepositoryAdapterConfig) WithOriginalRepo(repo interface{}) *RepositoryAdapterConfig {
	c.OriginalRepo = repo
	return c
}

// WithConversion adds a conversion function
func (c *RepositoryAdapterConfig) WithConversion(name string, fn interface{}) *RepositoryAdapterConfig {
	c.ConversionFuncs[name] = fn
	return c
}

// Example of a specific repository adapter:
/*
// UserRepositoryAdapter adapts DynamORM to the UserRepository interface
type UserRepositoryAdapter struct {
	GenericRepository
	originalRepo storage.UserRepository
}

// NewUserRepositoryAdapter creates a new UserRepositoryAdapter
func NewUserRepositoryAdapter(db core.DB, tableName string, originalRepo storage.UserRepository) *UserRepositoryAdapter {
	return &UserRepositoryAdapter{
		GenericRepository: *NewGenericRepository(db, tableName, "user"),
		originalRepo:      originalRepo,
	}
}

// GetUser gets a user by ID
func (r *UserRepositoryAdapter) GetUser(ctx context.Context, id string) (*storage.User, error) {
	// Create a DynamORM user model
	dynamoUser := &UserModel{}

	// Get the user from DynamoDB
	err := r.Get(ctx, id, dynamoUser)
	if err != nil {
		// If not found and we have an original repo, delegate to it
		if IsNotFoundError(err) && r.originalRepo != nil {
			return r.originalRepo.GetUser(ctx, id)
		}
		return nil, err
	}

	// Convert to storage model
	storageUser := &storage.User{
		ID:       dynamoUser.ID,
		Username: dynamoUser.Username,
		Email:    dynamoUser.Email,
		// Map other fields...
	}

	return storageUser, nil
}
*/
