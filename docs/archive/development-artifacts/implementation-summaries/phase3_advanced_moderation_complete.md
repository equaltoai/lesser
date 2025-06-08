# Lesser ActivityPub Implementation - Phase 3: Advanced Moderation Engine

## Overview

We've successfully implemented a sophisticated ML-powered moderation engine for Lesser that leverages AWS AI services (Comprehend and Rekognition) to provide comprehensive content moderation capabilities.

## Architecture

```
                       ┌─────────────────────────┐
                       │   Moderation Engine     │
                       │      (Main Entry)       │
                       └──────────┬──────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────┐
        │                         │                         │
        ▼                         ▼                         ▼
┌───────────────┐       ┌───────────────┐       ┌───────────────┐
│ Text Analyzer │       │ Image Analyzer │       │ Video Analyzer │
│ (AWS Comprehend)      │ (AWS Rekognition)     │ (Future)       │
└───────┬───────┘       └───────┬───────┘       └───────────────┘
        │                       │
        └───────────┬───────────┘
                    │
        ┌───────────┴───────────┬─────────────┬─────────────┐
        ▼                       ▼             ▼             ▼
┌───────────────┐     ┌───────────────┐ ┌─────────────┐ ┌──────────────┐
│Pattern Matcher│     │  Reputation   │ │   Threat    │ │   Decision   │
│ (Rule Engine) │     │    Scorer     │ │Intelligence │ │   Engine     │
└───────────────┘     └───────────────┘ └─────────────┘ └──────────────┘
                                                                │
                                                                ▼
                                                        ┌──────────────┐
                                                        │   Metrics    │
                                                        │  Tracking    │
                                                        └──────────────┘
```

## Components Implemented

### 1. **Core Types & Interfaces** (`types.go`)
- Comprehensive type system for content analysis
- Moderation actions (Allow, Flag, Quarantine, Remove, ShadowBan)
- Severity levels (Low, Medium, High, Critical)
- Analysis results for text, images, and video
- Configuration management

### 2. **Text Analyzer** (`text_analyzer.go`)
Integrates with AWS Comprehend for:
- **Sentiment Analysis**: Detects positive/negative/neutral sentiment
- **Toxicity Detection**: Identifies hate speech, profanity, threats
- **PII Detection**: Finds personally identifiable information
- **Entity Recognition**: Extracts people, organizations, locations
- **Threat Detection**: Identifies violence, self-harm, doxxing
- **Language Detection**: Supports multiple languages

### 3. **Image Analyzer** (`image_analyzer.go`)
Integrates with AWS Rekognition for:
- **Explicit Content Detection**: Nudity, suggestive content
- **Violence Detection**: Weapons, blood, violent scenes
- **Text Extraction**: OCR for text in images
- **Object Detection**: Identifies objects and scenes
- **Face Analysis**: Emotions, age range, gender
- **Celebrity Recognition**: Public figure identification

### 4. **Pattern Matcher** (`pattern_matcher.go`)
Rule-based content matching:
- **Regex Patterns**: Complex pattern matching
- **Keyword Matching**: Simple keyword detection
- **Phrase Detection**: Multi-word phrase matching
- **Dynamic Updates**: Hot-reload patterns without restart
- **Hit Tracking**: Analytics on pattern effectiveness
- **DynamoDB Storage**: Scalable pattern management

### 5. **Reputation Scorer** (`reputation.go`)
User behavior tracking:
- **Score Calculation**: 0-100 reputation score
- **Level Assignment**: trusted/normal/suspicious/bad_actor
- **Event Tracking**: Violations, false positives, good content
- **Decay System**: Scores trend toward neutral over time
- **Impact Calculation**: Different events have different weights
- **History Tracking**: Complete audit trail

### 6. **Threat Intelligence** (`threat_intel.go`)
Cross-instance threat sharing:
- **Threat Sharing**: Share identified threats with network
- **Indicator Matching**: Hashes, patterns, domains
- **Confidence Scoring**: Track threat reliability
- **TTL Management**: Automatic threat expiration
- **Hit Tracking**: Monitor threat effectiveness
- **Real-time Updates**: 15-minute refresh cycle

### 7. **Decision Engine** (`decision_engine.go`)
Intelligent decision making:
- **Signal Aggregation**: Combines all analysis results
- **Weighted Scoring**: Different signals have different importance
- **Reputation Factoring**: User history affects decisions
- **Confidence Calculation**: Measures decision certainty
- **Review Requirements**: Flags content for human review
- **Recommendation Generation**: Actionable next steps

### 8. **Main Engine** (`engine.go`)
Orchestrates all components:
- **Content Analysis**: Text, image, and video processing
- **Pattern Management**: CRUD operations for patterns
- **Reputation Management**: User score tracking
- **Threat Intelligence**: Threat sharing and checking
- **Decision Execution**: Applies moderation decisions
- **Batch Processing**: Analyze multiple items efficiently

### 9. **Metrics System** (`metrics.go`)
Performance tracking:
- **Real-time Metrics**: Current processing rates
- **Action Distribution**: Track decision types
- **Response Times**: P50/P95 latency tracking
- **False Positive Rate**: Accuracy monitoring
- **Historical Analysis**: Time-based reporting
- **Automatic Flushing**: 5-minute aggregation

