# Streaming Metrics Implementation

## Overview

This implementation replaces placeholder streaming metrics with comprehensive real-time tracking using CloudWatch. The system captures actual rebuffer events, quality switches, and Quality of Experience (QoE) scores to enable data-driven adaptive bitrate decisions.

## Key Components

### 1. StreamingMetricsTracker (`pkg/media/streaming/metrics.go`)

A comprehensive metrics tracking system that:

- **Real-time Session Tracking**: Monitors streaming sessions with detailed metrics
- **CloudWatch Integration**: Batch publishes metrics to reduce costs
- **QoE Calculation**: Computes Quality of Experience scores based on multiple factors
- **Buffer Health Monitoring**: Tracks buffer levels for adaptive bitrate decisions
- **Error Handling**: Graceful handling of CloudWatch API failures

**Key Features:**
- Batch publishing (max 20 metrics per request) to optimize CloudWatch costs
- Automatic session cleanup to prevent memory leaks
- QoE scoring based on quality, rebuffering, and switching frequency
- Background batch publisher for efficient metric delivery

### 2. Enhanced Quality Selector (`pkg/media/streaming/quality.go`)

Upgraded adaptive bitrate selection using real session metrics:

- **Historical Performance**: Uses past rebuffer events to inform decisions
- **Quality Stability**: Reduces thrashing by favoring current quality when appropriate  
- **QoE-based Adjustments**: Modifies buffer thresholds based on quality scores
- **Session-aware Selection**: Enhanced `SelectQualityWithSession()` method

**Decision Factors:**
- Bandwidth availability and stability
- Recent rebuffer history (last 60 seconds)
- Quality switch frequency (avoids >3 switches/minute)
- Segment success rates
- Buffer health trends

### 3. Updated Streamer (`pkg/media/streaming/streamer.go`)

Modified to integrate real metrics tracking:

- **Constructor Enhancement**: Now accepts CloudWatch client parameter
- **Real Metrics Capture**: Replaces placeholder `UpdateMetrics(sessionID, 0, 0)` calls
- **Quality Switch Detection**: Automatically detects and tracks quality changes
- **Buffer Health Integration**: Uses real buffer calculations for rebuffer detection
- **Session Lifecycle**: Full metrics tracking from start to end

### 4. Enhanced Analytics Service (`pkg/media/streaming.go`)

Updated CloudWatch queries for real streaming analytics:

- **Concurrent Queries**: Parallel CloudWatch metric retrieval for performance
- **Real Metrics**: Session counts, rebuffer events, quality distribution
- **Error Resilience**: Graceful fallback to estimates if queries fail
- **Quality Breakdown**: Per-quality usage time tracking

## Metrics Tracked

### Session-Level Metrics

| Metric | Unit | Purpose |
|--------|------|---------|
| SessionDuration | Seconds | Total streaming time |
| QualitySwitches | Count | Number of quality changes |
| RebufferEvents | Count | Number of stall events |
| RebufferRatio | Percent | Percentage of time spent rebuffering |
| QoEScore | 0-100 | Quality of Experience score |
| SegmentSuccessRate | Percent | Successful segment loads |
| BytesTransferred | Bytes | Total data transferred |

### Real-time Events

| Event | Dimensions | Data |
|-------|------------|------|
| session_start | MediaID, UserID | Session initialization |
| quality_switch | MediaID, Quality, PreviousQuality | Bitrate changes |
| rebuffer_start | MediaID, Quality | Stall events with duration |
| segment_load | MediaID, Quality | Successful loads (sampled) |
| segment_fail | MediaID, ErrorCode | Failed loads with reason |
| session_end | MediaID, UserID | Final session metrics |

### Quality Distribution

- **QualityTime**: Time spent in each quality level
- **Per-quality metrics**: Segmented by 240p, 360p, 480p, 720p, 1080p, 4K

## CloudWatch Integration

### Namespace Structure
```
Lesser/Streaming/
├── StreamingEvent (by EventType, MediaID)
├── SessionDuration (by MediaID)
├── QualitySwitches (by MediaID) 
├── RebufferEvents (by MediaID)
├── QualityTime (by MediaID, Quality)
└── BytesTransferred (by MediaID)
```

