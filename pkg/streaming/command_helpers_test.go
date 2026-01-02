package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubPublisher struct {
	publishToUser func(ctx context.Context, userID string, event *Event) error
}

func (p *stubPublisher) PublishToUser(ctx context.Context, userID string, event *Event) error {
	if p.publishToUser == nil {
		return nil
	}
	return p.publishToUser(ctx, userID, event)
}

func (p *stubPublisher) PublishToStream(context.Context, string, *Event) error       { return nil }
func (p *stubPublisher) PublishToConversation(context.Context, string, *Event) error { return nil }
func (p *stubPublisher) Close() error                                                { return nil }

func TestBulkHelpers_ValidationAndResponses(t *testing.T) {
	bch := NewBaseCommandHandler(zap.NewNop())
	cfg := DefaultBulkAccountConfig()

	_, resp := bch.ValidateBulkAccountCommand(&ConnectionInfo{}, &Command{ID: "c1", Payload: map[string]interface{}{}}, cfg)
	require.NotNil(t, resp)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)

	conn := &ConnectionInfo{IsAuthenticated: true, UserID: "u1"}
	_, resp = bch.ValidateBulkAccountCommand(conn, &Command{ID: "c1", Payload: map[string]interface{}{}}, cfg)
	require.NotNil(t, resp)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	_, resp = bch.ValidateBulkAccountCommand(conn, &Command{
		ID: "c1",
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"", "a2"},
		},
	}, cfg)
	require.NotNil(t, resp)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	tooMany := &BulkAccountValidationConfig{RequiredFields: []string{"account_ids"}, MaxAccounts: 1, MinAccounts: 1}
	_, resp = bch.ValidateBulkAccountCommand(conn, &Command{
		ID: "c1",
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"a1", "a2"},
		},
	}, tooMany)
	require.NotNil(t, resp)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	ids, resp := bch.ValidateBulkAccountCommand(conn, &Command{
		ID: "c1",
		Payload: map[string]interface{}{
			"account_ids": []interface{}{"a1", "a2"},
		},
	}, &BulkAccountValidationConfig{RequiredFields: []string{"account_ids"}, MaxAccounts: 10, MinAccounts: 1})
	require.Nil(t, resp)
	assert.Equal(t, []string{"a1", "a2"}, ids)

	r := bch.CreateBulkOperationResponse("cmd", "op1", StatusProcessing, 2, "hi")
	require.NotNil(t, r)
	assert.True(t, r.Success)
	assert.Equal(t, "op1", r.Data["operation_id"])

	r = bch.CreateBulkServiceResponse("cmd", struct{ A string }{A: "x"}, "", "")
	require.NotNil(t, r)
	assert.True(t, r.Success)

	r = bch.CreateBulkServiceResponse("cmd", make(chan int), "", "")
	require.NotNil(t, r)
	assert.False(t, r.Success)
	assert.Equal(t, "CONVERSION_ERROR", r.Error.Code)
}

func TestBulkProcessingTracker(t *testing.T) {
	cfg := DefaultBulkProcessingConfig()
	tracker := NewBulkProcessingTracker(3)

	assert.Equal(t, StatusProcessing, tracker.GetStatus())
	assert.True(t, tracker.ShouldSendProgress(cfg))

	tracker.AddSuccess()
	assert.False(t, tracker.ShouldSendProgress(cfg))

	tracker.AddFailure(errors.New("boom"), "e1")
	assert.Len(t, tracker.Errors, 1)

	tracker.AddSuccess()
	assert.Equal(t, StatusCompleted, tracker.GetStatus())
	assert.True(t, tracker.ShouldSendProgress(cfg))
}

func TestProgressUpdateHelper(t *testing.T) {
	var gotEventType string
	var gotStream string
	var gotProgress float64

	pub := &stubPublisher{
		publishToUser: func(_ context.Context, userID string, event *Event) error {
			assert.Equal(t, "u1", userID)
			gotEventType = event.Type
			gotStream = event.Stream
			gotProgress = event.Payload["progress"].(float64)
			return nil
		},
	}

	helper := NewProgressUpdateHelper(pub, zap.NewNop())
	tracker := NewBulkProcessingTracker(10)
	tracker.Processed = 5

	helper.SendProgressUpdate(&ConnectionInfo{UserID: "u1", IsAuthenticated: true}, "op1", tracker, "working")
	assert.Equal(t, "operation.progress", gotEventType)
	assert.Equal(t, "user:u1", gotStream)
	assert.Equal(t, 50.0, gotProgress)

	helper.SendFinalUpdate(&ConnectionInfo{UserID: "u1", IsAuthenticated: true}, "op1", tracker, "done")
}

