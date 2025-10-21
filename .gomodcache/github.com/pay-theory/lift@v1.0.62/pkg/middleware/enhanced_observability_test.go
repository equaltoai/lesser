package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/pay-theory/lift/pkg/observability"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

type mockLogger struct {
	logs    []map[string]any
	healthy bool
}

func (m *mockLogger) Info(msg string, fields ...map[string]any) {
	entry := map[string]any{"level": "info", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockLogger) Error(msg string, fields ...map[string]any) {
	entry := map[string]any{"level": "error", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockLogger) Warn(msg string, fields ...map[string]any) {
	entry := map[string]any{"level": "warn", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockLogger) Debug(msg string, fields ...map[string]any) {
	entry := map[string]any{"level": "debug", "message": msg}
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			entry[k] = v
		}
	}
	m.logs = append(m.logs, entry)
}

func (m *mockLogger) WithField(_ string, _ any) lift.Logger   { return m }
func (m *mockLogger) WithFields(_ map[string]any) lift.Logger { return m }

func (m *mockLogger) WithRequestID(_ string) observability.StructuredLogger { return m }
func (m *mockLogger) WithTenantID(_ string) observability.StructuredLogger  { return m }
func (m *mockLogger) WithUserID(_ string) observability.StructuredLogger    { return m }
func (m *mockLogger) WithTraceID(_ string) observability.StructuredLogger   { return m }
func (m *mockLogger) WithSpanID(_ string) observability.StructuredLogger    { return m }

func (m *mockLogger) Flush(_ context.Context) error { return nil }
func (m *mockLogger) Close() error                  { return nil }
func (m *mockLogger) IsHealthy() bool               { return m.healthy }
func (m *mockLogger) GetStats() observability.LoggerStats {
	return observability.LoggerStats{
		EntriesLogged:  int64(len(m.logs)),
		BufferSize:     0,
		BufferCapacity: 1000,
	}
}

type mockMetrics struct {
	metrics      map[string]any
	tags         map[string]string
	root         *mockMetrics
	recordedTags []map[string]string
}

func newMockMetricsCollector() *mockMetrics {
	m := &mockMetrics{
		metrics: make(map[string]any),
		tags:    make(map[string]string),
	}
	m.root = m
	return m
}

func (m *mockMetrics) Counter(name string, _ ...map[string]string) lift.Counter {
	return &mockCounter{metrics: m.metrics, name: name}
}

func (m *mockMetrics) Histogram(name string, _ ...map[string]string) lift.Histogram {
	return &mockHistogram{metrics: m.metrics, name: name}
}

func (m *mockMetrics) Gauge(name string, _ ...map[string]string) lift.Gauge {
	return &mockGauge{metrics: m.metrics, name: name}
}

func (m *mockMetrics) Flush() error { return nil }

func (m *mockMetrics) WithTags(tags map[string]string) observability.MetricsCollector {
	newTags := make(map[string]string)
	for k, v := range m.tags {
		newTags[k] = v
	}
	for k, v := range tags {
		newTags[k] = v
	}

	root := m.root
	if root == nil {
		root = m
		m.root = root
	}
	if root != nil && len(newTags) > 0 {
		recorded := make(map[string]string, len(newTags))
		for k, v := range newTags {
			recorded[k] = v
		}
		root.recordedTags = append(root.recordedTags, recorded)
	}

	return &mockMetrics{metrics: m.metrics, tags: newTags, root: root}
}

func (m *mockMetrics) WithTag(key, value string) observability.MetricsCollector {
	return m.WithTags(map[string]string{key: value})
}

func (m *mockMetrics) RecordBatch(_ []*observability.MetricEntry) error { return nil }
func (m *mockMetrics) Close() error                                     { return nil }
func (m *mockMetrics) GetStats() observability.MetricsStats {
	return observability.MetricsStats{
		MetricsRecorded: int64(len(m.metrics)),
	}
}

// Add missing methods to implement updated MetricsCollector interface
func (m *mockMetrics) RecordLatency(operation string, duration time.Duration) {
	m.metrics[operation+".latency"] = duration.Milliseconds()
}

func (m *mockMetrics) RecordError(operation string) {
	key := operation + ".errors"
	if val, exists := m.metrics[key]; exists {
		intVal, ok := val.(int)
		if ok {
			m.metrics[key] = intVal + 1
		} else {
			m.metrics[key] = 1
		}
	} else {
		m.metrics[key] = 1
	}
}

func (m *mockMetrics) RecordSuccess(operation string) {
	key := operation + ".success"
	if val, exists := m.metrics[key]; exists {
		intVal, ok := val.(int)
		if ok {
			m.metrics[key] = intVal + 1
		} else {
			m.metrics[key] = 1
		}
	} else {
		m.metrics[key] = 1
	}
}

// Helper method to get metrics count for testing
func (m *mockMetrics) GetMetricsCount() int {
	return len(m.metrics)
}

type mockCounter struct {
	metrics map[string]any
	name    string
}

func (c *mockCounter) Inc() {
	if val, exists := c.metrics[c.name]; exists {
		intVal, ok := val.(int)
		if ok {
			c.metrics[c.name] = intVal + 1
		} else {
			c.metrics[c.name] = 1
		}
	} else {
		c.metrics[c.name] = 1
	}
}

func (c *mockCounter) Add(value float64) {
	if val, exists := c.metrics[c.name]; exists {
		floatVal, ok := val.(float64)
		if ok {
			c.metrics[c.name] = floatVal + value
		} else {
			c.metrics[c.name] = value
		}
	} else {
		c.metrics[c.name] = value
	}
}

type mockHistogram struct {
	metrics map[string]any
	name    string
}

func (h *mockHistogram) Observe(value float64) {
	h.metrics[h.name] = value
}

type mockGauge struct {
	metrics map[string]any
	name    string
}

func (g *mockGauge) Set(value float64) {
	g.metrics[g.name] = value
}

func (g *mockGauge) Inc() {
	if val, exists := g.metrics[g.name]; exists {
		floatVal, ok := val.(float64)
		if ok {
			g.metrics[g.name] = floatVal + 1
		} else {
			g.metrics[g.name] = 1.0
		}
	} else {
		g.metrics[g.name] = 1.0
	}
}

func (g *mockGauge) Dec() {
	if val, exists := g.metrics[g.name]; exists {
		floatVal, ok := val.(float64)
		if ok {
			g.metrics[g.name] = floatVal - 1
		} else {
			g.metrics[g.name] = -1.0
		}
	} else {
		g.metrics[g.name] = -1.0
	}
}

func (g *mockGauge) Add(value float64) {
	if val, exists := g.metrics[g.name]; exists {
		floatVal, ok := val.(float64)
		if ok {
			g.metrics[g.name] = floatVal + value
		} else {
			g.metrics[g.name] = value
		}
	} else {
		g.metrics[g.name] = value
	}
}

func TestEnhancedObservabilityMiddleware(t *testing.T) {
	logger := &mockLogger{}
	metrics := newMockMetricsCollector()

	config := EnhancedObservabilityConfig{
		EnableLogging: true,
		EnableMetrics: true,
		EnableTracing: false, // Skip X-Ray for unit tests
		Logger:        logger,
		Metrics:       metrics,
		SampleRate:    1.0,
	}

	middleware := EnhancedObservabilityMiddleware(config)

	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"message": "success"})
	}))

	// Create a proper Request using the adapters package
	adapterRequest := &adapters.Request{
		Method:      "GET",
		Path:        "/test",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	ctx := &lift.Context{
		Context:  context.Background(),
		Request:  lift.NewRequest(adapterRequest),
		Response: &lift.Response{StatusCode: 200, Headers: make(map[string]string)},
	}

	err := handler.Handle(ctx)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify logs were generated
	if len(logger.logs) == 0 {
		t.Error("Expected logs to be generated")
	}

	// Verify metrics were recorded
	if metrics.GetMetricsCount() == 0 {
		t.Error("Expected metrics to be recorded")
	}
}

