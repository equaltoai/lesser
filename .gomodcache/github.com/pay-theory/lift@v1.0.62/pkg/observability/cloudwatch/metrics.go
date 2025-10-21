package cloudwatch

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// CloudWatchMetricsClient defines the interface for CloudWatch metrics operations
type CloudWatchMetricsClient interface {
	PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
}

// MetricsBuffer manages buffering of metric data points
type MetricsBuffer struct {
	data      []types.MetricDatum
	maxSize   int
	flushSize int
	mu        sync.Mutex
}

// NewMetricsBuffer creates a new metrics buffer
func NewMetricsBuffer(maxSize, flushSize int) *MetricsBuffer {
	return &MetricsBuffer{
		data:      make([]types.MetricDatum, 0, maxSize),
		maxSize:   maxSize,
		flushSize: flushSize,
	}
}

// Add adds a metric datum to the buffer
func (b *MetricsBuffer) Add(datum types.MetricDatum) (shouldFlush bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If buffer is full, drop oldest metrics
	if len(b.data) >= b.maxSize {
		copy(b.data, b.data[1:])
		b.data = b.data[:len(b.data)-1]
	}

	b.data = append(b.data, datum)
	return len(b.data) >= b.flushSize
}

// Drain removes and returns all metrics from the buffer
func (b *MetricsBuffer) Drain() []types.MetricDatum {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) == 0 {
		return nil
	}

	result := make([]types.MetricDatum, len(b.data))
	copy(result, b.data)
	b.data = b.data[:0]

	return result
}

// Size returns the current number of metrics in the buffer
func (b *MetricsBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data)
}

// CloudWatchMetrics implements metrics collection for CloudWatch
type CloudWatchMetrics struct {
	core       *metricsCore
	dimensions []types.Dimension
	ownsCore   bool

	mu        sync.RWMutex
	closeOnce sync.Once
}

type metricsCore struct {
	resources *metricsResources
	state     *metricsState

	flushInterval time.Duration
	flushTimeout  time.Duration

	metricsRecorded int64
	metricsDropped  int64
	errorCount      int64
	flushCount      int64

	refCount int64
}

type metricsResources struct {
	putMetricData func(context.Context, *cloudwatch.PutMetricDataInput) (*cloudwatch.PutMetricDataOutput, error)
	buffer        *MetricsBuffer
	namespace     *string
	channels      *metricsChannels
}

type metricsState struct {
	lastFlush atomic.Value
	lastError atomic.Value
	closeOnce sync.Once
}

type metricsChannels struct {
	flushNow chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func cloneDimensions(src []types.Dimension) []types.Dimension {
	if len(src) == 0 {
		return nil
	}

	dst := make([]types.Dimension, len(src))
	copy(dst, src)
	return dst
}

func normalizeDimensionKey(key string) string {
	trimmed := strings.TrimSpace(key)
	switch strings.ToLower(trimmed) {
	case "tenant_id", "tenantid":
		return "TenantID"
	case "user_id", "userid":
		return "UserID"
	default:
		return trimmed
	}
}

func combineDimensions(base []types.Dimension, extra map[string]string) []types.Dimension {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(base)+len(extra))
	order := make([]string, 0, len(base)+len(extra))

	for _, dim := range base {
		if dim.Name == nil || dim.Value == nil {
			continue
		}
		name := aws.ToString(dim.Name)
		if _, exists := normalized[name]; !exists {
			order = append(order, name)
		}
		normalized[name] = aws.ToString(dim.Value)
	}

	for rawKey, value := range extra {
		if value == "" {
			continue
		}
		name := normalizeDimensionKey(rawKey)
		if _, exists := normalized[name]; !exists {
			order = append(order, name)
		}
		normalized[name] = value
	}

	dims := make([]types.Dimension, 0, len(order))
	for _, name := range order {
		val := normalized[name]
		dims = append(dims, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(val),
		})
	}

	return dims
}

// CloudWatchMetricsConfig holds configuration for CloudWatch metrics
type CloudWatchMetricsConfig struct {
	Dimensions    map[string]string
	Namespace     string
	BufferSize    int
	FlushSize     int
	FlushInterval time.Duration
}

