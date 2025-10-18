package advanced

import (
	"time"
)

// ContentType represents the type of content being moderated
type ContentType string

const (
	// ContentTypeText represents text-based content
	ContentTypeText ContentType = "text"
	// ContentTypeImage represents image content
	ContentTypeImage ContentType = "image"
	// ContentTypeVideo represents video content
	ContentTypeVideo ContentType = "video"
	// ContentTypeAudio represents audio content
	ContentTypeAudio ContentType = "audio"
	// ContentTypeLink represents link content
	ContentTypeLink ContentType = "link"
)

// Severity represents the severity level of a moderation issue
type Severity string

const (
	// SeverityLow represents low severity issues
	SeverityLow Severity = "low"
	// SeverityMedium represents medium severity issues
	SeverityMedium Severity = "medium"
	// SeverityHigh represents high severity issues
	SeverityHigh Severity = "high"
	// SeverityCritical represents critical severity issues
	SeverityCritical Severity = "critical"
)

// ModerationAction represents the action to take
type ModerationAction string

const (
	// ActionAllow represents allowing content to pass through
	ActionAllow ModerationAction = "allow"
	// ActionFlag represents flagging content for review
	ActionFlag ModerationAction = "flag"
	// ActionQuarantine represents quarantining content temporarily
	ActionQuarantine ModerationAction = "quarantine"
	// ActionRemove represents removing content entirely
	ActionRemove ModerationAction = "remove"
	// ActionShadowBan represents shadow banning the content author
	ActionShadowBan ModerationAction = "shadow_ban"
	// ActionReportToAuth represents reporting content to authorities
	ActionReportToAuth ModerationAction = "report_to_authorities"
)

// ModerationEngine is the main interface for content moderation
type ModerationEngine interface {
	// Content analysis
	AnalyzeContent(content string, metadata ContentMetadata) (*ContentAnalysis, error)
	AnalyzeImage(imageURL string, metadata ContentMetadata) (*ImageAnalysis, error)
	AnalyzeVideo(videoURL string, metadata ContentMetadata) (*VideoAnalysis, error)

	// Pattern management
	CreatePattern(pattern *ModerationPattern) error
	UpdatePattern(patternID string, pattern *ModerationPattern) error
	DeletePattern(patternID string) error
	GetPatterns(filter PatternFilter) ([]*ModerationPattern, error)

	// Reputation management
	GetReputationScore(actorID string) (*ReputationScore, error)
	UpdateReputation(actorID string, event ReputationEvent) error

	// Threat intelligence
	ShareThreat(threat *ThreatIntel) error
	GetSharedThreats(since time.Time) ([]*ThreatIntel, error)

	// Decision making
	MakeDecision(analysis *ModerationAnalysis) (*ModerationDecision, error)

	// Reporting
	GetModerationStats(timeRange TimeRange) (*ModerationStats, error)
	GetFalsePositiveRate(timeRange TimeRange) (float64, error)
}

// ContentMetadata contains metadata about the content being analyzed
type ContentMetadata struct {
	ContentID    string
	AuthorID     string
	AuthorDomain string
	ContentType  ContentType
	Language     string
	Context      string // e.g., "post", "comment", "profile"
	Timestamp    time.Time
	ReplyTo      string // If this is a reply
	Mentions     []string
	Hashtags     []string
	URLs         []string
}

// ContentAnalysis represents the result of text content analysis
type ContentAnalysis struct {
	ContentID      string
	Sentiment      SentimentAnalysis
	Toxicity       ToxicityAnalysis
	PII            []PIIEntity
	Topics         []Topic
	Language       LanguageDetection
	Threats        []ThreatIndicator
	CustomFlags    []CustomFlag
	AnalyzedAt     time.Time
	ProcessingTime time.Duration
}

// ImageAnalysis represents the result of image analysis
type ImageAnalysis struct {
	ImageURL       string
	Explicit       ExplicitContent
	Violence       ViolenceDetection
	Text           []TextInImage
	Objects        []ObjectDetection
	Faces          []FaceAnalysis
	Celebrities    []CelebrityMatch
	CustomLabels   []CustomLabel
	AnalyzedAt     time.Time
	ProcessingTime time.Duration
}

// VideoAnalysis represents the result of video analysis
type VideoAnalysis struct {
	VideoURL       string
	Frames         []FrameAnalysis
	Audio          AudioAnalysis
	Duration       time.Duration
	AnalyzedAt     time.Time
	ProcessingTime time.Duration
}

// SentimentAnalysis contains sentiment analysis results
type SentimentAnalysis struct {
	Sentiment  string  // POSITIVE, NEGATIVE, NEUTRAL, MIXED
	Positive   float64 // 0.0 to 1.0
	Negative   float64
	Neutral    float64
	Mixed      float64
	Confidence float64
}

