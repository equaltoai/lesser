package advanced

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rekognitionTypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type stubAWSTransport struct {
	mu sync.Mutex

	s3Objects map[string][]byte

	failS3Get    bool
	failS3Put    bool
	failS3Delete bool

	failDynamoGetItem bool
	failDynamoPutItem bool
}

func newStubAWSTransport() *stubAWSTransport {
	return &stubAWSTransport{
		s3Objects: make(map[string][]byte),
	}
}

func (t *stubAWSTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Header.Get("X-Amz-Target")
	switch {
	case strings.HasPrefix(target, "RekognitionService."):
		return t.roundTripRekognition(target)
	case strings.HasPrefix(target, "DynamoDB_20120810."):
		return t.roundTripDynamoDB(target)
	default:
		// Assume S3.
		return t.roundTripS3(req)
	}
}

func (t *stubAWSTransport) roundTripRekognition(target string) (*http.Response, error) {
	op := strings.TrimPrefix(target, "RekognitionService.")
	switch op {
	case "StartContentModeration", "StartTextDetection", "StartFaceDetection", "StartLabelDetection":
		return jsonResponse(http.StatusOK, map[string]any{"JobId": "job-1"})
	case "GetContentModeration", "GetTextDetection", "GetFaceDetection", "GetLabelDetection":
		return jsonResponse(http.StatusOK, map[string]any{"JobStatus": "SUCCEEDED"})
	case "DetectModerationLabels", "DetectText", "DetectFaces", "DetectLabels":
		// ImageAnalyzer uses these and is tolerant of partial failures.
		return jsonResponse(http.StatusOK, map[string]any{})
	default:
		return jsonResponse(http.StatusBadRequest, map[string]any{"Message": "unsupported operation"})
	}
}

func (t *stubAWSTransport) roundTripDynamoDB(target string) (*http.Response, error) {
	op := strings.TrimPrefix(target, "DynamoDB_20120810.")
	switch op {
	case "GetItem":
		if t.failDynamoGetItem {
			return jsonResponse(http.StatusInternalServerError, map[string]any{"Message": "GetItem failed"})
		}
		// Return no Item so reputation scorer creates default score.
		return jsonResponse(http.StatusOK, map[string]any{})
	case "PutItem":
		if t.failDynamoPutItem {
			return jsonResponse(http.StatusInternalServerError, map[string]any{"Message": "PutItem failed"})
		}
		return jsonResponse(http.StatusOK, map[string]any{})
	default:
		return jsonResponse(http.StatusBadRequest, map[string]any{"Message": "unsupported operation"})
	}
}

func (t *stubAWSTransport) roundTripS3(req *http.Request) (*http.Response, error) {
	bucket, key := parseBucketKey(req)
	objectKey := fmt.Sprintf("%s/%s", bucket, key)

	switch req.Method {
	case http.MethodGet:
		if t.failS3Get {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("GetObject failed")),
				Request:    req,
			}, nil
		}

		t.mu.Lock()
		data := t.s3Objects[objectKey]
		t.mu.Unlock()

		// Return a small payload so transfermanager.DownloadObject uses a single request.
		if data == nil {
			data = []byte("stub")
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(data)),
			Request:    req,
		}
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
		resp.Header.Set("Content-Type", "application/octet-stream")
		return resp, nil
	case http.MethodPut:
		if t.failS3Put {
			_ = req.Body.Close()
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("PutObject failed")),
				Request:    req,
			}, nil
		}

		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()

		t.mu.Lock()
		t.s3Objects[objectKey] = body
		t.mu.Unlock()

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	case http.MethodDelete:
		if t.failS3Delete {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("DeleteObject failed")),
				Request:    req,
			}, nil
		}

		t.mu.Lock()
		delete(t.s3Objects, objectKey)
		t.mu.Unlock()

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("unsupported")),
			Request:    req,
		}, nil
	}
}

func parseBucketKey(req *http.Request) (bucket, key string) {
	host := req.URL.Hostname()
	path := strings.TrimPrefix(req.URL.Path, "/")

	// Path-style: /bucket/key
	if parts := strings.SplitN(path, "/", 2); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1]
	}

	// Virtual-host style: bucket.s3...
	if idx := strings.Index(host, ".s3"); idx > 0 {
		bucket = host[:idx]
		key = path
		return bucket, key
	}

	return "test-bucket", path
}

func jsonResponse(status int, payload map[string]any) (*http.Response, error) {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type":   []string{"application/x-amz-json-1.1"},
			"Content-Length": []string{fmt.Sprintf("%d", len(data))},
		},
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func awsConfigForStub(transport http.RoundTripper) aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
		HTTPClient:  &http.Client{Transport: transport},
	}
}

