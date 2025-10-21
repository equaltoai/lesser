package security

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockControlTester implements ControlTester for testing
type MockControlTester struct {
	mock.Mock
}

func (m *MockControlTester) TestControl(ctx context.Context, control SOC2Control) (*ControlTestResult, error) {
	args := m.Called(ctx, control)
	return args.Get(0).(*ControlTestResult), args.Error(1)
}

func (m *MockControlTester) TestAllControls(ctx context.Context) ([]*ControlTestResult, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*ControlTestResult), args.Error(1)
}

func (m *MockControlTester) GetControlStatus(controlID string) (*ControlStatus, error) {
	args := m.Called(controlID)
	return args.Get(0).(*ControlStatus), args.Error(1)
}

func (m *MockControlTester) ScheduleControlTest(controlID string, frequency time.Duration) error {
	args := m.Called(controlID, frequency)
	return args.Error(0)
}

// MockEvidenceCollector implements EvidenceCollector for testing
type MockEvidenceCollector struct {
	mock.Mock
}

func (m *MockEvidenceCollector) CollectEvidence(ctx context.Context, control SOC2Control) (*ControlEvidence, error) {
	args := m.Called(ctx, control)
	return args.Get(0).(*ControlEvidence), args.Error(1)
}

func (m *MockEvidenceCollector) CollectSystemEvidence(ctx context.Context) (*SystemEvidence, error) {
	args := m.Called(ctx)
	return args.Get(0).(*SystemEvidence), args.Error(1)
}

func (m *MockEvidenceCollector) ValidateEvidence(evidence *ControlEvidence) (*EvidenceValidation, error) {
	args := m.Called(evidence)
	return args.Get(0).(*EvidenceValidation), args.Error(1)
}

func (m *MockEvidenceCollector) ArchiveEvidence(evidence *ControlEvidence) error {
	args := m.Called(evidence)
	return args.Error(0)
}

// MockExceptionTracker implements ExceptionTracker for testing
type MockExceptionTracker struct {
	mock.Mock
}

func (m *MockExceptionTracker) RecordException(exception *ComplianceException) error {
	args := m.Called(exception)
	return args.Error(0)
}

func (m *MockExceptionTracker) GetExceptions(controlID string, since time.Time) ([]*ComplianceException, error) {
	args := m.Called(controlID, since)
	return args.Get(0).([]*ComplianceException), args.Error(1)
}

func (m *MockExceptionTracker) GetExceptionTrends() (*ExceptionTrends, error) {
	args := m.Called()
	return args.Get(0).(*ExceptionTrends), args.Error(1)
}

func (m *MockExceptionTracker) ResolveException(exceptionID string, resolution *ExceptionResolution) error {
	args := m.Called(exceptionID, resolution)
	return args.Error(0)
}

// MockAlertManager implements AlertManager for testing
type MockAlertManager struct {
	mock.Mock
}

func (m *MockAlertManager) SendAlert(alert *ComplianceAlert) error {
	args := m.Called(alert)
	return args.Error(0)
}

func (m *MockAlertManager) SendCriticalAlert(alert *ComplianceAlert) error {
	args := m.Called(alert)
	return args.Error(0)
}

func (m *MockAlertManager) GetAlertHistory(since time.Time) ([]*ComplianceAlert, error) {
	args := m.Called(since)
	return args.Get(0).([]*ComplianceAlert), args.Error(1)
}

func (m *MockAlertManager) ConfigureAlertRules(rules []AlertRule) error {
	args := m.Called(rules)
	return args.Error(0)
}

func TestNewSOC2ContinuousMonitor(t *testing.T) {
	config := SOC2MonitoringConfig{
		Enabled:             true,
		MonitoringInterval:  time.Hour,
		ExceptionThreshold:  5,
		AlertingEnabled:     true,
		ComplianceThreshold: 95.0,
	}

	monitor := NewSOC2ContinuousMonitor(config)

	assert.NotNil(t, monitor)
	assert.Equal(t, config, monitor.config)
	assert.NotNil(t, monitor.scheduler)
	assert.False(t, monitor.running)
}