// ToxicityAnalysis contains toxicity detection results
type ToxicityAnalysis struct {
	IsToxic        bool
	ToxicityScore  float64 // 0.0 to 1.0
	Categories     []ToxicCategory
	TargetedGroups []string
	Confidence     float64
}

// ToxicCategory represents a category of toxic content
type ToxicCategory struct {
	Category   string // e.g., "PROFANITY", "HATE_SPEECH", "THREAT"
	Score      float64
	Confidence float64
}

// PIIEntity represents personally identifiable information
type PIIEntity struct {
	Type       string // e.g., "EMAIL", "PHONE", "SSN", "CREDIT_CARD"
	Text       string
	BeginIndex int
	EndIndex   int
	Confidence float64
}

// Topic represents a detected topic
type Topic struct {
	Name     string
	Score    float64
	Category string
}

// LanguageDetection represents detected language
type LanguageDetection struct {
	LanguageCode string // e.g., "en", "es", "fr"
	Confidence   float64
}

// ThreatIndicator represents a potential threat
type ThreatIndicator struct {
	Type        string // e.g., "VIOLENCE", "SELF_HARM", "TERRORISM"
	Severity    Severity
	Confidence  float64
	Evidence    []string
	ActionItems []string
}

// ExplicitContent represents explicit content detection
type ExplicitContent struct {
	IsExplicit         bool
	NudityScore        float64
	SuggestiveScore    float64
	ViolenceScore      float64
	VisuallyDisturbing float64
	Confidence         float64
}

// ViolenceDetection represents violence detection results
type ViolenceDetection struct {
	HasViolence     bool
	WeaponsDetected []string
	BloodScore      float64
	ViolenceScore   float64
	Confidence      float64
}

// ModerationPattern represents a pattern for content matching
type ModerationPattern struct {
	ID          string
	Name        string
	Description string
	Pattern     string  // Regex or keyword pattern
	Type        string  // "regex", "keyword", "phrase"
	Category    string  // Primary category
	Severity    float64 // 0.0 to 1.0
	Action      ModerationAction
	Flags       []string // Additional flags or categories
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Active      bool
	HitCount    int64
	LastHit     time.Time
}

// ReputationScore represents an actor's reputation
type ReputationScore struct {
	ActorID            string
	Score              float64 // 0.0 to 100.0
	Level              string  // "trusted", "normal", "suspicious", "bad_actor"
	ViolationCount     int
	FalsePositiveCount int
	ContentCount       int
	LastViolation      time.Time
	Factors            []ReputationFactor
	UpdatedAt          time.Time
}

// ReputationFactor represents a factor affecting reputation
type ReputationFactor struct {
	Factor      string  // e.g., "content_violations", "user_reports", "false_positives"
	Impact      float64 // Positive or negative impact on score
	Description string
}

// ReputationEvent represents an event that affects reputation
type ReputationEvent struct {
	EventType   string // "violation", "false_positive", "good_content", "user_report"
	Severity    Severity
	Description string
	Timestamp   time.Time
}

// ThreatIntel represents shared threat intelligence
type ThreatIntel struct {
	ID           string
	ThreatType   string
	Indicators   []string // Hashes, patterns, domains, etc.
	Severity     Severity
	Description  string
	SourceDomain string
	FirstSeen    time.Time
	LastSeen     time.Time
	HitCount     int64
	Confidence   float64
	TTL          time.Duration
}

// ModerationAnalysis combines all analysis results
type ModerationAnalysis struct {
	ContentMetadata ContentMetadata
	TextAnalysis    *ContentAnalysis
	ImageAnalysis   *ImageAnalysis
	VideoAnalysis   *VideoAnalysis
	PatternMatches  []PatternMatch
	ReputationScore *ReputationScore
	ThreatMatches   []ThreatMatch
}

// PatternMatch represents a matched moderation pattern
type PatternMatch struct {
	PatternID   string
	PatternName string
	MatchText   string
	Location    string // Where in content
	Confidence  float64
}

// ThreatMatch represents a matched threat
type ThreatMatch struct {
	ThreatID   string
	ThreatType string
	Indicator  string
	Confidence float64
}

// ModerationDecision represents the final moderation decision
type ModerationDecision struct {
	ContentID       string
	Decision        ModerationAction
	Confidence      float64
	Reasons         []DecisionReason
	RequiresReview  bool
	ReviewPriority  int // 1-10, higher is more urgent
	Recommendations []string
	ExpiresAt       time.Time // For temporary actions
	DecidedAt       time.Time
}

