package ai

import (
	"time"
)

// AIAnalysis represents comprehensive AI analysis of content
type AIAnalysis struct {
	// Identity
	ID         string `json:"id" dynamodbav:"ID"`
	ObjectID   string `json:"object_id" dynamodbav:"ObjectID"`
	ObjectType string `json:"object_type" dynamodbav:"ObjectType"`

	// Text Analysis (AWS Comprehend)
	TextAnalysis *TextAnalysis `json:"text_analysis,omitempty" dynamodbav:"TextAnalysis,omitempty"`

	// Image Analysis (AWS Rekognition)
	ImageAnalysis *ImageAnalysis `json:"image_analysis,omitempty" dynamodbav:"ImageAnalysis,omitempty"`

	// AI Detection (AWS Bedrock)
	AIDetection *AIDetection `json:"ai_detection,omitempty" dynamodbav:"AIDetection,omitempty"`

	// Spam Detection (Custom)
	SpamAnalysis *SpamAnalysis `json:"spam_analysis,omitempty" dynamodbav:"SpamAnalysis,omitempty"`

	// Composite Scores
	OverallRisk      float64 `json:"overall_risk" dynamodbav:"OverallRisk"`
	ModerationAction string  `json:"moderation_action" dynamodbav:"ModerationAction"`
	Confidence       float64 `json:"confidence" dynamodbav:"Confidence"`

	// Metadata
	AnalyzedAt time.Time `json:"analyzed_at" dynamodbav:"AnalyzedAt"`
	Version    string    `json:"version" dynamodbav:"Version"`
	TTL        int64     `json:"-" dynamodbav:"TTL,omitempty"`
}

// TextAnalysis from AWS Comprehend
type TextAnalysis struct {
	// Sentiment
	Sentiment       string             `json:"sentiment" dynamodbav:"Sentiment"`
	SentimentScores map[string]float64 `json:"sentiment_scores" dynamodbav:"SentimentScores"`

	// Toxicity (using Comprehend custom classifier)
	ToxicityScore  float64  `json:"toxicity_score" dynamodbav:"ToxicityScore"`
	ToxicityLabels []string `json:"toxicity_labels" dynamodbav:"ToxicityLabels"`

	// Content Classification
	Categories []ContentCategory `json:"categories" dynamodbav:"Categories"`

	// PII Detection
	ContainsPII bool        `json:"contains_pii" dynamodbav:"ContainsPII"`
	PIIEntities []PIIEntity `json:"pii_entities,omitempty" dynamodbav:"PIIEntities,omitempty"`

	// Language
	DominantLanguage string             `json:"dominant_language" dynamodbav:"DominantLanguage"`
	LanguageScores   map[string]float64 `json:"language_scores" dynamodbav:"LanguageScores"`

	// Entities & Key Phrases
	Entities   []Entity `json:"entities" dynamodbav:"Entities"`
	KeyPhrases []string `json:"key_phrases" dynamodbav:"KeyPhrases"`
}

// ImageAnalysis from AWS Rekognition
type ImageAnalysis struct {
	// Moderation
	ModerationLabels []ModerationLabel `json:"moderation_labels" dynamodbav:"ModerationLabels"`
	IsNSFW           bool              `json:"is_nsfw" dynamodbav:"IsNSFW"`
	NSFWConfidence   float64           `json:"nsfw_confidence" dynamodbav:"NSFWConfidence"`

	// Violence Detection
	ViolenceScore   float64 `json:"violence_score" dynamodbav:"ViolenceScore"`
	WeaponsDetected bool    `json:"weapons_detected" dynamodbav:"WeaponsDetected"`

	// Text in Images
	DetectedText []string `json:"detected_text" dynamodbav:"DetectedText"`
	TextToxicity float64  `json:"text_toxicity" dynamodbav:"TextToxicity"`

	// Other Detection
	CelebrityFaces []Celebrity `json:"celebrity_faces,omitempty" dynamodbav:"CelebrityFaces,omitempty"`
	Logos          []Logo      `json:"logos,omitempty" dynamodbav:"Logos,omitempty"`

	// Synthetic Media Detection
	DeepfakeScore float64 `json:"deepfake_score" dynamodbav:"DeepfakeScore"`
}

// AIDetection using AWS Bedrock
type AIDetection struct {
	// AI-Generated Content Detection
	AIGeneratedProbability float64 `json:"ai_generated_probability" dynamodbav:"AIGeneratedProbability"`
	GenerationModel        string  `json:"generation_model,omitempty" dynamodbav:"GenerationModel,omitempty"`

	// Pattern Analysis
	PatternConsistency float64 `json:"pattern_consistency" dynamodbav:"PatternConsistency"`
	StyleDeviation     float64 `json:"style_deviation" dynamodbav:"StyleDeviation"`

	// Semantic Coherence
	SemanticCoherence float64 `json:"semantic_coherence" dynamodbav:"SemanticCoherence"`
	TopicConsistency  float64 `json:"topic_consistency" dynamodbav:"TopicConsistency"`

	// Suspicious Patterns
	SuspiciousPatterns []string `json:"suspicious_patterns" dynamodbav:"SuspiciousPatterns"`
}

