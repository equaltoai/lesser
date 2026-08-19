package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type agentRuntimeRepos struct {
	account *repositories.AccountRepository
}

func (r agentRuntimeRepos) Account() *repositories.AccountRepository      { return r.account }
func (agentRuntimeRepos) Actor() interfaces.ActorRepository               { return nil }
func (agentRuntimeRepos) Activity() interfaces.ActivityRepository         { return nil }
func (agentRuntimeRepos) Notification() interfaces.NotificationRepository { return nil }
func (agentRuntimeRepos) Recovery() *repositories.RecoveryRepository      { return nil }
func (agentRuntimeRepos) Audit() *repositories.AuditRepository            { return nil }

func newAgentRuntimeAccountRepo() (*repositories.AccountRepository, *mocks.MockDB, *mocks.MockQuery) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Maybe()

	return repositories.NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop()), mockDB, mockQuery
}

func TestAgentRuntimeHelpers(t *testing.T) {
	t.Parallel()

	require.True(t, IsAgentRuntimeClientID(" lesser-agent-delegation "))
	require.True(t, IsAgentRuntimeClientID("lesser-agent-self-sovereign"))
	require.False(t, IsAgentRuntimeClientID("web-client"))
	require.Equal(t, []string{DelegatedAgentRuntimeClientID, SelfSovereignAgentRuntimeClientID}, AgentRuntimeClientIDs())

	require.Equal(t, "runtime-a", CoalesceAgentRuntimeLabel(" runtime-a ", "fallback"))
	require.Equal(t, "fallback", CoalesceAgentRuntimeLabel(" ", " fallback "))
	require.Equal(t, DefaultAgentRuntimeDeviceLabel, CoalesceAgentRuntimeLabel(" ", " "))

	_, err := IssueAgentRuntimeTokens(context.Background(), nil, nil, AgentRuntimeTokenIssueParams{})
	require.ErrorIs(t, err, ErrSessionStorage)

	_, err = ListAgentRuntimeSessions(context.Background(), nil, "alice")
	require.ErrorIs(t, err, ErrSessionStorage)

	err = RevokeAgentRuntimeFamily(context.Background(), nil, &storage.RefreshToken{FamilyID: "fam-1"}, "reason", "", "")
	require.ErrorIs(t, err, ErrSessionStorage)

	repo, _, _ := newAgentRuntimeAccountRepo()
	err = RevokeAgentRuntimeFamily(context.Background(), agentRuntimeRepos{account: repo}, nil, "reason", "", "")
	require.NoError(t, err)

	err = RevokeAgentRuntimeFamily(context.Background(), agentRuntimeRepos{account: repo}, &storage.RefreshToken{}, "reason", "", "")
	require.NoError(t, err)

	err = RevokeAllAgentRuntimeSessions(context.Background(), nil, "alice", "reason", "", "")
	require.ErrorIs(t, err, ErrSessionStorage)
}

