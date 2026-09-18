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

	actor, err := mutations.UpdateProfile(round12AuthContext("alice"), model.UpdateProfileInput{
		DisplayName: &displayName,
		Bio:         &bio,
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

// TestRound12MutationResolvers_UpdateProfileRejectsAvatarURL documents that the
// profile avatar is written only by the REST avatar upload route. The ownership
// gate lives in the accounts service, so it also covers this GraphQL mutation:
// an avatar URL supplied here is rejected instead of being stored.
func TestRound12MutationResolvers_UpdateProfileRejectsAvatarURL(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	mutations := &mutationResolver{resolver}

	displayName := "Alice Example"
	avatar := "https://cdn.local/avatar.png"

	_, err := mutations.UpdateProfile(round12AuthContext("alice"), model.UpdateProfileInput{
		DisplayName: &displayName,
		Avatar:      &avatar,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot be set by URL")
}

// TestRound12MutationResolvers_UpdateProfileOmittingAvatarKeepsExistingAvatar
// guards a blocker: the resolver coalesced the account's stored avatar into the
// service command for every request, and the accounts-service gate rejects any
// non-empty avatar. Because the command carries no avatar when the input omits
// it, an account that already has an avatar can update its profile, and the
// stored avatar is left exactly as it was.
func TestRound12MutationResolvers_UpdateProfileOmittingAvatarKeepsExistingAvatar(t *testing.T) {
	resolver, graphStorage, _, _, _ := newRound12GraphResolverWithMocks(t)

	avatarURL := "https://localhost/api/v1/avatars/6f6b0d1e-2a91-4c34-9d2f-8a1b7c5e0d43"
	graphStorage.SeedAccountUser(&storage.User{
		Username: "alice",
		Role:     adminRoleUser,
		Approved: true,
		Version:  1,
		Avatar:   avatarURL,
	})

	mutations := &mutationResolver{resolver}
	ctx := round12AuthContext("alice")

	displayName := "Alice Example"
	actor, err := mutations.UpdateProfile(ctx, model.UpdateProfileInput{DisplayName: &displayName})
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.NotNil(t, actor.Icon)
	require.Equal(t, avatarURL, actor.Icon.URL)

	stored, err := resolver.Registry.Accounts().GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, displayName, stored.User.DisplayName)
	require.Equal(t, avatarURL, stored.User.Avatar)

	// The gate is unchanged for callers that do supply an avatar: the value is
	// still rejected, and the stored avatar survives the rejected request.
	rejected := "https://cdn.local/avatar.png"
	_, err = mutations.UpdateProfile(ctx, model.UpdateProfileInput{Avatar: &rejected})
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot be set by URL")

	stored, err = resolver.Registry.Accounts().GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, avatarURL, stored.User.Avatar)
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
			Image: &activitypub.Image{URL: "https://cdn.local/actor_header.png"},
		},
	}

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