## Key Features

### 1. **Multi-Layer Analysis**
- Text analysis with AWS Comprehend
- Image analysis with AWS Rekognition  
- Pattern-based rule matching
- Reputation-based adjustments
- Threat intelligence checking

### 2. **Intelligent Decision Making**
- Weighted signal aggregation
- Confidence-based actions
- Context-aware decisions
- Severity-based prioritization

### 3. **Performance Optimizations**
- In-memory caching for patterns and threats
- Parallel analysis execution
- Batch processing support
- Efficient DynamoDB queries with GSIs

### 4. **Cost Management**
- Request tracking for AWS services
- Configurable analysis features
- Smart caching to reduce API calls
- Monthly spend limits

### 5. **Human-in-the-Loop**
- Review queue for uncertain decisions
- Priority-based review ordering
- False positive reporting
- Decision appeals process

## Configuration Example

```go
config := &ModerationConfig{
    // Thresholds
    ToxicityThreshold:      0.7,
    ExplicitThreshold:      0.8,
    ViolenceThreshold:      0.75,
    ConfidenceThreshold:    0.6,
    
    // Actions
    AutoRemoveThreshold:    0.9,
    QuarantineThreshold:    0.7,
    FlagThreshold:          0.5,
    
    // Reputation
    ReputationDecayRate:    0.02,
    BadActorThreshold:      20.0,
    TrustedActorThreshold:  80.0,
    
    // Features
    EnableTextAnalysis:     true,
    EnableImageAnalysis:    true,
    EnablePatternMatching:  true,
    EnableReputationScoring: true,
    EnableThreatSharing:    true,
    
    // AWS
    ComprehendRegion:       "us-east-1",
    RekognitionRegion:      "us-east-1",
    S3Bucket:               "lesser-media",
    
    // Cost controls
    MaxMonthlySpend:        1000.0,
    EnableCostTracking:     true,
}
```

## Usage Example

```go
// Initialize engine
engine := NewEngine(
    config,
    comprehendClient,
    rekognitionClient,
    dynamoClient,
    "moderation-table",
    logger,
    costTracker,
)

// Analyze text content
analysis, err := engine.AnalyzeContent(
    "This is the content to analyze",
    ContentMetadata{
        ContentID:    "post-123",
        AuthorID:     "user-456",
        ContentType:  ContentTypeText,
        Context:      "post",
        Timestamp:    time.Now(),
    },
)

// Create moderation pattern
pattern := &ModerationPattern{
    Name:        "Spam URLs",
    Description: "Detects spammy URL patterns",
    Pattern:     `https?://[a-z0-9-]+\.(tk|ml|ga|cf)`,
    PatternType: "regex",
    Severity:    SeverityMedium,
    Action:      ActionQuarantine,
    Categories:  []string{"spam"},
    Active:      true,
}
engine.CreatePattern(pattern)

// Share threat intelligence
threat := &ThreatIntel{
    ThreatType:  "phishing",
    Indicators:  []string{"malicious-domain.com", "phishing-pattern"},
    Severity:    SeverityHigh,
    Description: "Phishing campaign targeting users",
    Confidence:  0.85,
    TTL:         7 * 24 * time.Hour,
}
engine.ShareThreat(threat)
```

## Performance Metrics

### Target Performance
- **Analysis Latency**: <500ms for text, <1s for images
- **Decision Latency**: <50ms after analysis
- **Pattern Matching**: <10ms for 1000 patterns
- **Threat Lookup**: <5ms with caching
- **Batch Processing**: 100+ items/second

### Scalability
- Horizontal scaling via stateless design
- DynamoDB auto-scaling for storage
- In-memory caching for hot data
- Async processing for non-critical paths

## Security Considerations

1. **PII Protection**: Automatic detection and optional redaction
2. **Audit Trail**: Complete decision history with TTL
3. **Access Control**: IAM-based AWS service access
4. **Data Retention**: Configurable TTL for all data
5. **Threat Sharing**: Domain-scoped threat visibility

## Cost Optimization

1. **Caching**: Reduces redundant API calls
2. **Batch Processing**: Efficient API usage
3. **Feature Toggles**: Disable expensive features
4. **Cost Tracking**: Real-time spend monitoring
5. **Tiered Analysis**: Light checks before heavy

## Future Enhancements

1. **Video Analysis**: Frame sampling and audio transcription
2. **Custom ML Models**: Train on platform-specific content
3. **Real-time Streaming**: Process live video/audio
4. **Multi-language**: Expand beyond AWS Comprehend languages
5. **Federation**: Cross-instance reputation sharing

## Summary

The Advanced Moderation Engine provides Lesser with enterprise-grade content moderation capabilities that can:
- Protect users from harmful content
- Maintain community standards
- Reduce moderator workload
- Provide detailed analytics
- Scale with platform growth

The system is designed to be accurate, fast, and cost-effective while maintaining flexibility for different use cases and community standards. 