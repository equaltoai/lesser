package theorydb

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormSchema "github.com/theory-cloud/tabletheory/v2/pkg/schema"
	pkgtypes "github.com/theory-cloud/tabletheory/v2/pkg/types"
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

func TestAgentCapabilitiesConverter_FromAttributeValue_JSONString_SnakeCase(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	av := &types.AttributeValueMemberS{Value: `{
  "can_post": "true",
  "can_reply": true,
  "can_boost": false,
  "can_follow": true,
  "can_dm": true,
  "max_posts_per_hour": 10,
  "requires_approval": "0",
  "restricted_domains": ["example.com"]
}`}

	var out agents.Capabilities
	require.NoError(t, conv.FromAttributeValue(av, &out))

	assert.True(t, out.CanPost)
	assert.True(t, out.CanReply)
	assert.False(t, out.CanBoost)
	assert.True(t, out.CanFollow)
	assert.True(t, out.CanDM)
	assert.Equal(t, 10, out.MaxPostsPerHour)
	assert.False(t, out.RequiresApproval)
	assert.Equal(t, []string{"example.com"}, out.RestrictedDomains)
}

func TestAgentCapabilitiesConverter_FromAttributeValue_MixedTypesAndAliases(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	av := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"can_post":           &types.AttributeValueMemberN{Value: "1"},
		"can_reply":          &types.AttributeValueMemberS{Value: "true"},
		"can_boost":          &types.AttributeValueMemberS{Value: "0"},
		"can_follow":         &types.AttributeValueMemberN{Value: "0"},
		"canDm":              &types.AttributeValueMemberBOOL{Value: true},
		"max_posts_per_hour": &types.AttributeValueMemberS{Value: "7"},
		"requires_approval":  &types.AttributeValueMemberN{Value: "1"},
		"restricted_domains": &types.AttributeValueMemberSS{Value: []string{"a.example", "b.example"}},
	}}

	var out agents.Capabilities
	require.NoError(t, conv.FromAttributeValue(av, &out))

	assert.True(t, out.CanPost)
	assert.True(t, out.CanReply)
	assert.False(t, out.CanBoost)
	assert.False(t, out.CanFollow)
	assert.True(t, out.CanDM)
	assert.Equal(t, 7, out.MaxPostsPerHour)
	assert.True(t, out.RequiresApproval)
	assert.Equal(t, []string{"a.example", "b.example"}, out.RestrictedDomains)
}

func TestAgentCapabilitiesConverter_FromAttributeValue_UnexpectedTypeYieldsEmpty(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	var out agents.Capabilities
	require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberN{Value: "1"}, &out))
	assert.Equal(t, agents.Capabilities{}, out)
}

func TestAgentCapabilitiesConverter_ToAttributeValue_TypedNilPointerYieldsNull(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	var in *agents.Capabilities
	av, err := conv.ToAttributeValue(in)
	require.NoError(t, err)

	nullAV, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)
	assert.True(t, nullAV.Value)
}

func TestAgentCapabilitiesConverter_ToAttributeValue_InvalidTypeErrors(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	_, err := conv.ToAttributeValue("nope")
	require.Error(t, err)
}

func TestAgentCapabilitiesConverter_FromAttributeValue_InvalidJSONErrors(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	var out agents.Capabilities
	require.Error(t, conv.FromAttributeValue(&types.AttributeValueMemberS{Value: "{not-json"}, &out))
}

func TestAgentCapabilitiesConverter_ToAttributeValue_NilYieldsNull(t *testing.T) {
	conv := agentCapabilitiesConverter{}

	av, err := conv.ToAttributeValue(nil)
	require.NoError(t, err)

	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)
}

func TestAgentCapabilitiesConverterHelpers_CoverAdditionalBranches(t *testing.T) {
	t.Run("boolFromAnyMap supports float64/json.Number/string", func(t *testing.T) {
		v, ok := boolFromAnyMap(map[string]any{"x": float64(1)}, "x")
		require.True(t, ok)
		assert.True(t, v)

		v, ok = boolFromAnyMap(map[string]any{"x": json.Number("1")}, "x")
		require.True(t, ok)
		assert.True(t, v)

		v, ok = boolFromAnyMap(map[string]any{"x": "2"}, "x")
		require.True(t, ok)
		assert.True(t, v)

		_, ok = boolFromAnyMap(map[string]any{"x": "nope"}, "x")
		assert.False(t, ok)
	})

	t.Run("intFromAnyMap supports json.Number/string", func(t *testing.T) {
		n, ok := intFromAnyMap(map[string]any{"x": json.Number("7")}, "x")
		require.True(t, ok)
		assert.Equal(t, 7, n)

		n, ok = intFromAnyMap(map[string]any{"x": "5"}, "x")
		require.True(t, ok)
		assert.Equal(t, 5, n)

		_, ok = intFromAnyMap(map[string]any{"x": "nope"}, "x")
		assert.False(t, ok)
	})

	t.Run("stringSliceFromAnyMap supports []string and default branch", func(t *testing.T) {
		out, ok := stringSliceFromAnyMap(map[string]any{"x": []string{"a", "b"}}, "x")
		require.True(t, ok)
		assert.Equal(t, []string{"a", "b"}, out)

		_, ok = stringSliceFromAnyMap(map[string]any{"x": "nope"}, "x")
		assert.False(t, ok)
	})
}

type recordingExtendedDB struct {
	fakeDB
	registered []reflect.Type
}

var _ dynamormcore.ExtendedDB = (*recordingExtendedDB)(nil)

func (db *recordingExtendedDB) AutoMigrateWithOptions(any, ...dynamormSchema.AutoMigrateOption) error {
	return nil
}
func (db *recordingExtendedDB) RegisterTypeConverter(typ reflect.Type, _ pkgtypes.CustomConverter) error {
	db.registered = append(db.registered, typ)
	return nil
}
func (db *recordingExtendedDB) CreateTable(any, ...dynamormSchema.TableOption) error { return nil }
func (db *recordingExtendedDB) EnsureTable(any) error                                { return nil }
func (db *recordingExtendedDB) DeleteTable(any) error                                { return nil }
func (db *recordingExtendedDB) DescribeTable(any) (any, error)                       { return nil, nil }
func (db *recordingExtendedDB) WithLambdaTimeout(context.Context) dynamormcore.DB {
	return db
}
func (db *recordingExtendedDB) WithLambdaTimeoutBuffer(time.Duration) dynamormcore.DB {
	return db
}
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
	assert.Contains(t, db.registered, activityPubNoteType)
	assert.Contains(t, db.registered, activityPubContextValueType)
	assert.Contains(t, db.registered, agentsCapabilitiesType)
}
