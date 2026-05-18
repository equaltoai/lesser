package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	awsinit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type fakeDB struct{}

func (f *fakeDB) Model(_ any) dynamormCore.Query                      { return nil }
func (f *fakeDB) Transaction(_ func(tx *dynamormCore.Tx) error) error { return nil }
func (f *fakeDB) Migrate() error                                      { return nil }
func (f *fakeDB) AutoMigrate(_ ...any) error                          { return nil }
func (f *fakeDB) Close() error                                        { return nil }
func (f *fakeDB) WithContext(_ context.Context) dynamormCore.DB       { return f }

type fakeBedrockClient struct {
	output    *bedrock.GetModelCustomizationJobOutput
	err       error
	gotInputs []*bedrock.GetModelCustomizationJobInput
}

func (f *fakeBedrockClient) GetModelCustomizationJob(_ context.Context, params *bedrock.GetModelCustomizationJobInput, _ ...func(*bedrock.Options)) (*bedrock.GetModelCustomizationJobOutput, error) {
	f.gotInputs = append(f.gotInputs, params)
	return f.output, f.err
}

type fakeS3Client struct {
	getObjectFn func(ctx context.Context, params *s3.GetObjectInput) (*s3.GetObjectOutput, error)
	gotKeys     []string
	gotBuckets  []string
}

func (f *fakeS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.gotKeys = append(f.gotKeys, aws.ToString(params.Key))
	f.gotBuckets = append(f.gotBuckets, aws.ToString(params.Bucket))
	if f.getObjectFn != nil {
		return f.getObjectFn(ctx, params)
	}
	return nil, errors.New("GetObject not configured")
}

type fakeModerationMLRepo struct {
	createPollRequests []*models.MLPollRequest
	createPollErr      error

	gotTrainingJobIDs []string
	trainingJob       *models.ModelTrainingJob
	getTrainingJobErr error

	updatedTrainingJobs []*models.ModelTrainingJob
	updateTrainingErr   error

	createdModelVersions []*models.ModerationModelVersion
	createModelErr       error

	activeModel       *models.ModerationModelVersion
	getActiveModelErr error
	updatedModel      *models.ModerationModelVersion
	updateModelVerErr error
}

type errReadCloser struct{ err error }

func (e errReadCloser) Read(_ []byte) (int, error) { return 0, e.err }
func (e errReadCloser) Close() error               { return nil }

func (f *fakeModerationMLRepo) CreatePollRequest(_ context.Context, pollReq *models.MLPollRequest) error {
	f.createPollRequests = append(f.createPollRequests, pollReq)
	return f.createPollErr
}

func (f *fakeModerationMLRepo) GetTrainingJob(_ context.Context, jobARN string) (*models.ModelTrainingJob, error) {
	f.gotTrainingJobIDs = append(f.gotTrainingJobIDs, jobARN)
	if f.getTrainingJobErr != nil {
		return nil, f.getTrainingJobErr
	}
	if f.trainingJob != nil {
		return f.trainingJob, nil
	}
	return &models.ModelTrainingJob{JobID: jobARN}, nil
}

func (f *fakeModerationMLRepo) UpdateTrainingJob(_ context.Context, job *models.ModelTrainingJob) error {
	f.updatedTrainingJobs = append(f.updatedTrainingJobs, job)
	return f.updateTrainingErr
}

func (f *fakeModerationMLRepo) CreateModelVersion(_ context.Context, version *models.ModerationModelVersion) error {
	f.createdModelVersions = append(f.createdModelVersions, version)
	return f.createModelErr
}

func (f *fakeModerationMLRepo) GetActiveModelVersion(_ context.Context) (*models.ModerationModelVersion, error) {
	if f.getActiveModelErr != nil {
		return nil, f.getActiveModelErr
	}
	if f.activeModel != nil {
		return f.activeModel, nil
	}
	return nil, errors.New("no active model")
}

func (f *fakeModerationMLRepo) UpdateModelVersion(_ context.Context, version *models.ModerationModelVersion) error {
	f.updatedModel = version
	return f.updateModelVerErr
}

const testRequestID = "req"

func newEventCtx() *apptheory.EventContext {
	return &apptheory.EventContext{RequestID: testRequestID}
}

func makePollInsertRecord(poll models.MLPollRequest) events.DynamoDBEventRecord {
	now := time.Now()
	image := map[string]events.DynamoDBAttributeValue{
		"PK":            events.NewStringAttribute("MLPOLL#" + poll.JobID),
		"SK":            events.NewStringAttribute("REQUEST#" + strconv.FormatInt(now.UnixNano(), 10)),
		"type":          events.NewStringAttribute("ML_POLL_REQUEST"),
		"jobID":         events.NewStringAttribute(poll.JobID),
		"jobName":       events.NewStringAttribute(poll.JobName),
		"attempt":       events.NewNumberAttribute(strconv.Itoa(poll.Attempt)),
		"maxAttempts":   events.NewNumberAttribute(strconv.Itoa(poll.MaxAttempts)),
		"nextPollAfter": events.NewStringAttribute(poll.NextPollAfter.Format(time.RFC3339)),
		"status":        events.NewStringAttribute(poll.Status),
		"createdAt":     events.NewStringAttribute(now.Format(time.RFC3339)),
		"updatedAt":     events.NewStringAttribute(now.Format(time.RFC3339)),
	}
	return events.DynamoDBEventRecord{
		EventName: eventNameInsert,
		EventID:   "evt-1",
		Change: events.DynamoDBStreamRecord{
			NewImage: image,
		},
	}
}

