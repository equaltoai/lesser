package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_UpdateProfile(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	mutations := &mutationResolver{resolver}

	now := model.Time(time.Now())
	displayName := "Alice Example"
	bio := "Hello world"
	avatar := "https://cdn.local/avatar.png"

	actor, err := mutations.UpdateProfile(round12AuthContext("alice"), model.UpdateProfileInput{
		DisplayName: &displayName,
		Bio:         &bio,
		Avatar:      &avatar,
		Locked:      ptrBool(true),
		Fields: []*model.ProfileFieldInput{
			{
				Name:       "website",
				Value:      "https://example.com",
				VerifiedAt: &now,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, actor)
}

func TestRound12ProfileHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, "fallback", coalesceStringPtr(nil, "fallback"))
	require.Equal(t, "value", coalesceStringPtr(strPtr("value"), "fallback"))
	require.True(t, coalesceBoolPtr(nil, true))
	require.False(t, coalesceBoolPtr(ptrBool(false), true))

	acc := &storage.Account{
		User: &storage.User{
			Avatar: "avatar.png",
			Header: "header.png",
			Fields: []map[string]string{
				{"name": "site", "value": "https://example.com"},
			},
			Metadata: map[string]any{"no_index": true},
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				Type: string(activitypub.ServiceType),
			},
			Icon:  &activitypub.Image{URL: "https://cdn.local/actor_icon.png"},
			Image: &activitypub.Image{URL: "https://cdn.local/actor_header.png"},
		},
	}

	require.Equal(t, "https://cdn.local/actor_icon.png", currentAvatar(acc))
	require.Equal(t, "https://cdn.local/actor_header.png", currentHeader(acc))
	require.True(t, isAccountBot(acc))
	require.True(t, isAccountNoIndex(acc))

	fields := convertStoredFields(acc)
	require.NotEmpty(t, fields)
	require.Equal(t, "site", fields[0].Name)

	out := resolveProfileFields(nil, acc)
	require.Len(t, out, len(fields))

	in := []*model.ProfileFieldInput{
		nil,
		{Name: "name", Value: "value"},
	}
	out = resolveProfileFields(in, acc)
	require.Len(t, out, 1)
	require.Equal(t, accounts.ProfileField{Name: "name", Value: "value"}, out[0])
}
