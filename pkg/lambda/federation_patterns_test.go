package lambda

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubFederationDeliverer struct {
	mu sync.Mutex

	deliverCalls    int
	followersCalls  int
	recipientsCalls int

	deliverErrs   []error
	followersErr  error
	recipientsErr error
}

func (s *stubFederationDeliverer) DeliverActivity(_ context.Context, _ *activitypub.Activity, _ string, _ *activitypub.Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliverCalls++
	if len(s.deliverErrs) == 0 {
		return nil
	}
	err := s.deliverErrs[0]
	s.deliverErrs = s.deliverErrs[1:]
	return err
}

func (s *stubFederationDeliverer) DeliverToFollowers(_ context.Context, _ *activitypub.Activity, _ *activitypub.Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followersCalls++
	return s.followersErr
}

func (s *stubFederationDeliverer) DeliverToRecipients(_ context.Context, _ *activitypub.Activity, _ *activitypub.Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recipientsCalls++
	return s.recipientsErr
}

func TestExtractDomainFromURL(t *testing.T) {
	require.Equal(t, "example.com", extractDomainFromURL("https://example.com/inbox"))
	require.Equal(t, "example.com", extractDomainFromURL("http://example.com:8443/inbox"))
	require.Equal(t, "example.com", extractDomainFromURL("example.com/inbox"))
	require.Equal(t, "", extractDomainFromURL(""))
}

func TestFederationDeliveryPattern_validateDeliveryMessage(t *testing.T) {
	fdp := &FederationDeliveryPattern{}

	require.ErrorIs(t, fdp.validateDeliveryMessage(ActivityDeliveryMessage{}), ErrMissingActivity)

	require.ErrorIs(t, fdp.validateDeliveryMessage(ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: activitypub.CreateType},
		},
	}), ErrMissingActor)

	require.ErrorIs(t, fdp.validateDeliveryMessage(ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: activitypub.CreateType},
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
		},
		TargetInbox: "",
	}), ErrMissingTargetInbox)
}

func TestFederationDeliveryPattern_isPublicOrUnlisted(t *testing.T) {
	fdp := &FederationDeliveryPattern{}

	require.True(t, fdp.isPublicOrUnlisted(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{To: []string{activitypub.PublicAddress}},
	}))
	require.True(t, fdp.isPublicOrUnlisted(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{CC: []string{activitypub.PublicAddress}},
	}))
	require.False(t, fdp.isPublicOrUnlisted(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{To: []string{"https://example.com/private"}},
	}))
}

func TestFederationDeliveryPattern_deliverActivityWithRetry_PermanentError(t *testing.T) {
	origSleep := federationTimeSleep
	federationTimeSleep = func(time.Duration) {}
	t.Cleanup(func() { federationTimeSleep = origSleep })

	deliverer := &stubFederationDeliverer{
		deliverErrs: []error{
			apperrors.DeliveryRejected("https://example.com/inbox", 403),
		},
	}

	fdp := &FederationDeliveryPattern{
		federationService: deliverer,
		costCalculator:    federation.NewCostCalculator(),
		logger:            zap.NewNop(),
	}

	msg := ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: activitypub.CreateType},
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
		},
		TargetInbox: "https://example.com/inbox",
	}

	result := fdp.deliverActivityWithRetry(context.Background(), msg)
	require.False(t, result.Success)
	require.Equal(t, 403, result.StatusCode)
	require.Equal(t, 1, result.Attempt)
	require.Equal(t, 1, deliverer.deliverCalls)
}

func TestFederationDeliveryPattern_deliverActivityWithRetry_RetryThenSuccess(t *testing.T) {
	origSleep := federationTimeSleep
	federationTimeSleep = func(time.Duration) {}
	t.Cleanup(func() { federationTimeSleep = origSleep })

	deliverer := &stubFederationDeliverer{
		deliverErrs: []error{
			stdErrors.New("transient"),
			nil,
		},
	}

	fdp := &FederationDeliveryPattern{
		federationService: deliverer,
		costCalculator:    federation.NewCostCalculator(),
		logger:            zap.NewNop(),
	}

	msg := ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: activitypub.CreateType},
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
		},
		TargetInbox: "https://example.com/inbox",
	}

	result := fdp.deliverActivityWithRetry(context.Background(), msg)
	require.True(t, result.Success)
	require.Equal(t, 200, result.StatusCode)
	require.Equal(t, 2, result.Attempt)
	require.Equal(t, 2, deliverer.deliverCalls)
}

func TestFederationDeliveryPattern_processMessage_InvalidJSON(t *testing.T) {
	fdp := &FederationDeliveryPattern{
		federationService: &stubFederationDeliverer{},
		costCalculator:    federation.NewCostCalculator(),
		logger:            zap.NewNop(),
	}

	req := liftPkg.NewRequest(nil)
	ctx := liftPkg.NewContext(context.Background(), req)

	result := fdp.processMessage(ctx, events.SQSMessage{
		MessageId: "m1",
		Body:      "not-json",
	})
	require.False(t, result.Success)
	require.Error(t, result.Error)
	require.ErrorIs(t, result.Error, ErrInvalidMessageFormat)
}

func TestFederationDeliveryPattern_processMessage_Success(t *testing.T) {
	deliverer := &stubFederationDeliverer{}

	fdp := &FederationDeliveryPattern{
		federationService: deliverer,
		costCalculator:    federation.NewCostCalculator(),
		logger:            zap.NewNop(),
	}

	msg := ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "a",
				Type: activitypub.CreateType,
				To:   []string{activitypub.PublicAddress},
			},
		},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
			PreferredUsername: "alice",
		},
		TargetInbox: "https://example.com/inbox",
	}
	payload, err := json.Marshal(msg)
	require.NoError(t, err)

	req := liftPkg.NewRequest(nil)
	ctx := liftPkg.NewContext(context.Background(), req)

	result := fdp.processMessage(ctx, events.SQSMessage{
		MessageId: "m1",
		Body:      string(payload),
	})
	require.True(t, result.Success)
	require.Equal(t, 200, result.StatusCode)
	require.Equal(t, 1, deliverer.deliverCalls)
}

func TestFederationDeliveryPattern_ProcessSQSEvent_PartialFailure(t *testing.T) {
	origSleep := federationTimeSleep
	federationTimeSleep = func(time.Duration) {}
	t.Cleanup(func() { federationTimeSleep = origSleep })

	deliverer := &stubFederationDeliverer{
		deliverErrs: []error{
			apperrors.DeliveryRejected("https://example.com/inbox", 400),
		},
	}

	lambdaCtx := &common.LambdaContext{
		Config:          &config.Config{JWTSecret: "secret"},
		Logger:          zap.NewNop(),
		DeliveryService: deliverer,
		CostCalculator:  federation.NewCostCalculator(),
	}
	fdp := NewFederationDeliveryPattern(lambdaCtx)

	msg := ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: activitypub.CreateType},
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
		},
		TargetInbox: "https://example.com/inbox",
	}
	payload, err := json.Marshal(msg)
	require.NoError(t, err)

	req := liftPkg.NewRequest(nil)
	ctx := liftPkg.NewContext(context.Background(), req)

	err = fdp.ProcessSQSEvent(ctx, events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "m1", Body: string(payload)},
		},
	})
	require.Error(t, err)

	liftErr, ok := err.(*liftPkg.LiftError)
	require.True(t, ok)
	require.Equal(t, "PARTIAL_FAILURE", liftErr.Code)
}
