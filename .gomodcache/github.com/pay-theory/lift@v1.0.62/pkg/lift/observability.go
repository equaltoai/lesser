package lift

// Logger represents a structured, leveled logger used throughout Lift.
// Implementations should be safe for concurrent use and support per‑entry fields
// as well as derived loggers with contextual fields (via WithField/WithFields).
type Logger interface {
	Debug(message string, fields ...map[string]any)
	Info(message string, fields ...map[string]any)
	Warn(message string, fields ...map[string]any)
	Error(message string, fields ...map[string]any)
	WithField(key string, value any) Logger
	WithFields(fields map[string]any) Logger
}

// MetricsCollector collects application metrics. Implementations typically map
// to a backend (e.g., CloudWatch) and expose common primitives. Optional tags
// can be provided to attach dimensions to a metric.
type MetricsCollector interface {
	Counter(name string, tags ...map[string]string) Counter
	Histogram(name string, tags ...map[string]string) Histogram
	Gauge(name string, tags ...map[string]string) Gauge
	Flush() error
}

// Counter is a monotonically increasing counter.
type Counter interface {
	Inc()
	Add(value float64)
}

// Histogram records observations (e.g., durations) that can be aggregated into
// percentiles or distributions by the backend.
type Histogram interface {
	Observe(value float64)
}

// Gauge is an instantaneous value that can go up or down.
type Gauge interface {
	Set(value float64)
	Inc()
	Dec()
	Add(value float64)
}

// NoOpLogger is a logger that does nothing (for testing)
type NoOpLogger struct{}

func (l *NoOpLogger) Debug(_ string, _ ...map[string]any) {}
func (l *NoOpLogger) Info(_ string, _ ...map[string]any)  {}
func (l *NoOpLogger) Warn(_ string, _ ...map[string]any)  {}
func (l *NoOpLogger) Error(_ string, _ ...map[string]any) {}
func (l *NoOpLogger) WithField(_ string, _ any) Logger    { return l }
func (l *NoOpLogger) WithFields(_ map[string]any) Logger  { return l }

// NoOpMetrics is a metrics collector that does nothing (for testing)
type NoOpMetrics struct{}

func (m *NoOpMetrics) Counter(_ string, _ ...map[string]string) Counter { return &NoOpCounter{} }
func (m *NoOpMetrics) Histogram(_ string, _ ...map[string]string) Histogram {
	return &NoOpHistogram{}
}
func (m *NoOpMetrics) Gauge(_ string, _ ...map[string]string) Gauge { return &NoOpGauge{} }
func (m *NoOpMetrics) Flush() error                                 { return nil }

type NoOpCounter struct{}

func (c *NoOpCounter) Inc()          {}
func (c *NoOpCounter) Add(_ float64) {}

type NoOpHistogram struct{}

func (h *NoOpHistogram) Observe(_ float64) {}

type NoOpGauge struct{}

func (g *NoOpGauge) Set(_ float64) {}
func (g *NoOpGauge) Inc()          {}
func (g *NoOpGauge) Dec()          {}
func (g *NoOpGauge) Add(_ float64) {}