func setupPermissiveDynamormMocks(db *mocks.MockDB, q *mocks.MockQuery, ub *mocks.MockUpdateBuilder) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()
	q.On("Cursor", mock.Anything).Return(q).Maybe()

	q.On("All", mock.Anything).Return(nil).Maybe()
	q.On("First", mock.Anything).Return(nil).Maybe()
	q.On("Create").Return(nil).Maybe()
	q.On("Update").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()
	q.On("Delete", mock.Anything).Return(nil).Maybe()

	q.On("UpdateBuilder").Return(ub).Maybe()
	ub.On("Set", mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Remove", mock.Anything).Return(ub).Maybe()
	ub.On("Add", mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(ub).Maybe()
	ub.On("Execute").Return(nil).Maybe()
}

type stubPatternRepo struct{}

func (stubPatternRepo) CreatePattern(context.Context, *ModerationPattern) error         { return nil }
func (stubPatternRepo) UpdatePattern(context.Context, string, *ModerationPattern) error { return nil }
func (stubPatternRepo) DeletePattern(context.Context, string) error                     { return nil }
func (stubPatternRepo) GetPattern(_ context.Context, patternID string) (*ModerationPattern, error) {
	return &ModerationPattern{
		ID:      patternID,
		Name:    "p",
		Pattern: "x",
		Type:    patternTypeKeyword,
		Active:  true,
	}, nil
}
func (stubPatternRepo) GetPatterns(context.Context, PatternFilter) ([]*ModerationPattern, error) {
	return nil, nil
}
func (stubPatternRepo) IncrementHitCount(context.Context, string) error { return nil }
func (stubPatternRepo) LoadActivePatterns(context.Context) ([]*ModerationPattern, error) {
	return nil, nil
}

type stubThreatRepo struct{}

func (stubThreatRepo) ShareThreat(context.Context, *repositories.ThreatIntel) error { return nil }
func (stubThreatRepo) GetSharedThreats(context.Context, time.Time) ([]*repositories.ThreatIntel, error) {
	return nil, nil
}
func (stubThreatRepo) GetThreatsByType(context.Context, string, int) ([]*repositories.ThreatIntel, error) {
	return nil, nil
}
func (stubThreatRepo) UpdateThreatConfidence(context.Context, string, float64) error { return nil }
func (stubThreatRepo) IncrementHitCount(context.Context, string) error               { return nil }
func (stubThreatRepo) LoadActiveThreats(context.Context) ([]*repositories.ThreatIntel, error) {
	return nil, nil
}
func (stubThreatRepo) GetThreatByID(context.Context, string) (*repositories.ThreatIntel, error) {
	return nil, nil
}
func (stubThreatRepo) GetIndicatorThreat(context.Context, string) (string, error) { return "", nil }

type stubTextAnalyzer struct {
	analysis *ContentAnalysis
	err      error
}

func (s stubTextAnalyzer) AnalyzeText(context.Context, string, ContentMetadata) (*ContentAnalysis, error) {
	return s.analysis, s.err
}

type textAnalyzerFunc func(ctx context.Context, text string, metadata ContentMetadata) (*ContentAnalysis, error)

func (f textAnalyzerFunc) AnalyzeText(ctx context.Context, text string, metadata ContentMetadata) (*ContentAnalysis, error) {
	return f(ctx, text, metadata)
}

type stubImageAnalyzer struct {
	analysis *ImageAnalysis
	err      error
}

func (s stubImageAnalyzer) AnalyzeImage(context.Context, string, ContentMetadata) (*ImageAnalysis, error) {
	return s.analysis, s.err
}

type fakeCostTracker struct {
	mu       sync.Mutex
	requests []string
}

func (f *fakeCostTracker) TrackComprehendRequest(operation string, units int) {}
func (f *fakeCostTracker) TrackTranscribeRequest(jobName string, estimatedMinutes int) {
}
func (f *fakeCostTracker) TrackRekognitionRequest(operation string, imageCount int) {
	f.mu.Lock()
	f.requests = append(f.requests, fmt.Sprintf("%s:%d", operation, imageCount))
	f.mu.Unlock()
}

func TestNewEngine_AnalyzerSelection_RespectsGlobalFlags(t *testing.T) {
	logger := zap.NewNop()
	cfg := &ModerationConfig{
		EnableTextAnalysis:  true,
		EnableImageAnalysis: true,
		S3Bucket:            "test-bucket",
		ConfidenceThreshold: 0.6,
		ViolenceThreshold:   0.8,
	}

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	global := appconfig.Get()
	origAWS := global.DisableAWSModeration
	origComp := global.DisableComprehend
	origRek := global.DisableRekognition
	t.Cleanup(func() {
		global.DisableAWSModeration = origAWS
		global.DisableComprehend = origComp
		global.DisableRekognition = origRek
	})

	transport := newStubAWSTransport()
	awsCfg := awsConfigForStub(transport)

	rekClient := rekognition.NewFromConfig(awsCfg)

	// AWS moderation disabled -> no-op analyzers.
	global.DisableAWSModeration = true
	engine := NewEngine(cfg, nil, rekClient, "test-table", stubPatternRepo{}, logger, nil, mockDB)
	_, ok := engine.imageAnalyzer.(*NoOpImageAnalyzer)
	assert.True(t, ok)

	// Rekognition disabled -> no-op image analyzer even when AWS is enabled.
	global.DisableAWSModeration = false
	global.DisableRekognition = true
	engine = NewEngine(cfg, nil, rekClient, "test-table", stubPatternRepo{}, logger, nil, mockDB)
	_, ok = engine.imageAnalyzer.(*NoOpImageAnalyzer)
	assert.True(t, ok)

	// Rekognition enabled and config enabled -> AWS analyzer.
	global.DisableRekognition = false
	engine = NewEngine(cfg, nil, rekClient, "test-table", stubPatternRepo{}, logger, nil, mockDB)
	_, ok = engine.imageAnalyzer.(*ImageAnalyzer)
	assert.True(t, ok)
}

func TestEngine_storeAnalysisResult_BuildsAllSections(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	engine := &Engine{
		config:      &ModerationConfig{ConfidenceThreshold: 0.6, ViolenceThreshold: 0.8},
		logger:      logger,
		dynamoRM:    mockDB,
		tableName:   "test-table",
		costTracker: nil,
	}

	now := time.Now()
	analysis := &ModerationAnalysis{
		ContentMetadata: ContentMetadata{
			ContentID:   "c1",
			AuthorID:    "alice",
			ContentType: ContentTypeText,
		},
		TextAnalysis: &ContentAnalysis{
			ContentID:  "c1",
			Sentiment:  SentimentAnalysis{Sentiment: "NEUTRAL", Confidence: 0.9},
			Toxicity:   ToxicityAnalysis{IsToxic: false, ToxicityScore: 0.1, Confidence: 0.9},
			PII:        []PIIEntity{{Type: "PHONE", Text: "+1-555-0100", BeginIndex: 0, EndIndex: 11, Confidence: 0.9}},
			Topics:     []Topic{{Name: "t", Score: 0.8, Category: "test"}},
			Language:   LanguageDetection{LanguageCode: "en", Confidence: 0.9},
			Threats:    []ThreatIndicator{{Type: "NONE", Severity: SeverityLow, Confidence: 0.1}},
			AnalyzedAt: now,
		},
		ImageAnalysis: &ImageAnalysis{
			ImageURL:   "s3://test-bucket/img.jpg",
			Explicit:   ExplicitContent{IsExplicit: false, Confidence: 0.2},
			Violence:   ViolenceDetection{HasViolence: false, Confidence: 0.1},
			Text:       []TextInImage{{Text: "hi", Confidence: 0.9}},
			Objects:    []ObjectDetection{{Name: "Object", Confidence: 0.9}},
			Faces:      []FaceAnalysis{{Confidence: 0.9}},
			AnalyzedAt: now,
		},
		VideoAnalysis: &VideoAnalysis{
			VideoURL:   "s3://test-bucket/video.mp4",
			Frames:     []FrameAnalysis{{Timestamp: time.Second}},
			Audio:      AudioAnalysis{Transcription: "hello", Language: "en"},
			Duration:   5 * time.Second,
			AnalyzedAt: now,
		},
		PatternMatches: []PatternMatch{{PatternID: "p1", PatternName: "pat", MatchText: "x", Location: "body", Confidence: 0.9}},
		ThreatMatches:  []ThreatMatch{{ThreatID: "t1", ThreatType: "spam", Indicator: "x", Confidence: 0.9}},
		ReputationScore: &ReputationScore{
			ActorID:            "alice",
			Score:              50,
			Level:              "normal",
			ViolationCount:     1,
			FalsePositiveCount: 0,
			ContentCount:       2,
			LastViolation:      now,
			Factors:            []ReputationFactor{{Factor: "a", Impact: 1, Description: "test"}},
			UpdatedAt:          now,
		},
	}
	decision := &ModerationDecision{Decision: ActionAllow, Confidence: 0.9}

	require.NoError(t, engine.storeAnalysisResult(ctx, analysis, decision))

	// Exercise "text"/"image"/"video" analysis type selection branches.
	textOnly := &ModerationAnalysis{ContentMetadata: analysis.ContentMetadata, TextAnalysis: analysis.TextAnalysis}
	require.NoError(t, engine.storeAnalysisResult(ctx, textOnly, decision))
	imageOnly := &ModerationAnalysis{ContentMetadata: analysis.ContentMetadata, ImageAnalysis: analysis.ImageAnalysis}
	require.NoError(t, engine.storeAnalysisResult(ctx, imageOnly, decision))
	videoOnly := &ModerationAnalysis{ContentMetadata: analysis.ContentMetadata, VideoAnalysis: analysis.VideoAnalysis}
	require.NoError(t, engine.storeAnalysisResult(ctx, videoOnly, decision))
}

func TestEngine_storeDecision_EnforcementBranches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	engine := &Engine{
		logger:    logger,
		dynamoRM:  mockDB,
		tableName: "test-table",
	}

	metadata := ContentMetadata{
		ContentID:    "c1",
		AuthorID:     "alice",
		ContentType:  ContentTypeText,
		AuthorDomain: "example.com",
		Language:     "en",
		Context:      "test",
		Mentions:     []string{"bob"},
		Hashtags:     []string{"tag"},
		URLs:         []string{"https://example.com"},
		Timestamp:    time.Now(),
	}

	now := time.Now()
	decision := &ModerationDecision{
		ContentID:       "c1",
		Decision:        ActionAllow,
		Confidence:      0.9,
		Reasons:         []DecisionReason{{Type: "t", Severity: SeverityLow, Description: "d"}},
		Recommendations: []string{"rec"},
		DecidedAt:       now,
	}

	require.NoError(t, engine.storeDecision(ctx, decision, metadata))

	decision.Decision = ActionFlag
	decision.RequiresReview = true
	decision.ReviewPriority = 9
	decision.ExpiresAt = time.Now().Add(time.Hour)
	require.NoError(t, engine.storeDecision(ctx, decision, metadata))

	decision.Decision = ActionQuarantine
	decision.ReviewPriority = 6
	require.NoError(t, engine.storeDecision(ctx, decision, metadata))

	decision.Decision = ActionRemove
	decision.ReviewPriority = 3
	require.NoError(t, engine.storeDecision(ctx, decision, metadata))
}