func TestListAndStandardCommandHelpers(t *testing.T) {
	bch := NewBaseCommandHandler(zap.NewNop())

	// ValidateListCommand covers auth and payload validation.
	resp := bch.ValidateListCommand(&ConnectionInfo{}, &Command{ID: "cmd", Payload: map[string]interface{}{}}, &ListCommandValidationConfig{
		RequiredFields: []string{"id"},
	})
	require.NotNil(t, resp)

	resp = bch.ValidateListCommand(&ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &Command{ID: "cmd", Payload: map[string]interface{}{}}, &ListCommandValidationConfig{
		RequiredFields: []string{"id"},
	})
	require.NotNil(t, resp)

	resp = bch.ValidateListCommand(&ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &Command{ID: "cmd", Payload: map[string]interface{}{"id": "l1"}}, &ListCommandValidationConfig{
		RequiredFields: []string{"id"},
	})
	require.Nil(t, resp)

	assert.Equal(t, 5*time.Second, DefaultPublisherConnectionConfig().Timeout)

	// ExecuteStandardCommandFlow variants.
	_, err := bch.ExecuteStandardCommandFlow(context.Background(), &ConnectionInfo{}, &Command{ID: "cmd"}, &CommandHandlerConfig{})
	require.NoError(t, err)

	_, err = bch.ExecuteStandardCommandFlow(context.Background(), &ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &Command{ID: "cmd", Payload: map[string]interface{}{}}, &CommandHandlerConfig{
		RequiredFields:  []string{"id"},
		ParameterName:   "id",
		ErrorCodePrefix: "DO_THING",
		OperationName:   "do thing",
		ResultExtractor: func(result interface{}) interface{} { return result },
		ServiceCall: func(context.Context, *ConnectionInfo, string) (interface{}, error) {
			return nil, errors.New("boom")
		},
	})
	require.NoError(t, err)

	resp2, err := bch.ExecuteStandardCommandFlow(context.Background(), &ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &Command{ID: "cmd", Payload: map[string]interface{}{"id": "x"}}, &CommandHandlerConfig{
		RequiredFields:  []string{"id"},
		ParameterName:   "id",
		ErrorCodePrefix: "DO_THING",
		OperationName:   "do thing",
		ResultExtractor: func(result interface{}) interface{} { return make(chan int) },
		ServiceCall: func(context.Context, *ConnectionInfo, string) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp2)
	assert.False(t, resp2.Success)

	resp2, err = bch.ExecuteStandardCommandFlow(context.Background(), &ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &Command{ID: "cmd", Payload: map[string]interface{}{"id": "x"}}, &CommandHandlerConfig{
		RequiredFields:  []string{"id"},
		ParameterName:   "id",
		ErrorCodePrefix: "DO_THING",
		OperationName:   "do thing",
		ResultExtractor: func(result interface{}) interface{} { return result },
		ServiceCall: func(context.Context, *ConnectionInfo, string) (interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp2)
	assert.True(t, resp2.Success)
}

func TestPublishConnectionHelper(t *testing.T) {
	helper := NewPublishConnectionHelper(zap.NewNop())

	called := 0
	err := helper.PublishToConnections(context.Background(), []*StreamConnection{}, &Event{}, func(context.Context, string, *Event) error {
		called++
		return nil
	}, map[string]interface{}{"k": "v"})
	require.NoError(t, err)
	assert.Equal(t, 0, called)

	conns := []*StreamConnection{{ConnectionID: "c1"}, {ConnectionID: "c2"}}
	event := &Event{Type: "t"}

	// Partial failures never return an error.
	err = helper.PublishToConnections(context.Background(), conns, event, func(_ context.Context, connectionID string, _ *Event) error {
		if connectionID == "c2" {
			return errors.New("nope")
		}
		return nil
	}, map[string]interface{}{})
	require.NoError(t, err)
	assert.False(t, event.Timestamp.IsZero())

	// All failures return an error.
	event2 := &Event{Type: "t"}
	err = helper.PublishToConnections(context.Background(), conns, event2, func(context.Context, string, *Event) error {
		return errors.New("nope")
	}, map[string]interface{}{})
	require.Error(t, err)
}
