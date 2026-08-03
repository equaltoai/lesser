package theorydb

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// GenericRepository provides a generic repository implementation
// that can be used to adapt DynamORM to existing repository interfaces
type GenericRepository struct {
	DB         core.DB
	TableName  string
	EntityType string
	// Transaction support
	tx *Transaction
}

// NewGenericRepository creates a new GenericRepository
func NewGenericRepository(db core.DB, tableName, entityType string) *GenericRepository {
	return &GenericRepository{
		DB:         db,
		TableName:  tableName,
		EntityType: entityType,
	}
}

// WithTransaction returns a new repository that uses the provided transaction
func (r *GenericRepository) WithTransaction(tx *Transaction) *GenericRepository {
	return &GenericRepository{
		DB:         r.DB,
		TableName:  r.TableName,
		EntityType: r.EntityType,
		tx:         tx,
	}
}

// Create creates a new entity
func (r *GenericRepository) Create(_ context.Context, entity any) error {
	// Set primary keys if the entity implements KeySetter
	if keySetter, ok := entity.(KeySetter); ok {
		keySetter.SetKeys()
	}

	// If we're in a transaction, use the transaction's Put method
	if r.tx != nil {
		err := r.tx.Put(entity)
		if err != nil {
			return MapRepositoryError(err, "Create", r.EntityType, getEntityID(entity))
		}
		return nil
	}

	// Otherwise, use the standard Create method
	err := r.DB.Model(entity).Create()
	if err != nil {
		return MapRepositoryError(err, "Create", r.EntityType, getEntityID(entity))
	}

	return nil
}

// Get retrieves an entity by ID
func (r *GenericRepository) Get(_ context.Context, id string, entity any) error {
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
func (r *GenericRepository) Update(_ context.Context, entity any) error {
	// Set primary keys if the entity implements KeySetter
	if keySetter, ok := entity.(KeySetter); ok {
		keySetter.SetKeys()
	}

	// If we're in a transaction, use the transaction's Update method
	if r.tx != nil {
		err := r.tx.Update(entity)
		if err != nil {
			return MapRepositoryError(err, "Update", r.EntityType, getEntityID(entity))
		}
		return nil
	}

	// Update the entity
	err := r.DB.Model(entity).Update()
	if err != nil {
		return MapRepositoryError(err, "Update", r.EntityType, getEntityID(entity))
	}

	return nil
}

// Delete deletes an entity by ID
func (r *GenericRepository) Delete(_ context.Context, id string, entityPtr any) error {
	// Generate keys for the entity type
	pk, sk := GenerateSimpleKeys(r.EntityType, id)

	// Create a new instance of the entity type with just the keys
	entityValue := reflect.ValueOf(entityPtr).Elem()
	entityType := entityValue.Type()
	entity := reflect.New(entityType).Interface()

	// Set the keys using reflection
	entityElem := reflect.ValueOf(entity).Elem()
	pkField := entityElem.FieldByName("PK")
	skField := entityElem.FieldByName("SK")

	if pkField.IsValid() && pkField.CanSet() && pkField.Kind() == reflect.String {
		pkField.SetString(pk)
	}

	if skField.IsValid() && skField.CanSet() && skField.Kind() == reflect.String {
		skField.SetString(sk)
	}

	// If we're in a transaction, use the transaction's Delete method
	if r.tx != nil {
		err := r.tx.Delete(entity)
		if err != nil {
			return MapRepositoryError(err, "Delete", r.EntityType, id)
		}
		return nil
	}

	// Otherwise, use the standard Delete method
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
func (r *GenericRepository) List(_ context.Context, filter map[string]any, entities any) error {
	// Start building the query
	entitiesType := reflect.TypeOf(entities)
	if entitiesType == nil || entitiesType.Kind() != reflect.Ptr || entitiesType.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("entities must be a pointer to slice, got %T", entities)
	}

	modelType := entitiesType.Elem().Elem()
	for modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	if modelType.Kind() != reflect.Struct {
		return fmt.Errorf("entities must contain structs, got %s", modelType.Kind())
	}

	query := r.DB.Model(reflect.New(modelType).Interface())

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
func (r *GenericRepository) BatchGet(ctx context.Context, ids []string, entities any) error {
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
func getEntityID(entity any) string {
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
	ToStorage() any

	// FromStorage converts a storage model to a DynamORM model
	FromStorage(any) error
}

// RepositoryAdapterConfig provides configuration for repository adapters
type RepositoryAdapterConfig struct {
	DB              core.DB
	TableName       string
	EntityType      string
	OriginalRepo    any
	ConversionFuncs map[string]any
}

// NewRepositoryAdapterConfig creates a new RepositoryAdapterConfig
func NewRepositoryAdapterConfig(db core.DB, tableName, entityType string) *RepositoryAdapterConfig {
	return &RepositoryAdapterConfig{
		DB:              db,
		TableName:       tableName,
		EntityType:      entityType,
		ConversionFuncs: make(map[string]any),
	}
}

// WithOriginalRepo sets the original repository
func (c *RepositoryAdapterConfig) WithOriginalRepo(repo any) *RepositoryAdapterConfig {
	c.OriginalRepo = repo
	return c
}

// WithConversion adds a conversion function
func (c *RepositoryAdapterConfig) WithConversion(name string, fn any) *RepositoryAdapterConfig {
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