// DecisionReason explains why a decision was made
type DecisionReason struct {
	Type        string // "toxicity", "explicit", "pattern", "reputation", etc.
	Severity    Severity
	Description string
	Evidence    any // Can be various types of evidence
}

// ModerationStats contains moderation statistics
type ModerationStats struct {
	TimeRange         TimeRange
	TotalAnalyzed     int64
	ActionCounts      map[ModerationAction]int64
	CategoryCounts    map[string]int64
	SeverityCounts    map[Severity]int64
	AverageConfidence float64
	FalsePositives    int64
	TruePositives     int64
	ResponseTime      time.Duration
}

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// PatternFilter for filtering patterns
type PatternFilter struct {
	Category    string  // Single category filter
	Type        string  // Pattern type filter
	Active      *bool   // Active status filter
	MinSeverity float64 // Minimum severity filter
	Limit       int     // Result limit
	CreatedBy   string  // Filter by creator
}

// RealtimeStats represents current real-time statistics
type RealtimeStats struct {
	Uptime          time.Duration
	TotalAnalyzed   int64
	AnalysisRate    float64 // per second
	AllowRate       float64
	FlagRate        float64
	RemoveRate      float64
	QuarantineRate  float64
	AvgResponseTime time.Duration
	P95ResponseTime time.Duration
}

// PatternStats represents pattern matching statistics
type PatternStats struct {
	PatternID   string
	PatternName string
	HitCount    int64
	LastHit     time.Time
}

// ModerationConfig contains configuration for the moderation engine
type ModerationConfig struct {
	// Thresholds
	ToxicityThreshold   float64
	ExplicitThreshold   float64
	ViolenceThreshold   float64
	ConfidenceThreshold float64

	// Actions
	AutoRemoveThreshold float64
	QuarantineThreshold float64
	FlagThreshold       float64

	// Reputation
	ReputationDecayRate   float64
	BadActorThreshold     float64
	TrustedActorThreshold float64

	// Performance
	MaxAnalysisTime time.Duration
	EnableCaching   bool
	CacheTTL        time.Duration

	// Features
	EnableTextAnalysis      bool
	EnableImageAnalysis     bool
	EnableVideoAnalysis     bool
	EnablePatternMatching   bool
	EnableReputationScoring bool
	EnableThreatSharing     bool

	// AWS Configuration
	ComprehendRegion  string
	RekognitionRegion string
	S3Bucket          string // Added S3 bucket for storing images

	// Cost controls
	MaxMonthlySpend    float64
	EnableCostTracking bool
}

// ModerationError represents an error in moderation operations
type ModerationError struct {
	Code    string
	Message string
	Details map[string]any
}

// Error returns the error message for ModerationError
func (e *ModerationError) Error() string {
	return e.Message
}

// Helper types for complex analysis

// TextInImage represents text detected in an image
type TextInImage struct {
	Text        string
	Confidence  float64
	BoundingBox BoundingBox
}

// ObjectDetection represents an object detected in an image
type ObjectDetection struct {
	Name        string
	Confidence  float64
	BoundingBox BoundingBox
	Parents     []string
}

// FaceAnalysis represents analysis of a detected face
type FaceAnalysis struct {
	BoundingBox BoundingBox
	Emotions    []Emotion
	AgeRange    AgeRange
	Gender      Gender
	Confidence  float64
}

// CelebrityMatch represents a detected celebrity in content
type CelebrityMatch struct {
	Name        string
	Confidence  float64
	BoundingBox BoundingBox
	URLs        []string
}

// BoundingBox represents the location of a detected element
type BoundingBox struct {
	Left   float64
	Top    float64
	Width  float64
	Height float64
}

// Emotion represents a detected emotion in a face
type Emotion struct {
	Type       string // "HAPPY", "SAD", "ANGRY", etc.
	Confidence float64
}

// AgeRange represents an estimated age range
type AgeRange struct {
	Low  int
	High int
}

// Gender represents detected gender information
type Gender struct {
	Value      string // "Male", "Female"
	Confidence float64
}

// FrameAnalysis represents analysis of a single video frame
type FrameAnalysis struct {
	Timestamp     time.Duration
	ImageAnalysis ImageAnalysis
}

// AudioAnalysis represents analysis of audio content
type AudioAnalysis struct {
	Transcription string
	Language      string
	TextAnalysis  *ContentAnalysis
}

// CustomFlag represents a custom moderation flag
type CustomFlag struct {
	Name       string
	Value      any
	Confidence float64
}

// CustomLabel represents a custom detected label
type CustomLabel struct {
	Name       string
	Confidence float64
	Parents    []string
}