func makeTrainingJobModifyRecord(job models.ModelTrainingJob, oldStatus, newStatus string) events.DynamoDBEventRecord {
	now := time.Now()
	base := map[string]events.DynamoDBAttributeValue{
		"PK":             events.NewStringAttribute("MLJOB#" + job.JobID),
		"SK":             events.NewStringAttribute("JOB"),
		"type":           events.NewStringAttribute("ML_TRAINING_JOB"),
		"jobID":          events.NewStringAttribute(job.JobID),
		"jobName":        events.NewStringAttribute(job.JobName),
		"modelARN":       events.NewStringAttribute(job.ModelARN),
		"errorMessage":   events.NewStringAttribute(job.ErrorMessage),
		"datasetSamples": events.NewNumberAttribute(strconv.Itoa(job.DatasetSamples)),
		"baseModelID":    events.NewStringAttribute(job.BaseModelID),
		"datasetS3Key":   events.NewStringAttribute(job.DatasetS3Key),
		"startedAt":      events.NewStringAttribute(now.Format(time.RFC3339)),
		"createdAt":      events.NewStringAttribute(now.Format(time.RFC3339)),
		"updatedAt":      events.NewStringAttribute(now.Format(time.RFC3339)),
		"metrics": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"accuracy":      events.NewNumberAttribute("0.91"),
			"precision":     events.NewNumberAttribute("0.92"),
			"recall":        events.NewNumberAttribute("0.93"),
			"f1_score":      events.NewNumberAttribute("0.94"),
			"training_time": events.NewNumberAttribute("100"),
		}),
	}

	oldImage := make(map[string]events.DynamoDBAttributeValue, len(base)+1)
	newImage := make(map[string]events.DynamoDBAttributeValue, len(base)+1)
	for k, v := range base {
		oldImage[k] = v
		newImage[k] = v
	}
	oldImage["status"] = events.NewStringAttribute(oldStatus)
	newImage["status"] = events.NewStringAttribute(newStatus)

	return events.DynamoDBEventRecord{
		EventName: eventNameModify,
		EventID:   "evt-2",
		Change: events.DynamoDBStreamRecord{
			NewImage: newImage,
			OldImage: oldImage,
		},
	}
}

func TestProcessPollRequest_SkipsNonPendingOrNotDue_Round12(t *testing.T) {
	repo := &fakeModerationMLRepo{}
	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		bedrockClient:    &fakeBedrockClient{err: errors.New("should not be called")},
		moderationMLRepo: repo,
	}
	ctx := context.Background()

	notPending := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "COMPLETED",
		Attempt:       0,
		MaxAttempts:   3,
		NextPollAfter: time.Now().Add(-time.Minute),
	})
	require.NoError(t, p.processPollRequest(ctx, testRequestID, notPending))
	require.Empty(t, repo.createPollRequests)

	notDue := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-2",
		JobName:       "j2",
		Status:        "PENDING",
		Attempt:       0,
		MaxAttempts:   3,
		NextPollAfter: time.Now().Add(time.Hour),
	})
	require.NoError(t, p.processPollRequest(ctx, testRequestID, notDue))
	require.Empty(t, repo.createPollRequests)
}

func TestProcessPollRequest_TimesOut_Round12(t *testing.T) {
	repo := &fakeModerationMLRepo{trainingJob: &models.ModelTrainingJob{JobID: "job-1"}}
	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		moderationMLRepo: repo,
	}
	ctx := context.Background()

	rec := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       3,
		MaxAttempts:   3,
		NextPollAfter: time.Now().Add(-time.Minute),
	})
	require.NoError(t, p.processPollRequest(ctx, testRequestID, rec))
	require.Len(t, repo.updatedTrainingJobs, 1)
	require.Equal(t, statusTimeout, repo.updatedTrainingJobs[0].Status)
	require.Contains(t, repo.updatedTrainingJobs[0].ErrorMessage, "timed out")
	require.False(t, repo.updatedTrainingJobs[0].CompletedAt.IsZero())
}

func TestProcessPollRequest_BedrockError_SchedulesNextPoll_Round12(t *testing.T) {
	repo := &fakeModerationMLRepo{}
	bedrockClient := &fakeBedrockClient{err: errors.New("bedrock down")}
	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		bedrockClient:    bedrockClient,
		moderationMLRepo: repo,
	}
	ctx := context.Background()

	rec := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       0,
		MaxAttempts:   5,
		NextPollAfter: time.Now().Add(-time.Minute),
	})

	require.NoError(t, p.processPollRequest(ctx, testRequestID, rec))
	require.Len(t, repo.createPollRequests, 1)
	require.Equal(t, 1, repo.createPollRequests[0].Attempt)
	require.Equal(t, "PENDING", repo.createPollRequests[0].Status)
	require.Len(t, bedrockClient.gotInputs, 1)
	require.Equal(t, "job-1", aws.ToString(bedrockClient.gotInputs[0].JobIdentifier))
}