// NewCloudWatchMetrics creates a new CloudWatch metrics collector
func NewCloudWatchMetrics(client CloudWatchMetricsClient, config CloudWatchMetricsConfig) *CloudWatchMetrics {
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	if config.FlushSize == 0 {
		config.FlushSize = 20
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 60 * time.Second
	}

	var namespace *string
	if config.Namespace != "" {
		ns := config.Namespace
		namespace = &ns
	}
	var putMetric func(context.Context, *cloudwatch.PutMetricDataInput) (*cloudwatch.PutMetricDataOutput, error)
	if client != nil {
		putMetric = func(ctx context.Context, input *cloudwatch.PutMetricDataInput) (*cloudwatch.PutMetricDataOutput, error) {
			return client.PutMetricData(ctx, input)
		}
	}
	resources := &metricsResources{
		putMetricData: putMetric,
		buffer:        NewMetricsBuffer(config.BufferSize, config.FlushSize),
		namespace:     namespace,
		channels: &metricsChannels{
			flushNow: make(chan struct{}, 1),
			stopCh:   make(chan struct{}),
			doneCh:   make(chan struct{}),
		},
	}
	core := &metricsCore{
		resources:     resources,
		state:         &metricsState{},
		flushInterval: config.FlushInterval,
		flushTimeout:  30 * time.Second,
		refCount:      1,
	}

	core.state.lastFlush.Store(time.Now())

	metrics := &CloudWatchMetrics{
		core:       core,
		dimensions: combineDimensions(nil, config.Dimensions),
		ownsCore:   true,
	}

	go core.backgroundFlusher()

	return metrics
}

// RecordMetric records a single metric value
func (m *CloudWatchMetrics) RecordMetric(name string, value float64, unit types.StandardUnit) {
	m.recordMetricWithTags(name, value, unit, nil)
}

// RecordCount records a count metric
func (m *CloudWatchMetrics) RecordCount(name string, count int64) {
	m.recordMetricWithTags(name, float64(count), types.StandardUnitCount, nil)
}

// RecordDuration records a duration metric in milliseconds
func (m *CloudWatchMetrics) RecordDuration(name string, duration time.Duration) {
	m.recordMetricWithTags(name, float64(duration.Milliseconds()), types.StandardUnitMilliseconds, nil)
}

// RecordGauge records a gauge metric
func (m *CloudWatchMetrics) RecordGauge(name string, value float64) {
	m.recordMetricWithTags(name, value, types.StandardUnitNone, nil)
}

func (m *CloudWatchMetrics) recordMetricWithTags(name string, value float64, unit types.StandardUnit, tags map[string]string) {
	if m == nil || m.core == nil {
		return
	}

	datum := types.MetricDatum{
		MetricName: aws.String(name),
		Value:      aws.Float64(value),
		Unit:       unit,
		Timestamp:  aws.Time(time.Now()),
		Dimensions: combineDimensions(m.getDimensions(), tags),
	}

	m.core.recordDatum(datum)
}

func (c *metricsCore) recordDatum(datum types.MetricDatum) {
	atomic.AddInt64(&c.metricsRecorded, 1)

	if c.resources == nil || c.resources.buffer == nil {
		return
	}

	if shouldFlush := c.resources.buffer.Add(datum); shouldFlush {
		c.signalFlush()
	}
}

func (c *metricsCore) signalFlush() {
	if c.resources == nil || c.resources.channels == nil {
		return
	}
	select {
	case c.resources.channels.flushNow <- struct{}{}:
	default:
	}
}

// WithDimensions returns a new metrics collector with additional dimensions
func (m *CloudWatchMetrics) WithDimensions(dims map[string]string) *CloudWatchMetrics {
	if m == nil || m.core == nil {
		return m
	}

	merged := combineDimensions(m.getDimensions(), dims)

	return &CloudWatchMetrics{
		core:       m.core,
		dimensions: merged,
		ownsCore:   false,
	}
}

// WithTenant returns a new metrics collector with tenant dimension
func (m *CloudWatchMetrics) WithTenant(tenantID string) *CloudWatchMetrics {
	return m.WithDimensions(map[string]string{"TenantID": tenantID})
}

// WithTag adds a single tag/dimension
func (m *CloudWatchMetrics) WithTag(key, value string) observability.MetricsCollector {
	return m.WithDimensions(map[string]string{key: value})
}

// WithTags adds multiple tags/dimensions
func (m *CloudWatchMetrics) WithTags(tags map[string]string) observability.MetricsCollector {
	return m.WithDimensions(tags)
}

// RecordBatch records multiple metric entries at once
func (m *CloudWatchMetrics) RecordBatch(entries []*observability.MetricEntry) error {
	if m == nil || m.core == nil {
		return nil
	}

	for _, entry := range entries {
		if entry == nil {
			continue
		}

		unit := m.parseUnit(entry.Unit)
		dims := combineDimensions(m.getDimensions(), entry.Tags)

		datum := types.MetricDatum{
			MetricName: aws.String(entry.Name),
			Value:      aws.Float64(entry.Value),
			Unit:       unit,
			Timestamp:  aws.Time(entry.Timestamp),
			Dimensions: dims,
		}

		m.core.recordDatum(datum)
	}

	return nil
}

