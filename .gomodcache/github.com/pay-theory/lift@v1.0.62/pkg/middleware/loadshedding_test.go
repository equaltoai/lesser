package middleware

import (
	"sync"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

type testMetricsCollector struct{}

type testCounter struct{}

type testHistogram struct{}

type testGauge struct{}

type metricsStats observability.MetricsStats

type testMetricsCollectorWithLock struct {
	sync.Mutex
	testMetricsCollector
}

func (m *testMetricsCollector) Counter(string, ...map[string]string) lift.Counter {
	return &testCounter{}
}
func (m *testMetricsCollector) Histogram(string, ...map[string]string) lift.Histogram {
	return &testHistogram{}
}
func (m *testMetricsCollector) Gauge(string, ...map[string]string) lift.Gauge             { return &testGauge{} }
func (m *testMetricsCollector) Flush() error                                              { return nil }
func (m *testMetricsCollector) RecordLatency(string, time.Duration)                       {}
func (m *testMetricsCollector) RecordError(string)                                        {}
func (m *testMetricsCollector) RecordSuccess(string)                                      {}
func (m *testMetricsCollector) WithTags(map[string]string) observability.MetricsCollector { return m }
func (m *testMetricsCollector) WithTag(string, string) observability.MetricsCollector     { return m }
func (m *testMetricsCollector) RecordBatch([]*observability.MetricEntry) error            { return nil }
func (m *testMetricsCollector) Close() error                                              { return nil }
func (m *testMetricsCollector) GetStats() observability.MetricsStats {
	return observability.MetricsStats{}
}

func (c *testCounter) Inc()              {}
func (c *testCounter) Add(float64)       {}
func (h *testHistogram) Observe(float64) {}
func (g *testGauge) Set(float64)         {}
func (g *testGauge) Inc()                {}
func (g *testGauge) Dec()                {}
func (g *testGauge) Add(float64)         {}

func TestLoadSheddingMiddlewareRegistersStopHook(t *testing.T) {
	var captured func()

	config := LoadSheddingConfig{
		Enabled:       true,
		EnableMetrics: false,
		RegisterStop:  func(stop func()) { captured = stop },
	}

	if middleware := LoadSheddingMiddleware(config); middleware == nil {
		t.Fatal("expected middleware to be created")
	}

	if captured == nil {
		t.Fatal("expected stop hook to be registered")
	}

	// Ensure stop hook is safe to call even when no goroutines started
	captured()
}

func TestLoadSheddingMiddlewareStopShutsDownMetricsCollector(t *testing.T) {
	app := lift.New()
	defer app.Stop()

	metrics := &testMetricsCollector{}

	config := ConfigureLoadSheddingForApp(app, LoadSheddingConfig{
		Enabled:                  true,
		EnableMetrics:            true,
		Metrics:                  metrics,
		Strategy:                 LoadSheddingRandom,
		MetricsCollectorInterval: 5 * time.Millisecond,
	})

	var captured func()
	originalRegister := config.RegisterStop
	config.RegisterStop = func(stop func()) {
		captured = stop
		if originalRegister != nil {
			originalRegister(stop)
		}
	}

	if middleware := LoadSheddingMiddleware(config); middleware == nil {
		t.Fatal("expected middleware to be created")
	}

	if captured == nil {
		t.Fatal("expected stop hook to be captured")
	}

	done := make(chan struct{})
	go func() {
		captured()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected stop to complete promptly")
	}

	app.Stop()

	// Stop should be idempotent
	captured()
}

func TestConfigureLoadSheddingForAppBindsLifecycle(t *testing.T) {
	app := lift.New()

	config := ConfigureLoadSheddingForApp(app, LoadSheddingConfig{})

	if config.LifecycleContext != app.LifecycleContext() {
		t.Fatal("expected lifecycle context to be sourced from app")
	}

	done := make(chan struct{})
	config.RegisterStop(func() {
		close(done)
	})

	app.Stop()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected registered hook to run on app stop")
	}

	// Reinitialize lifecycle for subsequent operations
	_ = app.LifecycleContext()
}