func TestProcessPollRequest_StatusCompleted_ParsesMetricsFromS3_Round12(t *testing.T) {
	job := &models.ModelTrainingJob{JobID: "job-1", Status: statusInProgress}
	repo := &fakeModerationMLRepo{trainingJob: job}

	s3Client := &fakeS3Client{}
	s3Client.getObjectFn = func(_ context.Context, params *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
		key := aws.ToString(params.Key)
		if strings.HasSuffix(key, "/metrics.json") {
			return nil, errors.New("missing")
		}
		if strings.HasSuffix(key, "/training_results.json") {
			body := `{"accuracy":0.9,"precision":0.8,"recall":0.7,"f1_score":0.75}`
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		return nil, errors.New("unexpected key")
	}

	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		bedrockClient:    &fakeBedrockClient{},
		s3Client:         s3Client,
		moderationMLRepo: repo,
	}
	ctx := context.Background()

	createdAt := time.Now().Add(-2 * time.Minute)
	modifiedAt := time.Now()
	out := &bedrock.GetModelCustomizationJobOutput{
		Status:           types.ModelCustomizationJobStatusCompleted,
		OutputModelArn:   aws.String("arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/version-123"),
		CreationTime:     &createdAt,
		LastModifiedTime: &modifiedAt,
		OutputDataConfig: &types.OutputDataConfig{S3Uri: aws.String("s3://bucket/path/")},
	}
	p.bedrockClient.(*fakeBedrockClient).output = out

	rec := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       0,
		MaxAttempts:   5,
		NextPollAfter: time.Now().Add(-time.Minute),
	})

	require.NoError(t, p.processPollRequest(ctx, testRequestID, rec))
	require.Len(t, repo.updatedTrainingJobs, 1)
	require.Equal(t, statusCompleted, repo.updatedTrainingJobs[0].Status)
	require.Equal(t, "arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/version-123", repo.updatedTrainingJobs[0].ModelARN)
	require.False(t, repo.updatedTrainingJobs[0].CompletedAt.IsZero())
	require.GreaterOrEqual(t, repo.updatedTrainingJobs[0].Metrics.TrainingTime, 0)
	require.InDelta(t, 0.9, repo.updatedTrainingJobs[0].Metrics.Accuracy, 0.0001)
	require.InDelta(t, 0.8, repo.updatedTrainingJobs[0].Metrics.Precision, 0.0001)
	require.InDelta(t, 0.7, repo.updatedTrainingJobs[0].Metrics.Recall, 0.0001)
	require.InDelta(t, 0.75, repo.updatedTrainingJobs[0].Metrics.F1Score, 0.0001)
	require.Equal(t, []string{"bucket", "bucket"}, s3Client.gotBuckets)
}

func TestProcessPollRequest_InProgress_UpdatesAndSchedulesNextPoll_Round12(t *testing.T) {
	job := &models.ModelTrainingJob{JobID: "job-1", Status: "SUBMITTED"}
	repo := &fakeModerationMLRepo{trainingJob: job}
	bedrockClient := &fakeBedrockClient{
		output: &bedrock.GetModelCustomizationJobOutput{Status: types.ModelCustomizationJobStatusInProgress},
	}

	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		bedrockClient:    bedrockClient,
		moderationMLRepo: repo,
	}
	ctx := context.Background()

	rec := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       2,
		MaxAttempts:   5,
		NextPollAfter: time.Now().Add(-time.Minute),
	})

	require.NoError(t, p.processPollRequest(ctx, testRequestID, rec))
	require.Len(t, repo.updatedTrainingJobs, 1)
	require.Equal(t, statusInProgress, repo.updatedTrainingJobs[0].Status)
	require.Len(t, repo.createPollRequests, 1)
	require.Equal(t, 3, repo.createPollRequests[0].Attempt)
}

func TestProcessPollRequest_UnexpectedStatus_SchedulesNextPoll_Round12(t *testing.T) {
	repo := &fakeModerationMLRepo{}
	bedrockClient := &fakeBedrockClient{
		output: &bedrock.GetModelCustomizationJobOutput{Status: types.ModelCustomizationJobStatus("WEIRD")},
	}
	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		bedrockClient:    bedrockClient,
		moderationMLRepo: repo,
	}
	ctx := context.Background()

	rec := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       0,
		MaxAttempts:   5,
		NextPollAfter: time.Now().Add(-time.Minute),
	})
	require.NoError(t, p.processPollRequest(ctx, testRequestID, rec))
	require.Len(t, repo.createPollRequests, 1)
}