// Flush forces a flush of buffered metrics
func (m *CloudWatchMetrics) Flush() error {
	if m == nil || m.core == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.core.flushTimeout)
	defer cancel()

	return m.core.flush(ctx)
}

// Close stops the metrics collector and flushes remaining metrics
func (m *CloudWatchMetrics) Close() error {
	if m == nil || m.core == nil {
		return nil
	}

	if !m.ownsCore {
		return nil
	}

	var err error
	m.closeOnce.Do(func() {
		err = m.core.release()
	})

	return err
}

func (c *metricsCore) release() error {
	if c == nil {
		return nil
	}

	newCount := atomic.AddInt64(&c.refCount, -1)
	if newCount > 0 {
		return nil
	}

	if newCount < 0 {
		atomic.StoreInt64(&c.refCount, 0)
		return nil
	}

	var flushErr error
	if c.state == nil {
		return nil
	}
	c.state.closeOnce.Do(func() {
		if c.resources != nil && c.resources.channels != nil {
			close(c.resources.channels.stopCh)

			select {
			case <-c.resources.channels.doneCh:
			case <-time.After(5 * time.Second):
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), c.flushTimeout)
		flushErr = c.flush(ctx)
		cancel()
	})

	return flushErr
}

// GetStats returns metrics collection statistics
func (m *CloudWatchMetrics) GetStats() observability.MetricsStats {
	if m == nil || m.core == nil {
		return observability.MetricsStats{}
	}

	lastFlushVal := m.core.state.lastFlush.Load()
	lastFlush, ok := lastFlushVal.(time.Time)
	if !ok {
		lastFlush = time.Time{} // zero time if assertion fails
	}
	lastErrorVal := m.core.state.lastError.Load()
	lastError, ok := lastErrorVal.(string)
	if !ok {
		lastError = "" // empty string if assertion fails
	}

	return observability.MetricsStats{
		MetricsRecorded: atomic.LoadInt64(&m.core.metricsRecorded),
		MetricsDropped:  atomic.LoadInt64(&m.core.metricsDropped),
		LastFlush:       lastFlush,
		ErrorCount:      atomic.LoadInt64(&m.core.errorCount),
		LastError:       lastError,
	}
}

// backgroundFlusher runs the periodic flush loop
func (c *metricsCore) backgroundFlusher() {
	if c.resources == nil || c.resources.channels == nil {
		return
	}
	defer close(c.resources.channels.doneCh)

	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.resources.channels.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), c.flushTimeout)
			if err := c.flush(ctx); err != nil {
				log.Printf("Warning: periodic metrics flush failed: %v", err)
			}
			cancel()
		case <-c.resources.channels.flushNow:
			ctx, cancel := context.WithTimeout(context.Background(), c.flushTimeout)
			if err := c.flush(ctx); err != nil {
				log.Printf("Warning: manual metrics flush failed: %v", err)
			}
			cancel()
		}
	}
}

// flush sends buffered metrics to CloudWatch
func (c *metricsCore) flush(ctx context.Context) error {
	if c.resources == nil || c.resources.buffer == nil {
		return nil
	}

	data := c.resources.buffer.Drain()
	if len(data) == 0 {
		return nil
	}

	atomic.AddInt64(&c.flushCount, 1)

	// CloudWatch allows up to 1000 metrics per request
	// We'll send in batches of 20 for better error handling
	const batchSize = 20

	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}

		batch := data[i:end]
		input := &cloudwatch.PutMetricDataInput{
			MetricData: batch,
		}
		if c.resources.namespace != nil {
			input.Namespace = aws.String(*c.resources.namespace)
		}

		if c.resources.putMetricData == nil {
			continue
		}

		_, err := c.resources.putMetricData(ctx, input)
		if err != nil {
			atomic.AddInt64(&c.errorCount, 1)
			atomic.AddInt64(&c.metricsDropped, int64(len(batch)))
			if c.state != nil {
				c.state.lastError.Store(err.Error())
			}
			// Continue with next batch even if this one fails
		}
	}

	if c.state != nil {
		c.state.lastFlush.Store(time.Now())
	}
	return nil
}

// getDimensions returns a copy of the current dimensions
func (m *CloudWatchMetrics) getDimensions() []types.Dimension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return cloneDimensions(m.dimensions)
}

