package lift

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAppStopCancelsLifecycleAndRunsHooks(t *testing.T) {
	app := New()

	ctx := app.LifecycleContext()
	require.NotNil(t, ctx)

	hookCalls := 0
	app.RegisterShutdownHook(func() {
		hookCalls++
	})

	require.NoError(t, app.Start())

	app.Stop()

	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected lifecycle context to be cancelled")
	}

	require.Equal(t, 1, hookCalls, "shutdown hook should execute exactly once")

	// Subsequent calls should be no-ops.
	app.Stop()
	require.Equal(t, 1, hookCalls)
}

func TestAppLifecycleContextRegeneratesAfterStop(t *testing.T) {
	app := New()

	ctx1 := app.LifecycleContext()
	require.NotNil(t, ctx1)

	app.Stop()

	ctx2 := app.LifecycleContext()
	require.NotNil(t, ctx2)
	require.NotSame(t, ctx1, ctx2, "expected new lifecycle context after stop")
}
