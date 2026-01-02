package routing

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestRouteThresholdManager_HelperMethods(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	t.Run("RouteHealthStatus_String_covers_default", func(t *testing.T) {
		assert.Equal(t, "unknown", RouteHealthStatus(-123).String())
	})

	t.Run("CalculateRouteCacheKey_and_size_class", func(t *testing.T) {
		key := manager.CalculateRouteCacheKey("src", "dst", types.MessageTypeCreate, manager.GetMessageSizeClass(1024))
		assert.Equal(t, "src:dst:create:0", key)

		assert.Equal(t, 0, manager.GetMessageSizeClass(1024))
		assert.Equal(t, 1, manager.GetMessageSizeClass(1025))
		assert.Equal(t, 1, manager.GetMessageSizeClass(10*1024))
		assert.Equal(t, 2, manager.GetMessageSizeClass(10*1024+1))
	})

	t.Run("GetRecoverySteps_has_expected_shape", func(t *testing.T) {
		steps := manager.GetRecoverySteps()
		assert.Len(t, steps, 4)
		assert.Equal(t, 0.1, steps[0].Load)
		assert.Equal(t, 1*time.Minute, steps[0].Duration)
		assert.Equal(t, 1.0, steps[len(steps)-1].Load)
	})

	t.Run("getMessagePriority_default", func(t *testing.T) {
		assert.Equal(t, PriorityNormal, manager.getMessagePriority(types.MessageTypeFlag))
		assert.Equal(t, PriorityNormal, manager.getMessagePriority(types.MessageTypeBlock))
	})
}