// parseUnit converts a string unit to StandardUnit
func (m *CloudWatchMetrics) parseUnit(unit string) types.StandardUnit {
	switch unit {
	case "Count":
		return types.StandardUnitCount
	case "Milliseconds":
		return types.StandardUnitMilliseconds
	case "Seconds":
		return types.StandardUnitSeconds
	case "Bytes":
		return types.StandardUnitBytes
	case "Percent":
		return types.StandardUnitPercent
	default:
		return types.StandardUnitNone
	}
}

// Implement lift.MetricsCollector interface methods

// RecordLatency records the latency of an operation
func (m *CloudWatchMetrics) RecordLatency(operation string, duration time.Duration) {
	m.recordMetricWithTags(fmt.Sprintf("%s.latency", operation), float64(duration.Milliseconds()), types.StandardUnitMilliseconds, nil)
}

// RecordError records that an error occurred
func (m *CloudWatchMetrics) RecordError(operation string) {
	m.recordMetricWithTags(fmt.Sprintf("%s.errors", operation), 1, types.StandardUnitCount, nil)
}

// RecordSuccess records a successful operation
func (m *CloudWatchMetrics) RecordSuccess(operation string) {
	m.recordMetricWithTags(fmt.Sprintf("%s.success", operation), 1, types.StandardUnitCount, nil)
}

// Counter returns a counter metric implementation
func (m *CloudWatchMetrics) Counter(name string, tags ...map[string]string) lift.Counter {
	return &cloudWatchCounter{
		metrics: m,
		name:    &name,
		tags:    copyTagMap(firstTagMap(tags...)),
	}
}

// Histogram returns a histogram metric implementation
func (m *CloudWatchMetrics) Histogram(name string, tags ...map[string]string) lift.Histogram {
	return &cloudWatchHistogram{
		metrics: m,
		name:    &name,
		tags:    copyTagMap(firstTagMap(tags...)),
	}
}

// Gauge returns a gauge metric implementation
func (m *CloudWatchMetrics) Gauge(name string, tags ...map[string]string) lift.Gauge {
	return &cloudWatchGauge{
		metrics: m,
		name:    &name,
		tags:    copyTagMap(firstTagMap(tags...)),
	}
}

func firstTagMap(tagArgs ...map[string]string) map[string]string {
	if len(tagArgs) == 0 {
		return nil
	}

	return tagArgs[0]
}

func copyTagMap(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}

	clone := make(map[string]string, len(tags))
	for k, v := range tags {
		clone[k] = v
	}
	return clone
}

// cloudWatchCounter implements lift.Counter
type cloudWatchCounter struct {
	metrics *CloudWatchMetrics
	name    *string
	tags    map[string]string
}

func (c *cloudWatchCounter) Inc() {
	if c.name == nil {
		return
	}
	c.metrics.recordMetricWithTags(*c.name, 1, types.StandardUnitCount, c.tags)
}

func (c *cloudWatchCounter) Add(value float64) {
	if c.name == nil {
		return
	}
	c.metrics.recordMetricWithTags(*c.name, value, types.StandardUnitCount, c.tags)
}

// cloudWatchHistogram implements lift.Histogram
type cloudWatchHistogram struct {
	metrics *CloudWatchMetrics
	name    *string
	tags    map[string]string
}

func (h *cloudWatchHistogram) Observe(value float64) {
	if h.name == nil {
		return
	}
	h.metrics.recordMetricWithTags(*h.name, value, types.StandardUnitNone, h.tags)
}

// cloudWatchGauge implements lift.Gauge
type cloudWatchGauge struct {
	metrics *CloudWatchMetrics
	name    *string
	tags    map[string]string
}

func (g *cloudWatchGauge) Set(value float64) {
	if g.name == nil {
		return
	}
	g.metrics.recordMetricWithTags(*g.name, value, types.StandardUnitNone, g.tags)
}

func (g *cloudWatchGauge) Inc() {
	if g.name == nil {
		return
	}
	g.metrics.recordMetricWithTags(*g.name, 1, types.StandardUnitNone, g.tags)
}

func (g *cloudWatchGauge) Dec() {
	if g.name == nil {
		return
	}
	g.metrics.recordMetricWithTags(*g.name, -1, types.StandardUnitNone, g.tags)
}

func (g *cloudWatchGauge) Add(value float64) {
	if g.name == nil {
		return
	}
	g.metrics.recordMetricWithTags(*g.name, value, types.StandardUnitNone, g.tags)
}