// SpamAnalysis using custom heuristics
type SpamAnalysis struct {
	SpamScore      float64         `json:"spam_score" dynamodbav:"SpamScore"`
	SpamIndicators []SpamIndicator `json:"spam_indicators" dynamodbav:"SpamIndicators"`

	// Behavioral Analysis
	PostingVelocity float64 `json:"posting_velocity" dynamodbav:"PostingVelocity"`
	RepetitionScore float64 `json:"repetition_score" dynamodbav:"RepetitionScore"`
	LinkDensity     float64 `json:"link_density" dynamodbav:"LinkDensity"`

	// Network Analysis
	FollowerRatio   float64 `json:"follower_ratio" dynamodbav:"FollowerRatio"`
	InteractionRate float64 `json:"interaction_rate" dynamodbav:"InteractionRate"`
	AccountAge      int     `json:"account_age_days" dynamodbav:"AccountAgeDays"`
}

// Supporting types
type ContentCategory struct {
	Name  string  `json:"name" dynamodbav:"Name"`
	Score float64 `json:"score" dynamodbav:"Score"`
}

type PIIEntity struct {
	Type        string  `json:"type" dynamodbav:"Type"`
	Text        string  `json:"text" dynamodbav:"Text"`
	Score       float64 `json:"score" dynamodbav:"Score"`
	BeginOffset int     `json:"begin_offset" dynamodbav:"BeginOffset"`
	EndOffset   int     `json:"end_offset" dynamodbav:"EndOffset"`
}

type Entity struct {
	Type  string  `json:"type" dynamodbav:"Type"`
	Text  string  `json:"text" dynamodbav:"Text"`
	Score float64 `json:"score" dynamodbav:"Score"`
}

type ModerationLabel struct {
	Name       string  `json:"name" dynamodbav:"Name"`
	Confidence float64 `json:"confidence" dynamodbav:"Confidence"`
	ParentName string  `json:"parent_name,omitempty" dynamodbav:"ParentName,omitempty"`
}

type Celebrity struct {
	Name       string   `json:"name" dynamodbav:"Name"`
	Confidence float64  `json:"confidence" dynamodbav:"Confidence"`
	URLs       []string `json:"urls,omitempty" dynamodbav:"URLs,omitempty"`
}

type Logo struct {
	Name       string  `json:"name" dynamodbav:"Name"`
	Confidence float64 `json:"confidence" dynamodbav:"Confidence"`
}

type SpamIndicator struct {
	Type        string  `json:"type" dynamodbav:"Type"`
	Description string  `json:"description" dynamodbav:"Description"`
	Severity    float64 `json:"severity" dynamodbav:"Severity"`
}

// Content represents content to be analyzed
type Content struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Text         string     `json:"text"`
	MediaURLs    []string   `json:"media_urls"`
	AuthorID     string     `json:"author_id"`
	CreatedAt    time.Time  `json:"created_at"`
	LastAnalyzed *time.Time `json:"last_analyzed,omitempty"`
}

// ModerationAction types
const (
	ActionNone      = "none"
	ActionFlag      = "flag"
	ActionHide      = "hide"
	ActionRemove    = "remove"
	ActionShadowBan = "shadow_ban"
	ActionReview    = "review"
)

// Sentiment types
const (
	SentimentPositive = "POSITIVE"
	SentimentNegative = "NEGATIVE"
	SentimentNeutral  = "NEUTRAL"
	SentimentMixed    = "MIXED"
)

// Entity types
const (
	EntityPerson       = "PERSON"
	EntityLocation     = "LOCATION"
	EntityOrganization = "ORGANIZATION"
	EntityCommercial   = "COMMERCIAL_ITEM"
	EntityEvent        = "EVENT"
	EntityDate         = "DATE"
	EntityQuantity     = "QUANTITY"
	EntityTitle        = "TITLE"
	EntityOther        = "OTHER"
)

// PII types
const (
	PiiEmail         = "EMAIL"
	PiiPhone         = "PHONE"
	PiiAddress       = "ADDRESS"
	PiiSsn           = "SSN"
	PiiCreditCard    = "CREDIT_CARD"
	PiiBankAccount   = "BANK_ACCOUNT"
	PiiDriverLicense = "DRIVER_LICENSE"
	PiiPassport      = "PASSPORT"
	PiiName          = "NAME"
	PiiAge           = "AGE"
)
