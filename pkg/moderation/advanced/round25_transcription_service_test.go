package advanced

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/transcribe"
	"github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type transcribeStubTransport struct {
	mu sync.Mutex

	transcribeStatus types.TranscriptionJobStatus
	failureReason    string
	transcriptURI    string

	startErr  error
	getErr    error
	deleteErr error

	afterGetJob func()

	s3Objects map[string][]byte
	s3Err     error
}

func (t *transcribeStubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Header.Get("X-Amz-Target")
	if strings.HasPrefix(target, "Transcribe.") {
		return t.roundTripTranscribe(req, target)
	}

	// Assume S3.
	return t.roundTripS3(req)
}

func (t *transcribeStubTransport) roundTripTranscribe(req *http.Request, target string) (*http.Response, error) {
	op := strings.TrimPrefix(target, "Transcribe.")

	t.mu.Lock()
	startErr := t.startErr
	getErr := t.getErr
	deleteErr := t.deleteErr
	status := t.transcribeStatus
	failureReason := t.failureReason
	transcriptURI := t.transcriptURI
	afterGetJob := t.afterGetJob
	t.mu.Unlock()

	switch op {
	case "StartTranscriptionJob":
		if startErr != nil {
			return nil, startErr
		}
		return jsonResponse(http.StatusOK, map[string]any{})
	case "GetTranscriptionJob":
		if getErr != nil {
			return nil, getErr
		}
		job := map[string]any{
			"TranscriptionJobName":   "job-1",
			"TranscriptionJobStatus": string(status),
			"LanguageCode":           "en-US",
			"Transcript":             map[string]any{"TranscriptFileUri": transcriptURI},
		}
		if status == types.TranscriptionJobStatusFailed {
			job["FailureReason"] = failureReason
		}
		if afterGetJob != nil {
			afterGetJob()
		}
		return jsonResponse(http.StatusOK, map[string]any{"TranscriptionJob": job})
	case "DeleteTranscriptionJob":
		if deleteErr != nil {
			return nil, deleteErr
		}
		return jsonResponse(http.StatusOK, map[string]any{})
	default:
		return jsonResponse(http.StatusBadRequest, map[string]any{"Message": "unsupported operation"})
	}
}

func parseS3BucketKeyPreferVirtualHost(req *http.Request) (bucket, key string) {
	host := req.URL.Hostname()
	path := strings.TrimPrefix(req.URL.Path, "/")

	// Virtual-host style: bucket.s3...
	if idx := strings.Index(host, ".s3"); idx > 0 {
		return host[:idx], path
	}

	// Path-style: /bucket/key
	if parts := strings.SplitN(path, "/", 2); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1]
	}

	return "test-bucket", path
}

func (t *transcribeStubTransport) roundTripS3(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.s3Err != nil {
		return nil, t.s3Err
	}

	if req.Method != http.MethodGet {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("unsupported")),
			Request:    req,
		}, nil
	}

	if t.s3Objects == nil {
		t.s3Objects = make(map[string][]byte)
	}

	bucket, key := parseS3BucketKeyPreferVirtualHost(req)
	objectKey := bucket + "/" + key
	data, ok := t.s3Objects[objectKey]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("not found")),
			Request:    req,
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(data))),
		Request:    req,
	}, nil
}

type fakeTranscribeCostTracker struct {
	mu               sync.Mutex
	transcribeJobs   []string
	transcribeMins   []int
	comprehendCalls  int
	comprehendUnits  int
	transcribeCalled bool
}

func (f *fakeTranscribeCostTracker) TrackComprehendRequest(operation string, units int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comprehendCalls++
	f.comprehendUnits += units
	_ = operation
}

func (f *fakeTranscribeCostTracker) TrackTranscribeRequest(jobName string, estimatedMinutes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transcribeJobs = append(f.transcribeJobs, jobName)
	f.transcribeMins = append(f.transcribeMins, estimatedMinutes)
	f.transcribeCalled = true
}

func TestTranscriptionService_parseTranscriptJSON_ParsesAndUnescapes(t *testing.T) {
	ts := NewTranscriptionService(nil, nil, zap.NewNop(), "bucket", nil)

	text, confidence, err := ts.parseTranscriptJSON(`{"results":{"transcripts":[{"transcript":"hello\\world"}]}}`)
	require.NoError(t, err)
	assert.Equal(t, "hello\\world", text)
	assert.Equal(t, 0.85, confidence)
}

