package advanced

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestNewEngineWithMode_ForcesBasicWhenAWSModerationDisabled(t *testing.T) {
	t.Setenv("DISABLE_AWS_MODERATION", "true")
	appconfig.ResetForTests()

	transport := newStubAWSTransport()
	awsCfg := awsConfigForStub(transport)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := DefaultModerationConfig()
	cfg.EnableVideoAnalysis = true

	engine := NewEngineWithMode(EngineOptions{
		Mode:              ModeAWS,
		Config:            cfg,
		ComprehendClient:  comprehend.NewFromConfig(awsCfg),
		RekognitionClient: rekognition.NewFromConfig(awsCfg),
		TableName:         "table",
		PatternRepo:       stubPatternRepo{},
		Logger:            zap.NewNop(),
		DynamoRM:          mockDB,
	})
	require.NotNil(t, engine)

	_, ok := engine.textAnalyzer.(*NoOpTextAnalyzer)
	assert.True(t, ok)
	_, ok = engine.imageAnalyzer.(*NoOpImageAnalyzer)
	assert.True(t, ok)
	assert.False(t, cfg.EnableVideoAnalysis)
}

func TestNewEngineWithMode_RespectsEnvironmentModeAndDisableFlags(t *testing.T) {
	t.Setenv("MODERATION_MODE", "hybrid")
	t.Setenv("DISABLE_COMPREHEND", "true")
	appconfig.ResetForTests()

	transport := newStubAWSTransport()
	awsCfg := awsConfigForStub(transport)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := DefaultModerationConfig()
	cfg.EnableTextAnalysis = true
	cfg.EnableImageAnalysis = true

	engine := NewEngineWithMode(EngineOptions{
		Mode:              "",
		Config:            cfg,
		ComprehendClient:  comprehend.NewFromConfig(awsCfg),
		RekognitionClient: rekognition.NewFromConfig(awsCfg),
		TableName:         "table",
		PatternRepo:       stubPatternRepo{},
		Logger:            zap.NewNop(),
		DynamoRM:          mockDB,
	})
	require.NotNil(t, engine)

	_, ok := engine.textAnalyzer.(*NoOpTextAnalyzer)
	assert.True(t, ok)
}

func TestNewEngineWithMode_DisableRekognitionForcesHybridAndNoOpImage(t *testing.T) {
	t.Setenv("MODERATION_MODE", "aws")
	t.Setenv("DISABLE_REKOGNITION", "true")
	appconfig.ResetForTests()

	transport := newStubAWSTransport()
	awsCfg := awsConfigForStub(transport)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := DefaultModerationConfig()
	cfg.EnableTextAnalysis = true
	cfg.EnableImageAnalysis = true

	engine := NewEngineWithMode(EngineOptions{
		Mode:              "",
		Config:            cfg,
		ComprehendClient:  comprehend.NewFromConfig(awsCfg),
		RekognitionClient: rekognition.NewFromConfig(awsCfg),
		TableName:         "table",
		PatternRepo:       stubPatternRepo{},
		Logger:            zap.NewNop(),
		DynamoRM:          mockDB,
	})
	require.NotNil(t, engine)

	_, ok := engine.textAnalyzer.(*TextAnalyzer)
	assert.True(t, ok)
	_, ok = engine.imageAnalyzer.(*NoOpImageAnalyzer)
	assert.True(t, ok)
}

func TestDefaultModerationConfig_HasSafeDefaults(t *testing.T) {
	cfg := DefaultModerationConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.EnablePatternMatching)
	assert.True(t, cfg.EnableReputationScoring)
	assert.True(t, cfg.EnableThreatSharing)
	assert.True(t, cfg.EnableTextAnalysis)
	assert.True(t, cfg.EnableImageAnalysis)
	assert.False(t, cfg.EnableVideoAnalysis)
}
