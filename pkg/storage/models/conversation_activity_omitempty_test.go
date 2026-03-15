package models

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ttcore "github.com/theory-cloud/tabletheory/pkg/core"
	ttmodel "github.com/theory-cloud/tabletheory/pkg/model"
	ttquery "github.com/theory-cloud/tabletheory/pkg/query"
)

type theorydbPutCapture struct {
	item map[string]types.AttributeValue
}

func (c *theorydbPutCapture) ExecuteQuery(*ttcore.CompiledQuery, any) error {
	return nil
}

func (c *theorydbPutCapture) ExecuteScan(*ttcore.CompiledQuery, any) error {
	return nil
}

func (c *theorydbPutCapture) ExecutePutItem(_ *ttcore.CompiledQuery, item map[string]types.AttributeValue) error {
	c.item = item
	return nil
}

type rawRegistryMetadataAdapter struct {
	meta *ttmodel.Metadata
}

func (a *rawRegistryMetadataAdapter) TableName() string {
	return a.meta.TableName
}

func (a *rawRegistryMetadataAdapter) PrimaryKey() ttcore.KeySchema {
	schema := ttcore.KeySchema{
		PartitionKey: a.meta.PrimaryKey.PartitionKey.Name,
	}
	if a.meta.PrimaryKey.SortKey != nil {
		schema.SortKey = a.meta.PrimaryKey.SortKey.Name
	}
	return schema
}

func (a *rawRegistryMetadataAdapter) Indexes() []ttcore.IndexSchema {
	indexes := make([]ttcore.IndexSchema, len(a.meta.Indexes))
	for i, idx := range a.meta.Indexes {
		indexes[i] = ttcore.IndexSchema{
			Name:            idx.Name,
			Type:            string(idx.Type),
			ProjectionType:  idx.ProjectionType,
			ProjectedFields: idx.ProjectedFields,
		}
		if idx.PartitionKey != nil {
			indexes[i].PartitionKey = idx.PartitionKey.Name
		}
		if idx.SortKey != nil {
			indexes[i].SortKey = idx.SortKey.Name
		}
	}
	return indexes
}

func (a *rawRegistryMetadataAdapter) AttributeMetadata(field string) *ttcore.AttributeMetadata {
	if meta, ok := a.meta.Fields[field]; ok {
		return &ttcore.AttributeMetadata{Name: meta.Name, Type: meta.Type.String(), DynamoDBName: meta.DBName}
	}
	if meta, ok := a.meta.FieldsByDBName[field]; ok {
		return &ttcore.AttributeMetadata{Name: meta.Name, Type: meta.Type.String(), DynamoDBName: meta.DBName}
	}
	return nil
}

func (a *rawRegistryMetadataAdapter) VersionFieldName() string {
	if a.meta.VersionField == nil {
		return ""
	}
	if a.meta.VersionField.DBName != "" {
		return a.meta.VersionField.DBName
	}
	return a.meta.VersionField.Name
}

func (a *rawRegistryMetadataAdapter) RawMetadata() *ttmodel.Metadata {
	return a.meta
}

func marshalWithTableTheory(t *testing.T, item any) map[string]types.AttributeValue {
	t.Helper()

	registry := ttmodel.NewRegistry()
	require.NoError(t, registry.Register(item))

	meta, err := registry.GetMetadata(item)
	require.NoError(t, err)

	executor := &theorydbPutCapture{}
	query := ttquery.New(item, &rawRegistryMetadataAdapter{meta: meta}, executor)
	require.NoError(t, query.Create())
	require.NotNil(t, executor.item)

	return executor.item
}

func TestConversationCreateOmitsEmptyConversationGSIKeys(t *testing.T) {
	conversation := &Conversation{
		ID:        "conv-1",
		CreatedAt: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, conversation.BeforeCreate())

	item := marshalWithTableTheory(t, conversation)

	assert.NotContains(t, item, "gsi1PK")
	assert.NotContains(t, item, "gsi1SK")
	assert.Contains(t, item, "PK")
	assert.Contains(t, item, "SK")
}

func TestConversationParticipantKeyOmitsEmptyReverseLookupSortKey(t *testing.T) {
	lookupKey := &ConversationParticipantKey{
		PK:             "CONVERSATION_PARTICIPANTS#alice,bob",
		SK:             "LOOKUP",
		GSI1PK:         "CONVERSATION_PARTICIPANTS#alice,bob",
		ConversationID: "conv-1",
	}

	item := marshalWithTableTheory(t, lookupKey)

	assert.Contains(t, item, "gsi1PK")
	assert.NotContains(t, item, "gsi1SK")
}

func TestActivityCreateOmitsEmptyOptionalGSIKeys(t *testing.T) {
	activity := &Activity{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1"},
			Actor:      "https://example.com/users/alice",
		},
		CreatedAt: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, activity.UpdateKeys())

	item := marshalWithTableTheory(t, activity)

	assert.NotContains(t, item, "gsi1PK")
	assert.NotContains(t, item, "gsi1SK")
	assert.NotContains(t, item, "gsi2PK")
	assert.NotContains(t, item, "gsi2SK")
	assert.Contains(t, item, "PK")
	assert.Contains(t, item, "SK")
}