### Cost Optimization
- **Batch Publishing**: Groups up to 20 metrics per API call
- **Sampling**: Segment events sampled at 1/10 rate
- **Timeout Handling**: 30-second timeout with retry logic
- **Efficient Dimensions**: Minimal dimension set for cost control

## Quality of Experience (QoE) Calculation

The QoE score (0.0-1.0) considers:

```go
qoeScore := qualityScore - rebufferPenalty - switchPenalty - failurePenalty
```

**Components:**
- **Quality Score**: Weighted by time spent in each quality level
- **Rebuffer Penalty**: Heavy penalty (3x ratio) for stalling
- **Switch Penalty**: 2% penalty per quality change
- **Failure Penalty**: Up to 50% penalty for segment failures

**Quality Weights:**
- 4K: 1.0, 1080p: 0.9, 720p: 0.7, 480p: 0.5, 360p: 0.3, 240p: 0.2

## Adaptive Bitrate Enhancement

The enhanced quality selector uses metrics for smarter decisions:

### Panic Mode
- Buffer < 0.5 seconds → Drop to lowest sustainable quality
- Uses 50% of available bandwidth for stability

### Conservative Mode  
- Buffer < 5 seconds → Step down one quality level
- Recent rebuffers → 50% buffer health reduction

### Optimal Mode
- Healthy buffer → ML-like scoring algorithm
- Stability bonus for current quality
- Historical performance penalties

### Anti-thrashing Logic
- >3 switches/minute → Lock to current quality if sustainable
- QoE-based buffer adjustments (±20%)
- Success rate penalties for problematic qualities

## Usage Example

```go
// Create metrics tracker
metricsTracker := streaming.NewStreamingMetricsTracker(cloudWatchClient, logger)

// Track session
metricsTracker.StartSession(sessionID, userID, mediaID)

// Track events
metricsTracker.TrackQualitySwitch(sessionID, oldQuality, newQuality)
metricsTracker.TrackRebufferEvent(sessionID, duration)
metricsTracker.TrackSegmentLoad(sessionID, index, loadTime, bytes)

// Enhanced quality selection
qualitySelector.SetMetricsTracker(metricsTracker)
quality := qualitySelector.SelectQualityWithSession(sessionID, bandwidth, bufferHealth, qualities)

// End session
finalMetrics := metricsTracker.EndSession(sessionID)
```

## Performance Impact

### Overhead Budget
- **CPU**: <2% overhead per stream
- **Memory**: ~10MB for metrics tracking
- **Network**: <0.1% of stream bandwidth for metrics
- **Latency**: <1ms added latency per segment

### Efficiency Measures
- Lock-free ring buffers for metric collection
- Background batch processing
- Automatic cleanup of old session data
- Graceful degradation on CloudWatch failures

## Benefits Achieved

1. **Real Metrics**: Eliminated placeholder zeros with actual rebuffer/switch tracking
2. **Better QoE**: Comprehensive quality scoring drives better user experience  
3. **Cost Efficiency**: Batch publishing reduces CloudWatch costs
4. **Smart ABR**: Historical performance informs quality decisions
5. **Operational Visibility**: Rich dashboards from comprehensive metrics
6. **Error Resilience**: Graceful handling of CloudWatch API issues

## Migration Notes

### Constructor Changes
The `NewStreamer` constructor now requires a CloudWatch client:

```go
// Before
streamer := NewStreamer(config, analytics, s3Client, db, logger, costTracker)

// After  
streamer := NewStreamer(config, analytics, s3Client, cloudWatch, db, logger, costTracker)
```

### Backwards Compatibility
- Original `SelectQuality()` method preserved
- Existing quality selector interface unchanged
- Graceful fallback when metrics unavailable

## Monitoring and Alerts

### Recommended CloudWatch Alarms
- **Rebuffer Ratio** > 2% → Alert
- **QoE Score** < 30 → Warning  
- **Segment Failure Rate** > 1% → Alert
- **Quality Switches** > 5/minute → Investigation

### Dashboard Metrics
- Real-time concurrent streams
- Quality distribution over time
- Rebuffer events by geography
- QoE trends and correlations

This implementation provides the foundation for data-driven streaming optimization and comprehensive operational monitoring.