func TestAgentRuntimeSessionHelperBranches(t *testing.T) {
	t.Parallel()

	currentBySessionID := map[string]storage.RefreshToken{}
	now := time.Now().UTC()

	recordCurrentAgentSession(currentBySessionID, storage.RefreshToken{})
	recordCurrentAgentSession(currentBySessionID, storage.RefreshToken{
		SessionID: "sess-1",
		Current:   false,
	})
	require.Empty(t, currentBySessionID)

	recordCurrentAgentSession(currentBySessionID, storage.RefreshToken{
		SessionID:         "sess-1",
		ClientID:          DelegatedAgentRuntimeClientID,
		FamilyID:          "fam-1",
		Current:           true,
		Generation:        1,
		DeviceLabel:       "first",
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(1 * time.Hour),
		AbsoluteExpiresAt: now.Add(2 * time.Hour),
		SessionCreatedAt:  now.Add(-1 * time.Hour),
		AccessTTLSeconds:  1800,
	})
	require.Equal(t, "first", currentBySessionID["sess-1"].DeviceLabel)

	recordCurrentAgentSession(currentBySessionID, storage.RefreshToken{
		SessionID:         "sess-1",
		ClientID:          DelegatedAgentRuntimeClientID,
		FamilyID:          "fam-1",
		Current:           true,
		Generation:        0,
		DeviceLabel:       "older",
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(1 * time.Hour),
		AbsoluteExpiresAt: now.Add(2 * time.Hour),
		SessionCreatedAt:  now.Add(-1 * time.Hour),
		AccessTTLSeconds:  1800,
	})
	require.Equal(t, "first", currentBySessionID["sess-1"].DeviceLabel)

	recordCurrentAgentSession(currentBySessionID, storage.RefreshToken{
		SessionID:         "sess-1",
		ClientID:          DelegatedAgentRuntimeClientID,
		FamilyID:          "fam-1",
		Current:           true,
		Generation:        2,
		DeviceLabel:       "newer",
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(1 * time.Hour),
		AbsoluteExpiresAt: now.Add(2 * time.Hour),
		SessionCreatedAt:  now.Add(-1 * time.Hour),
		AccessTTLSeconds:  1800,
	})
	require.Equal(t, "newer", currentBySessionID["sess-1"].DeviceLabel)

	recordCurrentAgentSession(currentBySessionID, storage.RefreshToken{
		SessionID:   "sess-public",
		ClientID:    "legacy-agent-client",
		FamilyID:    "fam-public",
		Current:     true,
		Generation:  1,
		DeviceLabel: "compat",
	})
	require.NotContains(t, currentBySessionID, "sess-public")

	require.False(t, IsAgentRuntimeRefreshToken(&storage.RefreshToken{
		ClientID:          DelegatedAgentRuntimeClientID,
		SessionID:         "sess-partial",
		LastAuthSuccessAt: time.Now().UTC(),
	}))
}