func TestSOC2ContinuousMonitor_SetComponents(t *testing.T) {
	config := SOC2MonitoringConfig{Enabled: true}
	monitor := NewSOC2ContinuousMonitor(config)

	mockTester := &MockControlTester{}
	mockCollector := &MockEvidenceCollector{}
	mockTracker := &MockExceptionTracker{}
	mockAlertManager := &MockAlertManager{}

	monitor.SetControlTester(mockTester)
	monitor.SetEvidenceCollector(mockCollector)
	monitor.SetExceptionTracker(mockTracker)
	monitor.SetAlertManager(mockAlertManager)

	assert.Equal(t, mockTester, monitor.controlTester)
	assert.Equal(t, mockCollector, monitor.evidenceCollector)
	assert.Equal(t, mockTracker, monitor.exceptionTracker)
	assert.Equal(t, mockAlertManager, monitor.alertManager)
}

func TestSOC2ContinuousMonitor_Start_Disabled(t *testing.T) {
	config := SOC2MonitoringConfig{Enabled: false}
	monitor := NewSOC2ContinuousMonitor(config)

	ctx := context.Background()
	err := monitor.Start(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "continuous monitoring not enabled")
	assert.False(t, monitor.running)
}

func TestSOC2ContinuousMonitor_Start_AlreadyRunning(t *testing.T) {
	config := SOC2MonitoringConfig{Enabled: true}
	monitor := NewSOC2ContinuousMonitor(config)
	monitor.running = true

	ctx := context.Background()
	err := monitor.Start(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "continuous monitoring already running")
}

func TestSOC2ContinuousMonitor_Start_Success(t *testing.T) {
	config := SOC2MonitoringConfig{
		Enabled:            true,
		MonitoringInterval: time.Minute,
		ControlTestFrequency: map[string]time.Duration{
			"CC1": time.Hour,
			"CC2": 2 * time.Hour,
		},
	}
	monitor := NewSOC2ContinuousMonitor(config)

	ctx := context.Background()
	err := monitor.Start(ctx)

	assert.NoError(t, err)
	assert.True(t, monitor.running)

	// Clean up
	monitor.Stop()
}

func TestSOC2ContinuousMonitor_Stop(t *testing.T) {
	config := SOC2MonitoringConfig{
		Enabled:            true,
		MonitoringInterval: time.Minute,
	}
	monitor := NewSOC2ContinuousMonitor(config)

	// Start first
	ctx := context.Background()
	monitor.Start(ctx)
	assert.True(t, monitor.running)

	// Stop
	err := monitor.Stop()
	assert.NoError(t, err)
	assert.False(t, monitor.running)

	// Stop again (should not error)
	err = monitor.Stop()
	assert.NoError(t, err)
}

func TestSOC2ContinuousMonitor_GetComplianceStatus_NoTester(t *testing.T) {
	config := SOC2MonitoringConfig{Enabled: true}
	monitor := NewSOC2ContinuousMonitor(config)

	ctx := context.Background()
	status, err := monitor.GetComplianceStatus(ctx)

	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "control tester not configured")
}

func TestSOC2ContinuousMonitor_GetComplianceStatus_Success(t *testing.T) {
	config := SOC2MonitoringConfig{Enabled: true}
	monitor := NewSOC2ContinuousMonitor(config)

	mockTester := &MockControlTester{}
	mockTracker := &MockExceptionTracker{}
	monitor.SetControlTester(mockTester)
	monitor.SetExceptionTracker(mockTracker)

	// Mock test results
	testResults := []*ControlTestResult{
		{
			ControlID: "CC1",
			Passed:    true,
			Score:     95.0,
		},
		{
			ControlID: "CC2",
			Passed:    false,
			Score:     70.0,
		},
		{
			ControlID: "CC3",
			Passed:    true,
			Score:     98.0,
		},
	}

	// Mock exception trends
	exceptionTrends := &ExceptionTrends{
		Period:             "monthly",
		TotalExceptions:    5,
		OpenExceptions:     2,
		ResolvedExceptions: 3,
		ComplianceRate:     85.0,
		TrendDirection:     "improving",
	}

	mockTester.On("TestAllControls", mock.Anything).Return(testResults, nil)
	mockTracker.On("GetExceptionTrends").Return(exceptionTrends, nil)

	ctx := context.Background()
	status, err := monitor.GetComplianceStatus(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, 3, status.TotalControls)
	assert.Equal(t, 2, status.EffectiveControls)
	assert.InDelta(t, 66.67, status.ComplianceRate, 0.01)
	assert.Equal(t, testResults, status.ControlResults)
	assert.Equal(t, exceptionTrends, status.ExceptionTrends)

	mockTester.AssertExpectations(t)
	mockTracker.AssertExpectations(t)
}