func TestUpdateJobStatus_TrainingMetricsBranch_Round12(t *testing.T) {
	job := &models.ModelTrainingJob{JobID: "job-1", Status: "SUBMITTED"}
	repo := &fakeModerationMLRepo{trainingJob: job}

	p := &MLTrainingProcessor{
		logger: zaptest.NewLogger(t),
		s3Client: &fakeS3Client{getObjectFn: func(context.Context, *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			t.Fatal("unexpected S3 access")
			return nil, nil
		}},
		moderationMLRepo: repo,
	}

	loss := float32(0.25)
	out := &bedrock.GetModelCustomizationJobOutput{
		OutputModelArn: aws.String("arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/ver"),
		TrainingMetrics: &types.TrainingMetrics{
			TrainingLoss: &loss,
		},
	}

	require.NoError(t, p.updateJobStatus(context.Background(), "job-1", statusCompleted, out))
	require.Len(t, repo.updatedTrainingJobs, 1)
	require.Equal(t, statusCompleted, repo.updatedTrainingJobs[0].Status)
	require.Equal(t, "arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/ver", repo.updatedTrainingJobs[0].ModelARN)
}

func TestHandleJobCompletion_AndFailure_Branches_Round12(t *testing.T) {
	originalWrite := writeStreamingEventFn
	t.Cleanup(func() { writeStreamingEventFn = originalWrite })

	repo := &fakeModerationMLRepo{
		activeModel:       &models.ModerationModelVersion{VersionID: "v-old", IsActive: true},
		updateModelVerErr: errors.New("cannot update"),
	}

	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		moderationMLRepo: repo,
	}

	writeStreamingEventFn = func(_ context.Context, _ dynamormCore.DB, _ *models.StreamingEvent) error {
		return errors.New("write failed")
	}

	job := &models.ModelTrainingJob{
		JobID:          "job-1",
		JobName:        "name",
		Status:         statusCompleted,
		BaseModelID:    "base",
		DatasetS3Key:   "k",
		DatasetSamples: 10,
		ModelARN:       "arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/version-456",
		Metrics: models.TrainingMetrics{
			Accuracy:     0.9,
			Precision:    0.8,
			Recall:       0.7,
			F1Score:      0.75,
			TrainingTime: 123,
		},
	}

	require.NoError(t, p.handleJobCompletion(context.Background(), testRequestID, job))
	require.Len(t, repo.createdModelVersions, 1)
	require.Equal(t, "version-456", repo.createdModelVersions[0].VersionID)
	require.True(t, repo.createdModelVersions[0].IsActive)
	require.Equal(t, statusCompleted, repo.createdModelVersions[0].TrainingStatus)

	failedJob := &models.ModelTrainingJob{JobID: "job-2", JobName: "name2", ErrorMessage: "bad"}
	require.NoError(t, p.handleJobFailure(context.Background(), testRequestID, failedJob))
}

func TestProcessRecord_AndHandleStream_Round12(t *testing.T) {
	originalWrite := writeStreamingEventFn
	t.Cleanup(func() { writeStreamingEventFn = originalWrite })
	writeStreamingEventFn = func(_ context.Context, _ dynamormCore.DB, _ *models.StreamingEvent) error { return nil }

	repo := &fakeModerationMLRepo{trainingJob: &models.ModelTrainingJob{JobID: "job-1"}}
	p := &MLTrainingProcessor{
		db:               &fakeDB{},
		logger:           zaptest.NewLogger(t),
		bedrockClient:    &fakeBedrockClient{output: &bedrock.GetModelCustomizationJobOutput{Status: types.ModelCustomizationJobStatusInProgress}},
		moderationMLRepo: repo,
	}

	ctx := context.Background()

	// Unknown entity type is ignored.
	ignored := events.DynamoDBEventRecord{
		EventName: eventNameInsert,
		EventID:   "evt-ignore",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("UNKNOWN#1"),
			},
		},
	}
	require.NoError(t, p.processRecord(ctx, testRequestID, ignored))

	// Poll insert routes through processPollRequest and schedules next poll.
	poll := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       0,
		MaxAttempts:   5,
		NextPollAfter: time.Now().Add(-time.Minute),
	})

	// Job status change routes through processJobStatusChange and triggers completion path.
	job := models.ModelTrainingJob{
		JobID:          "job-1",
		JobName:        "name",
		ModelARN:       "arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/version-123",
		DatasetSamples: 1,
	}
	jobStatusChange := makeTrainingJobModifyRecord(job, statusInProgress, statusCompleted)

	require.NoError(t, p.processRecord(ctx, testRequestID, poll))
	require.NoError(t, p.processRecord(ctx, testRequestID, jobStatusChange))
	require.NotEmpty(t, repo.createPollRequests)
}

