package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
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