func TestSOC2ContinuousMonitor_GetComplianceStatus_TestError(t *testing.T) {
	config := SOC2MonitoringConfig{Enabled: true}
	monitor := NewSOC2ContinuousMonitor(config)

	mockTester := &MockControlTester{}
	monitor.SetControlTester(mockTester)

	mockTester.On("TestAllControls", mock.Anything).Return([]*ControlTestResult{}, assert.AnError)

	ctx := context.Background()
	status, err := monitor.GetComplianceStatus(ctx)

	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "failed to test controls")

	mockTester.AssertExpectations(t)
}

func TestMonitoringScheduler_NewMonitoringScheduler(t *testing.T) {
	scheduler := NewMonitoringScheduler()

	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.tasks)
	assert.NotNil(t, scheduler.stopCh)
}

func TestMonitoringScheduler_AddTask(t *testing.T) {
	scheduler := NewMonitoringScheduler()

	task := &ScheduledTask{
		ID:        "test-task",
		Name:      "Test Task",
		Type:      "test",
		Frequency: time.Hour,
		Enabled:   true,
		TaskFunc:  func() error { return nil },
	}

	scheduler.AddTask(task)

	assert.Equal(t, task, scheduler.tasks["test-task"])
}

func TestMonitoringScheduler_Start_Stop(t *testing.T) {
	scheduler := NewMonitoringScheduler()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := scheduler.Start(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, scheduler.ticker)

	err = scheduler.Stop()
	assert.NoError(t, err)
}

func TestMonitoringScheduler_RunScheduledTasks(t *testing.T) {
	scheduler := NewMonitoringScheduler()

	executed := false
	task := &ScheduledTask{
		ID:        "test-task",
		Name:      "Test Task",
		Type:      "test",
		Frequency: time.Hour,
		NextRun:   time.Now().Add(-time.Minute), // Past due
		Enabled:   true,
		TaskFunc: func() error {
			executed = true
			return nil
		},
	}

	scheduler.AddTask(task)
	scheduler.runScheduledTasks()

	// Give goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	assert.True(t, executed)
}

func TestMonitoringScheduler_RunScheduledTasks_Disabled(t *testing.T) {
	scheduler := NewMonitoringScheduler()

	executed := false
	task := &ScheduledTask{
		ID:        "test-task",
		Name:      "Test Task",
		Type:      "test",
		Frequency: time.Hour,
		NextRun:   time.Now().Add(-time.Minute), // Past due
		Enabled:   false,                        // Disabled
		TaskFunc: func() error {
			executed = true
			return nil
		},
	}

	scheduler.AddTask(task)
	scheduler.runScheduledTasks()

	// Give goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	assert.False(t, executed)
}

func TestMonitoringScheduler_RunScheduledTasks_NotDue(t *testing.T) {
	scheduler := NewMonitoringScheduler()

	executed := false
	task := &ScheduledTask{
		ID:        "test-task",
		Name:      "Test Task",
		Type:      "test",
		Frequency: time.Hour,
		NextRun:   time.Now().Add(time.Hour), // Future
		Enabled:   true,
		TaskFunc: func() error {
			executed = true
			return nil
		},
	}

	scheduler.AddTask(task)
	scheduler.runScheduledTasks()

	// Give goroutine time to execute
	time.Sleep(100 * time.Millisecond)

	assert.False(t, executed)
}

func TestSOC2Control_Validation(t *testing.T) {
	control := SOC2Control{
		ID:               "CC1",
		Name:             "Control Environment",
		AutomatedTesting: true,
		ComplianceTarget: 95.0,
		CriticalControl:  true,
	}

	assert.Equal(t, "CC1", control.ID)
	assert.Equal(t, "Control Environment", control.Name)
	assert.True(t, control.AutomatedTesting)
	assert.True(t, control.CriticalControl)
	assert.Equal(t, 95.0, control.ComplianceTarget)
}

