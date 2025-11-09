package accounts

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEnsureQuotePermissionsForNewUser(t *testing.T) {
	svc := &Service{logger: zap.NewNop()}
	ctx := context.Background()
	repo := &quotePermissionsRepoMock{}

	testCases := []struct {
		name            string
		visibility      string
		expectPublic    bool
		expectFollowers bool
		expectMentioned bool
	}{
		{
			name:            "public_defaults",
			visibility:      models.VisibilityPublic,
			expectPublic:    true,
			expectFollowers: true,
			expectMentioned: true,
		},
		{
			name:            "unlisted_maps_to_public",
			visibility:      models.VisibilityUnlisted,
			expectPublic:    true,
			expectFollowers: true,
			expectMentioned: true,
		},
		{
			name:            "private_limits_to_followers",
			visibility:      models.VisibilityPrivate,
			expectPublic:    false,
			expectFollowers: true,
			expectMentioned: true,
		},
		{
			name:            "direct_mentions_only",
			visibility:      models.VisibilityDirect,
			expectPublic:    false,
			expectFollowers: false,
			expectMentioned: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			repo.Reset()
			err := svc.ensureQuotePermissionsForNewUser(ctx, repo, "alice", tc.visibility)
			require.NoError(t, err)

			require.NotNil(t, repo.last)
			assert.Equal(t, fmt.Sprintf(models.KeyPatternUser, "alice"), repo.last.PK)
			assert.Equal(t, "QUOTE_PERMISSIONS", repo.last.SK)
			assert.Equal(t, tc.expectPublic, repo.last.AllowPublic)
			assert.Equal(t, tc.expectFollowers, repo.last.AllowFollowers)
			assert.Equal(t, tc.expectMentioned, repo.last.AllowMentioned)
			assert.Empty(t, repo.last.BlockList)
		})
	}
}

func TestEnsureQuotePermissionsForNewUserMissingRepo(t *testing.T) {
	svc := &Service{logger: zap.NewNop()}
	err := svc.ensureQuotePermissionsForNewUser(context.Background(), nil, "alice", models.VisibilityPublic)
	require.ErrorIs(t, err, ErrQuoteRepositoryNotAvailable)
}

func TestPersistDefaultPostingVisibility(t *testing.T) {
	svc := &Service{}
	repo := &accountRepoMock{}
	ctx := context.Background()

	err := svc.persistDefaultPostingVisibility(ctx, repo, "alice", models.VisibilityPrivate)
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"default_posting_visibility": models.VisibilityPrivate}, repo.lastPrefs)
}

func TestPersistDefaultPostingVisibilityError(t *testing.T) {
	svc := &Service{}
	repo := &accountRepoMock{updateErr: errors.New("boom")}

	err := svc.persistDefaultPostingVisibility(context.Background(), repo, "alice", models.VisibilityPrivate)
	require.Error(t, err)
	assert.Nil(t, repo.lastPrefs)
}

func TestRollbackAccountCreation(t *testing.T) {
	logger := zap.NewNop()
	svc := &Service{logger: logger}
	repo := &accountRepoMock{}

	svc.rollbackAccountCreation(context.Background(), repo, "alice", errors.New("failure"))
	assert.Equal(t, 1, repo.deleteCalls)
}

type quotePermissionsRepoMock struct {
	last *models.QuotePermissions
}

func (m *quotePermissionsRepoMock) CreateQuotePermissions(_ context.Context, permissions *models.QuotePermissions) error {
	m.last = permissions
	return nil
}

func (m *quotePermissionsRepoMock) Reset() {
	m.last = nil
}

type accountRepoMock struct {
	lastPrefs   map[string]interface{}
	updateErr   error
	deleteErr   error
	deleteCalls int
}

func (m *accountRepoMock) UpdateAccountPreferences(_ context.Context, _ string, preferences map[string]interface{}) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.lastPrefs = preferences
	return nil
}

func (m *accountRepoMock) DeleteAccount(_ context.Context, _ string) error {
	m.deleteCalls++
	return m.deleteErr
}
