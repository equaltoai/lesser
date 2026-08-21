package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func TestStreamingConnectionRepository_DuplicateSubscriptionRefreshesTTL(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.WebSocketSubscription{}))

	seed := &models.WebSocketSubscription{
		ConnectionID: "connection-1",
		UserID:       "alice",
		Stream:       "user",
		SubscribedAt: time.Unix(1, 0).UTC(),
		TTL:          1,
	}
	require.NoError(t, seed.UpdateKeys())
	require.NoError(t, db.Model(seed).Create())

	repo := NewStreamingConnectionRepository(
		db,
		models.MainTableName,
		db,
		models.MainTableName,
		zap.NewNop(),
		nil,
	)
	require.NoError(t, repo.WriteSubscription(context.Background(), "connection-1", "alice", "user"))
	require.NoError(t, repo.WriteSubscription(context.Background(), "connection-1", "alice", "user"),
		"duplicate subscribe must remain an idempotent confirmation path")

	var persisted models.WebSocketSubscription
	require.NoError(t, db.Model(&models.WebSocketSubscription{}).
		Where("PK", "=", "SUB#user").
		Where("SK", "=", "CONN#connection-1").
		First(&persisted))
	require.Greater(t, persisted.TTL, int64(1))
	require.WithinDuration(t, time.Now(), persisted.SubscribedAt, 5*time.Second)
	require.Len(t, fake.Items(models.MainTableName), 1)
}