func TestParseBedrockMetricsJSON_Branches_Round12(t *testing.T) {
	_, err := parseBedrockMetricsJSON("{bad json")
	require.Error(t, err)

	metrics, err := parseBedrockMetricsJSON(`{"accuracy":0.9}`)
	require.NoError(t, err)
	require.InDelta(t, 0.9, metrics.Accuracy, 0.0001)

	metrics, err = parseBedrockMetricsJSON(`{"validation_metrics":{"precision":0.8}}`)
	require.NoError(t, err)
	require.InDelta(t, 0.8, metrics.Precision, 0.0001)

	metrics, err = parseBedrockMetricsJSON(`{"evaluation":{"recall":0.7,"f1":0.6}}`)
	require.NoError(t, err)
	require.InDelta(t, 0.7, metrics.Recall, 0.0001)
	require.InDelta(t, 0.6, metrics.F1Score, 0.0001)

	_, err = parseBedrockMetricsJSON(`{"accuracy":0,"precision":0}`)
	require.Error(t, err)
}

func TestNewMLTrainingProcessor_AndMain_Round12(t *testing.T) {
	originalLambdaCtx := lambdaCtx
	originalNewBedrock := newBedrockClientFn
	originalNewS3 := newS3ClientFn
	originalNewRepo := newModerationMLRepoFn
	originalLambdaStart := lambdaStartFn
	originalProcessor := processor
	t.Cleanup(func() {
		lambdaCtx = originalLambdaCtx
		newBedrockClientFn = originalNewBedrock
		newS3ClientFn = originalNewS3
		newModerationMLRepoFn = originalNewRepo
		lambdaStartFn = originalLambdaStart
		processor = originalProcessor
	})

	fakeBedrock := &fakeBedrockClient{}
	fakeS3 := &fakeS3Client{}
	fakeRepo := &fakeModerationMLRepo{}

	lambdaCtx = &common.LambdaContext{
		Config: &config.Config{DynamoTableName: "test-table"},
		Logger: zaptest.NewLogger(t),
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
		DynamoDB: &fakeDB{},
	}

	newBedrockClientFn = func(aws.Config) bedrockJobGetter { return fakeBedrock }
	newS3ClientFn = func(aws.Config) s3ObjectGetter { return fakeS3 }
	newModerationMLRepoFn = func(dynamormCore.DB, string, *zap.Logger) moderationMLRepository { return fakeRepo }

	p, err := NewMLTrainingProcessor()
	require.NoError(t, err)
	require.Equal(t, "test-table", p.tableName)
	require.Same(t, fakeBedrock, p.bedrockClient)
	require.Same(t, fakeS3, p.s3Client)
	require.Same(t, fakeRepo, p.moderationMLRepo)

	called := false
	lambdaStartFn = func(h any) {
		called = true
		fn, ok := h.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "1",
				EventName:      "INSERT",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("UNKNOWN#1"),
					},
				},
			},
		}}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.DynamoDBEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}
	processor = p
	main()
	require.True(t, called)
}

func TestInitializeMLTraining_Branches_Round12(t *testing.T) {
	originalRunning := runningUnitTestsFn
	originalMustInit := mustInitializeLambdaFn
	originalNewProc := newMLTrainingProcessorFn
	originalLambdaCtx := lambdaCtx
	originalProcessor := processor
	t.Cleanup(func() {
		runningUnitTestsFn = originalRunning
		mustInitializeLambdaFn = originalMustInit
		newMLTrainingProcessorFn = originalNewProc
		lambdaCtx = originalLambdaCtx
		processor = originalProcessor
	})

	fakeLambda := &common.LambdaContext{
		Config: &config.Config{DynamoTableName: "tbl"},
		Logger: zaptest.NewLogger(t),
		AWSServices: &awsinit.AWSServices{
			Config: aws.Config{Region: "us-east-1"},
		},
		DynamoDB: &fakeDB{},
	}

	mustInitializeLambdaFn = func(_ common.LambdaConfig) *common.LambdaContext { return fakeLambda }
	newMLTrainingProcessorFn = func() (*MLTrainingProcessor, error) {
		return &MLTrainingProcessor{logger: zaptest.NewLogger(t)}, nil
	}

	require.NoError(t, initializeMLTraining())
	require.NotNil(t, lambdaCtx)
	require.NotNil(t, processor)

	newMLTrainingProcessorFn = func() (*MLTrainingProcessor, error) { return nil, errors.New("boom") }
	require.Error(t, initializeMLTraining())

	runningUnitTestsFn = func() bool { return true }
	initializeMLTrainingOnStart()

	runningUnitTestsFn = func() bool { return false }
	newMLTrainingProcessorFn = func() (*MLTrainingProcessor, error) {
		return &MLTrainingProcessor{logger: zaptest.NewLogger(t)}, nil
	}
	initializeMLTrainingOnStart()
}

func TestGetStatusFromStreamImage_Branches_Round12(t *testing.T) {
	require.Equal(t, "COMPLETED", getStatusFromStreamImage(map[string]events.DynamoDBAttributeValue{
		"status": events.NewStringAttribute("COMPLETED"),
	}))
	require.Equal(t, "FAILED", getStatusFromStreamImage(map[string]events.DynamoDBAttributeValue{
		"Status": events.NewStringAttribute("FAILED"),
	}))
	require.Equal(t, "", getStatusFromStreamImage(map[string]events.DynamoDBAttributeValue{}))
	require.Equal(t, "", getStatusFromStreamImage(map[string]events.DynamoDBAttributeValue{
		"status": events.NewNumberAttribute("1"),
	}))
}

