// Package mocks provides mock implementations for Lift framework components and handlers for testing.
package mocks

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/mock"
)

// MockHandler mocks a Lift handler
type MockHandler struct {
	mock.Mock
}

// Handle mocks the handler method
func (m *MockHandler) Handle(ctx *lift.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockMiddleware creates a mock middleware function
func MockMiddleware(name string, mockObj *mock.Mock) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Call the mock to record the invocation
			args := mockObj.Called(name, ctx)

			// If mock returns an error, return it
			if args.Error(0) != nil {
				return args.Error(0)
			}

			// Otherwise, call the next handler
			return next.Handle(ctx)
		})
	}
}

// MockLogger mocks the Lift logger interface
type MockLogger struct {
	mock.Mock
}

// Debug mocks the Debug method
func (m *MockLogger) Debug(message string, fields ...map[string]any) {
	m.Called(message, fields)
}

// Info mocks the Info method
func (m *MockLogger) Info(message string, fields ...map[string]any) {
	m.Called(message, fields)
}

// Warn mocks the Warn method
func (m *MockLogger) Warn(message string, fields ...map[string]any) {
	m.Called(message, fields)
}

// Error mocks the Error method
func (m *MockLogger) Error(message string, fields ...map[string]any) {
	m.Called(message, fields)
}

// WithField mocks the WithField method
func (m *MockLogger) WithField(key string, value any) lift.Logger {
	args := m.Called(key, value)
	if logger, ok := args.Get(0).(lift.Logger); ok {
		return logger
	}
	return m
}

// WithFields mocks the WithFields method
func (m *MockLogger) WithFields(fields map[string]any) lift.Logger {
	args := m.Called(fields)
	if logger, ok := args.Get(0).(lift.Logger); ok {
		return logger
	}
	return m
}

// MockCounter mocks a counter metric
type MockCounter struct {
	mock.Mock
}

// Inc mocks the Inc method
func (m *MockCounter) Inc() {
	m.Called()
}

// Add mocks the Add method
func (m *MockCounter) Add(value float64) {
	m.Called(value)
}

// MockHistogram mocks a histogram metric
type MockHistogram struct {
	mock.Mock
}

// Observe mocks the Observe method
func (m *MockHistogram) Observe(value float64) {
	m.Called(value)
}

// MockGauge mocks a gauge metric
type MockGauge struct {
	mock.Mock
}

// Set mocks the Set method
func (m *MockGauge) Set(value float64) {
	m.Called(value)
}

// Inc mocks the Inc method
func (m *MockGauge) Inc() {
	m.Called()
}

// Dec mocks the Dec method
func (m *MockGauge) Dec() {
	m.Called()
}

// Add mocks the Add method
func (m *MockGauge) Add(value float64) {
	m.Called(value)
}

// MockMetricsCollector mocks the metrics collector interface
type MockMetricsCollector struct {
	mock.Mock
}

// Counter mocks the Counter method
func (m *MockMetricsCollector) Counter(name string, tags ...map[string]string) lift.Counter {
	args := m.Called(name, tags)
	if counter, ok := args.Get(0).(lift.Counter); ok {
		return counter
	}
	return &MockCounter{}
}

// Histogram mocks the Histogram method
func (m *MockMetricsCollector) Histogram(name string, tags ...map[string]string) lift.Histogram {
	args := m.Called(name, tags)
	if histogram, ok := args.Get(0).(lift.Histogram); ok {
		return histogram
	}
	return &MockHistogram{}
}

// Gauge mocks the Gauge method
func (m *MockMetricsCollector) Gauge(name string, tags ...map[string]string) lift.Gauge {
	args := m.Called(name, tags)
	if gauge, ok := args.Get(0).(lift.Gauge); ok {
		return gauge
	}
	return &MockGauge{}
}

// Flush mocks the Flush method
func (m *MockMetricsCollector) Flush() error {
	args := m.Called()
	return args.Error(0)
}

// LiftTestContext provides a test context with all mocks
type LiftTestContext struct {
	Ctx     *lift.Context
	Logger  *MockLogger
	Metrics *MockMetricsCollector
	Mocks   *mock.Mock
}

// NewLiftTestContext creates a new test context
func NewLiftTestContext(_ *testing.T) *LiftTestContext {
	logger := new(MockLogger)
	metrics := new(MockMetricsCollector)
	mocks := new(mock.Mock)

	// Set up default expectations
	logger.On("Debug", mock.Anything, mock.Anything).Maybe()
	logger.On("Info", mock.Anything, mock.Anything).Maybe()
	logger.On("WithField", mock.Anything, mock.Anything).Return(logger).Maybe()
	logger.On("WithFields", mock.Anything).Return(logger).Maybe()

	metrics.On("Counter", mock.Anything, mock.Anything).Return(&MockCounter{}).Maybe()
	metrics.On("Histogram", mock.Anything, mock.Anything).Return(&MockHistogram{}).Maybe()
	metrics.On("Gauge", mock.Anything, mock.Anything).Return(&MockGauge{}).Maybe()
	metrics.On("Flush").Return(nil).Maybe()

	// Create context
	ctx := &lift.Context{
		Context: context.Background(),
		Logger:  logger,
		Metrics: metrics,
	}

	return &LiftTestContext{
		Ctx:     ctx,
		Logger:  logger,
		Metrics: metrics,
		Mocks:   mocks,
	}
}

// ExpectLog sets up an expectation for a log message
func (tc *LiftTestContext) ExpectLog(level, message string) {
	switch level {
	case "debug":
		tc.Logger.On("Debug", message, mock.Anything).Once()
	case "info":
		tc.Logger.On("Info", message, mock.Anything).Once()
	case "warn":
		tc.Logger.On("Warn", message, mock.Anything).Once()
	case "error":
		tc.Logger.On("Error", message, mock.Anything).Once()
	}
}

// ExpectMetric sets up an expectation for a metric
func (tc *LiftTestContext) ExpectMetric(metricType, name string, value float64) {
	switch metricType {
	case "counter":
		counter := new(MockCounter)
		counter.On("Add", value).Once()
		tc.Metrics.On("Counter", name, mock.Anything).Return(counter).Once()
	case "histogram":
		histogram := new(MockHistogram)
		histogram.On("Record", value).Once()
		tc.Metrics.On("Histogram", name, mock.Anything).Return(histogram).Once()
	case "gauge":
		gauge := new(MockGauge)
		gauge.On("Set", value).Once()
		tc.Metrics.On("Gauge", name, mock.Anything).Return(gauge).Once()
	}
}

// AssertExpectations asserts all mock expectations were met
func (tc *LiftTestContext) AssertExpectations(t *testing.T) {
	tc.Logger.AssertExpectations(t)
	tc.Metrics.AssertExpectations(t)
	tc.Mocks.AssertExpectations(t)
}