func TestIssueAgentRuntimeTokens(t *testing.T) {
	t.Parallel()

	repo, _, query := newAgentRuntimeAccountRepo()
	query.On("Create").Return(nil).Once()

	cfg := &config.Config{JWTSecret: "test-secret"}
	bundle, err := IssueAgentRuntimeTokens(context.Background(), cfg, agentRuntimeRepos{account: repo}, AgentRuntimeTokenIssueParams{
		Username: "alice",
		ClientID: "lesser-agent-delegation",
		Scopes:   []string{ScopeRead, ScopeWrite},
	})
	require.NoError(t, err)
	require.NotEmpty(t, bundle.AccessToken)
	require.NotEmpty(t, bundle.RefreshToken)
	require.Equal(t, ClientClassAgent, bundle.Session.ClientClass)
	require.Equal(t, DefaultAgentRuntimeDeviceLabel, bundle.Session.DeviceLabel)
	require.NotEmpty(t, bundle.Session.SessionID)
	require.NotEmpty(t, bundle.Session.FamilyID)
	require.NotEqual(t, bundle.Session.SessionID, bundle.Session.FamilyID)
	require.True(t, bundle.Session.Current)
	require.Equal(t, 1, bundle.Session.Generation)
	require.Equal(t, int(AccessTokenDuration.Seconds()), bundle.Session.AccessTTLSeconds)
	require.WithinDuration(t, bundle.Session.CreatedAt.Add(AgentRuntimeRefreshIdleTTL), bundle.Session.IdleExpiresAt, 2*time.Second)
	require.WithinDuration(t, bundle.Session.CreatedAt.Add(AgentRuntimeRefreshAbsoluteTTL), bundle.Session.AbsoluteExpiresAt, 2*time.Second)

	oauthSvc := NewOAuthService(cfg.JWTSecret, cfg, nil, nil)
	claims, err := oauthSvc.ValidateAccessToken(bundle.AccessToken)
	require.NoError(t, err)
	require.Equal(t, "alice", claims.Username)
	require.Equal(t, "lesser-agent-delegation", claims.ClientID)
	require.Equal(t, []string{ScopeRead, ScopeWrite}, claims.Scopes)
	require.Equal(t, ClientClassAgent, claims.ClientClass)
	require.Equal(t, bundle.Session.SessionID, claims.SessionID)

	t.Run("respects explicit ttl and label", func(t *testing.T) {
		repoCustom, _, queryCustom := newAgentRuntimeAccountRepo()
		queryCustom.On("Create").Return(nil).Once()

		customTTL := 45 * time.Minute
		customBundle, err := IssueAgentRuntimeTokens(context.Background(), cfg, agentRuntimeRepos{account: repoCustom}, AgentRuntimeTokenIssueParams{
			Username:    "alice",
			ClientID:    "lesser-agent-self-sovereign",
			Scopes:      []string{ScopeRead},
			AccessTTL:   customTTL,
			DeviceLabel: "shell-runtime",
		})
		require.NoError(t, err)
		require.Equal(t, "shell-runtime", customBundle.Session.DeviceLabel)
		require.Equal(t, int(customTTL.Seconds()), customBundle.Session.AccessTTLSeconds)
	})

	t.Run("caps refresh session to delegated absolute ttl", func(t *testing.T) {
		repoDelegated, _, queryDelegated := newAgentRuntimeAccountRepo()
		queryDelegated.On("Create").Return(nil).Once()

		delegatedTTL := 90 * time.Minute
		delegatedBundle, err := IssueAgentRuntimeTokens(context.Background(), cfg, agentRuntimeRepos{account: repoDelegated}, AgentRuntimeTokenIssueParams{
			Username:           "alice",
			ClientID:           DelegatedAgentRuntimeClientID,
			Scopes:             []string{ScopeRead},
			AccessTTL:          delegatedTTL,
			RefreshIdleTTL:     delegatedTTL,
			RefreshAbsoluteTTL: delegatedTTL,
			DeviceLabel:        "delegated-runtime",
		})
		require.NoError(t, err)
		require.Equal(t, int(delegatedTTL.Seconds()), delegatedBundle.Session.AccessTTLSeconds)
		require.WithinDuration(t, delegatedBundle.Session.CreatedAt.Add(delegatedTTL), delegatedBundle.Session.IdleExpiresAt, 2*time.Second)
		require.WithinDuration(t, delegatedBundle.Session.CreatedAt.Add(delegatedTTL), delegatedBundle.Session.AbsoluteExpiresAt, 2*time.Second)
		require.WithinDuration(t, delegatedBundle.Session.AbsoluteExpiresAt, delegatedBundle.Session.ExpiresAt, time.Second)
	})

	t.Run("propagates refresh storage failures", func(t *testing.T) {
		repoErr, _, queryErr := newAgentRuntimeAccountRepo()
		queryErr.On("Create").Return(errors.New("create failed")).Once()

		_, err := IssueAgentRuntimeTokens(context.Background(), cfg, agentRuntimeRepos{account: repoErr}, AgentRuntimeTokenIssueParams{
			Username: "alice",
			ClientID: "lesser-agent-delegation",
			Scopes:   []string{ScopeRead},
		})
		require.Error(t, err)
	})
}

