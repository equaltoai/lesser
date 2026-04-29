package theorydb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapStringAnyConverter_FromAttributeValue(t *testing.T) {
	conv := mapStringAnyConverter{}

	av := &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{
			"registration_challenge_id": &types.AttributeValueMemberS{Value: "c1"},
			"ok":                        &types.AttributeValueMemberBOOL{Value: true},
			"n":                         &types.AttributeValueMemberN{Value: "7"},
			"f":                         &types.AttributeValueMemberN{Value: "1.25"},
			"list": &types.AttributeValueMemberL{
				Value: []types.AttributeValue{
					&types.AttributeValueMemberS{Value: "read"},
					&types.AttributeValueMemberN{Value: "9"},
				},
			},
			"nested": &types.AttributeValueMemberM{
				Value: map[string]types.AttributeValue{
					"k": &types.AttributeValueMemberS{Value: "v"},
				},
			},
		},
	}

	var out map[string]any
	require.NoError(t, conv.FromAttributeValue(av, &out))

	assert.Equal(t, "c1", out["registration_challenge_id"])
	assert.Equal(t, true, out["ok"])
	assert.Equal(t, int64(7), out["n"])
	assert.Equal(t, 1.25, out["f"])

	list, ok := out["list"].([]any)
	require.True(t, ok)
	require.Len(t, list, 2)
	assert.Equal(t, "read", list[0])
	assert.Equal(t, int64(9), list[1])

	nested, ok := out["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "v", nested["k"])
}

func TestMapStringAnyConverter_RoundTrip(t *testing.T) {
	conv := mapStringAnyConverter{}

	in := map[string]any{
		"registration_challenge_id": "c1",
		"agent_verified":            true,
		"agent_quarantine_end":      "2026-01-01T00:00:00Z",
		"agent_self_scopes":         []any{"read", "write", int64(7)},
		"nested":                    map[string]any{"k": "v"},
	}

	av, err := conv.ToAttributeValue(in)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, conv.FromAttributeValue(av, &out))

	assert.Equal(t, "c1", out["registration_challenge_id"])
	assert.Equal(t, true, out["agent_verified"])
	assert.Equal(t, "2026-01-01T00:00:00Z", out["agent_quarantine_end"])

	scopes, ok := out["agent_self_scopes"].([]any)
	require.True(t, ok)
	require.Len(t, scopes, 3)
	assert.Equal(t, "read", scopes[0])
	assert.Equal(t, "write", scopes[1])
	assert.Equal(t, int64(7), scopes[2])

	nested, ok := out["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "v", nested["k"])
}

func TestSliceAnyConverter_FromAttributeValue(t *testing.T) {
	conv := sliceAnyConverter{}

	av := &types.AttributeValueMemberL{
		Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "read"},
			&types.AttributeValueMemberN{Value: "2"},
			&types.AttributeValueMemberBOOL{Value: false},
		},
	}

	var out []any
	require.NoError(t, conv.FromAttributeValue(av, &out))
	require.Len(t, out, 3)
	assert.Equal(t, "read", out[0])
	assert.Equal(t, int64(2), out[1])
	assert.Equal(t, false, out[2])
}

func TestActivityPubContextValueConverter_RoundTrip(t *testing.T) {
	conv := activityPubContextValueConverter{}

	in := activitypub.ContextValue{
		"https://www.w3.org/ns/activitystreams",
		map[string]any{
			"lessersoul": "https://spec.lessersoul.ai/ns/agent-attribution/v1#",
		},
	}

	av, err := conv.ToAttributeValue(in)
	require.NoError(t, err)

	var out activitypub.ContextValue
	require.NoError(t, conv.FromAttributeValue(av, &out))
	require.Len(t, out, 2)
	assert.Equal(t, "https://www.w3.org/ns/activitystreams", out[0])

	nested, ok := out[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://spec.lessersoul.ai/ns/agent-attribution/v1#", nested["lessersoul"])
}

func TestActivityPubContextValueConverter_LegacyAttributeShapes(t *testing.T) {
	conv := activityPubContextValueConverter{}

	t.Run("raw string context", func(t *testing.T) {
		var out activitypub.ContextValue
		require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberS{
			Value: "https://www.w3.org/ns/activitystreams",
		}, &out))
		require.Equal(t, activitypub.ContextValue{"https://www.w3.org/ns/activitystreams"}, out)
	})

	t.Run("json string context", func(t *testing.T) {
		var out activitypub.ContextValue
		require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberS{
			Value: `"https://www.w3.org/ns/activitystreams"`,
		}, &out))
		require.Equal(t, activitypub.ContextValue{"https://www.w3.org/ns/activitystreams"}, out)
	})

	t.Run("null context", func(t *testing.T) {
		var out activitypub.ContextValue
		require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberNULL{Value: true}, &out))
		require.Nil(t, out)
	})
}