func TestVideoAnalyzer_FrameIOHelpers(t *testing.T) {
	logger := zap.NewNop()

	transport := newStubAWSTransport()
	transport.s3Objects["test-bucket/video.mp4"] = []byte("tiny-video")
	awsCfg := awsConfigForStub(transport)

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })
	transferClient := transfermanager.New(s3Client, func(o *transfermanager.Options) {
		o.PartSizeBytes = 10 * 1024 * 1024
		o.Concurrency = 1
		o.MultipartUploadThreshold = 1024 * 1024 * 1024
	})

	va := &VideoAnalyzer{
		logger:     logger,
		config:     &ModerationConfig{S3Bucket: "test-bucket", ConfidenceThreshold: 0.6, ViolenceThreshold: 0.8},
		s3Client:   s3Client,
		s3Transfer: transferClient,
		bucketName: "test-bucket",
		cacheTTL:   time.Minute,
		imageAnalyzer: NewImageAnalyzer(rekognition.NewFromConfig(awsCfg), logger, &ModerationConfig{
			S3Bucket:            "test-bucket",
			EnableImageAnalysis: false,
			ConfidenceThreshold: 0.6,
			ViolenceThreshold:   0.8,
		}, nil),
	}

	t.Run("download video", func(t *testing.T) {
		path, err := va.downloadVideoFromS3(context.Background(), "test-bucket", "video.mp4")
		require.NoError(t, err)
		t.Cleanup(func() { os.Remove(path) })
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("upload and cleanup frame", func(t *testing.T) {
		framePath := filepath.Join(t.TempDir(), "frame.jpg")
		require.NoError(t, os.WriteFile(framePath, []byte("frame-bytes"), 0o600))

		frameURL, err := va.uploadFrameToS3(context.Background(), framePath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(frameURL, "s3://"))

		// Cleanup should not panic.
		va.cleanupTemporaryFrame(context.Background(), frameURL)
	})
}

func TestEngine_GetFalsePositiveRate_Branches(t *testing.T) {
	engine := &Engine{
		metrics: &ModerationMetrics{
			repo: stubMetricsRepo{
				stats: &models.ModerationMetricsStats{
					FalsePositives: 2,
					TruePositives:  6,
				},
			},
			logger: zap.NewNop(),
		},
	}

	rate, err := engine.GetFalsePositiveRate(TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()})
	require.NoError(t, err)
	assert.InDelta(t, 0.25, rate, 0.0001)

	engine.metrics.repo = stubMetricsRepo{stats: &models.ModerationMetricsStats{}}
	rate, err = engine.GetFalsePositiveRate(TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()})
	require.NoError(t, err)
	assert.Equal(t, 0.0, rate)

	engine.metrics.repo = stubMetricsRepo{err: fmt.Errorf("boom")}
	_, err = engine.GetFalsePositiveRate(TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()})
	assert.Error(t, err)
}

type stubMetricsRepo struct {
	stats *models.ModerationMetricsStats
	err   error
}

func (s stubMetricsRepo) RecordMetricsEntry(context.Context, *models.ModerationMetricsEntry) error {
	return nil
}
func (s stubMetricsRepo) RecordMetricsEntries(context.Context, []*models.ModerationMetricsEntry) error {
	return nil
}
func (s stubMetricsRepo) RecordFalsePositive(context.Context, *models.ModerationFalsePositive) error {
	return nil
}
func (s stubMetricsRepo) GetFalsePositives(context.Context, models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error) {
	return nil, nil
}
func (s stubMetricsRepo) RecordDecisionSample(context.Context, *models.ModerationDecisionSample) error {
	return nil
}
func (s stubMetricsRepo) GetDecisionSamples(context.Context, models.ModerationMetricsTimeRange, string) ([]*models.ModerationDecisionSample, error) {
	return nil, nil
}
func (s stubMetricsRepo) UpdatePatternStats(context.Context, *models.ModerationPatternStats) error {
	return nil
}
func (s stubMetricsRepo) GetTopPatterns(context.Context, int) ([]*models.ModerationPatternStats, error) {
	return nil, nil
}
func (s stubMetricsRepo) IncrementPatternHit(context.Context, string, string) error {
	return nil
}
func (s stubMetricsRepo) GetMetricsEntries(context.Context, models.ModerationMetricsTimeRange, []string) ([]*models.ModerationMetricsEntry, error) {
	return nil, nil
}
func (s stubMetricsRepo) GetAggregatedStats(context.Context, models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.stats == nil {
		return &models.ModerationMetricsStats{}, nil
	}
	return s.stats, nil
}

