package handlers

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

func TestRound12_ZeroCoverageTargets(t *testing.T) {
	require.NotNil(t, failedToConvertStatus())

	app := apptheory.NewSecure(apptheory.SecureOptions{Tier: apptheory.TierP2})
	RegisterHealthRoutes(app, nil, zap.NewNop())

	h := &Handler{
		cfg:    &config.Config{Domain: "example.com"},
		logger: zap.NewNop(),
	}

	ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/announcements/a/reactions/b", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(h.HandleRemoveAnnouncementReactionLift(ctx))
}
