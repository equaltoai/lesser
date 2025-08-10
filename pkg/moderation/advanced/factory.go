package advanced

import (
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/comprehend"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ModerationMode defines the operation mode for the moderation engine
type ModerationMode string

const (
	// ModeAWS uses AWS services for advanced analysis
	ModeAWS ModerationMode = "aws"
	
	// ModeBasic uses basic implementations without AWS
	ModeBasic ModerationMode = "basic"
	
	// ModeHybrid uses AWS when available, falls back to basic
	ModeHybrid ModerationMode = "hybrid"
)

// EngineOptions contains options for creating a moderation engine
type EngineOptions struct {
	Mode              ModerationMode
	Config            *ModerationConfig
	ComprehendClient  *comprehend.Client
	RekognitionClient *rekognition.Client
	TableName         string
	PatternRepo       PatternRepository
	Logger            *zap.Logger
	CostTracker       CostTracker
	DynamoRM          core.DB
}

// NewEngineWithMode creates a moderation engine with the specified mode
func NewEngineWithMode(opts EngineOptions) *Engine {
	// Check global AWS moderation flags first
	awsModDisabled := getEnvBool("DISABLE_AWS_MODERATION", false)
	comprehendDisabled := getEnvBool("DISABLE_COMPREHEND", false)
	rekognitionDisabled := getEnvBool("DISABLE_REKOGNITION", false)

	// If AWS moderation is globally disabled, force basic mode
	if awsModDisabled {
		opts.Logger.Info("AWS moderation disabled globally, forcing basic mode")
		opts.Mode = ModeBasic
	}

	// Determine mode from environment if not specified
	if opts.Mode == "" {
		modeStr := os.Getenv("MODERATION_MODE")
		switch modeStr {
		case "aws":
			opts.Mode = ModeAWS
		case "basic":
			opts.Mode = ModeBasic
		case "hybrid":
			opts.Mode = ModeHybrid
		default:
			// Default to hybrid mode for maximum compatibility
			opts.Mode = ModeHybrid
		}
	}

	// Override AWS clients if services are specifically disabled
	if comprehendDisabled && opts.ComprehendClient != nil {
		opts.Logger.Info("Comprehend disabled by feature flag, removing client")
		opts.ComprehendClient = nil
	}
	
	if rekognitionDisabled && opts.RekognitionClient != nil {
		opts.Logger.Info("Rekognition disabled by feature flag, removing client")
		opts.RekognitionClient = nil
	}

	// Apply mode-specific configuration
	switch opts.Mode {
	case ModeBasic:
		// Force AWS clients to nil for basic mode
		opts.ComprehendClient = nil
		opts.RekognitionClient = nil
		
		// Disable AWS-dependent features
		if opts.Config != nil {
			opts.Config.EnableTextAnalysis = true    // Use basic text analysis
			opts.Config.EnableImageAnalysis = true   // Use basic image analysis
			opts.Config.EnableVideoAnalysis = false  // No basic video analysis yet
		}
		
	case ModeAWS:
		// Require AWS clients for AWS mode
		if opts.ComprehendClient == nil || opts.RekognitionClient == nil {
			opts.Logger.Warn("AWS mode requested but clients not provided, falling back to hybrid mode")
			opts.Mode = ModeHybrid
		}
		
	case ModeHybrid:
		// Use whatever is available
		if opts.ComprehendClient == nil {
			opts.Logger.Info("Comprehend client not available, will use basic text analysis")
		}
		if opts.RekognitionClient == nil {
			opts.Logger.Info("Rekognition client not available, will use basic image analysis")
		}
	}

	opts.Logger.Info("creating moderation engine",
		zap.String("mode", string(opts.Mode)),
		zap.Bool("aws_text_available", opts.ComprehendClient != nil),
		zap.Bool("aws_image_available", opts.RekognitionClient != nil))

	// Create engine with appropriate configuration
	return NewEngine(
		opts.Config,
		opts.ComprehendClient,
		opts.RekognitionClient,
		opts.TableName,
		opts.PatternRepo,
		opts.Logger,
		opts.CostTracker,
		opts.DynamoRM,
	)
}

// DefaultModerationConfig returns a default configuration that works without AWS
func DefaultModerationConfig() *ModerationConfig {
	return &ModerationConfig{
		// Core features that work without AWS
		EnablePatternMatching:   true,
		EnableReputationScoring: true,
		EnableThreatSharing:     true,
		
		// Features that can work with basic implementations
		EnableTextAnalysis:  true,
		EnableImageAnalysis: true,
		EnableVideoAnalysis: false, // No basic video analysis yet
		
		// Decision thresholds
		ToxicityThreshold:   0.7,
		ExplicitThreshold:   0.7,
		ViolenceThreshold:   0.7,
		ConfidenceThreshold: 0.5,
		
		// Action thresholds
		AutoRemoveThreshold: 0.8,
		QuarantineThreshold: 0.6,
		FlagThreshold:       0.4,
		
		// Reputation settings
		ReputationDecayRate:   0.1,
		BadActorThreshold:     0.3,
		TrustedActorThreshold: 0.8,
		
		// Performance
		MaxAnalysisTime: 30 * time.Second,
		EnableCaching:   true,
		CacheTTL:        10 * time.Minute,
		
		// AWS Configuration (safe defaults)
		ComprehendRegion:  "us-east-1",
		RekognitionRegion: "us-east-1", 
		S3Bucket:          "default-bucket",
		
		// Cost controls
		MaxMonthlySpend:    1000.0, // $1000 default limit
		EnableCostTracking: true,
	}
}

