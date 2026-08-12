package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
)

// requireActingIdentity resolves the effective acting identity for identity-scoped
// GraphQL operations. Without an X-Lesser-Act-As indicator it returns the
// authenticated username exactly as requireAuth does. With the indicator, it performs
// the per-request share-grant check and returns the agent username plus the mandatory
// caller-attribution resolution.
func (r *Resolver) requireActingIdentity(ctx context.Context) (string, *auth.ActAsResolution, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return "", nil, err
	}

	indicator, _ := ctx.Value(common.ContextKeyActAsAgent).(string)
	if strings.TrimSpace(indicator) == "" {
		return username, nil, nil
	}

	claims, ok := auth.GetClaims(ctx)
	if !ok || claims == nil {
		r.Logger.Warn("act-as indicator present but full claims unavailable")
		return "", nil, apperrors.Internal("act-as resolution unavailable")
	}

	resolution, err := auth.ResolveActAs(ctx, claims, indicator, r.actAsAccountLookup(), r.actAsGrantCheck())
	if err != nil {
		return "", nil, mapActAsGraphQLError(err)
	}
	return resolution.AgentUsername, resolution, nil
}

func (r *Resolver) actAsAccountLookup() auth.ActAsAccountLookup {
	if r == nil {
		return nil
	}
	if r.Registry != nil && r.Registry.Accounts() != nil {
		accounts := r.Registry.Accounts()
		return func(ctx context.Context, username string) (*storage.Account, error) {
			return accounts.GetAccount(ctx, username)
		}
	}
	if r.Storage != nil && r.Storage.Account() != nil {
		accounts := r.Storage.Account()
		return func(ctx context.Context, username string) (*storage.Account, error) {
			return accounts.GetAccount(ctx, username)
		}
	}
	return nil
}

func (r *Resolver) actAsGrantCheck() auth.ActAsGrantCheck {
	if r == nil || r.Registry == nil || r.Registry.AgentShares() == nil {
		return nil
	}
	return r.Registry.AgentShares().IsActive
}

func mapActAsGraphQLError(err error) error {
	switch {
	case errors.Is(err, auth.ErrActAsMalformedIndicator):
		return apperrors.BadRequest("invalid " + auth.ActAsAgentHeader + " agent indicator")
	case errors.Is(err, auth.ErrActAsUnknownAgent):
		return apperrors.BadRequest("act-as agent not found")
	case errors.Is(err, auth.ErrActAsNotGranted):
		return apperrors.Forbidden("no active agent share grant authorizes this caller")
	default:
		return apperrors.InternalWithCause(err, "act-as resolution failed")
	}
}

// auditActAs writes the mandatory caller-attribution audit record for an act-as
// GraphQL mutation. It fires only when an act-as resolution is present so owner-path
// behavior is unchanged. The record names both identities: the agent in the username
// position and the real caller in the acted_by metadata field.
func (r *Resolver) auditActAs(ctx context.Context, acting *auth.ActAsResolution, action, targetID string, metadata map[string]any) {
	if r == nil || acting == nil || r.Storage == nil || r.Storage.Audit() == nil {
		return
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["acted_by"] = acting.ActedBy
	metadata["agent_username"] = acting.AgentUsername
	if targetID != "" {
		metadata["target_id"] = targetID
	}
	if err := r.Storage.Audit().StoreAuditEvent(ctx, action, "INFO", acting.AgentUsername, acting.AgentUsername, "", "", "", "", "", true, "", metadata); err != nil {
		r.Logger.Debug("failed to store act-as audit event")
	}
}