func TestObservabilityWithTenantContext(t *testing.T) {
	logger := &mockLogger{}
	metrics := newMockMetricsCollector()

	config := EnhancedObservabilityConfig{
		EnableLogging: true,
		EnableMetrics: true,
		EnableTracing: false,
		Logger:        logger,
		Metrics:       metrics,
		SampleRate:    1.0,
	}

	middleware := EnhancedObservabilityMiddleware(config)

	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"tenant": ctx.TenantID()})
	}))

	// Create a proper Request using the adapters package
	adapterRequest := &adapters.Request{
		Method:      "GET",
		Path:        "/test",
		Headers:     map[string]string{"X-Tenant-ID": "test-tenant"},
		QueryParams: make(map[string]string),
	}

	ctx := &lift.Context{
		Context:  context.Background(),
		Request:  lift.NewRequest(adapterRequest),
		Response: &lift.Response{StatusCode: 200, Headers: make(map[string]string)},
	}
	ctx.SetTenantID("test-tenant")

	err := handler.Handle(ctx)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify tenant context was captured
	found := false
	for _, log := range logger.logs {
		if tenantID, exists := log["tenant_id"]; exists && tenantID == "test-tenant" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected tenant context in logs")
	}
}

func TestEnhancedObservabilityInjectsTenantAndUserTags(t *testing.T) {
	metrics := newMockMetricsCollector()
	config := EnhancedObservabilityConfig{
		EnableMetrics: true,
		Metrics:       metrics,
		SampleRate:    1.0,
	}

	middleware := EnhancedObservabilityMiddleware(config)

	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.SetTenantID("tenant-abc")
		ctx.SetUserID("user-xyz")
		return nil
	}))

	adapterRequest := &adapters.Request{
		Method:      "POST",
		Path:        "/resource",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	ctx := &lift.Context{
		Context:  context.Background(),
		Request:  lift.NewRequest(adapterRequest),
		Response: &lift.Response{StatusCode: 200, Headers: make(map[string]string)},
	}

	require.NoError(t, handler.Handle(ctx))

	if len(metrics.recordedTags) == 0 {
		t.Fatalf("expected metrics tags to be recorded")
	}

	found := false
	for _, tags := range metrics.recordedTags {
		if tags["tenant_id"] == "tenant-abc" && tags["user_id"] == "user-xyz" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected tenant and user tags to be present, got %#v", metrics.recordedTags)
	}
}

