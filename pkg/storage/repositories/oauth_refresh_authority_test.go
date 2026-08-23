package repositories

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	tablemocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func newOAuthRefreshAuthorityRepository(t *testing.T, dataKeyCalls int) (*AccountRepository, *fakedb.Fake, *tablemocks.MockKMSClient) {
	t.Helper()
	fake := fakedb.New()
	kmsClient := new(tablemocks.MockKMSClient)
	kmsClient.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
		Return(&kms.GenerateDataKeyOutput{
			Plaintext:      bytes.Repeat([]byte{0x2a}, 32),
			CiphertextBlob: []byte("encrypted-data-key"),
		}, nil).
		Times(dataKeyCalls)
	db, err := tabletheory.NewWithClient(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
		KMSClient: kmsClient,
	}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.RefreshToken{}))
	return NewAccountRepository(db, models.MainTableName, "example.com", zap.NewNop()), fake, kmsClient
}

func oauthRefreshAuthorityToken(raw string, now time.Time) *storage.RefreshToken {
	return &storage.RefreshToken{
		Token: raw, Username: "alice", ClientID: "client-1", Resource: "https://example.com/mcp/alice",
		Scopes: []string{"read"}, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		FamilyID: "family-1", Generation: 1, Current: true,
	}
}

func TestOAuthRefreshAuthorityConcurrentCASPersistsOneEncryptedSuccessor(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	repo, fake, kmsClient := newOAuthRefreshAuthorityRepository(t, 2)
	predecessor := oauthRefreshAuthorityToken("predecessor", now)
	require.NoError(t, repo.CreateRefreshToken(ctx, predecessor))

	first, err := repo.GetRefreshTokenConsistent(ctx, predecessor.Token)
	require.NoError(t, err)
	second, err := repo.GetRefreshTokenConsistent(ctx, predecessor.Token)
	require.NoError(t, err)
	successors := []*storage.RefreshToken{
		oauthRefreshAuthorityToken("successor-a", now),
		oauthRefreshAuthorityToken("successor-b", now),
	}
	for _, successor := range successors {
		successor.Generation = 2
	}

	starts := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, current := range []*storage.RefreshToken{first, second} {
		wg.Add(1)
		go func(current, successor *storage.RefreshToken) {
			defer wg.Done()
			<-starts
			nextAuthority := OAuthRefreshAuthorityWithHead(
				nil, current.Username, current.ClientID, current.Resource, current.FamilyID,
				storage.RefreshTokenReplacementHash(successor.Token), successor.Generation, successor.ExpiresAt, now,
			)
			errs <- repo.RotateRefreshTokenWithAuthority(ctx, current, successor, nil, nextAuthority, now)
		}(current, successors[i])
	}
	close(starts)
	wg.Wait()
	close(errs)

	successes, conflicts := 0, 0
	for rotateErr := range errs {
		if rotateErr == nil {
			successes++
			continue
		}
		if IsOAuthRefreshCASConflict(rotateErr) {
			conflicts++
			continue
		}
		t.Logf("unexpected rotation error: %v", rotateErr)
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)

	authority, err := repo.GetOAuthRefreshAuthority(ctx, predecessor.Username, predecessor.ClientID, predecessor.Resource)
	require.NoError(t, err)
	require.Len(t, authority.Slots, 1)
	winnerHash := authority.Slots[0].HeadTokenHash
	require.Contains(t, []string{
		storage.RefreshTokenReplacementHash("successor-a"),
		storage.RefreshTokenReplacementHash("successor-b"),
	}, winnerHash)

	var encryptedArtifact map[string]types.AttributeValue
	for _, item := range fake.Items(models.MainTableName) {
		if _, ok := item["successorToken"]; ok {
			encryptedArtifact = item
			break
		}
	}
	require.NotNil(t, encryptedArtifact)
	_, plaintext := encryptedArtifact["successorToken"].(*types.AttributeValueMemberS)
	require.False(t, plaintext, "successor credential must not persist as plaintext")
	_, envelope := encryptedArtifact["successorToken"].(*types.AttributeValueMemberM)
	require.True(t, envelope, "successor credential must persist as an encrypted envelope")

	winnerRaw := "successor-a"
	if winnerHash == storage.RefreshTokenReplacementHash("successor-b") {
		winnerRaw = "successor-b"
	}
	kmsClient.On("Decrypt", mock.Anything, mock.Anything, mock.Anything).
		Return(&kms.DecryptOutput{Plaintext: bytes.Repeat([]byte{0x2a}, 32)}, nil).
		Once()
	artifact, err := repo.GetOAuthRefreshSuccessorArtifact(
		ctx, predecessor.FamilyID, storage.RefreshTokenReplacementHash(predecessor.Token),
	)
	require.NoError(t, err)
	require.Equal(t, winnerRaw, artifact.SuccessorToken)
	kmsClient.AssertExpectations(t)
}
