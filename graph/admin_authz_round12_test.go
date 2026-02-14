package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestAdminAccountsRequiresAdmin_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	adminCtx := round12AuthContext("admin")
	mut := resolver.Mutation()
	qry := resolver.Query()

	_, err := mut.AdminCreateUser(adminCtx, model.AdminCreateUserInput{Username: "bob"})
	require.NoError(t, err)

	first := 1
	_, err = qry.AdminAccounts(round12AuthContext("bob"), &first, nil)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
	require.Equal(t, 403, apperrors.GetHTTPStatus(err))
}

func TestUpdateAdminAgentPolicyRequiresAdminScope_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	mut := resolver.Mutation()

	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username: "bob",
		Scopes:   []string{"read"},
	})

	_, err := mut.UpdateAdminAgentPolicy(ctx, model.UpdateAdminAgentPolicyInput{
		AllowAgents:                    true,
		AllowAgentRegistration:         false,
		DefaultQuarantineDays:          0,
		MaxAgentsPerOwner:              0,
		AllowRemoteAgents:              false,
		RemoteQuarantineDays:           0,
		BlockedAgentDomains:            nil,
		TrustedAgentDomains:            nil,
		AgentMaxPostsPerHour:           0,
		VerifiedAgentMaxPostsPerHour:   0,
		AgentMaxFollowsPerHour:         0,
		VerifiedAgentMaxFollowsPerHour: 0,
		HybridRetrievalEnabled:         false,
		HybridRetrievalMaxCandidates:   0,
	})
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))
	require.Equal(t, 403, apperrors.GetHTTPStatus(err))
}