func TestListAgentRuntimeSessions(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)
	repo, _, query := newAgentRuntimeAccountRepo()

	allCalls := 0
	query.On("All", mock.Anything).Return(nil).Times(2).Run(func(args mock.Arguments) {
		allCalls++
		switch target := args.Get(0).(type) {
		case *[]models.RefreshToken:
			switch allCalls {
			case 1:
				*target = []models.RefreshToken{
					{Token: "old", ClientID: "lesser-agent-delegation", Username: "alice", SessionID: "sid-1", FamilyID: "fam-1", Current: true, Generation: 1, LastUsedAt: baseTime.Add(-2 * time.Hour), IdleExpiresAt: baseTime.Add(1 * time.Hour), AbsoluteExpiresAt: baseTime.Add(2 * time.Hour), SessionCreatedAt: baseTime.Add(-4 * time.Hour), AccessTTLSeconds: 1800, Version: 1},
					{Token: "new", ClientID: "lesser-agent-delegation", Username: "alice", SessionID: "sid-1", FamilyID: "fam-1", Current: true, Generation: 2, LastUsedAt: baseTime.Add(-1 * time.Hour), IdleExpiresAt: baseTime.Add(1 * time.Hour), AbsoluteExpiresAt: baseTime.Add(2 * time.Hour), SessionCreatedAt: baseTime.Add(-4 * time.Hour), AccessTTLSeconds: 1800, DeviceLabel: "desktop", Version: 1},
					{Token: "skip-noncurrent", ClientID: "lesser-agent-delegation", Username: "alice", SessionID: "sid-2", FamilyID: "fam-2", Current: false, Generation: 1, LastUsedAt: baseTime, IdleExpiresAt: baseTime.Add(1 * time.Hour), AbsoluteExpiresAt: baseTime.Add(2 * time.Hour), SessionCreatedAt: baseTime.Add(-4 * time.Hour), AccessTTLSeconds: 1800, Version: 1},
					{Token: "skip-blank", ClientID: "lesser-agent-delegation", Username: "alice", SessionID: " ", FamilyID: "fam-blank", Current: true, Generation: 1, LastUsedAt: baseTime, IdleExpiresAt: baseTime.Add(1 * time.Hour), AbsoluteExpiresAt: baseTime.Add(2 * time.Hour), SessionCreatedAt: baseTime.Add(-4 * time.Hour), AccessTTLSeconds: 1800, Version: 1},
					{Token: "skip-public", ClientID: "legacy-agent-client", Username: "alice", SessionID: "sid-public", FamilyID: "fam-public", Current: true, Generation: 1, LastUsedAt: baseTime.Add(30 * time.Minute), DeviceLabel: "compat", Version: 1},
				}
			case 2:
				*target = []models.RefreshToken{
					{Token: "other", ClientID: "lesser-agent-self-sovereign", Username: "alice", SessionID: "sid-3", FamilyID: "fam-3", Current: true, Generation: 1, LastUsedAt: baseTime, IdleExpiresAt: baseTime.Add(1 * time.Hour), AbsoluteExpiresAt: baseTime.Add(2 * time.Hour), SessionCreatedAt: baseTime.Add(-4 * time.Hour), AccessTTLSeconds: 1800, DeviceLabel: "cloud", Version: 1},
				}
			}
		}
	})

	sessions, err := ListAgentRuntimeSessions(context.Background(), agentRuntimeRepos{account: repo}, "alice")
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "sid-3", sessions[0].SessionID)
	require.Equal(t, "sid-1", sessions[1].SessionID)
	require.Equal(t, 2, sessions[1].Generation)
	require.Equal(t, "desktop", sessions[1].DeviceLabel)

	repoErr, _, queryErr := newAgentRuntimeAccountRepo()
	queryErr.On("All", mock.Anything).Return(errors.New("boom")).Once()

	_, err = ListAgentRuntimeSessions(context.Background(), agentRuntimeRepos{account: repoErr}, "alice")
	require.Error(t, err)
}

