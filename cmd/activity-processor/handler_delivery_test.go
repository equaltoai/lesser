package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubRouteManager struct {
	result *types.DeliveryResult
	err    error
}

func (s stubRouteManager) DeliverMessage(_ context.Context, _ *types.FederationMessage, _ types.DeliveryOptions) (*types.DeliveryResult, error) {
	return s.result, s.err
}

func TestActivityHandler_DeliverActivity_RouteManagerBranches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "deliver-1",
			Type: ActivityTypeCreate,
			To:   []string{"https://remote.example/users/bob"},
		},
		Actor: "https://example.com/users/alice",
	}

	handler := &ActivityHandler{Logger: zap.NewNop()}

	// No recipients -> skip
	require.NoError(t, handler.deliverActivity(ctx, &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "deliver-0", Type: ActivityTypeCreate},
		Actor:      "https://example.com/users/alice",
	}, "alice"))

	// Route manager present -> error path.
	handler.RouteManager = stubRouteManager{err: errors.New("boom")}
	require.NoError(t, handler.deliverActivity(ctx, activity, "alice"))

	// Route manager present -> success path.
	handler.RouteManager = stubRouteManager{result: &types.DeliveryResult{Success: true, Attempts: 1, Duration: time.Millisecond}}
	require.NoError(t, handler.deliverActivity(ctx, activity, "alice"))
}