func TestVideoAnalyzer_extractFrameWithFFmpeg_StubBinary(t *testing.T) {
	logger := zap.NewNop()
	va := &VideoAnalyzer{logger: logger}

	// Stub ffmpeg binary that writes to last arg.
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nout=\"\"\nfor arg in \"$@\"; do out=\"$arg\"; done\necho test > \"$out\"\n"
	require.NoError(t, os.WriteFile(ffmpegPath, []byte(script), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	videoPath := filepath.Join(t.TempDir(), "video.mp4")
	require.NoError(t, os.WriteFile(videoPath, []byte("video-bytes"), 0o600))

	framePath, err := va.extractFrameWithFFmpeg(videoPath, 1234*time.Millisecond)
	require.NoError(t, err)
	assert.FileExists(t, framePath)
}

func TestVideoAnalyzer_AnalyzeVideo_CacheAndNonS3(t *testing.T) {
	logger := zap.NewNop()

	transport := newStubAWSTransport()
	awsCfg := awsConfigForStub(transport)

	recClient := rekognition.NewFromConfig(awsCfg)
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })

	va := &VideoAnalyzer{
		client:        recClient,
		logger:        logger,
		config:        &ModerationConfig{S3Bucket: "test-bucket", EnableTextAnalysis: false, ConfidenceThreshold: 0.6, ViolenceThreshold: 0.8},
		costTracker:   &fakeCostTracker{},
		s3Client:      s3Client,
		s3Transfer:    transfermanager.New(s3Client),
		bucketName:    "test-bucket",
		cacheTTL:      time.Minute,
		imageAnalyzer: NewImageAnalyzer(recClient, logger, &ModerationConfig{S3Bucket: "test-bucket", EnableImageAnalysis: false, ConfidenceThreshold: 0.6, ViolenceThreshold: 0.8}, nil),
	}

	_, err := va.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", ContentMetadata{})
	assert.Error(t, err)

	analysis1, err := va.AnalyzeVideo(context.Background(), "s3://test-bucket/video.mp4", ContentMetadata{})
	require.NoError(t, err)
	require.NotNil(t, analysis1)

	analysis2, err := va.AnalyzeVideo(context.Background(), "s3://test-bucket/video.mp4", ContentMetadata{})
	require.NoError(t, err)
	require.Equal(t, analysis1.VideoURL, analysis2.VideoURL)
}