func TestControlTestResult_Validation(t *testing.T) {
	result := &ControlTestResult{
		ControlID: "CC1",
		Status:    "effective",
		Score:     95.0,
		Passed:    true,
	}

	assert.Equal(t, "CC1", result.ControlID)
	assert.True(t, result.Passed)
	assert.Equal(t, 95.0, result.Score)
	assert.Equal(t, "effective", result.Status)
}

func TestControlEvidence_Validation(t *testing.T) {
	evidence := &ControlEvidence{
		ID:           "EV-001",
		ControlID:    "CC1",
		EvidenceType: "log_file",
		Verified:     true,
	}

	assert.Equal(t, "EV-001", evidence.ID)
	assert.Equal(t, "CC1", evidence.ControlID)
	assert.True(t, evidence.Verified)
	assert.Equal(t, "log_file", evidence.EvidenceType)
}

func TestComplianceException_Validation(t *testing.T) {
	exception := &ComplianceException{
		ID:        "EX-001",
		ControlID: "CC1",
		Severity:  "medium",
		Status:    "open",
	}

	assert.Equal(t, "EX-001", exception.ID)
	assert.Equal(t, "CC1", exception.ControlID)
	assert.Equal(t, "medium", exception.Severity)
	assert.Equal(t, "open", exception.Status)
}

func TestComplianceAlert_Validation(t *testing.T) {
	alert := &ComplianceAlert{
		ID:           "AL-001",
		Type:         "control_failure",
		Severity:     "high",
		Escalated:    false,
		Acknowledged: false,
	}

	assert.Equal(t, "AL-001", alert.ID)
	assert.Equal(t, "control_failure", alert.Type)
	assert.Equal(t, "high", alert.Severity)
	assert.False(t, alert.Escalated)
	assert.False(t, alert.Acknowledged)
}

func TestExceptionTrends_Calculation(t *testing.T) {
	trends := &ExceptionTrends{
		TotalExceptions:    10,
		OpenExceptions:     3,
		ResolvedExceptions: 7,
		TrendDirection:     "improving",
		ComplianceRate:     85.0,
	}

	assert.Equal(t, 10, trends.TotalExceptions)
	assert.Equal(t, 3, trends.OpenExceptions)
	assert.Equal(t, 7, trends.ResolvedExceptions)
	assert.Equal(t, "improving", trends.TrendDirection)
	assert.Equal(t, 85.0, trends.ComplianceRate)
}

func TestSOC2ComplianceStatus_Calculation(t *testing.T) {
	status := &SOC2ComplianceStatus{
		TotalControls:     5,
		EffectiveControls: 4,
		ComplianceRate:    80.0,
	}

	assert.Equal(t, 5, status.TotalControls)
	assert.Equal(t, 4, status.EffectiveControls)
	assert.Equal(t, 80.0, status.ComplianceRate)
}

// Benchmark tests for performance validation
func BenchmarkSOC2ContinuousMonitor_GetComplianceStatus(b *testing.B) {
	config := SOC2MonitoringConfig{Enabled: true}
	monitor := NewSOC2ContinuousMonitor(config)

	mockTester := &MockControlTester{}
	monitor.SetControlTester(mockTester)

	// Create test results
	testResults := make([]*ControlTestResult, 100)
	for i := 0; i < 100; i++ {
		testResults[i] = &ControlTestResult{
			ControlID: fmt.Sprintf("CC%d", i),
			Passed:    i%2 == 0,
			Score:     float64(80 + i%20),
		}
	}

	mockTester.On("TestAllControls", mock.Anything).Return(testResults, nil)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := monitor.GetComplianceStatus(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMonitoringScheduler_RunScheduledTasks(b *testing.B) {
	scheduler := NewMonitoringScheduler()

	// Add multiple tasks
	for i := 0; i < 100; i++ {
		task := &ScheduledTask{
			ID:        fmt.Sprintf("task-%d", i),
			Name:      fmt.Sprintf("Task %d", i),
			Type:      "test",
			Frequency: time.Hour,
			NextRun:   time.Now().Add(-time.Minute), // Past due
			Enabled:   true,
			TaskFunc:  func() error { return nil },
		}
		scheduler.AddTask(task)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scheduler.runScheduledTasks()
	}
}
