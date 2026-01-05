package lambda

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFederationDeliveryPattern_TriggerFederationDelivery_SwitchCases(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
	}

	t.Run("targeted activity delivers to recipients", func(t *testing.T) {
		deliverer := &stubFederationDeliverer{}
		fdp := &FederationDeliveryPattern{
			federationService: deliverer,
			costCalculator:    federation.NewCostCalculator(),
			logger:            zap.NewNop(),
		}

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: activitypub.FollowType},
		}

		require.NoError(t, fdp.TriggerFederationDelivery(context.Background(), activity, actor))
		require.Equal(t, 0, deliverer.followersCalls)
		require.Equal(t, 1, deliverer.recipientsCalls)
	})

	t.Run("public create delivers to followers and recipients", func(t *testing.T) {
		deliverer := &stubFederationDeliverer{}
		fdp := &FederationDeliveryPattern{
			federationService: deliverer,
			costCalculator:    federation.NewCostCalculator(),
			logger:            zap.NewNop(),
		}

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "a",
				Type: activitypub.CreateType,
				To:   []string{activitypub.PublicAddress},
			},
		}

		require.NoError(t, fdp.TriggerFederationDelivery(context.Background(), activity, actor))
		require.Equal(t, 1, deliverer.followersCalls)
		require.Equal(t, 1, deliverer.recipientsCalls)
	})

	t.Run("unknown activity type does nothing", func(t *testing.T) {
		deliverer := &stubFederationDeliverer{}
		fdp := &FederationDeliveryPattern{
			federationService: deliverer,
			costCalculator:    federation.NewCostCalculator(),
			logger:            zap.NewNop(),
		}

		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "a", Type: "Unknown"},
		}

		require.NoError(t, fdp.TriggerFederationDelivery(context.Background(), activity, actor))
		require.Equal(t, 0, deliverer.followersCalls)
		require.Equal(t, 0, deliverer.recipientsCalls)
	})
}

func TestFederationDeliveryPattern_deliverToFollowersAndRecipients_ErrorPaths(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "actor", Type: activitypub.PersonType},
	}
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "a",
			Type: activitypub.CreateType,
			To:   []string{activitypub.PublicAddress},
		},
	}

	t.Run("recipient failure returns wrapped error", func(t *testing.T) {
		deliverer := &stubFederationDeliverer{
			recipientsErr: stdErrors.New("boom"),
		}
		fdp := &FederationDeliveryPattern{
			federationService: deliverer,
			costCalculator:    federation.NewCostCalculator(),
			logger:            zap.NewNop(),
		}

		err := fdp.deliverToFollowersAndRecipients(context.Background(), activity, actor)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrDeliveryToRecipients)
	})

	t.Run("follower failure is logged but ignored", func(t *testing.T) {
		deliverer := &stubFederationDeliverer{
			followersErr: stdErrors.New("boom"),
		}
		fdp := &FederationDeliveryPattern{
			federationService: deliverer,
			costCalculator:    federation.NewCostCalculator(),
			logger:            zap.NewNop(),
		}

		require.NoError(t, fdp.deliverToFollowersAndRecipients(context.Background(), activity, actor))
		require.Equal(t, 1, deliverer.followersCalls)
		require.Equal(t, 1, deliverer.recipientsCalls)
	})

	t.Run("private activity skips followers", func(t *testing.T) {
		deliverer := &stubFederationDeliverer{}
		fdp := &FederationDeliveryPattern{
			federationService: deliverer,
			costCalculator:    federation.NewCostCalculator(),
			logger:            zap.NewNop(),
		}

		privateActivity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "a",
				Type: activitypub.CreateType,
				To:   []string{"https://example.com/private"},
			},
		}

		require.NoError(t, fdp.deliverToFollowersAndRecipients(context.Background(), privateActivity, actor))
		require.Equal(t, 0, deliverer.followersCalls)
		require.Equal(t, 1, deliverer.recipientsCalls)
	})
}
