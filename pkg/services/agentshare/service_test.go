package agentshare

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type testRepository struct {
	grants map[string]*models.AgentShareGrant
}

func newTestRepository() *testRepository {
	return &testRepository{grants: map[string]*models.AgentShareGrant{}}
}

func testGrantKey(agent, grantee string) string { return agent + "/" + grantee }

func cloneGrant(grant *models.AgentShareGrant) *models.AgentShareGrant {
	if grant == nil {
		return nil
	}
	copy := *grant
	return &copy
}

func (r *testRepository) CreateAgentShareGrant(_ context.Context, grant *models.AgentShareGrant) error {
	key := testGrantKey(grant.AgentUsername, grant.GranteeUsername)
	if _, exists := r.grants[key]; exists {
		return errors.New("already exists")
	}
	grant.Version = 1
	r.grants[key] = cloneGrant(grant)
	return nil
}

func (r *testRepository) RegrantAgentShareGrant(_ context.Context, grant *models.AgentShareGrant) error {
	grant.Version++
	grant.RevokedAt = nil
	grant.RevokedBy = ""
	r.grants[testGrantKey(grant.AgentUsername, grant.GranteeUsername)] = cloneGrant(grant)
	return nil
}

func (r *testRepository) RevokeAgentShareGrant(_ context.Context, grant *models.AgentShareGrant) error {
	grant.Version++
	r.grants[testGrantKey(grant.AgentUsername, grant.GranteeUsername)] = cloneGrant(grant)
	return nil
}

func (r *testRepository) GetAgentShareGrant(_ context.Context, agent, grantee string) (*models.AgentShareGrant, error) {
	grant := r.grants[testGrantKey(agent, grantee)]
	if grant == nil {
		return nil, storage.ErrNotFound
	}
	return cloneGrant(grant), nil
}

func (r *testRepository) ListAgentShareGrantsByAgent(_ context.Context, agent string) ([]*models.AgentShareGrant, error) {
	result := []*models.AgentShareGrant{}
	for _, grant := range r.grants {
		if grant.AgentUsername == agent {
			result = append(result, cloneGrant(grant))
		}
	}
	return result, nil
}

func (r *testRepository) ListActiveAgentShareGrantsByGrantee(_ context.Context, grantee string) ([]*models.AgentShareGrant, error) {
	result := []*models.AgentShareGrant{}
	for _, grant := range r.grants {
		if grant.GranteeUsername == grantee && grant.RevokedAt == nil {
			result = append(result, cloneGrant(grant))
		}
	}
	return result, nil
}

func (r *testRepository) IsActiveAgentShareGrant(ctx context.Context, agent, grantee string) (bool, error) {
	grant, err := r.GetAgentShareGrant(ctx, agent, grantee)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}
	return err == nil && grant.RevokedAt == nil, err
}

type testAccounts struct {
	agents map[string]*storage.Account
	users  map[string]*storage.User
}

func (a *testAccounts) GetAccount(_ context.Context, username string) (*storage.Account, error) {
	account := a.agents[username]
	if account == nil {
		return nil, storage.ErrNotFound
	}
	return account, nil
}

func (a *testAccounts) GetUser(_ context.Context, username string) (*storage.User, error) {
	user := a.users[username]
	if user == nil {
		return nil, storage.ErrNotFound
	}
	return user, nil
}

type auditEvent struct {
	eventType string
	username  string
	metadata  map[string]interface{}
}

type testAudit struct{ events []auditEvent }

func (a *testAudit) StoreAuditEvent(_ context.Context, eventType, _ string, username, _ string, _, _, _, _, _ string, _ bool, _ string, metadata map[string]interface{}) error {
	a.events = append(a.events, auditEvent{eventType: eventType, username: username, metadata: metadata})
	return nil
}

func newTestService() (*Service, *testRepository, *testAudit) {
	repo := newTestRepository()
	audit := &testAudit{}
	accounts := &testAccounts{
		agents: map[string]*storage.Account{
			"agent-one": {User: &storage.User{Username: "agent-one", IsAgent: true, AgentOwner: "@owner"}},
		},
		users: map[string]*storage.User{
			"owner": {Username: "owner"},
			"alice": {Username: "alice"},
			"admin": {Username: "admin"},
		},
	}
	service := NewService(repo, accounts, audit, zap.NewNop())
	service.now = func() time.Time { return time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC) }
	return service, repo, audit
}