func TestExtractVersionFromARN_Fallbacks_Round12(t *testing.T) {
	require.Equal(t, "abcd1234", extractVersionFromARN("arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/abcd1234"))
	require.True(t, strings.HasPrefix(extractVersionFromARN(""), "v"))
	require.True(t, strings.HasPrefix(extractVersionFromARN("arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/"), "v"))
}

func TestScheduleNextPoll_ErrorBranch_Round12(t *testing.T) {
	repo := &fakeModerationMLRepo{createPollErr: errors.New("nope")}
	p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
	err := p.scheduleNextPoll(models.MLPollRequest{JobID: "job-1", JobName: "j1", Attempt: 0, MaxAttempts: 3}, time.Second)
	require.Error(t, err)
	require.Len(t, repo.createPollRequests, 1)
}

func TestMarkJobAsTimeout_ErrorBranches_Round12(t *testing.T) {
	t.Run("get_training_job_error", func(t *testing.T) {
		repo := &fakeModerationMLRepo{getTrainingJobErr: errors.New("no job")}
		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
		p.markJobAsTimeout(context.Background(), "job-1")
		require.Empty(t, repo.updatedTrainingJobs)
	})

	t.Run("update_training_job_error", func(t *testing.T) {
		repo := &fakeModerationMLRepo{
			trainingJob:       &models.ModelTrainingJob{JobID: "job-1"},
			updateTrainingErr: errors.New("update failed"),
		}
		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
		p.markJobAsTimeout(context.Background(), "job-1")
		require.Len(t, repo.updatedTrainingJobs, 1)
		require.Equal(t, statusTimeout, repo.updatedTrainingJobs[0].Status)
	})
}

func TestUpdateJobStatus_ErrorAndDefaultBranches_Round12(t *testing.T) {
	t.Run("get_training_job_error", func(t *testing.T) {
		repo := &fakeModerationMLRepo{getTrainingJobErr: errors.New("missing")}
		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
		err := p.updateJobStatus(context.Background(), "job-1", statusCompleted, &bedrock.GetModelCustomizationJobOutput{})
		require.Error(t, err)
	})

	t.Run("update_training_job_error", func(t *testing.T) {
		repo := &fakeModerationMLRepo{
			trainingJob:       &models.ModelTrainingJob{JobID: "job-1"},
			updateTrainingErr: errors.New("update failed"),
		}
		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
		err := p.updateJobStatus(context.Background(), "job-1", statusCompleted, &bedrock.GetModelCustomizationJobOutput{})
		require.Error(t, err)
	})

	t.Run("parse_metrics_failure_uses_defaults", func(t *testing.T) {
		job := &models.ModelTrainingJob{
			JobID:   "job-1",
			Status:  statusInProgress,
			Metrics: models.TrainingMetrics{Accuracy: 0.1, Precision: 0.2, Recall: 0.3, F1Score: 0.4},
		}
		repo := &fakeModerationMLRepo{trainingJob: job}
		s3Client := &fakeS3Client{
			getObjectFn: func(context.Context, *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return nil, errors.New("not found")
			},
		}

		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo, s3Client: s3Client}
		out := &bedrock.GetModelCustomizationJobOutput{
			OutputDataConfig: &types.OutputDataConfig{S3Uri: aws.String("s3://bucket/prefix/")},
		}

		require.NoError(t, p.updateJobStatus(context.Background(), "job-1", statusCompleted, out))
		require.Len(t, repo.updatedTrainingJobs, 1)
		require.Equal(t, 0.0, repo.updatedTrainingJobs[0].Metrics.Accuracy)
		require.Equal(t, 0.0, repo.updatedTrainingJobs[0].Metrics.Precision)
		require.Equal(t, 0.0, repo.updatedTrainingJobs[0].Metrics.Recall)
		require.Equal(t, 0.0, repo.updatedTrainingJobs[0].Metrics.F1Score)
	})

	t.Run("no_metrics_available_defaults", func(t *testing.T) {
		repo := &fakeModerationMLRepo{trainingJob: &models.ModelTrainingJob{JobID: "job-1"}}
		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
		require.NoError(t, p.updateJobStatus(context.Background(), "job-1", statusCompleted, &bedrock.GetModelCustomizationJobOutput{}))
		require.Len(t, repo.updatedTrainingJobs, 1)
		require.Equal(t, 0.0, repo.updatedTrainingJobs[0].Metrics.Accuracy)
	})

	t.Run("failure_message_sets_error", func(t *testing.T) {
		repo := &fakeModerationMLRepo{trainingJob: &models.ModelTrainingJob{JobID: "job-1"}}
		p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: repo}
		out := &bedrock.GetModelCustomizationJobOutput{FailureMessage: aws.String("nope")}
		require.NoError(t, p.updateJobStatus(context.Background(), "job-1", statusFailed, out))
		require.Len(t, repo.updatedTrainingJobs, 1)
		require.Equal(t, statusFailed, repo.updatedTrainingJobs[0].Status)
		require.Equal(t, "nope", repo.updatedTrainingJobs[0].ErrorMessage)
	})
}