func TestRevokeAgentRuntimeFlows(t *testing.T) {
	t.Parallel()

	t.Run("family sweep continues after an item failure and reconciles once", func(t *testing.T) {
		repo, db, query := newAgentRuntimeAccountRepo()
		db.ExpectedCalls = nil
		db.On("WithContext", mock.Anything).Return(db).Maybe()
		currentToken := ""
		db.On("Model", mock.Anything).Return(query).Maybe().Run(func(args mock.Arguments) {
			if model, ok := args.Get(0).(*models.RefreshToken); ok {
				currentToken = model.Token
			}
		})

		listCalls := 0
		query.On("All", mock.Anything).Return(nil).Twice().Run(func(args mock.Arguments) {
			listCalls++
			target := args.Get(0).(*[]models.RefreshToken)
			if listCalls == 1 {
				*target = []models.RefreshToken{
					{Token: "rt-first-fails", FamilyID: "fam-reconcile", Username: "alice", ClientID: "client-1", Version: 1},
					{Token: "rt-second-succeeds", FamilyID: "fam-reconcile", Username: "alice", ClientID: "client-1", Version: 1},
				}
				return
			}
			*target = []models.RefreshToken{
				{Token: "rt-first-fails", FamilyID: "fam-reconcile", Username: "alice", ClientID: "client-1", Version: 1},
			}
		})

		var attempts []string
		query.On("Update", mock.Anything).Return(errors.New("first CAS failed")).Once().Run(func(mock.Arguments) {
			attempts = append(attempts, currentToken)
		})
		query.On("Update", mock.Anything).Return(nil).Twice().Run(func(mock.Arguments) {
			attempts = append(attempts, currentToken)
		})

		err := RevokeAgentRuntimeFamily(context.Background(), agentRuntimeRepos{account: repo}, &storage.RefreshToken{FamilyID: "fam-reconcile"}, "reuse", "198.51.100.5", "client/1")
		require.NoError(t, err)
		require.Equal(t, []string{"rt-first-fails", "rt-second-succeeds", "rt-first-fails"}, attempts)
	})

	t.Run("family revocation updates active and reused tokens", func(t *testing.T) {
		repo, db, query := newAgentRuntimeAccountRepo()

		var updated []models.RefreshToken
		db.ExpectedCalls = nil
		db.On("WithContext", mock.Anything).Return(db).Maybe()
		db.On("Model", mock.Anything).Return(query).Maybe().Run(func(args mock.Arguments) {
			model, ok := args.Get(0).(*models.RefreshToken)
			if ok && model.Token != "" {
				updated = append(updated, *model)
			}
		})

		query.On("All", mock.Anything).Return(nil).Twice().Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.RefreshToken)
			*target = []models.RefreshToken{
				{Token: "rt-active", FamilyID: "fam-1", Username: "alice", ClientID: "lesser-agent-delegation", Revoked: false, Version: 1},
				{Token: "rt-revoked", FamilyID: "fam-1", Username: "alice", ClientID: "lesser-agent-delegation", Revoked: true, Version: 1},
			}
		})
		query.On("Update", mock.Anything).Return(nil).Twice()

		err := RevokeAgentRuntimeFamily(context.Background(), agentRuntimeRepos{account: repo}, &storage.RefreshToken{FamilyID: "fam-1"}, "manual", "198.51.100.10", "agent/1.0")
		require.NoError(t, err)
		require.Len(t, updated, 2)

		updatesByToken := map[string]models.RefreshToken{}
		for _, item := range updated {
			updatesByToken[item.Token] = item
		}

		require.True(t, updatesByToken["rt-active"].Revoked)
		require.Equal(t, "manual", updatesByToken["rt-active"].RevokedReason)
		require.Equal(t, "198.51.100.10", updatesByToken["rt-active"].ReuseDetectedFromIP)
		require.Equal(t, "agent/1.0", updatesByToken["rt-active"].ReuseDetectedFromUA)
		require.False(t, updatesByToken["rt-active"].ReuseDetectedAt.IsZero())

		require.True(t, updatesByToken["rt-revoked"].Revoked)
		require.Equal(t, "198.51.100.10", updatesByToken["rt-revoked"].ReuseDetectedFromIP)
		require.Equal(t, "agent/1.0", updatesByToken["rt-revoked"].ReuseDetectedFromUA)
		require.False(t, updatesByToken["rt-revoked"].ReuseDetectedAt.IsZero())
	})

	t.Run("family errors and pre-marked reuse", func(t *testing.T) {
		repoErr, _, queryErr := newAgentRuntimeAccountRepo()
		queryErr.On("All", mock.Anything).Return(errors.New("list failed")).Twice()

		err := RevokeAgentRuntimeFamily(context.Background(), agentRuntimeRepos{account: repoErr}, &storage.RefreshToken{FamilyID: "fam-err"}, "manual", "", "")
		require.Error(t, err)

		repoSkip, _, querySkip := newAgentRuntimeAccountRepo()
		querySkip.On("All", mock.Anything).Return(nil).Twice().Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.RefreshToken)
			*target = []models.RefreshToken{
				{
					Token:           "rt-reused",
					FamilyID:        "fam-skip",
					Username:        "alice",
					ClientID:        "lesser-agent-delegation",
					Revoked:         true,
					ReuseDetectedAt: time.Now().UTC(),
					Version:         1,
				},
			}
		})

		err = RevokeAgentRuntimeFamily(context.Background(), agentRuntimeRepos{account: repoSkip}, &storage.RefreshToken{FamilyID: "fam-skip"}, "manual", "", "")
		require.NoError(t, err)
	})

	t.Run("session revoke and not found", func(t *testing.T) {
		repo, _, query := newAgentRuntimeAccountRepo()

		allCalls := 0
		query.On("All", mock.Anything).Return(nil).Times(4).Run(func(args mock.Arguments) {
			allCalls++
			switch target := args.Get(0).(type) {
			case *[]models.RefreshToken:
				switch allCalls {
				case 1:
					*target = []models.RefreshToken{
						{Token: "rt-current", FamilyID: "fam-1", SessionID: "sid-1", ClientID: "lesser-agent-delegation", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				case 2:
					*target = []models.RefreshToken{}
				case 3, 4:
					*target = []models.RefreshToken{
						{Token: "rt-current", FamilyID: "fam-1", SessionID: "sid-1", ClientID: "lesser-agent-delegation", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				}
			}
		})
		query.On("Update", mock.Anything).Return(nil).Once()

		err := RevokeAgentRuntimeSession(context.Background(), agentRuntimeRepos{account: repo}, "alice", "sid-1", "manual", "", "")
		require.NoError(t, err)

		repoMissing, _, queryMissing := newAgentRuntimeAccountRepo()
		queryMissing.On("All", mock.Anything).Return(nil).Times(2).Run(func(args mock.Arguments) {
			switch target := args.Get(0).(type) {
			case *[]models.RefreshToken:
				*target = []models.RefreshToken{}
			}
		})

		err = RevokeAgentRuntimeSession(context.Background(), agentRuntimeRepos{account: repoMissing}, "alice", "missing", "manual", "", "")
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("session list errors propagate", func(t *testing.T) {
		repoErr, _, queryErr := newAgentRuntimeAccountRepo()
		queryErr.On("All", mock.Anything).Return(errors.New("list failed")).Once()

		err := RevokeAgentRuntimeSession(context.Background(), agentRuntimeRepos{account: repoErr}, "alice", "sid-1", "manual", "", "")
		require.Error(t, err)
	})

	t.Run("revoke all sessions", func(t *testing.T) {
		repo, _, query := newAgentRuntimeAccountRepo()

		allCalls := 0
		query.On("All", mock.Anything).Return(nil).Times(6).Run(func(args mock.Arguments) {
			allCalls++
			switch target := args.Get(0).(type) {
			case *[]models.RefreshToken:
				switch allCalls {
				case 1:
					*target = []models.RefreshToken{
						{Token: "delegation", FamilyID: "fam-delegation", SessionID: "sid-delegation", ClientID: "lesser-agent-delegation", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				case 2:
					*target = []models.RefreshToken{
						{Token: "self", FamilyID: "fam-self", SessionID: "sid-self", ClientID: "lesser-agent-self-sovereign", Current: true, Generation: 1, LastUsedAt: time.Now().Add(-1 * time.Minute).UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				case 3:
					*target = []models.RefreshToken{
						{Token: "delegation", FamilyID: "fam-delegation", SessionID: "sid-delegation", ClientID: "lesser-agent-delegation", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				case 5, 6:
					*target = []models.RefreshToken{
						{Token: "self", FamilyID: "fam-self", SessionID: "sid-self", ClientID: "lesser-agent-self-sovereign", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				}
			}
		})
		query.On("Update", mock.Anything).Return(nil).Twice()

		err := RevokeAllAgentRuntimeSessions(context.Background(), agentRuntimeRepos{account: repo}, "alice", "owner_signout", "203.0.113.20", "agent/2.0")
		require.NoError(t, err)
	})

	t.Run("revoke all surfaces family failures", func(t *testing.T) {
		repoErr, _, queryErr := newAgentRuntimeAccountRepo()
		allCalls := 0
		queryErr.On("All", mock.Anything).Return(nil).Times(4).Run(func(args mock.Arguments) {
			allCalls++
			switch target := args.Get(0).(type) {
			case *[]models.RefreshToken:
				switch allCalls {
				case 1:
					*target = []models.RefreshToken{
						{Token: "delegation", FamilyID: "fam-delegation", SessionID: "sid-delegation", ClientID: "lesser-agent-delegation", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				case 2:
					*target = []models.RefreshToken{}
				case 3, 4:
					*target = []models.RefreshToken{
						{Token: "delegation", FamilyID: "fam-delegation", SessionID: "sid-delegation", ClientID: "lesser-agent-delegation", Current: true, Generation: 1, LastUsedAt: time.Now().UTC(), IdleExpiresAt: time.Now().Add(1 * time.Hour).UTC(), AbsoluteExpiresAt: time.Now().Add(2 * time.Hour).UTC(), SessionCreatedAt: time.Now().Add(-1 * time.Hour).UTC(), AccessTTLSeconds: 1800, Version: 1},
					}
				}
			}
		})
		queryErr.On("Update", mock.Anything).Return(errors.New("update failed")).Twice()

		err := RevokeAllAgentRuntimeSessions(context.Background(), agentRuntimeRepos{account: repoErr}, "alice", "owner_signout", "", "")
		require.Error(t, err)
	})
}

func TestAgentRuntimeAuthDiagnostics(t *testing.T) {
	t.Parallel()

	t.Run("status reflects latest auth event and lifecycle state", func(t *testing.T) {
		now := time.Date(2026, time.March, 20, 16, 0, 0, 0, time.UTC)

		healthy := RuntimeSessionAuthDiagnostic(now, storage.RefreshToken{
			LastAuthSuccessAt: now.Add(-5 * time.Minute),
		})
		require.Equal(t, AgentRuntimeAuthStatusHealthy, healthy.Status)

		failed := RuntimeSessionAuthDiagnostic(now, storage.RefreshToken{
			LastAuthSuccessAt:   now.Add(-10 * time.Minute),
			LastAuthFailureCode: "invalid_client",
			LastAuthFailureMsg:  "Invalid client credentials",
			LastAuthFailureAt:   now.Add(-1 * time.Minute),
		})
		require.Equal(t, AgentRuntimeAuthStatusFailed, failed.Status)
		require.Equal(t, "invalid_client", failed.FailureCode)

		expired := RuntimeSessionAuthDiagnostic(now, storage.RefreshToken{
			IdleExpiresAt: now.Add(-1 * time.Minute),
		})
		require.Equal(t, AgentRuntimeAuthStatusExpired, expired.Status)

		revoked := RuntimeSessionAuthDiagnostic(now, storage.RefreshToken{
			Revoked:       true,
			RevokedReason: "manual_runtime_session_revocation",
		})
		require.Equal(t, AgentRuntimeAuthStatusRevoked, revoked.Status)
	})

	t.Run("record failure and success across runtime session storage targets", func(t *testing.T) {
		repo, db, query := newAgentRuntimeAccountRepo()

		var updated []models.RefreshToken
		db.ExpectedCalls = nil
		db.On("WithContext", mock.Anything).Return(db).Maybe()
		db.On("Model", mock.Anything).Return(query).Maybe().Run(func(args mock.Arguments) {
			model, ok := args.Get(0).(*models.RefreshToken)
			if ok && model.Token != "" {
				updated = append(updated, *model)
			}
		})

		query.On("All", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.RefreshToken)
			*target = []models.RefreshToken{
				{Token: "rt-1", FamilyID: "fam-1", Version: 1},
				{Token: "rt-2", FamilyID: "fam-1", Version: 1},
			}
		})
		query.On("All", mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.RefreshToken)
			*target = []models.RefreshToken{
				{Token: "rt-session", SessionID: "sid-1", Version: 1},
			}
		})
		query.On("Update", mock.Anything).Return(nil).Times(3)

		failureAt := time.Date(2026, time.March, 20, 16, 5, 0, 0, time.UTC)
		familyToken := &storage.RefreshToken{Token: "rt-1", ClientID: DelegatedAgentRuntimeClientID, SessionID: "sid-1", FamilyID: "fam-1", Generation: 1, LastUsedAt: failureAt, IdleExpiresAt: failureAt.Add(1 * time.Hour), AbsoluteExpiresAt: failureAt.Add(2 * time.Hour), SessionCreatedAt: failureAt.Add(-1 * time.Hour), AccessTTLSeconds: 1800}
		err := RecordAgentRuntimeAuthFailure(context.Background(), agentRuntimeRepos{account: repo}, familyToken, "invalid_client", "Invalid client credentials", failureAt)
		require.NoError(t, err)
		require.Equal(t, "invalid_client", familyToken.LastAuthFailureCode)
		require.Equal(t, failureAt, familyToken.LastAuthFailureAt)

		sessionToken := &storage.RefreshToken{Token: "rt-session", ClientID: DelegatedAgentRuntimeClientID, SessionID: "sid-1", FamilyID: "fam-1", Generation: 1, LastUsedAt: failureAt, IdleExpiresAt: failureAt.Add(1 * time.Hour), AbsoluteExpiresAt: failureAt.Add(2 * time.Hour), SessionCreatedAt: failureAt.Add(-1 * time.Hour), AccessTTLSeconds: 1800}
		successAt := failureAt.Add(2 * time.Minute)
		err = RecordAgentRuntimeAuthSuccess(context.Background(), agentRuntimeRepos{account: repo}, sessionToken, successAt)
		require.NoError(t, err)
		require.Equal(t, successAt, sessionToken.LastAuthSuccessAt)

		require.Len(t, updated, 3)
		require.Equal(t, "invalid_client", updated[0].LastAuthFailureCode)
		require.Equal(t, "Invalid client credentials", updated[0].LastAuthFailureMsg)
		require.Equal(t, failureAt, updated[1].LastAuthFailureAt)
		require.Equal(t, successAt, updated[2].LastAuthSuccessAt)
	})

	t.Run("non-runtime tokens do not fan out through runtime diagnostics", func(t *testing.T) {
		repo, _, _ := newAgentRuntimeAccountRepo()

		failureAt := time.Date(2026, time.March, 20, 17, 0, 0, 0, time.UTC)
		token := &storage.RefreshToken{
			Token:     "rt-public",
			ClientID:  "legacy-agent-client",
			SessionID: "sid-public",
			FamilyID:  "fam-public",
		}

		err := RecordAgentRuntimeAuthFailure(context.Background(), agentRuntimeRepos{account: repo}, token, "invalid_client", "Invalid client credentials", failureAt)
		require.NoError(t, err)
		require.Equal(t, "invalid_client", token.LastAuthFailureCode)
		require.Equal(t, "Invalid client credentials", token.LastAuthFailureMsg)
		require.Equal(t, failureAt, token.LastAuthFailureAt)
	})
}