func TestEnhancedObservabilityCustomIdentityFuncs(t *testing.T) {
	metrics := newMockMetricsCollector()
	config := EnhancedObservabilityConfig{
		EnableMetrics: true,
		Metrics:       metrics,
		TenantIDFunc: func(*lift.Context) string {
			return "custom-tenant"
		},
		UserIDFunc: func(*lift.Context) string {
			return "custom-user"
		},
		SampleRate: 1.0,
	}

	middleware := EnhancedObservabilityMiddleware(config)
	adapterRequest := &adapters.Request{
		Method:      "GET",
		Path:        "/",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}
	ctx := &lift.Context{
		Context:  context.Background(),
		Request:  lift.NewRequest(adapterRequest),
		Response: &lift.Response{Headers: make(map[string]string)},
	}

	require.NoError(t, middleware(lift.HandlerFunc(func(_ *lift.Context) error { return nil })).Handle(ctx))

	found := false
	for _, tags := range metrics.recordedTags {
		if tags["tenant_id"] == "custom-tenant" && tags["user_id"] == "custom-user" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected custom tenant/user tags, got %#v", metrics.recordedTags)
	}
}

func TestEnhancedObservabilitySampleRate(t *testing.T) {
	t.Run("defaults to sampling when unset", func(t *testing.T) {
		logger := &mockLogger{}
		metrics := newMockMetricsCollector()
		config := EnhancedObservabilityConfig{
			EnableLogging: true,
			EnableMetrics: true,
			Logger:        logger,
			Metrics:       metrics,
			Sampler:       func() float64 { return 0.6 },
		}

		middleware := EnhancedObservabilityMiddleware(config)
		handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error { return nil }))

		adapterRequest := &adapters.Request{Method: "GET", Path: "/default"}
		ctx := &lift.Context{
			Context:  context.Background(),
			Request:  lift.NewRequest(adapterRequest),
			Response: &lift.Response{Headers: make(map[string]string)},
		}

		require.NoError(t, handler.Handle(ctx))

		if len(logger.logs) == 0 {
			t.Fatalf("expected logs when sampling defaults to enabled")
		}
		if metrics.GetMetricsCount() == 0 {
			t.Fatalf("expected metrics when sampling defaults to enabled")
		}
	})

	t.Run("disable sampling flag", func(t *testing.T) {
		logger := &mockLogger{}
		metrics := newMockMetricsCollector()
		config := EnhancedObservabilityConfig{
			EnableLogging:   true,
			EnableMetrics:   true,
			Logger:          logger,
			Metrics:         metrics,
			DisableSampling: true,
		}

		middleware := EnhancedObservabilityMiddleware(config)
		handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error { return nil }))

		adapterRequest := &adapters.Request{Method: "GET", Path: "/disabled"}
		ctx := &lift.Context{
			Context:  context.Background(),
			Request:  lift.NewRequest(adapterRequest),
			Response: &lift.Response{Headers: make(map[string]string)},
		}

		require.NoError(t, handler.Handle(ctx))

		if len(logger.logs) != 0 {
			t.Fatalf("expected no logs when sampling disabled, got %d", len(logger.logs))
		}
		if metrics.GetMetricsCount() != 0 {
			t.Fatalf("expected no metrics when sampling disabled, got %d", metrics.GetMetricsCount())
		}
		if sampled, ok := ctx.Get("observability_sampled").(bool); ok && sampled {
			t.Fatalf("expected sampled flag to be false when sampling disabled")
		}
	})

	t.Run("unsampled request skips instrumentation", func(t *testing.T) {
		logger := &mockLogger{}
		metrics := newMockMetricsCollector()
		config := EnhancedObservabilityConfig{
			EnableLogging: true,
			EnableMetrics: true,
			Logger:        logger,
			Metrics:       metrics,
			SampleRate:    0.5,
			Sampler:       func() float64 { return 0.9 },
		}

		middleware := EnhancedObservabilityMiddleware(config)
		handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error { return nil }))

		adapterRequest := &adapters.Request{Method: "GET", Path: "/sample"}
		ctx := &lift.Context{
			Context:  context.Background(),
			Request:  lift.NewRequest(adapterRequest),
			Response: &lift.Response{Headers: make(map[string]string)},
		}

		require.NoError(t, handler.Handle(ctx))

		if len(logger.logs) != 0 {
			t.Fatalf("expected no logs when unsampled, got %d", len(logger.logs))
		}
		if metrics.GetMetricsCount() != 0 {
			t.Fatalf("expected no metrics when unsampled, got %d", metrics.GetMetricsCount())
		}
		if sampled, ok := ctx.Get("observability_sampled").(bool); !ok || sampled {
			t.Fatalf("expected sampled flag to be false, got %v", ctx.Get("observability_sampled"))
		}
	})

	t.Run("sampled request runs instrumentation", func(t *testing.T) {
		logger := &mockLogger{}
		metrics := newMockMetricsCollector()
		config := EnhancedObservabilityConfig{
			EnableLogging: true,
			EnableMetrics: true,
			Logger:        logger,
			Metrics:       metrics,
			SampleRate:    0.5,
			Sampler:       func() float64 { return 0.1 },
		}

		middleware := EnhancedObservabilityMiddleware(config)
		handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error { return nil }))

		adapterRequest := &adapters.Request{Method: "GET", Path: "/sample"}
		ctx := &lift.Context{
			Context:  context.Background(),
			Request:  lift.NewRequest(adapterRequest),
			Response: &lift.Response{Headers: make(map[string]string)},
		}

		require.NoError(t, handler.Handle(ctx))

		if len(logger.logs) == 0 {
			t.Fatalf("expected logs when sampled")
		}
		if metrics.GetMetricsCount() == 0 {
			t.Fatalf("expected metrics when sampled")
		}
		if sampled, ok := ctx.Get("observability_sampled").(bool); !ok || !sampled {
			t.Fatalf("expected sampled flag to be true, got %v", ctx.Get("observability_sampled"))
		}
	})
}