func TestTranscriptionService_parseTranscriptJSON_ErrorsOnMissingTranscript(t *testing.T) {
	ts := NewTranscriptionService(nil, nil, zap.NewNop(), "bucket", nil)
	_, _, err := ts.parseTranscriptJSON(`{"nope":true}`)
	require.Error(t, err)
}

func TestTranscriptionService_parseTranscriptJSON_ErrorsOnUnterminatedTranscript(t *testing.T) {
	ts := NewTranscriptionService(nil, nil, zap.NewNop(), "bucket", nil)
	_, _, err := ts.parseTranscriptJSON(`{"transcript":"unterminated}`)
	require.Error(t, err)
}

func TestTranscriptionService_parseTranscriptJSON_UsesConfidenceBranchWhenPresent(t *testing.T) {
	ts := NewTranscriptionService(nil, nil, zap.NewNop(), "bucket", nil)
	text, confidence, err := ts.parseTranscriptJSON(`{"transcript":"ok","confidence":"0.92"}`)
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
	assert.Equal(t, 0.85, confidence)
}

func TestTranscriptionService_downloadTranscript_ValidatesAndDownloads(t *testing.T) {
	transport := &transcribeStubTransport{
		transcribeStatus: types.TranscriptionJobStatusCompleted,
		transcriptURI:    "https://s3.us-east-1.amazonaws.com/transcript-bucket/transcripts/job-1.json",
		s3Objects: map[string][]byte{
			"transcript-bucket/transcripts/job-1.json": []byte(`{"transcript":"ok"}`),
		},
	}

	cfg := awsConfigForStub(transport)
	s3Client := s3.NewFromConfig(cfg)

	ts := NewTranscriptionService(nil, s3Client, zap.NewNop(), "bucket", nil)
	text, confidence, err := ts.downloadTranscript(context.Background(), &transport.transcriptURI)
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
	assert.Equal(t, 0.85, confidence)
}

func TestTranscriptionService_downloadTranscript_ReturnsErrorWhenS3Fails(t *testing.T) {
	transport := &transcribeStubTransport{
		s3Err: errors.New("s3 down"),
	}
	cfg := awsConfigForStub(transport)

	ts := NewTranscriptionService(nil, s3.NewFromConfig(cfg), zap.NewNop(), "bucket", nil)
	uri := "https://s3.us-east-1.amazonaws.com/transcript-bucket/transcripts/job-1.json"
	_, _, err := ts.downloadTranscript(context.Background(), &uri)
	require.Error(t, err)
}

func TestTranscriptionService_downloadTranscript_ErrorsOnBadURI(t *testing.T) {
	transport := &transcribeStubTransport{}
	cfg := awsConfigForStub(transport)

	ts := NewTranscriptionService(nil, s3.NewFromConfig(cfg), zap.NewNop(), "bucket", nil)
	_, _, err := ts.downloadTranscript(context.Background(), nil)
	require.Error(t, err)

	bad := "https://no-slashes"
	_, _, err = ts.downloadTranscript(context.Background(), &bad)
	require.Error(t, err)

	alsoBad := "https://s3.us-east-1.amazonaws.com/bucketonly"
	_, _, err = ts.downloadTranscript(context.Background(), &alsoBad)
	require.Error(t, err)
}

func TestTranscriptionService_pollForCompletion_CompletedAndFailedAndCanceled(t *testing.T) {
	transport := &transcribeStubTransport{
		transcribeStatus: types.TranscriptionJobStatusCompleted,
		transcriptURI:    "https://s3.us-east-1.amazonaws.com/bucket/key",
	}
	cfg := awsConfigForStub(transport)
	tc := transcribe.NewFromConfig(cfg)

	ts := NewTranscriptionService(tc, nil, zap.NewNop(), "bucket", nil)
	job, err := ts.pollForCompletion(context.Background(), "job-1")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, types.TranscriptionJobStatusCompleted, job.TranscriptionJobStatus)

	transport.mu.Lock()
	transport.transcribeStatus = types.TranscriptionJobStatusFailed
	transport.failureReason = "bad audio"
	transport.mu.Unlock()
	_, err = ts.pollForCompletion(context.Background(), "job-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad audio")

	transport.mu.Lock()
	transport.transcribeStatus = types.TranscriptionJobStatus("UNKNOWN")
	transport.mu.Unlock()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ts.pollForCompletion(canceled, "job-3")
	require.Error(t, err)
}

