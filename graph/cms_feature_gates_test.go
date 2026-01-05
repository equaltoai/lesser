package graph

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestCMSFeatureGatesRequireLongForm(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		Config: &config.Config{
			InstanceMode:                  config.InstanceModeSocial,
			CMSLongFormPublishingEnabled:  true,
			CMSDraftSystemEnabled:         true,
			CMSRevisionHistoryEnabled:     true,
			CMSScheduledPublishingEnabled: true,
			CMSSeriesEnabled:              true,
			CMSCategoriesEnabled:          true,
		},
	}

	err := resolver.requireCMSLongFormEnabled()
	require.ErrorIs(t, err, errCMSDisabled)
}

func TestCMSFeatureGatesRequireDrafts(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		Config: &config.Config{
			InstanceMode:                  config.InstanceModeHybrid,
			CMSLongFormPublishingEnabled:  true,
			CMSDraftSystemEnabled:         false,
			CMSScheduledPublishingEnabled: true,
		},
	}

	err := resolver.requireCMSDraftsEnabled()
	require.ErrorIs(t, err, errCMSDraftsDisabled)
}

func TestCMSFeatureGatesRequireScheduling(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		Config: &config.Config{
			InstanceMode:                  config.InstanceModeHybrid,
			CMSLongFormPublishingEnabled:  true,
			CMSDraftSystemEnabled:         true,
			CMSScheduledPublishingEnabled: false,
		},
	}

	err := resolver.requireCMSSchedulingEnabled()
	require.ErrorIs(t, err, errCMSSchedulingDisabled)
}