func TestServiceGrantRegrantRevokeAndList(t *testing.T) {
	service, _, audit := newTestService()
	ctx := context.Background()
	input := ManageInput{AgentUsername: "Agent-One", GranteeUsername: "Alice", ActorUsername: "Owner"}

	grant, err := service.Grant(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "owner", grant.GrantedBy)
	require.Equal(t, 1, grant.Version)
	require.Len(t, audit.events, 1)
	require.Equal(t, "agent.share.grant", audit.events[0].eventType)
	require.Equal(t, "agent-one", audit.events[0].username)

	service.now = func() time.Time { return time.Date(2026, time.August, 10, 13, 0, 0, 0, time.UTC) }
	refreshed, err := service.Grant(ctx, input)
	require.NoError(t, err)
	require.Equal(t, 2, refreshed.Version)
	require.Equal(t, 13, refreshed.GrantedAt.Hour())

	ownerView, err := service.ListByAgent(ctx, "agent-one", "owner", false)
	require.NoError(t, err)
	require.Len(t, ownerView, 1)
	sharedWith, err := service.ListSharedWith(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, sharedWith, 1)
	active, err := service.IsActive(ctx, "agent-one", "alice")
	require.NoError(t, err)
	require.True(t, active)

	revoked, err := service.Revoke(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, revoked.RevokedAt)
	require.Equal(t, "owner", revoked.RevokedBy)
	require.Equal(t, "agent.share.revoke", audit.events[len(audit.events)-1].eventType)
	active, err = service.IsActive(ctx, "agent-one", "alice")
	require.NoError(t, err)
	require.False(t, active, "revoked grants must be inactive without a cache window")
	sharedWith, err = service.ListSharedWith(ctx, "alice")
	require.NoError(t, err)
	require.Empty(t, sharedWith)
	ownerView, err = service.ListByAgent(ctx, "agent-one", "owner", false)
	require.NoError(t, err)
	require.Len(t, ownerView, 1)
	require.NotNil(t, ownerView[0].RevokedAt)
}

func TestServiceGrantAuthorizationAndGranteeValidation(t *testing.T) {
	service, _, _ := newTestService()
	ctx := context.Background()

	tests := []struct {
		name  string
		input ManageInput
		want  error
	}{
		{name: "non-owner", input: ManageInput{AgentUsername: "agent-one", GranteeUsername: "alice", ActorUsername: "intruder"}, want: ErrNotAuthorized},
		{name: "unknown grantee", input: ManageInput{AgentUsername: "agent-one", GranteeUsername: "missing", ActorUsername: "owner"}, want: ErrGranteeNotFound},
		{name: "remote grantee", input: ManageInput{AgentUsername: "agent-one", GranteeUsername: "alice@remote.example", ActorUsername: "owner"}, want: ErrInvalidGrantee},
		{name: "owner self grant", input: ManageInput{AgentUsername: "agent-one", GranteeUsername: "owner", ActorUsername: "owner"}, want: ErrSelfGrant},
		{name: "agent self grant", input: ManageInput{AgentUsername: "agent-one", GranteeUsername: "agent-one", ActorUsername: "owner"}, want: ErrAgentSelfGrant},
		{name: "admin self grant", input: ManageInput{AgentUsername: "agent-one", GranteeUsername: "admin", ActorUsername: "admin", ActorIsAdmin: true}, want: ErrSelfGrant},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Grant(ctx, tc.input)
			require.ErrorIs(t, err, tc.want)
		})
	}

	grant, err := service.Grant(ctx, ManageInput{AgentUsername: "agent-one", GranteeUsername: "alice", ActorUsername: "admin", ActorIsAdmin: true})
	require.NoError(t, err)
	require.Equal(t, "admin", grant.GrantedBy)
}

func TestServiceRevokeMissingGrant(t *testing.T) {
	service, _, _ := newTestService()
	_, err := service.Revoke(context.Background(), ManageInput{
		AgentUsername:   "agent-one",
		GranteeUsername: "alice",
		ActorUsername:   "owner",
	})
	require.ErrorIs(t, err, ErrGrantNotFound)
}
