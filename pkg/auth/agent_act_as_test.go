package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func actAsTestLookup(account *storage.Account, err error) ActAsAccountLookup {
	return func(context.Context, string) (*storage.Account, error) {
		return account, err
	}
}

func actAsTestGrantCheck(active bool, err error) ActAsGrantCheck {
	return func(context.Context, string, string) (bool, error) {
		return active, err
	}
}

func actAsAgentAccount() *storage.Account {
	return &storage.Account{User: &storage.User{Username: "agent-one", IsAgent: true}}
}

// actAsClaims builds the shipped agent-subject shape: the token subject is always the
// agent, and the authorizing human is carried in DelegatedBy.
func actAsClaims(delegatedBy string) *Claims {
	return &Claims{Username: "agent-one", IsAgent: true, DelegatedBy: delegatedBy}
}

func TestResolveActAsBlankIndicatorIsOwnerPath(t *testing.T) {
	t.Parallel()

	for _, indicator := range []string{"", "   "} {
		resolution, err := ResolveActAs(context.Background(), actAsClaims("@alice"), indicator, nil, nil)
		require.NoError(t, err)
		require.Nil(t, resolution, "blank indicator must be the byte-identical owner path")
	}
}

func TestResolveActAsMalformedIndicator(t *testing.T) {
	t.Parallel()

	for _, indicator := range []string{"agent@example.com", "https://example.com/users/agent", "a/b", "a\\b", "not a user"} {
		_, err := ResolveActAs(context.Background(), actAsClaims("@alice"), indicator, actAsTestLookup(nil, nil), actAsTestGrantCheck(true, nil))
		require.ErrorIs(t, err, ErrActAsMalformedIndicator, "indicator %q", indicator)
	}
}

func TestResolveActAsUnknownNonAgentOrSuspendedTarget(t *testing.T) {
	t.Parallel()

	claims := actAsClaims("@alice")

	_, err := ResolveActAs(context.Background(), claims, "ghost", actAsTestLookup(nil, storage.ErrNotFound), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, ErrActAsUnknownAgent)

	_, err = ResolveActAs(context.Background(), claims, "human", actAsTestLookup(&storage.Account{User: &storage.User{Username: "human"}}, nil), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, ErrActAsUnknownAgent, "non-agent accounts are not act-as targets")

	_, err = ResolveActAs(context.Background(), claims, "agent-one", actAsTestLookup(&storage.Account{User: &storage.User{Username: "agent-one", IsAgent: true, Suspended: true}}, nil), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, ErrActAsUnknownAgent, "suspended agents are not act-as targets")
}

func TestResolveActAsNoActiveGrant(t *testing.T) {
	t.Parallel()

	_, err := ResolveActAs(context.Background(), actAsClaims("@alice"), "agent-one", actAsTestLookup(actAsAgentAccount(), nil), actAsTestGrantCheck(false, nil))
	require.ErrorIs(t, err, ErrActAsNotGranted)
}

func TestResolveActAsFailsClosedOnErrors(t *testing.T) {
	t.Parallel()

	lookupErr := errors.New("dynamodb timeout")
	_, err := ResolveActAs(context.Background(), actAsClaims("@alice"), "agent-one", actAsTestLookup(nil, lookupErr), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, lookupErr)
	require.NotErrorIs(t, err, ErrActAsUnknownAgent, "storage errors must fail closed, not masquerade as unknown agent")

	grantErr := errors.New("grant read failed")
	_, err = ResolveActAs(context.Background(), actAsClaims("@alice"), "agent-one", actAsTestLookup(actAsAgentAccount(), nil), actAsTestGrantCheck(false, grantErr))
	require.ErrorIs(t, err, grantErr)
	require.NotErrorIs(t, err, ErrActAsNotGranted, "check errors must fail closed, not masquerade as no-grant")
}

func TestResolveActAsRejectsBlankOrUnreadableDelegatedBy(t *testing.T) {
	t.Parallel()

	_, err := ResolveActAs(context.Background(), nil, "agent-one", actAsTestLookup(actAsAgentAccount(), nil), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, ErrActAsNotGranted, "nil claims can never be grant-authorized")

	for _, delegatedBy := range []string{"", "   ", "@", "not a user", "@https://example.com/users/owner"} {
		_, err = ResolveActAs(context.Background(), actAsClaims(delegatedBy), "agent-one", actAsTestLookup(actAsAgentAccount(), nil), actAsTestGrantCheck(true, nil))
		require.ErrorIs(t, err, ErrActAsNotGranted, "delegated_by %q is not a grantable local username", delegatedBy)
	}
}

func TestResolveActAsRequiresConfiguredDependencies(t *testing.T) {
	t.Parallel()

	claims := actAsClaims("@alice")

	_, err := ResolveActAs(context.Background(), claims, "agent-one", nil, actAsTestGrantCheck(true, nil))
	require.Error(t, err, "missing account lookup must fail closed")

	_, err = ResolveActAs(context.Background(), claims, "agent-one", actAsTestLookup(actAsAgentAccount(), nil), nil)
	require.Error(t, err, "missing grant check must fail closed")
}

func TestResolveActAsRejectsIncompleteAccountShapes(t *testing.T) {
	t.Parallel()

	claims := actAsClaims("@alice")

	_, err := ResolveActAs(context.Background(), claims, "agent-one", actAsTestLookup(nil, nil), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, ErrActAsUnknownAgent, "nil account is not an act-as target")

	_, err = ResolveActAs(context.Background(), claims, "agent-one", actAsTestLookup(&storage.Account{}, nil), actAsTestGrantCheck(true, nil))
	require.ErrorIs(t, err, ErrActAsUnknownAgent, "account without a user is not an act-as target")
}

func TestResolveActAsSuccess(t *testing.T) {
	t.Parallel()

	var checkedAgent, checkedGrantee string
	grantCheck := func(_ context.Context, agent, grantee string) (bool, error) {
		checkedAgent, checkedGrantee = agent, grantee
		return true, nil
	}

	resolution, err := ResolveActAs(context.Background(), actAsClaims("@Alice"), " Agent-One ", actAsTestLookup(actAsAgentAccount(), nil), grantCheck)
	require.NoError(t, err)
	require.NotNil(t, resolution)
	require.Equal(t, "agent-one", resolution.AgentUsername)
	require.Equal(t, "alice", resolution.ActedBy)
	require.Equal(t, "agent-one", checkedAgent)
	require.Equal(t, "alice", checkedGrantee, "grant check must use the human from DelegatedBy")
}
