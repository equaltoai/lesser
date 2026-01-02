package common

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeActor struct {
	id     string
	inbox  string
	outbox string
}

func (a fakeActor) GetID() string        { return a.id }
func (a fakeActor) GetType() string      { return "Person" }
func (a fakeActor) GetInbox() string     { return a.inbox }
func (a fakeActor) GetOutbox() string    { return a.outbox }
func (a fakeActor) GetFollowers() string { return "" }
func (a fakeActor) GetFollowing() string { return "" }

type fakeOperation struct {
	validateErr error
	execErrs    []error

	execCalls    int
	metricsCalls []struct {
		outcome  string
		attempts int
	}
}

func (o *fakeOperation) Validate(context.Context) error { return o.validateErr }

func (o *fakeOperation) Execute(context.Context) error {
	if o.execCalls >= len(o.execErrs) {
		o.execCalls++
		return nil
	}
	err := o.execErrs[o.execCalls]
	o.execCalls++
	return err
}

func (o *fakeOperation) RecordMetrics(_ context.Context, outcome string, attempts int) error {
	o.metricsCalls = append(o.metricsCalls, struct {
		outcome  string
		attempts int
	}{outcome: outcome, attempts: attempts})
	return nil
}

func TestActivityPubBusinessLogic_MoreCoverage(t *testing.T) {
	t.Run("CalculateDeliveryTargets dedupes and skips Public", func(t *testing.T) {
		audience := ActivityPubAudience{
			To: []string{
				"https://www.w3.org/ns/activitystreams#Public",
				"https://example.com/users/alice",
				"https://example.com/users/alice",
			},
			CC: []string{"https://example.com/users/bob"},
		}

		actors := map[string]fakeActor{
			"https://example.com/users/alice": {id: "https://example.com/users/alice", inbox: "https://example.com/inbox/alice"},
			"https://example.com/users/bob":   {id: "https://example.com/users/bob", inbox: ""},
		}

		targets, err := CalculateDeliveryTargets(context.Background(), audience, func(id string) (ActivityPubActor, error) {
			if a, ok := actors[id]; ok {
				return a, nil
			}
			return nil, stdErrors.New("missing")
		})
		require.NoError(t, err)
		require.Len(t, targets, 1)
		assert.Equal(t, "https://example.com/users/alice", targets[0].ActorID)
	})

	t.Run("CreateActivityPubCollectionPage sets ordered items", func(t *testing.T) {
		items := []interface{}{"a", "b"}

		ordered := CreateActivityPubCollectionPage("id", "OrderedCollectionPage", "partOf", items, "next", "prev")
		assert.Equal(t, items, ordered.OrderedItems)
		assert.Nil(t, ordered.Items)

		unordered := CreateActivityPubCollectionPage("id", "CollectionPage", "partOf", items, "", "")
		assert.Equal(t, items, unordered.Items)
		assert.Nil(t, unordered.OrderedItems)
	})

	t.Run("ActivityPubError mapping and helpers", func(t *testing.T) {
		err := NewActivityPubError("timeout", "x", true)
		assert.True(t, err.IsTemporary())
		assert.Contains(t, err.Error(), "activitypub error")

		mapped := MapActivityPubError(stdErrors.New("deadline exceeded"), "actor", "obj")
		assert.Equal(t, "timeout", mapped.Type)
		assert.True(t, mapped.Temporary)

		mapped = MapActivityPubError(stdErrors.New("connection reset"), "actor", "obj")
		assert.Equal(t, "network", mapped.Type)

		mapped = MapActivityPubError(stdErrors.New("401 unauthorized"), "actor", "obj")
		assert.Equal(t, "auth", mapped.Type)

		mapped = MapActivityPubError(stdErrors.New("404 not found"), "actor", "obj")
		assert.Equal(t, "not_found", mapped.Type)

		mapped = MapActivityPubError(stdErrors.New("500 server error"), "actor", "obj")
		assert.Equal(t, "server", mapped.Type)

		mapped = MapActivityPubError(stdErrors.New("something else"), "actor", "obj")
		assert.Equal(t, "unknown", mapped.Type)
	})

	t.Run("ExecuteActivityPubOperation validation failure", func(t *testing.T) {
		ap := NewActivityPubBusinessLogic(&FederationConfig{}, zap.NewNop())
		op := &fakeOperation{validateErr: stdErrors.New("bad")}
		err := ap.ExecuteActivityPubOperation(context.Background(), op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activitypub operation validation failed")
	})

	t.Run("ExecuteActivityPubOperation retries temporary ActivityPubError", func(t *testing.T) {
		ap := NewActivityPubBusinessLogic(&FederationConfig{
			MaxRetries: 1,
			RetryDelay: time.Millisecond,
		}, zap.NewNop())

		op := &fakeOperation{
			execErrs: []error{
				ActivityPubError{Type: "timeout", Message: "x", Temporary: true},
				nil,
			},
		}

		err := ap.ExecuteActivityPubOperation(context.Background(), op)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, op.execCalls, 2)
		require.NotEmpty(t, op.metricsCalls)
		assert.Equal(t, "success", op.metricsCalls[len(op.metricsCalls)-1].outcome)
	})

	t.Run("ExecuteActivityPubOperation does not retry non-federation errors", func(t *testing.T) {
		ap := NewActivityPubBusinessLogic(&FederationConfig{MaxRetries: 3}, zap.NewNop())
		op := &fakeOperation{
			execErrs: []error{stdErrors.New("boom")},
		}

		err := ap.ExecuteActivityPubOperation(context.Background(), op)
		require.Error(t, err)
		assert.Equal(t, 1, op.execCalls)
	})
}