func TestProcessPollRequest_AdditionalStatusAndErrorBranches_Round12(t *testing.T) {
	t.Run("failed_and_stopped_mark_failed", func(t *testing.T) {
		job := &models.ModelTrainingJob{JobID: "job-1", Status: statusInProgress}
		repo := &fakeModerationMLRepo{trainingJob: job}
		p := &MLTrainingProcessor{
			logger:           zaptest.NewLogger(t),
			bedrockClient:    &fakeBedrockClient{},
			moderationMLRepo: repo,
		}
		p.bedrockClient.(*fakeBedrockClient).output = &bedrock.GetModelCustomizationJobOutput{Status: types.ModelCustomizationJobStatusFailed}
		require.NoError(t, p.processPollRequest(context.Background(), testRequestID, makePollInsertRecord(models.MLPollRequest{
			JobID:         "job-1",
			JobName:       "j1",
			Status:        "PENDING",
			Attempt:       0,
			MaxAttempts:   5,
			NextPollAfter: time.Now().Add(-time.Minute),
		})))
		require.Len(t, repo.updatedTrainingJobs, 1)
		require.Equal(t, statusFailed, repo.updatedTrainingJobs[0].Status)

		repo.updatedTrainingJobs = nil
		p.bedrockClient.(*fakeBedrockClient).output = &bedrock.GetModelCustomizationJobOutput{Status: types.ModelCustomizationJobStatusStopped}
		require.NoError(t, p.processPollRequest(context.Background(), testRequestID, makePollInsertRecord(models.MLPollRequest{
			JobID:         "job-1",
			JobName:       "j1",
			Status:        "PENDING",
			Attempt:       0,
			MaxAttempts:   5,
			NextPollAfter: time.Now().Add(-time.Minute),
		})))
		require.Len(t, repo.updatedTrainingJobs, 1)
		require.Equal(t, statusFailed, repo.updatedTrainingJobs[0].Status)
	})

	t.Run("update_job_status_error_propagates", func(t *testing.T) {
		repo := &fakeModerationMLRepo{getTrainingJobErr: errors.New("missing")}
		p := &MLTrainingProcessor{
			logger:           zaptest.NewLogger(t),
			bedrockClient:    &fakeBedrockClient{output: &bedrock.GetModelCustomizationJobOutput{Status: types.ModelCustomizationJobStatusInProgress}},
			moderationMLRepo: repo,
		}
		err := p.processPollRequest(context.Background(), testRequestID, makePollInsertRecord(models.MLPollRequest{
			JobID:         "job-1",
			JobName:       "j1",
			Status:        "PENDING",
			Attempt:       0,
			MaxAttempts:   5,
			NextPollAfter: time.Now().Add(-time.Minute),
		}))
		require.Error(t, err)
	})
}

func TestHandleStream_PartialFailureStillReturnsNil_Round12(t *testing.T) {
	repo := &fakeModerationMLRepo{createPollErr: errors.New("fail")}
	p := &MLTrainingProcessor{
		logger:           zaptest.NewLogger(t),
		bedrockClient:    &fakeBedrockClient{err: errors.New("bedrock down")},
		moderationMLRepo: repo,
	}

	rec := makePollInsertRecord(models.MLPollRequest{
		JobID:         "job-1",
		JobName:       "j1",
		Status:        "PENDING",
		Attempt:       0,
		MaxAttempts:   5,
		NextPollAfter: time.Now().Add(-time.Minute),
	})

	require.NoError(t, p.HandleDynamoDBRecord(newEventCtx(), rec))
}

func TestProcessRecord_MissingPK_Skips_Round12(t *testing.T) {
	p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), moderationMLRepo: &fakeModerationMLRepo{}}
	rec := events.DynamoDBEventRecord{EventName: eventNameInsert, EventID: "evt", Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}}}
	require.NoError(t, p.processRecord(context.Background(), testRequestID, rec))
}

func TestProcessJobStatusChange_Branches_Round12(t *testing.T) {
	originalWrite := writeStreamingEventFn
	t.Cleanup(func() { writeStreamingEventFn = originalWrite })
	writeStreamingEventFn = func(_ context.Context, _ dynamormCore.DB, _ *models.StreamingEvent) error { return nil }

	p := &MLTrainingProcessor{
		db:               &fakeDB{},
		logger:           zaptest.NewLogger(t),
		moderationMLRepo: &fakeModerationMLRepo{},
	}

	ctx := context.Background()

	job := models.ModelTrainingJob{
		JobID:          "job-1",
		JobName:        "name",
		ModelARN:       "arn:aws:bedrock:us-east-1:123:custom-model/foo/bar/version-123",
		DatasetSamples: 1,
	}

	t.Run("no_status_change", func(t *testing.T) {
		rec := makeTrainingJobModifyRecord(job, statusInProgress, statusInProgress)
		require.NoError(t, p.processJobStatusChange(ctx, testRequestID, rec))
	})

	t.Run("failed_status", func(t *testing.T) {
		rec := makeTrainingJobModifyRecord(job, statusInProgress, statusFailed)
		require.NoError(t, p.processJobStatusChange(ctx, testRequestID, rec))
	})

	t.Run("default_status_change", func(t *testing.T) {
		rec := makeTrainingJobModifyRecord(job, "SUBMITTED", statusInProgress)
		require.NoError(t, p.processJobStatusChange(ctx, testRequestID, rec))
	})
}

