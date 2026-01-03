package lift

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	liftframework "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound12_ZeroCoverageTargets(t *testing.T) {
	require.NotNil(t, failedToConvertStatus())

	app := liftframework.New()
	RegisterHealthRoutes(app, nil, zap.NewNop())

	h := &Handler{
		cfg:    &config.Config{Domain: "example.com"},
		logger: zap.NewNop(),
	}

	ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/announcements/a/reactions/b", nil, nil, nil)
	require.NoError(t, err)

	require.NoError(t, h.HandleRemoveAnnouncementReactionLift(ctx))
	require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
}

