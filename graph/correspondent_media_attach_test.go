package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

// graphActAsCorrespondentContext builds the share-grant correspondent session
// shape: the caller authenticates as themselves (raw token subject = the
// caller) and drives a shared agent via X-Lesser-Act-As. The server-minted
// DelegatedBy carries the caller so act-as resolution can attribute the
// request, while the raw token subject stays the caller — the exact divergence
// the media admission resolvers ignored pre-R4 (they read the raw subject via
// requireAuth while the draft-side resolvers read the act-as agent via
// requireActingIdentity).
func graphActAsCorrespondentContext(caller, indicator string) context.Context {
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username:    caller,
		DelegatedBy: "@" + caller,
		Scopes:      []string{"read", "write"},
	})
	return context.WithValue(ctx, common.ContextKeyActAsAgent, indicator)
}

// TestCorrespondentMediaAttachActAsFlow is the R4 end-to-end correspondent
// flow (#1470): a share-grant caller creates a draft for the agent, mints a
// presigned upload grant, PUTs the declared bytes, finalizes the grant into an
// internal editorial media record, attaches it to the agent's draft, and reads
// the binding back — all under one act-as session.
//
// Pre-fix this flow was unsatisfiable: the media admission resolvers resolved
// the raw token subject while the draft-side resolvers resolved the act-as
// agent, so the attach ownership invariant (media owner == draft author) could
// never hold and the final attach step failed with "editorial media does not
// belong to draft author". Post-fix the whole flow completes with every
// admitted identity being the acting agent.
func TestCorrespondentMediaAttachActAsFlow(t *testing.T) {
	resolver, storage, state := newRound12UploadGrantResolver(t)
	seedGraphActAsAgentAndGrant(storage)
	ctx := graphActAsCorrespondentContext("alice", "agent-one")

	// 1. The agent's draft is created under the correspondent session with
	//    actedBy attribution.
	title := "Correspondent draft"
	draft, err := resolver.Mutation().CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       "# correspondent draft",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.Equal(t, "agent-one", draft.AuthorID)
	require.NotNil(t, draft.ActedBy, "caller attribution must be surfaced on the draft")
	require.Contains(t, draft.ActedBy.ID, "/users/alice")

	// 2. Mint the presigned upload grant under the same session. The grant
	//    owner must be the acting agent, not the raw token subject.
	minted, err := resolver.Mutation().MintUploadGrant(ctx, model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.NoError(t, err)
	require.NotNil(t, minted)

	// 3. Simulate the presigned-companion PUT of the declared bytes.
	stored, err := storage.UploadGrant().GetUploadGrant(context.Background(), minted.OwnerID, minted.ID)
	require.NoError(t, err)
	round12Put(state, stored, round12TinyPNG())

	// 4. Finalize: the digest-verified object becomes an internal editorial
	//    media record owned by the acting agent.
	result, err := resolver.Mutation().FinalizeUploadGrant(ctx, minted.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Media)
	mediaID := result.Media.MediaID
	media, err := storage.Media().GetMedia(context.Background(), mediaID)
	require.NoError(t, err)
	require.NotNil(t, media)

	// 5. Attach the admitted asset to the agent's draft. Pre-fix this was the
	//    unsatisfiable step: the media record was owned by the raw token
	//    subject while the draft is owned by the act-as agent.
	bound, err := resolver.Mutation().SetDraftEditorialMedia(ctx, draft.ID, []*model.EditorialMediaUsageInput{{
		MediaID: mediaID,
		Role:    model.EditorialMediaRoleHero,
	}})
	require.NoError(t, err)
	require.Len(t, bound.EditorialMedia, 1)
	require.Equal(t, mediaID, bound.EditorialMedia[0].MediaID)

	// 6. Read-back: the agent's draft shows the bound asset, and every
	//    admitted identity is the acting agent (grant owner, media owner,
	//    draft author).
	got, err := resolver.Query().Draft(ctx, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "agent-one", got.AuthorID)
	require.Len(t, got.EditorialMedia, 1)
	require.Equal(t, mediaID, got.EditorialMedia[0].MediaID)
	require.Equal(t, "agent-one", minted.OwnerID, "grant must be owned by the acting agent")
	require.Equal(t, "agent-one", media.UserID, "admitted media must be owned by the acting agent")
}

// TestEditorialMediaAttachActionableOwnershipError pins the actionable
// ownership-mismatch error (issue point 6): attaching media that belongs to a
// different owner than the draft author must fail with an error naming the
// media and both identities, never a bare rejection.
func TestEditorialMediaAttachActionableOwnershipError(t *testing.T) {
	resolver, storage, state := newRound12UploadGrantResolver(t)
	seedGraphActAsAgentAndGrant(storage)

	// The agent's draft exists under the correspondent session.
	ctx := graphActAsCorrespondentContext("alice", "agent-one")
	title := "Ownership error draft"
	draft, err := resolver.Mutation().CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       "# ownership error draft",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)

	// The caller mints and finalizes media in their own owner session, so the
	// admitted media belongs to the caller, not the agent.
	ownerCtx := round12AuthContext("alice")
	minted, err := resolver.Mutation().MintUploadGrant(ownerCtx, model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.NoError(t, err)
	stored, err := storage.UploadGrant().GetUploadGrant(context.Background(), minted.OwnerID, minted.ID)
	require.NoError(t, err)
	round12Put(state, stored, round12TinyPNG())
	result, err := resolver.Mutation().FinalizeUploadGrant(ownerCtx, minted.ID)
	require.NoError(t, err)
	require.NotNil(t, result.Media)
	media, err := storage.Media().GetMedia(context.Background(), result.Media.MediaID)
	require.NoError(t, err)
	require.Equal(t, "alice", media.UserID, "owner-session media is owned by the caller")

	// Attaching caller-owned media to the agent's draft fails closed with an
	// actionable error naming the media and both identities — never a bare
	// ownership rejection.
	_, err = resolver.Mutation().SetDraftEditorialMedia(ctx, draft.ID, []*model.EditorialMediaUsageInput{{
		MediaID: result.Media.MediaID,
		Role:    model.EditorialMediaRoleHero,
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), result.Media.MediaID)
	require.Contains(t, err.Error(), "alice")
	require.Contains(t, err.Error(), "agent-one")
}