func TestParseMetricsFromS3_ErrorBranches_Round12(t *testing.T) {
	p := &MLTrainingProcessor{logger: zaptest.NewLogger(t), s3Client: &fakeS3Client{}}

	_, err := p.parseMetricsFromS3(context.Background(), "s3://bucket-only")
	require.Error(t, err)

	readErrClient := &fakeS3Client{
		getObjectFn: func(context.Context, *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: errReadCloser{err: errors.New("read failed")}}, nil
		},
	}
	p.s3Client = readErrClient
	_, err = p.parseMetricsFromS3(context.Background(), "s3://bucket/prefix/")
	require.Error(t, err)

	parseErrClient := &fakeS3Client{
		getObjectFn: func(_ context.Context, params *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			key := aws.ToString(params.Key)
			if strings.HasSuffix(key, "/metrics.json") {
				return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(`{"accuracy":0}`))}, nil
			}
			return nil, errors.New("not found")
		},
	}
	p.s3Client = parseErrClient
	_, err = p.parseMetricsFromS3(context.Background(), "s3://bucket/prefix/")
	require.Error(t, err)
}

func TestMetricsExtractionHelpers_Branches_Round12(t *testing.T) {
	p := &MLTrainingProcessor{logger: zaptest.NewLogger(t)}

	t.Run("extract_float_metric", func(t *testing.T) {
		metrics := &models.TrainingMetrics{}
		ok := p.extractFloatMetric(map[string]interface{}{"accuracy": 0.9}, "accuracy", &metrics.Accuracy)
		require.True(t, ok)
		require.InDelta(t, 0.9, metrics.Accuracy, 0.0001)
		ok = p.extractFloatMetric(map[string]interface{}{}, "accuracy", &metrics.Accuracy)
		require.False(t, ok)
		ok = p.extractFloatMetric(map[string]interface{}{"accuracy": "0.9"}, "accuracy", &metrics.Accuracy)
		require.False(t, ok)
	})

	t.Run("extract_nested_metrics", func(t *testing.T) {
		metrics := &models.TrainingMetrics{}
		p.extractNestedMetrics(map[string]interface{}{
			"validation_metrics": map[string]interface{}{"accuracy": 0.8},
		}, "validation_metrics", metrics)
		require.InDelta(t, 0.8, metrics.Accuracy, 0.0001)

		metrics = &models.TrainingMetrics{}
		p.extractNestedMetrics(map[string]interface{}{
			"validation_metrics": "not-a-map",
		}, "validation_metrics", metrics)
		require.Equal(t, 0.0, metrics.Accuracy)
	})

	t.Run("marshal_raw_metrics_error", func(t *testing.T) {
		nan := float32(math.NaN())
		_, err := p.marshalToRawMetrics(&types.TrainingMetrics{TrainingLoss: &nan})
		require.Error(t, err)
	})
}

func TestInitializeMLTrainingStorage_FailsClosed(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() { newLambdaOptimizedClientFn = origNewClient })

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return nil, errors.New("storage unavailable")
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
		Logger: zap.NewNop(),
	}
	err := initializeMLTrainingStorage(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "storage client initialization failed")
}

func TestInitializeMLTrainingStorage_CreatesClient(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() { newLambdaOptimizedClientFn = origNewClient })

	db := &fakeDB{}
	var gotRegion string
	newLambdaOptimizedClientFn = func(_ context.Context, region string) (dynamormCore.DB, error) {
		gotRegion = region
		return db, nil
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "ml-table"},
		Logger: zap.NewNop(),
	}
	err := initializeMLTrainingStorage(ctx)
	require.NoError(t, err)
	require.Same(t, db, ctx.DynamoDB)
	require.Equal(t, "us-east-1", gotRegion)
}

func TestNewMLTrainingProcessor_FailsClosedWithoutStorage(t *testing.T) {
	origLambdaCtx := lambdaCtx
	t.Cleanup(func() { lambdaCtx = origLambdaCtx })

	lambdaCtx = &common.LambdaContext{
		Config:      &config.Config{DynamoTableName: "test-table"},
		Logger:      zap.NewNop(),
		AWSServices: &awsinit.AWSServices{Config: aws.Config{Region: "us-east-1"}},
	}
	_, err := NewMLTrainingProcessor()
	require.Error(t, err)
	require.Contains(t, err.Error(), "dynamodb client is not initialized")
}