func BenchmarkEnhancedObservabilityMiddleware(b *testing.B) {
	config := EnhancedObservabilityConfig{
		EnableLogging: false, // Disable for pure performance test
		EnableMetrics: false,
		EnableTracing: false,
	}

	middleware := EnhancedObservabilityMiddleware(config)

	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	// Create a proper Request using the adapters package
	adapterRequest := &adapters.Request{
		Method:      "GET",
		Path:        "/test",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	ctx := &lift.Context{
		Context:  context.Background(),
		Request:  lift.NewRequest(adapterRequest),
		Response: &lift.Response{StatusCode: 200, Headers: make(map[string]string)},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := handler.Handle(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEnhancedObservabilityDefaults(t *testing.T) {
	config := EnhancedObservabilityConfig{}
	middleware := EnhancedObservabilityMiddleware(config)

	// Test that defaults are set correctly
	ctx := &lift.Context{
		Context:  context.Background(),
		Request:  lift.NewRequest(&adapters.Request{Method: "GET", Path: "/test", Headers: make(map[string]string)}),
		Response: &lift.Response{},
	}

	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	err := handler.Handle(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestGetObservabilityStats(t *testing.T) {
	logger := &mockLogger{healthy: true}
	metrics := newMockMetricsCollector()

	config := EnhancedObservabilityConfig{
		Logger:  logger,
		Metrics: metrics,
	}

	stats := GetObservabilityStats(config)

	if stats.Logger == nil {
		t.Error("Expected logger stats to be present")
	}

	if stats.Metrics == nil {
		t.Error("Expected metrics stats to be present")
	}

	if stats.Tracing == nil {
		t.Error("Expected tracing stats to be present")
	}
}

func TestHealthCheckObservability(t *testing.T) {
	tests := []struct {
		name        string
		config      EnhancedObservabilityConfig
		expectError bool
	}{
		{
			name: "healthy logger",
			config: EnhancedObservabilityConfig{
				Logger:        &mockLogger{healthy: true},
				EnableLogging: true,
			},
			expectError: false,
		},
		{
			name: "unhealthy logger",
			config: EnhancedObservabilityConfig{
				Logger:        &mockLogger{healthy: false},
				EnableLogging: true,
			},
			expectError: true,
		},
		{
			name: "healthy metrics",
			config: EnhancedObservabilityConfig{
				Metrics:       newMockMetricsCollector(),
				EnableMetrics: true,
			},
			expectError: false,
		},
		{
			name: "disabled components",
			config: EnhancedObservabilityConfig{
				EnableLogging: false,
				EnableMetrics: false,
				EnableTracing: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthCheck := HealthCheckObservability(tt.config)
			err := healthCheck()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func BenchmarkEnhancedObservabilityLoggingOnly(b *testing.B) {
	logger := &mockLogger{healthy: true}

	config := EnhancedObservabilityConfig{
		Logger:        logger,
		EnableLogging: true,
		EnableMetrics: false,
		EnableTracing: false,
	}

	middleware := EnhancedObservabilityMiddleware(config)

	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx := &lift.Context{
			Context:  context.Background(),
			Request:  lift.NewRequest(&adapters.Request{Method: "GET", Path: "/test", Headers: make(map[string]string)}),
			Response: &lift.Response{},
		}

		if err := handler.Handle(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEnhancedObservabilityMetricsOnly(b *testing.B) {
	metrics := newMockMetricsCollector()

	config := EnhancedObservabilityConfig{
		Metrics:       metrics,
		EnableLogging: false,
		EnableMetrics: true,
		EnableTracing: false,
	}

	middleware := EnhancedObservabilityMiddleware(config)

	handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
		return nil
	}))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx := &lift.Context{
			Context:  context.Background(),
			Request:  lift.NewRequest(&adapters.Request{Method: "GET", Path: "/test", Headers: make(map[string]string)}),
			Response: &lift.Response{},
		}

		if err := handler.Handle(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
