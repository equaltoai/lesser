package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleCreateScheduledStatus(t *testing.T) {
	t.Parallel()

	h, _, _ := round11NewHandlerSliceC(t, nil)
	claims := &auth.Claims{Username: "alice"}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	require.NoError(t, err)

	t.Run("service unavailable", func(t *testing.T) {
		h.registry = &RegistryStub{}

		ts := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
		req := &models.CreateStatusRequest{Status: "hello", ScheduledAt: &ts}

		resp, err := h.handleCreateScheduledStatus(ctx, claims, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		h.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{}}

		bad := "not-a-time"
		req := &models.CreateStatusRequest{Status: "hello", ScheduledAt: &bad}

		resp, err := h.handleCreateScheduledStatus(ctx, claims, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("scheduled service error", func(t *testing.T) {
		wantErr := errors.New("boom")
		h.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{
			CreateScheduledStatusFunc: func(_ context.Context, _ *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
				return nil, wantErr
			},
		}}

		ts := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
		req := &models.CreateStatusRequest{Status: "hello", ScheduledAt: &ts}

		resp, err := h.handleCreateScheduledStatus(ctx, claims, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("success", func(t *testing.T) {
		scheduledAt := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
		h.registry = &RegistryStub{ScheduledSvc: &ScheduledServiceStub{
			CreateScheduledStatusFunc: func(_ context.Context, cmd *scheduled.CreateScheduledStatusCommand) (*scheduled.StatusResult, error) {
				require.Equal(t, "alice", cmd.Username)
				require.Equal(t, "hello", cmd.Status)
				return &scheduled.StatusResult{
					ScheduledStatus: &storage.ScheduledStatus{
						ID:          "sch-1",
						Status:      cmd.Status,
						ScheduledAt: scheduledAt,
					},
					MediaAttachments: []*storagemodels.Media{
						{MediaID: "m1", CDNUrl: "https://cdn.example/m1", ContentType: "image/jpeg", Width: 100, Height: 200},
					},
				}, nil
			},
		}}

		ts := scheduledAt.Format(time.RFC3339)
		req := &models.CreateStatusRequest{Status: "hello", ScheduledAt: &ts}

		resp, err := h.handleCreateScheduledStatus(ctx, claims, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusCreated, resp.Status)
	})
}