func TestTranscriptionService_pollForCompletion_InProgressReturnsContextError(t *testing.T) {
	transport := &transcribeStubTransport{
		transcribeStatus: types.TranscriptionJobStatusInProgress,
		transcriptURI:    "https://s3.us-east-1.amazonaws.com/bucket/key",
	}
	cfg := awsConfigForStub(transport)
	tc := transcribe.NewFromConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	transport.mu.Lock()
	transport.afterGetJob = cancel
	transport.mu.Unlock()

	ts := NewTranscriptionService(tc, nil, zap.NewNop(), "bucket", nil)
	_, err := ts.pollForCompletion(ctx, "job-1")
	require.Error(t, err)
}

func TestTranscriptionService_deleteTranscriptionJob_PropagatesErrors(t *testing.T) {
	transport := &transcribeStubTransport{
		transcribeStatus: types.TranscriptionJobStatusCompleted,
		transcriptURI:    "https://s3.us-east-1.amazonaws.com/bucket/key",
	}
	cfg := awsConfigForStub(transport)
	tc := transcribe.NewFromConfig(cfg)

	ts := NewTranscriptionService(tc, nil, zap.NewNop(), "bucket", nil)
	require.NoError(t, ts.deleteTranscriptionJob(context.Background(), "job-1"))

	transport.mu.Lock()
	transport.deleteErr = errors.New("delete failed")
	transport.mu.Unlock()
	err := ts.deleteTranscriptionJob(context.Background(), "job-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete transcription job")
}

func TestTranscriptionService_TranscribeAudio_SuccessAndStartError(t *testing.T) {
	cost := &fakeTranscribeCostTracker{}
	transport := &transcribeStubTransport{
		transcribeStatus: types.TranscriptionJobStatusCompleted,
		transcriptURI:    "https://s3.us-east-1.amazonaws.com/transcript-bucket/transcripts/job-1.json",
		s3Objects: map[string][]byte{
			"transcript-bucket/transcripts/job-1.json": []byte(`{"transcript":"hello"}`),
		},
	}

	cfg := awsConfigForStub(transport)
	tc := transcribe.NewFromConfig(cfg)
	s3c := s3.NewFromConfig(cfg)

	ts := NewTranscriptionService(tc, s3c, zap.NewNop(), "out-bucket", cost)
	result, err := ts.TranscribeAudio(context.Background(), "s3://in-bucket/file.mp4")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "hello", result.Transcription)
	assert.Equal(t, "en-US", result.Language)
	assert.NotEmpty(t, result.JobName)
	assert.Greater(t, result.Duration, time.Duration(0))

	cost.mu.Lock()
	require.True(t, cost.transcribeCalled)
	require.Len(t, cost.transcribeJobs, 1)
	assert.True(t, strings.HasPrefix(cost.transcribeJobs[0], "moderation-job-"))
	cost.mu.Unlock()

	transport.mu.Lock()
	transport.startErr = errors.New("start failed")
	transport.mu.Unlock()
	_, err = ts.TranscribeAudio(context.Background(), "s3://in-bucket/file.mp4")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start transcription job")
}

func TestTranscriptionService_TranscribeAudio_ReturnsErrorWhenJobFails(t *testing.T) {
	transport := &transcribeStubTransport{
		transcribeStatus: types.TranscriptionJobStatusFailed,
		failureReason:    "bad audio",
		transcriptURI:    "https://s3.us-east-1.amazonaws.com/bucket/key",
	}

	cfg := awsConfigForStub(transport)
	tc := transcribe.NewFromConfig(cfg)
	s3c := s3.NewFromConfig(cfg)

	ts := NewTranscriptionService(tc, s3c, zap.NewNop(), "out-bucket", nil)
	_, err := ts.TranscribeAudio(context.Background(), "s3://in-bucket/file.mp4")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transcription job failed")
}
