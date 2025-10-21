package model_test

import (
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test models with various struct tag configurations

type BasicModel struct {
	ID   string `dynamorm:"pk"`
	Name string
}

type CompositeKeyModel struct {
	UserID    string    `dynamorm:"pk"`
	Timestamp time.Time `dynamorm:"sk"`
	Data      string
}

type IndexedModel struct {
	ID       string  `dynamorm:"pk"`
	Email    string  `dynamorm:"index:gsi-email"`
	Category string  `dynamorm:"index:gsi-category-price,pk"`
	Price    float64 `dynamorm:"index:gsi-category-price,sk"`
	Status   string  `dynamorm:"lsi:lsi-status"`
}

type SpecialFieldsModel struct {
	ID        string    `dynamorm:"pk"`
	Version   int       `dynamorm:"version"`
	TTL       int64     `dynamorm:"ttl"`
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

type CustomAttributeModel struct {
	ID       string   `dynamorm:"pk,attr:user_id"`
	UserName string   `dynamorm:"attr:username"`
	Tags     []string `dynamorm:"set"`
	Optional string   `dynamorm:"omitempty"`
}

type InvalidModel struct {
	Name string // No primary key
}

func TestNewRegistry(t *testing.T) {
	registry := model.NewRegistry()
	assert.NotNil(t, registry)
}

func TestRegisterBasicModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&BasicModel{})
	require.NoError(t, err)

	// Get metadata
	metadata, err := registry.GetMetadata(&BasicModel{})
	require.NoError(t, err)

	// Check table name
	assert.Equal(t, "BasicModels", metadata.TableName)

	// Check primary key
	require.NotNil(t, metadata.PrimaryKey)
	require.NotNil(t, metadata.PrimaryKey.PartitionKey)
	assert.Equal(t, "ID", metadata.PrimaryKey.PartitionKey.Name)
	assert.True(t, metadata.PrimaryKey.PartitionKey.IsPK)
	assert.Nil(t, metadata.PrimaryKey.SortKey)

	// Check fields
	assert.Len(t, metadata.Fields, 2)
	assert.Contains(t, metadata.Fields, "ID")
	assert.Contains(t, metadata.Fields, "Name")
}

func TestRegisterCompositeKeyModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&CompositeKeyModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&CompositeKeyModel{})
	require.NoError(t, err)

	// Check composite key
	require.NotNil(t, metadata.PrimaryKey)
	require.NotNil(t, metadata.PrimaryKey.PartitionKey)
	require.NotNil(t, metadata.PrimaryKey.SortKey)

	assert.Equal(t, "UserID", metadata.PrimaryKey.PartitionKey.Name)
	assert.Equal(t, "Timestamp", metadata.PrimaryKey.SortKey.Name)
	assert.True(t, metadata.PrimaryKey.SortKey.IsSK)
}

func TestRegisterIndexedModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&IndexedModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&IndexedModel{})
	require.NoError(t, err)

	// Check indexes
	assert.Len(t, metadata.Indexes, 3) // 2 GSIs + 1 LSI

	// Find GSI by name
	var emailGSI, categoryGSI, statusLSI *model.IndexSchema
	for i := range metadata.Indexes {
		switch metadata.Indexes[i].Name {
		case "gsi-email":
			emailGSI = &metadata.Indexes[i]
		case "gsi-category-price":
			categoryGSI = &metadata.Indexes[i]
		case "lsi-status":
			statusLSI = &metadata.Indexes[i]
		}
	}

	// Check email GSI
	require.NotNil(t, emailGSI)
	assert.Equal(t, model.GlobalSecondaryIndex, emailGSI.Type)
	assert.Equal(t, "Email", emailGSI.PartitionKey.Name)
	assert.Nil(t, emailGSI.SortKey)

	// Check category-price GSI
	require.NotNil(t, categoryGSI)
	assert.Equal(t, model.GlobalSecondaryIndex, categoryGSI.Type)
	assert.Equal(t, "Category", categoryGSI.PartitionKey.Name)
	assert.Equal(t, "Price", categoryGSI.SortKey.Name)

	// Check status LSI
	require.NotNil(t, statusLSI)
	assert.Equal(t, model.LocalSecondaryIndex, statusLSI.Type)
	assert.Equal(t, "Status", statusLSI.SortKey.Name)
}

func TestRegisterSpecialFieldsModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&SpecialFieldsModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&SpecialFieldsModel{})
	require.NoError(t, err)

	// Check special fields
	require.NotNil(t, metadata.VersionField)
	assert.Equal(t, "Version", metadata.VersionField.Name)
	assert.True(t, metadata.VersionField.IsVersion)

	require.NotNil(t, metadata.TTLField)
	assert.Equal(t, "TTL", metadata.TTLField.Name)
	assert.True(t, metadata.TTLField.IsTTL)

	require.NotNil(t, metadata.CreatedAtField)
	assert.Equal(t, "CreatedAt", metadata.CreatedAtField.Name)
	assert.True(t, metadata.CreatedAtField.IsCreatedAt)

	require.NotNil(t, metadata.UpdatedAtField)
	assert.Equal(t, "UpdatedAt", metadata.UpdatedAtField.Name)
	assert.True(t, metadata.UpdatedAtField.IsUpdatedAt)
}

func TestRegisterCustomAttributeModel(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&CustomAttributeModel{})
	require.NoError(t, err)

	metadata, err := registry.GetMetadata(&CustomAttributeModel{})
	require.NoError(t, err)

	// Check custom attribute names
	idField := metadata.Fields["ID"]
	require.NotNil(t, idField)
	assert.Equal(t, "user_id", idField.DBName)

	usernameField := metadata.Fields["UserName"]
	require.NotNil(t, usernameField)
	assert.Equal(t, "username", usernameField.DBName)

	// Check set type
	tagsField := metadata.Fields["Tags"]
	require.NotNil(t, tagsField)
	assert.True(t, tagsField.IsSet)

	// Check omitempty
	optionalField := metadata.Fields["Optional"]
	require.NotNil(t, optionalField)
	assert.True(t, optionalField.OmitEmpty)

	// Check fields by DB name
	assert.Equal(t, idField, metadata.FieldsByDBName["user_id"])
	assert.Equal(t, usernameField, metadata.FieldsByDBName["username"])
}

func TestRegisterInvalidModel(t *testing.T) {
	registry := model.NewRegistry()

	// Should fail - no primary key
	err := registry.Register(&InvalidModel{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing primary key")
}

func TestRegisterDuplicatePrimaryKey(t *testing.T) {
	type DuplicatePKModel struct {
		ID1 string `dynamorm:"pk"`
		ID2 string `dynamorm:"pk"`
	}

	registry := model.NewRegistry()

	err := registry.Register(&DuplicatePKModel{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate primary key")
}

func TestRegisterInvalidTagTypes(t *testing.T) {
	tests := []struct {
		name  string
		model any
		error string
	}{
		{
			name: "invalid version type",
			model: &struct {
				ID      string `dynamorm:"pk"`
				Version string `dynamorm:"version"`
			}{},
			error: "version field must be numeric",
		},
		{
			name: "invalid ttl type",
			model: &struct {
				ID  string `dynamorm:"pk"`
				TTL string `dynamorm:"ttl"`
			}{},
			error: "ttl field must be int64 or uint64",
		},
		{
			name: "invalid set type",
			model: &struct {
				ID   string `dynamorm:"pk"`
				Tags string `dynamorm:"set"`
			}{},
			error: "set tag can only be used on slice types",
		},
		{
			name: "invalid timestamp type",
			model: &struct {
				ID        string `dynamorm:"pk"`
				CreatedAt string `dynamorm:"created_at"`
			}{},
			error: "created_at/updated_at fields must be time.Time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(tt.model)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.error)
		})
	}
}

func TestGetMetadataByTable(t *testing.T) {
	registry := model.NewRegistry()

	err := registry.Register(&BasicModel{})
	require.NoError(t, err)

	// Get by table name
	metadata, err := registry.GetMetadataByTable("BasicModels")
	require.NoError(t, err)
	assert.Equal(t, "BasicModels", metadata.TableName)

	// Non-existent table
	_, err = registry.GetMetadataByTable("NonExistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "table not found")
}

func TestTableNameDerivation(t *testing.T) {
	tests := []struct {
		model     any
		tableName string
	}{
		{&BasicModel{}, "BasicModels"},
		{&struct {
			ID string `dynamorm:"pk"`
		}{}, "s"},
		{
			&struct {
				ID string `dynamorm:"pk"`
			}{},
			"s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.tableName, func(t *testing.T) {
			registry := model.NewRegistry()
			err := registry.Register(tt.model)
			require.NoError(t, err)

			metadata, err := registry.GetMetadata(tt.model)
			require.NoError(t, err)
			assert.Equal(t, tt.tableName, metadata.TableName)
		})
	}
}

func TestRegisterPointerVsValue(t *testing.T) {
	registry := model.NewRegistry()

	// Register with pointer
	err := registry.Register(&BasicModel{})
	require.NoError(t, err)

	// Get metadata with value
	metadata1, err := registry.GetMetadata(BasicModel{})
	require.NoError(t, err)

	// Get metadata with pointer
	metadata2, err := registry.GetMetadata(&BasicModel{})
	require.NoError(t, err)

	// Should be the same metadata
	assert.Equal(t, metadata1, metadata2)
}