func TestJobPoller_CollectAllPages_WarnsAndBreaksOnError(t *testing.T) {
	logger := zap.NewNop()
	va := &VideoAnalyzer{logger: logger}

	var calls int
	nextToken := aws.String("next")
	handler := &rekognitionJobHandler{
		jobType:       "test",
		operationType: "GetTest",
		getResult: func(context.Context, string, *string) (interface{}, error) {
			calls++
			if calls > 1 {
				return nil, fmt.Errorf("boom")
			}
			return &rekognition.GetContentModerationOutput{
				JobStatus:     "SUCCEEDED",
				NextToken:     nextToken,
				StatusMessage: nil,
			}, nil
		},
		getJobStatus:     func(interface{}) rekognitionTypes.VideoJobStatus { return rekognitionTypes.VideoJobStatusSucceeded },
		getNextToken:     func(interface{}) *string { return nextToken },
		getStatusMessage: func(interface{}) *string { return nil },
	}

	p := &jobPoller{va: va, handler: handler}
	results, err := p.collectAllPages(context.Background(), "job-1", &rekognition.GetContentModerationOutput{JobStatus: rekognitionTypes.VideoJobStatusSucceeded, NextToken: nextToken})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestEngine_AnalyzeContent_AndExecuteDecision_Paths(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := getTestConfig()
	cfg.EnableImageAnalysis = false
	cfg.EnableVideoAnalysis = false

	reputationScorer := NewReputationScorer(nil, logger, cfg)

	engine := &Engine{
		config:           cfg,
		logger:           logger,
		textAnalyzer:     stubTextAnalyzer{analysis: &ContentAnalysis{ContentID: "c1", Toxicity: ToxicityAnalysis{IsToxic: true, ToxicityScore: 0.99, Confidence: 0.99}, AnalyzedAt: time.Now()}},
		imageAnalyzer:    NewNoOpImageAnalyzer(logger, cfg),
		patternMatcher:   NewPatternMatcher(stubPatternRepo{}, logger),
		reputationScorer: reputationScorer,
		decisionEngine:   NewDecisionEngine(cfg, logger, reputationScorer),
		threatIntel:      NewThreatIntelligence(stubThreatRepo{}, logger),
		dynamoRM:         mockDB,
		tableName:        "test-table",
		metrics:          NewModerationMetrics(stubMetricsRepo{}, logger),
	}

	analysis, err := engine.AnalyzeContent("very unsafe text", ContentMetadata{
		ContentID:   "c1",
		AuthorID:    "alice",
		ContentType: ContentTypeText,
		Timestamp:   time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, analysis)

	// Error branch: text analyzer fails.
	engine.textAnalyzer = stubTextAnalyzer{err: fmt.Errorf("analyzer down")}
	_, err = engine.AnalyzeContent("text", ContentMetadata{ContentID: "c2", AuthorID: "alice", ContentType: ContentTypeText, Timestamp: time.Now()})
	assert.Error(t, err)
}

func TestEngine_AnalyzeImage_SuccessAndError(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := getTestConfig()
	cfg.EnableImageAnalysis = false

	reputationScorer := NewReputationScorer(nil, logger, cfg)

	engine := &Engine{
		config:           cfg,
		logger:           logger,
		textAnalyzer:     NewNoOpTextAnalyzer(logger, cfg),
		imageAnalyzer:    stubImageAnalyzer{analysis: &ImageAnalysis{ImageURL: "s3://test-bucket/img.jpg", Text: []TextInImage{{Text: "hello", Confidence: 0.9}}, AnalyzedAt: time.Now()}},
		patternMatcher:   NewPatternMatcher(stubPatternRepo{}, logger),
		reputationScorer: reputationScorer,
		decisionEngine:   NewDecisionEngine(cfg, logger, reputationScorer),
		threatIntel:      NewThreatIntelligence(stubThreatRepo{}, logger),
		dynamoRM:         mockDB,
		tableName:        "test-table",
		metrics:          NewModerationMetrics(stubMetricsRepo{}, logger),
	}

	result, err := engine.AnalyzeImage("s3://test-bucket/img.jpg", ContentMetadata{
		ContentID:   "img-1",
		AuthorID:    "alice",
		ContentType: ContentTypeImage,
		Timestamp:   time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	engine.imageAnalyzer = stubImageAnalyzer{err: fmt.Errorf("image analysis failed")}
	_, err = engine.AnalyzeImage("s3://test-bucket/img.jpg", ContentMetadata{
		ContentID:   "img-2",
		AuthorID:    "alice",
		ContentType: ContentTypeImage,
		Timestamp:   time.Now(),
	})
	assert.Error(t, err)
}

func TestEngine_AnalyzeVideo_NoOpPath(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := getTestConfig()
	cfg.EnableVideoAnalysis = true

	reputationScorer := NewReputationScorer(nil, logger, cfg)

	engine := &Engine{
		config:           cfg,
		logger:           logger,
		textAnalyzer:     NewNoOpTextAnalyzer(logger, cfg),
		imageAnalyzer:    NewNoOpImageAnalyzer(logger, cfg),
		patternMatcher:   NewPatternMatcher(stubPatternRepo{}, logger),
		reputationScorer: reputationScorer,
		decisionEngine:   NewDecisionEngine(cfg, logger, reputationScorer),
		threatIntel:      NewThreatIntelligence(stubThreatRepo{}, logger),
		dynamoRM:         mockDB,
		tableName:        "test-table",
		metrics:          NewModerationMetrics(stubMetricsRepo{}, logger),
	}

	global := appconfig.Get()
	origDisable := global.DisableAWSModeration
	t.Cleanup(func() { global.DisableAWSModeration = origDisable })
	global.DisableAWSModeration = true

	analysis, err := engine.AnalyzeVideo("s3://test-bucket/video.mp4", ContentMetadata{
		ContentID:   "vid-1",
		AuthorID:    "alice",
		ContentType: ContentTypeVideo,
		Timestamp:   time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, analysis)
}

func TestEngine_sendToReviewQueue_DeadlineAndSeverity(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	engine := &Engine{
		logger:   logger,
		dynamoRM: mockDB,
	}

	metadata := ContentMetadata{
		ContentID: "c1",
		AuthorID:  "alice",
	}

	decision := &ModerationDecision{
		Decision:        ActionRemove,
		ReviewPriority:  9,
		Reasons:         []DecisionReason{{Type: "t", Severity: SeverityHigh, Description: "d"}},
		Recommendations: []string{"rec"},
		Confidence:      0.9,
	}

	require.NoError(t, engine.sendToReviewQueue(ctx, decision, metadata))

	decision.Decision = ActionFlag
	decision.ReviewPriority = 5
	require.NoError(t, engine.sendToReviewQueue(ctx, decision, metadata))

	decision.Decision = ActionQuarantine
	decision.ReviewPriority = 1
	require.NoError(t, engine.sendToReviewQueue(ctx, decision, metadata))

	// DB error is returned.
	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	mockUpdateBuilder2 := new(mocks.MockUpdateBuilder)
	mockDB2.On("WithContext", mock.Anything).Return(mockDB2)
	mockDB2.On("Model", mock.Anything).Return(mockQuery2)
	mockQuery2.On("Create").Return(fmt.Errorf("create failed")).Once()
	setupPermissiveDynamormMocks(mockDB2, mockQuery2, mockUpdateBuilder2)

	engine.dynamoRM = mockDB2
	assert.Error(t, engine.sendToReviewQueue(ctx, decision, metadata))
}

func TestNewVideoAnalyzer_AndTranscribeAudio_Branches(t *testing.T) {
	logger := zap.NewNop()
	cfg := &ModerationConfig{S3Bucket: "test-bucket", EnableTextAnalysis: true}

	awsCfg := awsConfigForStub(newStubAWSTransport())
	recClient := rekognition.NewFromConfig(awsCfg)

	va := NewVideoAnalyzer(recClient, logger, cfg, &fakeCostTracker{})
	require.NotNil(t, va)
	assert.NotEmpty(t, va.bucketName)

	_, err := va.transcribeAudio(context.Background(), &rekognitionTypes.Video{})
	assert.Error(t, err)

	video := &rekognitionTypes.Video{
		S3Object: &rekognitionTypes.S3Object{
			Bucket: aws.String("test-bucket"),
			Name:   aws.String("video.mp4"),
		},
	}
	audio, err := va.transcribeAudio(context.Background(), video)
	require.NoError(t, err)
	assert.Equal(t, "en", audio.Language)
}

func TestJobPoller_FailedAndUnexpectedStatus(t *testing.T) {
	logger := zap.NewNop()
	va := &VideoAnalyzer{logger: logger}

	t.Run("failed", func(t *testing.T) {
		handler := &rekognitionJobHandler{
			jobType: "test",
			getResult: func(context.Context, string, *string) (interface{}, error) {
				msg := "nope"
				return &rekognition.GetContentModerationOutput{
					JobStatus:     rekognitionTypes.VideoJobStatusFailed,
					StatusMessage: &msg,
				}, nil
			},
			getJobStatus: func(result interface{}) rekognitionTypes.VideoJobStatus {
				return result.(*rekognition.GetContentModerationOutput).JobStatus
			},
			getNextToken: func(interface{}) *string { return nil },
			getStatusMessage: func(result interface{}) *string {
				return result.(*rekognition.GetContentModerationOutput).StatusMessage
			},
		}
		p := &jobPoller{va: va, handler: handler}
		_, err := p.poll(context.Background(), "job-1")
		assert.Error(t, err)
	})

	t.Run("unexpected status", func(t *testing.T) {
		handler := &rekognitionJobHandler{
			jobType: "test",
			getResult: func(context.Context, string, *string) (interface{}, error) {
				return &rekognition.GetContentModerationOutput{JobStatus: "UNKNOWN"}, nil
			},
			getJobStatus: func(result interface{}) rekognitionTypes.VideoJobStatus {
				return result.(*rekognition.GetContentModerationOutput).JobStatus
			},
			getNextToken:     func(interface{}) *string { return nil },
			getStatusMessage: func(interface{}) *string { return nil },
		}
		p := &jobPoller{va: va, handler: handler}
		_, err := p.poll(context.Background(), "job-1")
		assert.Error(t, err)
	})
}

func TestJobPoller_checkContext_And_waitWithBackoff(t *testing.T) {
	handler := &rekognitionJobHandler{jobType: "test"}
	p := &jobPoller{va: &VideoAnalyzer{logger: zap.NewNop()}, handler: handler}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, p.checkContext(ctx, "job-1"))

	assert.Equal(t, time.Duration(0), p.waitWithBackoff(0, 30*time.Second))
	assert.Equal(t, time.Duration(0), p.waitWithBackoff(0, 0))
	assert.Equal(t, 2*time.Millisecond, p.waitWithBackoff(1*time.Millisecond, 30*time.Second))
	assert.Equal(t, 100*time.Millisecond, p.waitWithBackoff(50*time.Millisecond, 30*time.Second))
}

func TestVideoAnalyzer_ProcessResults_AndLabelHelpers(t *testing.T) {
	va := &VideoAnalyzer{
		logger: zap.NewNop(),
		config: &ModerationConfig{
			ConfidenceThreshold: 0.6,
			ViolenceThreshold:   0.8,
		},
	}

	// Moderation labels (exercise switch categories).
	conf := float32(99)
	results := []rekognition.GetContentModerationOutput{{
		ModerationLabels: []rekognitionTypes.ContentModerationDetection{
			{ModerationLabel: &rekognitionTypes.ModerationLabel{Name: aws.String("Explicit Nudity"), Confidence: &conf}, Timestamp: 1000},
			{ModerationLabel: &rekognitionTypes.ModerationLabel{Name: aws.String("Suggestive"), Confidence: &conf}, Timestamp: 2000},
			{ModerationLabel: &rekognitionTypes.ModerationLabel{Name: aws.String("Violence"), Confidence: &conf}, Timestamp: 3000},
			{ModerationLabel: &rekognitionTypes.ModerationLabel{Name: aws.String("Visually Disturbing"), Confidence: &conf}, Timestamp: 4000},
			{ModerationLabel: &rekognitionTypes.ModerationLabel{Name: aws.String("Other"), Confidence: &conf}, Timestamp: 5000},
		},
	}}
	va.processModerationResults(results, &VideoAnalysis{})

	// Text detections (line vs word, threshold filtering, bounding box).
	textConfHigh := float32(99)
	textConfLow := float32(10)
	detected := aws.String("hello")
	textResults := []rekognition.GetTextDetectionOutput{{
		TextDetections: []rekognitionTypes.TextDetectionResult{
			{TextDetection: &rekognitionTypes.TextDetection{DetectedText: detected, Confidence: &textConfLow, Type: rekognitionTypes.TextTypesLine}},
			{TextDetection: &rekognitionTypes.TextDetection{DetectedText: detected, Confidence: &textConfHigh, Type: rekognitionTypes.TextTypesWord}},
			{
				TextDetection: &rekognitionTypes.TextDetection{
					DetectedText: detected,
					Confidence:   &textConfHigh,
					Type:         rekognitionTypes.TextTypesLine,
					Geometry: &rekognitionTypes.Geometry{
						BoundingBox: &rekognitionTypes.BoundingBox{Left: aws.Float32(0.1), Top: aws.Float32(0.2), Width: aws.Float32(0.3), Height: aws.Float32(0.4)},
					},
				},
			},
		},
	}}
	video := &VideoAnalysis{Audio: AudioAnalysis{Transcription: ""}}
	va.processTextResults(textResults, video)
	assert.NotEmpty(t, video.Audio.Transcription)

	// Face results.
	faceConfHigh := float32(99)
	faceConfLow := float32(10)
	faceResults := []rekognition.GetFaceDetectionOutput{{
		Faces: []rekognitionTypes.FaceDetection{
			{Face: &rekognitionTypes.FaceDetail{Confidence: &faceConfLow}},
			{
				Face: &rekognitionTypes.FaceDetail{
					Confidence: &faceConfHigh,
					BoundingBox: &rekognitionTypes.BoundingBox{
						Left: aws.Float32(0.1), Top: aws.Float32(0.2), Width: aws.Float32(0.3), Height: aws.Float32(0.4),
					},
					Emotions: []rekognitionTypes.Emotion{{Type: rekognitionTypes.EmotionNameHappy, Confidence: aws.Float32(99)}},
					AgeRange: &rekognitionTypes.AgeRange{Low: aws.Int32(1), High: aws.Int32(2)},
					Gender:   &rekognitionTypes.Gender{Value: rekognitionTypes.GenderTypeMale, Confidence: aws.Float32(99)},
				},
			},
		},
	}}
	va.processFaceResults(faceResults, &VideoAnalysis{})

	// Label results.
	labelConfHigh := float32(99)
	labelConfLow := float32(10)
	labelResults := []rekognition.GetLabelDetectionOutput{{
		Labels: []rekognitionTypes.LabelDetection{
			{Label: &rekognitionTypes.Label{Name: aws.String("Gun"), Confidence: &labelConfLow}},
			{
				Label: &rekognitionTypes.Label{
					Name:       aws.String("Gun"),
					Confidence: &labelConfHigh,
					Parents:    []rekognitionTypes.Parent{{Name: aws.String("Weapon")}},
				},
			},
			{
				Label: &rekognitionTypes.Label{
					Name:       aws.String("Violence"),
					Confidence: &labelConfHigh,
				},
			},
		},
	}}
	va.processLabelResults(labelResults, &VideoAnalysis{})

	assert.True(t, va.isViolenceLabel("gun"))
	assert.True(t, va.isViolenceLabel("Gun Shot"))
	assert.False(t, va.isViolenceLabel("cat"))
	assert.True(t, va.isWeaponLabel("Gun"))
	assert.False(t, va.isWeaponLabel("Cat"))
}

func TestJobHandlerFactory_createStandardJobConfig_UsesAccessor(t *testing.T) {
	factory := &jobHandlerFactory{va: &VideoAnalyzer{logger: zap.NewNop()}}

	next := aws.String("next")
	msg := aws.String("msg")

	config := factory.createStandardJobConfig(
		"test job",
		"GetTest",
		func(context.Context, string, *string) (interface{}, error) { return nil, nil },
		func(interface{}) (rekognitionTypes.VideoJobStatus, *string, *string) {
			return rekognitionTypes.VideoJobStatusSucceeded, next, msg
		},
	)

	assert.Equal(t, rekognitionTypes.VideoJobStatusSucceeded, config.getJobStatus(struct{}{}))
	assert.Equal(t, next, config.getNextToken(struct{}{}))
	assert.Equal(t, msg, config.getStatusMessage(struct{}{}))
}

func TestVideoAnalyzer_analyzeKeyFrames_ExtractFrameSuccess(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	transport := newStubAWSTransport()
	transport.s3Objects["test-bucket/video.mp4"] = []byte("tiny-video")
	awsCfg := awsConfigForStub(transport)

	recClient := rekognition.NewFromConfig(awsCfg)
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })

	// Stub ffmpeg binary that writes to last arg.
	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nout=\"\"\nfor arg in \"$@\"; do out=\"$arg\"; done\necho frame > \"$out\"\n"
	require.NoError(t, os.WriteFile(ffmpegPath, []byte(script), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	va := &VideoAnalyzer{
		client:        recClient,
		logger:        logger,
		config:        &ModerationConfig{S3Bucket: "test-bucket", EnableTextAnalysis: false, ConfidenceThreshold: 0.6, ViolenceThreshold: 0.8},
		s3Client:      s3Client,
		s3Transfer:    transfermanager.New(s3Client),
		bucketName:    "test-bucket",
		cacheTTL:      time.Minute,
		imageAnalyzer: NewImageAnalyzer(recClient, logger, &ModerationConfig{S3Bucket: "test-bucket", EnableImageAnalysis: false, ConfidenceThreshold: 0.6, ViolenceThreshold: 0.8}, nil),
	}

	video := &rekognitionTypes.Video{
		S3Object: &rekognitionTypes.S3Object{
			Bucket: aws.String("test-bucket"),
			Name:   aws.String("video.mp4"),
		},
	}

	frames, err := va.analyzeKeyFrames(ctx, video, []time.Duration{0})
	require.NoError(t, err)
	require.Len(t, frames, 1)

	// Cover early-return/error helpers.
	_, err = va.extractFrameAtTimestamp(ctx, &rekognitionTypes.Video{}, 0)
	assert.Error(t, err)

	_, err = va.uploadFrameToS3(ctx, filepath.Join(t.TempDir(), "does-not-exist.jpg"))
	assert.Error(t, err)

	va.cleanupTemporaryFrame(ctx, "not-s3")
	va.cleanupTemporaryFrame(ctx, "https://bucket.s3.us-east-1.amazonaws.com/key")

	path, err := va.createTempFile("tmp_", ".txt")
	require.NoError(t, err)
	require.NoError(t, path.Close())
	va.cleanupLocalFile(path.Name())
	va.cleanupLocalFile(filepath.Join(t.TempDir(), "missing.txt"))

	_, err = va.getVideoDuration(ctx, &rekognitionTypes.Video{})
	assert.Error(t, err)

	assert.NotEmpty(t, va.extractVideoID("s3://bucket/path/to/file.mp4"))
	assert.True(t, strings.HasPrefix(va.extractVideoID("https://example.com/video.mp4"), "video_"))

	assert.Equal(t, 1, va.countSuccessfulFrames([]FrameAnalysis{
		{ImageAnalysis: ImageAnalysis{Explicit: ExplicitContent{Confidence: 0.1}}},
		{ImageAnalysis: ImageAnalysis{}},
	}))
}

func TestEngine_Wrappers_And_AnalyzeContentBatch(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := getTestConfig()
	cfg.EnableImageAnalysis = false

	reputationScorer := NewReputationScorer(nil, logger, cfg)
	patternMatcher := NewPatternMatcher(stubPatternRepo{}, logger)
	threatIntel := NewThreatIntelligence(stubThreatRepo{}, logger)

	engine := &Engine{
		config: cfg,
		logger: logger,
		textAnalyzer: textAnalyzerFunc(func(_ context.Context, text string, _ ContentMetadata) (*ContentAnalysis, error) {
			return &ContentAnalysis{ContentID: text, AnalyzedAt: time.Now()}, nil
		}),
		imageAnalyzer:    NewNoOpImageAnalyzer(logger, cfg),
		patternMatcher:   patternMatcher,
		reputationScorer: reputationScorer,
		decisionEngine:   NewDecisionEngine(cfg, logger, reputationScorer),
		threatIntel:      threatIntel,
		dynamoRM:         mockDB,
		tableName:        "test-table",
		metrics:          NewModerationMetrics(stubMetricsRepo{}, logger),
	}

	// Pattern wrappers.
	require.NoError(t, engine.CreatePattern(&ModerationPattern{Name: "p", Pattern: "x", Type: patternTypeKeyword, Active: true}))
	require.NoError(t, engine.UpdatePattern("p1", &ModerationPattern{Name: "p", Pattern: "x", Type: patternTypeKeyword, Active: true}))
	require.NoError(t, engine.DeletePattern("p1"))
	_, err := engine.GetPatterns(PatternFilter{})
	require.NoError(t, err)

	// Reputation wrappers.
	_, _ = engine.GetReputationScore("alice")
	_ = engine.UpdateReputation("alice", ReputationEvent{EventType: "violation", Severity: SeverityLow, Description: "d", Timestamp: time.Now()})

	// Threat wrappers.
	require.NoError(t, engine.ShareThreat(&ThreatIntel{ThreatType: "spam", Indicators: []string{"x"}, Severity: SeverityLow, Confidence: 0.5}))
	_, err = engine.GetSharedThreats(time.Now().Add(-time.Hour))
	require.NoError(t, err)

	// Decision wrapper.
	_, err = engine.MakeDecision(&ModerationAnalysis{ContentMetadata: ContentMetadata{ContentID: "c", AuthorID: "alice"}})
	require.NoError(t, err)

	// Moderation stats wrapper.
	_, err = engine.GetModerationStats(TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()})
	require.NoError(t, err)

	// Batch analysis wrapper.
	engine.textAnalyzer = textAnalyzerFunc(func(_ context.Context, text string, _ ContentMetadata) (*ContentAnalysis, error) {
		if text == "bad" {
			return nil, fmt.Errorf("boom")
		}
		return &ContentAnalysis{ContentID: text, AnalyzedAt: time.Now()}, nil
	})

	results, err := engine.AnalyzeContentBatch([]struct {
		Content  string
		Metadata ContentMetadata
	}{
		{Content: "ok", Metadata: ContentMetadata{ContentID: "ok", AuthorID: "alice", ContentType: ContentTypeText, Timestamp: time.Now()}},
		{Content: "bad", Metadata: ContentMetadata{ContentID: "bad", AuthorID: "alice", ContentType: ContentTypeText, Timestamp: time.Now()}},
	})
	assert.Len(t, results, 2)
	assert.Error(t, err)
}

func TestEngine_AnalyzeVideo_BranchSelection(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	transport := newStubAWSTransport()
	awsCfg := awsConfigForStub(transport)
	recClient := rekognition.NewFromConfig(awsCfg)

	cfg := getTestConfig()
	cfg.EnableVideoAnalysis = true
	cfg.EnableImageAnalysis = false
	cfg.S3Bucket = "test-bucket"

	reputationScorer := NewReputationScorer(nil, logger, cfg)
	patternMatcher := NewPatternMatcher(stubPatternRepo{}, logger)
	threatIntel := NewThreatIntelligence(stubThreatRepo{}, logger)

	engine := &Engine{
		config:           cfg,
		logger:           logger,
		textAnalyzer:     NewNoOpTextAnalyzer(logger, cfg),
		patternMatcher:   patternMatcher,
		reputationScorer: reputationScorer,
		decisionEngine:   NewDecisionEngine(cfg, logger, reputationScorer),
		threatIntel:      threatIntel,
		dynamoRM:         mockDB,
		tableName:        "test-table",
		metrics:          NewModerationMetrics(stubMetricsRepo{}, logger),
		costTracker:      &fakeCostTracker{},
	}

	global := appconfig.Get()
	origAWS := global.DisableAWSModeration
	origRek := global.DisableRekognition
	t.Cleanup(func() {
		global.DisableAWSModeration = origAWS
		global.DisableRekognition = origRek
	})

	t.Run("no-op fallback when image analyzer not AWS", func(t *testing.T) {
		global.DisableAWSModeration = false
		global.DisableRekognition = false
		engine.imageAnalyzer = NewNoOpImageAnalyzer(logger, cfg)

		analysis, err := engine.AnalyzeVideo("s3://test-bucket/video.mp4", ContentMetadata{
			ContentID:   "vid-fallback",
			AuthorID:    "alice",
			ContentType: ContentTypeVideo,
			Timestamp:   time.Now(),
		})
		require.NoError(t, err)
		require.NotNil(t, analysis)
	})

	t.Run("aws branch when image analyzer is AWS-based", func(t *testing.T) {
		global.DisableAWSModeration = false
		global.DisableRekognition = false
		engine.imageAnalyzer = NewImageAnalyzer(recClient, logger, cfg, &fakeCostTracker{})

		analysis, err := engine.AnalyzeVideo("s3://test-bucket/video.mp4", ContentMetadata{
			ContentID:   "vid-aws",
			AuthorID:    "alice",
			ContentType: ContentTypeVideo,
			Timestamp:   time.Now(),
		})
		require.NoError(t, err)
		require.NotNil(t, analysis)
	})

	t.Run("aws branch errors on non-s3 urls", func(t *testing.T) {
		global.DisableAWSModeration = false
		global.DisableRekognition = false
		engine.imageAnalyzer = NewImageAnalyzer(recClient, logger, cfg, &fakeCostTracker{})

		_, err := engine.AnalyzeVideo("https://example.com/video.mp4", ContentMetadata{
			ContentID:   "vid-aws-error",
			AuthorID:    "alice",
			ContentType: ContentTypeVideo,
			Timestamp:   time.Now(),
		})
		assert.Error(t, err)
	})
}

func TestVideoAnalyzer_storeThumbnails_EmptyFrames_NoOp(t *testing.T) {
	va := &VideoAnalyzer{logger: zap.NewNop(), config: &ModerationConfig{}}
	require.NoError(t, va.storeThumbnails(context.Background(), &VideoAnalysis{VideoURL: "s3://bucket/key"}))
}

func TestEngine_AnalyzeContent_LogsOnDecisionAndStoreFailures(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// First Create() is StoreDecision; second is StoreAnalysisResult.
	mockQuery.On("Create").Return(fmt.Errorf("create failed")).Twice()
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	cfg := getTestConfig()
	cfg.EnableImageAnalysis = false
	cfg.EnableVideoAnalysis = false

	reputationScorer := NewReputationScorer(nil, logger, cfg)

	engine := &Engine{
		config:           cfg,
		logger:           logger,
		textAnalyzer:     stubTextAnalyzer{analysis: &ContentAnalysis{ContentID: "c1", Toxicity: ToxicityAnalysis{IsToxic: false, ToxicityScore: 0.0, Confidence: 1.0}, AnalyzedAt: time.Now()}},
		imageAnalyzer:    NewNoOpImageAnalyzer(logger, cfg),
		patternMatcher:   NewPatternMatcher(stubPatternRepo{}, logger),
		reputationScorer: reputationScorer,
		decisionEngine:   NewDecisionEngine(cfg, logger, reputationScorer),
		threatIntel:      NewThreatIntelligence(stubThreatRepo{}, logger),
		dynamoRM:         mockDB,
		tableName:        "test-table",
		metrics:          NewModerationMetrics(stubMetricsRepo{}, logger),
	}

	analysis, err := engine.AnalyzeContent("safe text", ContentMetadata{
		ContentID:   "c1",
		AuthorID:    "alice",
		ContentType: ContentTypeText,
		Timestamp:   time.Now(),
	})
	require.NoError(t, err)
	require.NotNil(t, analysis)
}

func TestEngine_executeDecision_StoreDecisionFailure(t *testing.T) {
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	mockQuery.On("Create").Return(fmt.Errorf("create failed")).Once()
	setupPermissiveDynamormMocks(mockDB, mockQuery, mockUpdateBuilder)

	engine := &Engine{
		logger:    logger,
		dynamoRM:  mockDB,
		tableName: "test-table",
	}

	err := engine.executeDecision(context.Background(), &ModerationDecision{
		Decision:   ActionAllow,
		Confidence: 0.9,
		DecidedAt:  time.Now(),
	}, ContentMetadata{ContentID: "c1", AuthorID: "alice"})
	assert.Error(t, err)
}

func TestVideoAnalyzer_S3AndFFmpeg_ErrorBranches(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	t.Run("download video fails when S3 get errors", func(t *testing.T) {
		transport := newStubAWSTransport()
		transport.failS3Get = true
		awsCfg := awsConfigForStub(transport)
		s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })

		va := &VideoAnalyzer{logger: logger, s3Client: s3Client, s3Transfer: transfermanager.New(s3Client)}
		_, err := va.downloadVideoFromS3(ctx, "test-bucket", "video.mp4")
		assert.Error(t, err)
	})

	t.Run("upload frame fails when S3 put errors", func(t *testing.T) {
		transport := newStubAWSTransport()
		transport.failS3Put = true
		awsCfg := awsConfigForStub(transport)
		s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })

		framePath := filepath.Join(t.TempDir(), "frame.jpg")
		require.NoError(t, os.WriteFile(framePath, []byte("frame"), 0o600))

		va := &VideoAnalyzer{
			logger:     logger,
			s3Client:   s3Client,
			s3Transfer: transfermanager.New(s3Client),
			bucketName: "test-bucket",
		}

		_, err := va.uploadFrameToS3(ctx, framePath)
		assert.Error(t, err)
	})

	t.Run("cleanup warns when S3 delete errors", func(t *testing.T) {
		transport := newStubAWSTransport()
		transport.failS3Delete = true
		awsCfg := awsConfigForStub(transport)
		s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })

		va := &VideoAnalyzer{logger: logger, s3Client: s3Client}
		va.cleanupTemporaryFrame(ctx, "s3://test-bucket/key")
	})

	t.Run("ffmpeg success with empty output fails validation", func(t *testing.T) {
		va := &VideoAnalyzer{logger: logger}

		dir := t.TempDir()
		ffmpegPath := filepath.Join(dir, "ffmpeg")
		script := "#!/bin/sh\nout=\"\"\nfor arg in \"$@\"; do out=\"$arg\"; done\n: > \"$out\"\n"
		require.NoError(t, os.WriteFile(ffmpegPath, []byte(script), 0o700))
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

		videoPath := filepath.Join(t.TempDir(), "video.mp4")
		require.NoError(t, os.WriteFile(videoPath, []byte("video-bytes"), 0o600))

		_, err := va.extractFrameWithFFmpeg(videoPath, time.Second)
		assert.Error(t, err)
	})

	t.Run("ffmpeg command failure returns stderr", func(t *testing.T) {
		va := &VideoAnalyzer{logger: logger}

		dir := t.TempDir()
		ffmpegPath := filepath.Join(dir, "ffmpeg")
		script := "#!/bin/sh\necho boom 1>&2\nexit 1\n"
		require.NoError(t, os.WriteFile(ffmpegPath, []byte(script), 0o700))
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

		videoPath := filepath.Join(t.TempDir(), "video.mp4")
		require.NoError(t, os.WriteFile(videoPath, []byte("video-bytes"), 0o600))

		_, err := va.extractFrameWithFFmpeg(videoPath, time.Second)
		assert.Error(t, err)
	})
}

func TestNewVideoAnalyzer_BucketNameSelection(t *testing.T) {
	logger := zap.NewNop()
	cfg := &ModerationConfig{S3Bucket: "test-bucket"}

	global := appconfig.Get()
	origMedia := global.MediaBucketName
	origS3 := global.S3BucketName
	t.Cleanup(func() {
		global.MediaBucketName = origMedia
		global.S3BucketName = origS3
	})

	global.MediaBucketName = "media-bucket"
	global.S3BucketName = "s3-bucket"
	va := NewVideoAnalyzer(nil, logger, cfg, nil)
	require.Equal(t, "media-bucket", va.bucketName)

	global.MediaBucketName = ""
	global.S3BucketName = "s3-bucket"
	va = NewVideoAnalyzer(nil, logger, cfg, nil)
	require.Equal(t, "s3-bucket", va.bucketName)

	global.MediaBucketName = ""
	global.S3BucketName = ""
	va = NewVideoAnalyzer(nil, logger, cfg, nil)
	require.Equal(t, "lesser-media", va.bucketName)
}
