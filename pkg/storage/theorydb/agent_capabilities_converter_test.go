package theorydb

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	pkgtypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func TestAgentCapabilitiesConverter_FromAttributeValue_SnakeCase(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	av := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"can_post":           &types.AttributeValueMemberBOOL{Value: true},
		"can_reply":          &types.AttributeValueMemberBOOL{Value: true},
		"can_boost":          &types.AttributeValueMemberBOOL{Value: true},
		"can_follow":         &types.AttributeValueMemberBOOL{Value: true},
		"can_dm":             &types.AttributeValueMemberBOOL{Value: true},
		"max_posts_per_hour": &types.AttributeValueMemberN{Value: "0"},
		"requires_approval":  &types.AttributeValueMemberBOOL{Value: false},
		"restricted_domains": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "example.com"},
		}},
	}}

	var out agents.Capabilities
	require.NoError(t, conv.FromAttributeValue(av, &out))

	assert.True(t, out.CanPost)
	assert.True(t, out.CanReply)
	assert.True(t, out.CanBoost)
	assert.True(t, out.CanFollow)
	assert.True(t, out.CanDM)
	assert.Equal(t, 0, out.MaxPostsPerHour)
	assert.False(t, out.RequiresApproval)
	assert.Equal(t, []string{"example.com"}, out.RestrictedDomains)
}

func TestAgentCapabilitiesConverter_FromAttributeValue_CamelCase(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	av := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"canPost":          &types.AttributeValueMemberBOOL{Value: true},
		"canReply":         &types.AttributeValueMemberBOOL{Value: true},
		"canBoost":         &types.AttributeValueMemberBOOL{Value: false},
		"canFollow":        &types.AttributeValueMemberBOOL{Value: false},
		"canDM":            &types.AttributeValueMemberBOOL{Value: true},
		"maxPostsPerHour":  &types.AttributeValueMemberN{Value: "25"},
		"requiresApproval": &types.AttributeValueMemberBOOL{Value: true},
		"restrictedDomains": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "a.example"},
			&types.AttributeValueMemberS{Value: "b.example"},
		}},
	}}

	var out agents.Capabilities
	require.NoError(t, conv.FromAttributeValue(av, &out))

	assert.True(t, out.CanPost)
	assert.True(t, out.CanReply)
	assert.False(t, out.CanBoost)
	assert.False(t, out.CanFollow)
	assert.True(t, out.CanDM)
	assert.Equal(t, 25, out.MaxPostsPerHour)
	assert.True(t, out.RequiresApproval)
	assert.Equal(t, []string{"a.example", "b.example"}, out.RestrictedDomains)
}

func TestAgentCapabilitiesConverter_ToAttributeValue_WritesCamelCaseKeys(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	in := &agents.Capabilities{
		CanPost:           true,
		CanReply:          true,
		CanBoost:          false,
		CanFollow:         true,
		CanDM:             true,
		RestrictedDomains: []string{"example.com"},
		MaxPostsPerHour:   0,
		RequiresApproval:  false,
	}

	av, err := conv.ToAttributeValue(in)
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)

	_, hasSnake := m.Value["can_post"]
	assert.False(t, hasSnake)
	_, hasCamel := m.Value["canPost"]
	assert.True(t, hasCamel)
}

type recordingExtendedDB struct {
	fakeDB
	registered []reflect.Type
}

var _ dynamormcore.ExtendedDB = (*recordingExtendedDB)(nil)

func (db *recordingExtendedDB) AutoMigrateWithOptions(any, ...any) error { return nil }
func (db *recordingExtendedDB) RegisterTypeConverter(typ reflect.Type, _ pkgtypes.CustomConverter) error {
	db.registered = append(db.registered, typ)
	return nil
}
func (db *recordingExtendedDB) CreateTable(any, ...any) error  { return nil }
func (db *recordingExtendedDB) EnsureTable(any) error          { return nil }
func (db *recordingExtendedDB) DeleteTable(any) error          { return nil }
func (db *recordingExtendedDB) DescribeTable(any) (any, error) { return nil, nil }
func (db *recordingExtendedDB) WithLambdaTimeout(context.Context) dynamormcore.DB {
	return db
}
func (db *recordingExtendedDB) WithLambdaTimeoutBuffer(time.Duration) dynamormcore.DB {
	return db
}
func (db *recordingExtendedDB) TransactionFunc(func(any) error) error { return nil }
func (db *recordingExtendedDB) Transact() dynamormcore.TransactionBuilder {
	return nil
}
func (db *recordingExtendedDB) TransactWrite(context.Context, func(dynamormcore.TransactionBuilder) error) error {
	return nil
}

func TestRegisterDefaultTypeConverters_RegistersCapabilitiesConverter(t *testing.T) {
	db := &recordingExtendedDB{}

	require.NoError(t, registerDefaultTypeConverters(db))
	assert.Contains(t, db.registered, mapStringAnyType)
	assert.Contains(t, db.registered, sliceAnyType)
	assert.Contains(t, db.registered, agentsCapabilitiesType)
}
