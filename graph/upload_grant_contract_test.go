package graph

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// round12TinyPNG is a 1x1 transparent PNG the media processor decodes, giving a
// deterministic ready-path for the GraphQL contract flow.
const round12TinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func round12TinyPNG() []byte {
	data, err := base64.StdEncoding.DecodeString(round12TinyPNGBase64)
	if err != nil {
		panic(err)
	}
	return data
}

func round12UploadDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newRound12UploadGrantResolver(t *testing.T) (*Resolver, *round12GraphStorage, *round12PermissiveQueryState) {
	t.Helper()
	resolver, storage, _, _, state := newRound12GraphResolverWithMocks(t)
	state.persistMedia = true
	return resolver, storage, state
}

// round12Put simulates the presigned-companion PUT of the declared bytes into
// the grant's object key.
func round12Put(state *round12PermissiveQueryState, grant *models.UploadGrant, data []byte) {
	if state.uploadObjects == nil {
		state.uploadObjects = make(map[string][]byte)
		state.uploadTypes = make(map[string]string)
	}
	state.uploadObjects["lesser-media-bucket/"+grant.S3Key] = append([]byte(nil), data...)
	state.uploadTypes["lesser-media-bucket/"+grant.S3Key] = grant.ContentType
}

func TestUploadGrantMintFinalizeGraphQLFlow(t *testing.T) {
	resolver, storage, state := newRound12UploadGrantResolver(t)
	ctx := round12AuthContext("alice")
	before := time.Now().UTC()

	// 1. Mint: the owner receives a one-time, hash-bound grant + presigned PUT.
	minted, err := resolver.Mutation().MintUploadGrant(ctx, model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.NoError(t, err)
	require.NotNil(t, minted)
	require.NotEmpty(t, minted.ID)
	require.Equal(t, "alice", minted.OwnerID)
	require.Equal(t, model.UploadGrantStatusMinted, minted.Status)
	require.NotNil(t, minted.PresignedURL)
	require.Contains(t, *minted.PresignedURL, "lesser-media-bucket/")
	require.False(t, time.Time(minted.ExpiresAt).Before(before.Add(15*time.Minute-2*time.Second)), "grant TTL must be bounded near 15 minutes")
	require.True(t, time.Time(minted.ExpiresAt).After(time.Now().UTC()), "grant expiry must be in the future")
	require.Nil(t, minted.UsedAt)
	require.Nil(t, minted.FailureReason)

	// The presigned PUT was minted against the grant's constraints.
	require.Len(t, state.uploadGrantPresigns, 1)
	presign := state.uploadGrantPresigns[0]
	require.Equal(t, "lesser-media-bucket", presign.bucket)
	require.NotNil(t, minted.MediaID)
	require.Equal(t, *minted.MediaID, grantIDFromS3Key(t, presign.key), "the PUT key must be scoped to the grant's minted media ID")
	require.Equal(t, "image/png", presign.contentType)
	require.Equal(t, round12UploadDigest(round12TinyPNG()), presign.contentSHA256)
	require.Equal(t, 15*time.Minute, presign.expiry)

	// 2. PUT the exact declared bytes (simulated against the same fake store).
	stored, err := storage.UploadGrant().GetUploadGrant(context.Background(), "alice", minted.ID)
	require.NoError(t, err)
	round12Put(state, stored, round12TinyPNG())

	// 3. Finalize: the digest is verified before any media record exists.
	result, err := resolver.Mutation().FinalizeUploadGrant(ctx, minted.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Media)
	require.NotEmpty(t, result.Media.MediaID)
	require.Equal(t, "image/png", result.Media.ContentType)
	require.Equal(t, "sha256:"+round12UploadDigest(round12TinyPNG()), result.Media.ContentHash)
	require.Equal(t, "ready", result.Media.Status, "verified internal images enter the ready pipeline like M0")
	require.Equal(t, "internal", result.Media.Visibility)
	require.Equal(t, model.UploadGrantStatusUsed, result.Grant.Status)

	// The M1 media record exists and is bound to the verified bytes.
	media, err := storage.Media().GetMedia(context.Background(), result.Media.MediaID)
	require.NoError(t, err)
	require.NotNil(t, media)
	require.Equal(t, models.MediaVisibilityInternal, media.Visibility)
	require.Equal(t, "sha256:"+round12UploadDigest(round12TinyPNG()), media.ContentHash)
	require.Equal(t, "alice", media.UserID)
	require.NotNil(t, media.Provenance)
	require.Equal(t, media.ContentHash, media.Provenance.ContentIntegrity)

	// 4. The owner can inspect the grant lifecycle; the used grant has no URL.
	queried, err := resolver.Query().UploadGrant(ctx, minted.ID)
	require.NoError(t, err)
	require.Equal(t, model.UploadGrantStatusUsed, queried.Status)
	require.Nil(t, queried.PresignedURL)
	require.NotNil(t, queried.UsedAt)
}

func TestUploadGrantDigestMismatchGraphQLSurface(t *testing.T) {
	resolver, storage, state := newRound12UploadGrantResolver(t)
	ctx := round12AuthContext("alice")

	minted, err := resolver.Mutation().MintUploadGrant(ctx, model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.NoError(t, err)
	stored, err := storage.UploadGrant().GetUploadGrant(context.Background(), "alice", minted.ID)
	require.NoError(t, err)
	// The client PUTs different bytes than it declared.
	round12Put(state, stored, []byte("definitely not the declared bytes"))

	_, err = resolver.Mutation().FinalizeUploadGrant(ctx, minted.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "digest")

	// The grant is consumed to FAILED_DIGEST with an inspectable reason and no
	// asset ever existed for the minted media ID.
	queried, err := resolver.Query().UploadGrant(ctx, minted.ID)
	require.NoError(t, err)
	require.Equal(t, model.UploadGrantStatusFailedDigest, queried.Status)
	require.NotNil(t, queried.FailureReason)
	require.Contains(t, *queried.FailureReason, "digest")
	require.NotNil(t, queried.MediaID, "the media ID is minted with the grant")
	require.Nil(t, state.seededMedia[*queried.MediaID], "no editorial media record may exist for a failed digest")
}

func TestUploadGrantDoubleFinalizeFailsClosed(t *testing.T) {
	resolver, storage, state := newRound12UploadGrantResolver(t)
	ctx := round12AuthContext("alice")

	minted, err := resolver.Mutation().MintUploadGrant(ctx, model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.NoError(t, err)
	stored, err := storage.UploadGrant().GetUploadGrant(context.Background(), "alice", minted.ID)
	require.NoError(t, err)
	round12Put(state, stored, round12TinyPNG())

	first, err := resolver.Mutation().FinalizeUploadGrant(ctx, minted.ID)
	require.NoError(t, err)
	require.NotNil(t, first.Media)

	// The single-use grant cannot finalize twice; no second asset is created.
	second, err := resolver.Mutation().FinalizeUploadGrant(ctx, minted.ID)
	require.Error(t, err)
	require.Nil(t, second)
	_, err = storage.Media().GetMedia(context.Background(), first.Media.MediaID)
	require.NoError(t, err, "the single admitted asset survives")
}

func TestUploadGrantExpiredFailsClosedGraphQL(t *testing.T) {
	resolver, storage, _ := newRound12UploadGrantResolver(t)
	ctx := round12AuthContext("alice")
	now := time.Now().UTC()
	expired := &models.UploadGrant{
		Owner: "alice", GrantID: "grant-expired-gql", ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024,
		ContentSHA256: round12UploadDigest(round12TinyPNG()), S3Bucket: "lesser-media-bucket",
		S3Key: "media/2026/08/24/expired-gql.png", MediaID: "media-expired-gql", FileName: "expired-gql.png",
		Status: models.UploadGrantStatusMinted, GrantedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}
	require.NoError(t, expired.UpdateKeys())
	storage.SeedUploadGrant(expired)

	_, err := resolver.Mutation().FinalizeUploadGrant(ctx, "grant-expired-gql")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")

	queried, err := resolver.Query().UploadGrant(ctx, "grant-expired-gql")
	require.NoError(t, err)
	require.Equal(t, model.UploadGrantStatusExpired, queried.Status)
	require.Nil(t, queried.PresignedURL)
}

func TestUploadGrantActorIsolationGraphQL(t *testing.T) {
	resolver, storage, state := newRound12UploadGrantResolver(t)

	minted, err := resolver.Mutation().MintUploadGrant(round12AuthContext("alice"), model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 5 * 1024 * 1024, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.NoError(t, err)
	stored, err := storage.UploadGrant().GetUploadGrant(context.Background(), "alice", minted.ID)
	require.NoError(t, err)
	round12Put(state, stored, round12TinyPNG())

	// Bob cannot finalize alice's grant: the owner-scoped key construction
	// makes it invisible to him.
	_, err = resolver.Mutation().FinalizeUploadGrant(round12AuthContext("bob"), minted.ID)
	require.Error(t, err)
	_, err = resolver.Query().UploadGrant(round12AuthContext("bob"), minted.ID)
	require.Error(t, err)

	// Alice's grant is untouched.
	queried, err := resolver.Query().UploadGrant(round12AuthContext("alice"), minted.ID)
	require.NoError(t, err)
	require.Equal(t, model.UploadGrantStatusMinted, queried.Status)
}

func TestUploadGrantRequiresAuthentication(t *testing.T) {
	resolver, _, _ := newRound12UploadGrantResolver(t)
	_, err := resolver.Mutation().MintUploadGrant(context.Background(), model.MintUploadGrantInput{
		ContentType: "image/png", MaxSizeBytes: 100, Sha256: round12UploadDigest(round12TinyPNG()),
	})
	require.Error(t, err)
}

func grantIDFromS3Key(t *testing.T, key string) string {
	t.Helper()
	// Key format: media/{yyyy/mm/dd}/{mediaID}{ext}
	parts := strings.Split(key, "/")
	require.GreaterOrEqual(t, len(parts), 4, "key must follow the media/{date}/{mediaID}{ext} convention")
	return strings.TrimSuffix(parts[len(parts)-1], ".png")
}
