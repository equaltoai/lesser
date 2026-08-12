package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
)

// ActAsAgentHeader is the HTTP header a caller authenticated as themselves uses to
// request agent-scoped action under an active (agent, caller) share grant. The value
// is the plain local username of the shared agent.
const ActAsAgentHeader = "X-Lesser-Act-As"

var (
	// ErrActAsMalformedIndicator means the act-as indicator was present but not a
	// plausible local username. Surfaces map this to 400.
	ErrActAsMalformedIndicator = errors.New("malformed act-as agent indicator")
	// ErrActAsUnknownAgent means the indicator did not resolve to a live local agent
	// account. Surfaces map this to 400.
	ErrActAsUnknownAgent = errors.New("act-as agent not found")
	// ErrActAsNotGranted means no active share grant authorizes the caller on the
	// agent. Surfaces map this to 403.
	ErrActAsNotGranted = errors.New("no active agent share grant for caller")
)

// ActAsResolution describes a successfully authorized act-as request: the caller
// (ActedBy) acts agent-scoped on AgentUsername with mandatory caller attribution.
type ActAsResolution struct {
	// AgentUsername is the canonical local agent username whose account is driven.
	AgentUsername string
	// ActedBy is the canonical username of the real authenticated caller.
	ActedBy string
}

// ActAsAccountLookup resolves a local account for act-as validation.
type ActAsAccountLookup func(ctx context.Context, username string) (*storage.Account, error)

// ActAsGrantCheck performs the per-request active share-grant membership check. It
// must be an uncached direct base-table read with strong consistency; GSI reads are
// discovery-only and never acceptable here.
type ActAsGrantCheck func(ctx context.Context, agentUsername, granteeUsername string) (bool, error)

// ResolveActAs validates an act-as indicator against the authenticated claims and the
// per-request share-grant check. A blank indicator returns (nil, nil): the request is
// an ordinary owner-scoped request and callers must behave byte-identically to today.
//
// Fail-closed contract: malformed indicators and unknown/non-agent targets return
// ErrActAsMalformedIndicator / ErrActAsUnknownAgent; a missing or inactive grant
// returns ErrActAsNotGranted; any storage or check error is returned wrapped and must
// be surfaced as an operational failure without authorizing the caller.
func ResolveActAs(ctx context.Context, claims *Claims, indicator string, lookup ActAsAccountLookup, grantCheck ActAsGrantCheck) (*ActAsResolution, error) {
	indicator = strings.TrimSpace(indicator)
	if indicator == "" {
		return nil, nil
	}
	if claims == nil {
		return nil, ErrActAsNotGranted
	}

	agent := strings.ToLower(indicator)
	if strings.ContainsAny(agent, "@/\\") || common.ValidateUsername(agent) != nil {
		return nil, ErrActAsMalformedIndicator
	}
	caller := strings.ToLower(strings.TrimSpace(claims.Username))
	if caller == "" || common.ValidateUsername(caller) != nil {
		return nil, ErrActAsNotGranted
	}

	if lookup == nil || grantCheck == nil {
		return nil, fmt.Errorf("act-as resolution is not configured")
	}

	account, err := lookup(ctx, agent)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || common.IsNotFound(err) {
			return nil, ErrActAsUnknownAgent
		}
		return nil, fmt.Errorf("act-as agent lookup failed: %w", err)
	}
	if account == nil || account.User == nil || !account.User.IsAgent || account.User.Suspended {
		return nil, ErrActAsUnknownAgent
	}

	active, err := grantCheck(ctx, agent, caller)
	if err != nil {
		return nil, fmt.Errorf("act-as grant check failed: %w", err)
	}
	if !active {
		return nil, ErrActAsNotGranted
	}

	return &ActAsResolution{AgentUsername: agent, ActedBy: caller}, nil
}
