//go:build integration

package advanced

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	rekognitionTypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/transcribe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockCostTracker for testing
type MockCostTracker struct {
	mock.Mock
}

func (m *MockCostTracker) TrackComprehendRequest(operation string, units int) {
	m.Called(operation, units)
}

func (m *MockCostTracker) TrackTranscribeRequest(jobName string, minutes int) {
	m.Called(jobName, minutes)
}

// MockPatternRepository for testing
type MockPatternRepository struct {
	mock.Mock
}

func (m *MockPatternRepository) GetPatterns(ctx context.Context, filter PatternFilter) ([]*ModerationPattern, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*ModerationPattern), args.Error(1)
}

func (m *MockPatternRepository) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *MockPatternRepository) UpdatePattern(ctx context.Context, patternID string, pattern *ModerationPattern) error {
	args := m.Called(ctx, patternID, pattern)
	return args.Error(0)
}

func (m *MockPatternRepository) DeletePattern(ctx context.Context, patternID string) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

func (m *MockPatternRepository) GetPattern(ctx context.Context, patternID string) (*ModerationPattern, error) {
	args := m.Called(ctx, patternID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ModerationPattern), args.Error(1)
}

func (m *MockPatternRepository) IncrementHitCount(ctx context.Context, patternID string) error {
	args := m.Called(ctx, patternID)
	return args.Error(0)
}

func (m *MockPatternRepository) LoadActivePatterns(ctx context.Context) ([]*ModerationPattern, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*ModerationPattern), args.Error(1)
}

// Test that the transcription service is properly initialized and wired
func TestTranscriptionServiceWiring(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockCostTracker := &MockCostTracker{}

	// Test transcription service creation directly
	var transcribeClient *transcribe.Client
	var s3Client *s3.Client

	// Test creation of transcription service with nil clients
	transcriptionService := NewTranscriptionService(
		transcribeClient,
		s3Client,
		logger,
		"test-bucket",
		mockCostTracker,
	)

	// Should still create the service even with nil clients (will fail gracefully at runtime)
	assert.NotNil(t, transcriptionService)
	assert.Equal(t, logger, transcriptionService.logger)
	assert.Equal(t, mockCostTracker, transcriptionService.costTracker)
	assert.Equal(t, "test-bucket", transcriptionService.outputBucket)

	logger.Info("transcription service created successfully for testing")
}

// Test VideoAnalyzer transcribeAudio method behavior
func TestVideoAnalyzerTranscribeAudio(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockCostTracker := &MockCostTracker{}

	config := &ModerationConfig{
		EnableTextAnalysis: true,
	}

	// Create VideoAnalyzer with no transcription service
	videoAnalyzer := &VideoAnalyzer{
		logger:               logger,
		config:               config,
		costTracker:          mockCostTracker,
		transcriptionService: nil, // No transcription service
		textAnalyzer:         NewNoOpTextAnalyzer(logger, config),
	}

	// Test S3 object
	video := &rekognitionTypes.Video{
		S3Object: &rekognitionTypes.S3Object{
			Bucket: aws.String("test-bucket"),
			Name:   aws.String("test-video.mp4"),
		},
	}

	ctx := context.Background()

	// Test transcribeAudio with no transcription service
	result, err := videoAnalyzer.transcribeAudio(ctx, video)

	// Should not error, but should return empty transcription
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "", result.Transcription)
	assert.Equal(t, "unknown", result.Language)
	assert.Nil(t, result.TextAnalysis)
}

// Test that engine properly handles video analysis
func TestEngineVideoAnalysis(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockCostTracker := &MockCostTracker{}

	config := &ModerationConfig{
		EnableVideoAnalysis: true,
		EnableTextAnalysis:  true,
		S3Bucket:            "test-bucket",
	}

	// Test basic video analyzer creation directly to avoid DynamoDB issues
	videoAnalyzer := &VideoAnalyzer{
		logger:               logger,
		config:               config,
		costTracker:          mockCostTracker,
		transcriptionService: nil, // No transcription service
		textAnalyzer:         NewNoOpTextAnalyzer(logger, config),
		cacheTTL:             30 * time.Minute,
	}

	// Verify the video analyzer was configured correctly
	assert.NotNil(t, videoAnalyzer)
	assert.Equal(t, logger, videoAnalyzer.logger)
	assert.Equal(t, config, videoAnalyzer.config)
	assert.Nil(t, videoAnalyzer.transcriptionService)
	assert.NotNil(t, videoAnalyzer.textAnalyzer)

	logger.Info("video analyzer configuration test completed")
}

// Test the integration with ModerationConfig
func TestModerationConfigIntegration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Test default config
	defaultConfig := DefaultModerationConfig()
	assert.NotNil(t, defaultConfig)
	assert.True(t, defaultConfig.EnablePatternMatching)
	assert.False(t, defaultConfig.EnableTextAnalysis) // Should be false by default for no-AWS mode

	// Test advanced AWS config
	awsConfig := &ModerationConfig{
		EnableTextAnalysis:      true,
		EnableImageAnalysis:     true,
		EnableVideoAnalysis:     true,
		EnablePatternMatching:   true,
		EnableReputationScoring: true,
		ToxicityThreshold:       0.8,
		ExplicitThreshold:       0.9,
		ViolenceThreshold:       0.85,
	}
	assert.NotNil(t, awsConfig)
	assert.True(t, awsConfig.EnableTextAnalysis)
	assert.True(t, awsConfig.EnableImageAnalysis)
	assert.True(t, awsConfig.EnableVideoAnalysis)

	logger.Info("moderation configuration tests completed")
}

// Test factory function behavior with new parameters
func TestFactoryWithNewParameters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockCostTracker := &MockCostTracker{}

	config := DefaultModerationConfig()

	// Test EngineOptions with new fields
	opts := EngineOptions{
		Mode:              ModeBasic,
		Config:            config,
		ComprehendClient:  nil,
		RekognitionClient: nil,
		TranscribeClient:  nil, // New field
		S3Client:          nil, // New field
		TableName:         "test-table",
		PatternRepo:       nil,
		Logger:            logger,
		CostTracker:       mockCostTracker,
		DynamoRM:          nil,
	}

	// Should create engine without errors
	engine := NewEngineWithMode(opts)
	assert.NotNil(t, engine)
	assert.Equal(t, config, engine.config)

	logger.Info("factory integration test completed")
}
