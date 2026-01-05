package moderationml

import (
	"context"
	stderrors "errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeModerationRepo struct {
	createSampleErr error
	sampleSeq       int
	createSamples   []*models.ModerationSample

	samplesByID map[string]*models.ModerationSample
	getSampleErr map[string]error

	activeModel    *models.ModerationModelVersion
	activeModelErr error

	createPredictionErr error
	predictionsCreated  []*models.MLPrediction

	effectivenessMetric    *models.ModerationEffectivenessMetric
	effectivenessMetricErr error

	predictionsByModel []*models.MLPrediction
	predictionsErr     error

	createMetricErr error
	metricsCreated  []*models.ModerationEffectivenessMetric

	createTrainingJobErr error
	trainingJobsCreated  []*models.ModelTrainingJob

	createPollRequestErr error
	pollRequestsCreated  []*models.MLPollRequest
}

func (r *fakeModerationRepo) CreateSample(_ context.Context, sample *models.ModerationSample) error {
	r.createSamples = append(r.createSamples, sample)
	if r.createSampleErr != nil {
		return r.createSampleErr
	}
	r.sampleSeq++
	sample.ID = "sample-" + string(rune('0'+r.sampleSeq))
	if r.samplesByID == nil {
		r.samplesByID = map[string]*models.ModerationSample{}
	}
	r.samplesByID[sample.ID] = sample
	return nil
}

func (r *fakeModerationRepo) GetSample(_ context.Context, sampleID string) (*models.ModerationSample, error) {
	if err := r.getSampleErr[sampleID]; err != nil {
		return nil, err
	}
	sample, ok := r.samplesByID[sampleID]
	if !ok {
		return nil, stderrors.New("not found")
	}
	return sample, nil
}

func (r *fakeModerationRepo) GetActiveModelVersion(context.Context) (*models.ModerationModelVersion, error) {
	if r.activeModelErr != nil {
		return nil, r.activeModelErr
	}
	return r.activeModel, nil
}

func (r *fakeModerationRepo) CreatePrediction(_ context.Context, prediction *models.MLPrediction) error {
	r.predictionsCreated = append(r.predictionsCreated, prediction)
	return r.createPredictionErr
}

func (r *fakeModerationRepo) GetEffectivenessMetric(context.Context, string, string, time.Time) (*models.ModerationEffectivenessMetric, error) {
	if r.effectivenessMetricErr != nil {
		return nil, r.effectivenessMetricErr
	}
	return r.effectivenessMetric, nil
}

func (r *fakeModerationRepo) GetPredictionsByModelVersion(context.Context, string, time.Time, time.Time, int) ([]*models.MLPrediction, error) {
	if r.predictionsErr != nil {
		return nil, r.predictionsErr
	}
	return r.predictionsByModel, nil
}

func (r *fakeModerationRepo) CreateEffectivenessMetric(_ context.Context, metric *models.ModerationEffectivenessMetric) error {
	r.metricsCreated = append(r.metricsCreated, metric)
	return r.createMetricErr
}

func (r *fakeModerationRepo) CreateTrainingJob(_ context.Context, job *models.ModelTrainingJob) error {
	r.trainingJobsCreated = append(r.trainingJobsCreated, job)
	return r.createTrainingJobErr
}

func (r *fakeModerationRepo) CreatePollRequest(_ context.Context, request *models.MLPollRequest) error {
	r.pollRequestsCreated = append(r.pollRequestsCreated, request)
	return r.createPollRequestErr
}

type fakeStatusRepo struct {
	byID map[string]*models.Status
	err  error
}

func (r *fakeStatusRepo) GetStatus(context.Context, string) (*models.Status, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byID["status1"], nil
}

type fakeS3Client struct {
	putCalls []fakePutCall
	err      error
}

type fakePutCall struct {
	bucket      string
	key         string
	contentType string
	body        string
}

func (c *fakeS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	c.putCalls = append(c.putCalls, fakePutCall{
		bucket:      aws.ToString(params.Bucket),
		key:         aws.ToString(params.Key),
		contentType: aws.ToString(params.ContentType),
		body:        string(data),
	})
	if c.err != nil {
		return nil, c.err
	}
	return &s3.PutObjectOutput{}, nil
}

type fakeBedrockClient struct {
	calls []*bedrock.CreateModelCustomizationJobInput
	out   *bedrock.CreateModelCustomizationJobOutput
	err   error
}

func (c *fakeBedrockClient) CreateModelCustomizationJob(_ context.Context, params *bedrock.CreateModelCustomizationJobInput, _ ...func(*bedrock.Options)) (*bedrock.CreateModelCustomizationJobOutput, error) {
	c.calls = append(c.calls, params)
	if c.err != nil {
		return nil, c.err
	}
	if c.out != nil {
		return c.out, nil
	}
	return &bedrock.CreateModelCustomizationJobOutput{JobArn: aws.String("arn:job")}, nil
}

type fakeBedrockRuntime struct {
	calls []*bedrockruntime.InvokeModelInput
	out   *bedrockruntime.InvokeModelOutput
	err   error
}

func (r *fakeBedrockRuntime) InvokeModel(_ context.Context, params *bedrockruntime.InvokeModelInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	r.calls = append(r.calls, params)
	if r.err != nil {
		return nil, r.err
	}
	if r.out != nil {
		return r.out, nil
	}
	return &bedrockruntime.InvokeModelOutput{Body: []byte(`{"completion":" clean"}`), ContentType: aws.String("application/json")}, nil
}

func TestService_QueueSamples_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("unsupported_type_errors", func(t *testing.T) {
		svc := &Service{repo: &fakeModerationRepo{}, logger: zap.NewNop()}
		_, err := svc.QueueSamples(ctx, []SampleInput{{ObjectID: "x", ObjectType: "unknown", Label: "spam"}})
		require.Error(t, err)
	})

	t.Run("status_repo_missing_errors", func(t *testing.T) {
		svc := &Service{repo: &fakeModerationRepo{}, logger: zap.NewNop()}
		_, err := svc.QueueSamples(ctx, []SampleInput{{ObjectID: "status1", ObjectType: "status", Label: "spam"}})
		require.Error(t, err)
	})

	t.Run("status_content_from_note_with_summary", func(t *testing.T) {
		repo := &fakeModerationRepo{}
		status := &models.Status{
			Content: "",
			Note: &models.NoteField{
				Note: &activitypub.Note{
					BaseObject: activitypub.BaseObject{Summary: "cw"},
					Content:    "hello",
				},
			},
		}
		svc := &Service{
			repo:       repo,
			statusRepo: &fakeStatusRepo{byID: map[string]*models.Status{"status1": status}},
			logger:     zap.NewNop(),
		}

		ids, err := svc.QueueSamples(ctx, []SampleInput{{ObjectID: "status1", ObjectType: "status", Label: "spam", Metadata: map[string]interface{}{"k": "v"}}})
		require.NoError(t, err)
		require.Len(t, ids, 1)
		require.Len(t, repo.createSamples, 1)
		assert.Equal(t, "cw\n\nhello", repo.createSamples[0].Metadata["content"])
	})

	t.Run("status_no_content_errors", func(t *testing.T) {
		status := &models.Status{Content: ""}
		svc := &Service{
			repo:       &fakeModerationRepo{},
			statusRepo: &fakeStatusRepo{byID: map[string]*models.Status{"status1": status}},
			logger:     zap.NewNop(),
		}
		_, err := svc.QueueSamples(ctx, []SampleInput{{ObjectID: "status1", ObjectType: "status", Label: "spam"}})
		require.Error(t, err)
	})
}

func TestService_prepareTrainingDataset_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("no_valid_samples_errors", func(t *testing.T) {
		repo := &fakeModerationRepo{samplesByID: map[string]*models.ModerationSample{}}
		svc := &Service{repo: repo, s3Client: &fakeS3Client{}, trainingBucket: "bucket", logger: zap.NewNop()}
		_, err := svc.prepareTrainingDataset(ctx, []string{"s1"}, "data.jsonl")
		require.Error(t, err)
	})

	t.Run("sample_missing_content_errors", func(t *testing.T) {
		repo := &fakeModerationRepo{
			samplesByID: map[string]*models.ModerationSample{
				"s1": {ID: "s1", Label: "spam", Metadata: map[string]interface{}{}},
			},
		}
		svc := &Service{repo: repo, s3Client: &fakeS3Client{}, trainingBucket: "bucket", logger: zap.NewNop()}
		_, err := svc.prepareTrainingDataset(ctx, []string{"s1"}, "data.jsonl")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no content")
	})

	t.Run("uploads_jsonl_and_returns_provided_key", func(t *testing.T) {
		s3Client := &fakeS3Client{}
		repo := &fakeModerationRepo{
			samplesByID: map[string]*models.ModerationSample{
				"s1": {ID: "s1", Label: "spam", Metadata: map[string]interface{}{"content": "hello"}},
			},
		}
		svc := &Service{repo: repo, s3Client: s3Client, trainingBucket: "bucket", logger: zap.NewNop()}
		key, err := svc.prepareTrainingDataset(ctx, []string{"s1"}, "data.jsonl")
		require.NoError(t, err)
		assert.Equal(t, "data.jsonl", key)
		require.Len(t, s3Client.putCalls, 1)
		assert.Equal(t, "bucket", s3Client.putCalls[0].bucket)
		assert.Equal(t, "data.jsonl", s3Client.putCalls[0].key)
		assert.Equal(t, "application/jsonlines", s3Client.putCalls[0].contentType)
		assert.Contains(t, s3Client.putCalls[0].body, "\"completion\"")
	})

	t.Run("generates_key_when_empty", func(t *testing.T) {
		s3Client := &fakeS3Client{}
		repo := &fakeModerationRepo{
			samplesByID: map[string]*models.ModerationSample{
				"s1": {ID: "s1", Label: "spam", Metadata: map[string]interface{}{"content": "hello"}},
			},
		}
		svc := &Service{repo: repo, s3Client: s3Client, trainingBucket: "bucket", logger: zap.NewNop()}
		key, err := svc.prepareTrainingDataset(ctx, []string{"s1"}, "")
		require.NoError(t, err)
		assert.Contains(t, key, "training-data/moderation-")
		require.Len(t, s3Client.putCalls, 1)
		assert.Equal(t, key, s3Client.putCalls[0].key)
	})
}

func TestService_launchBedrockTraining_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("missing_role_arn_errors", func(t *testing.T) {
		svc := &Service{bedrockClient: &fakeBedrockClient{}, trainingBucket: "bucket", logger: zap.NewNop()}
		_, _, err := svc.launchBedrockTraining(ctx, TrainingOptions{BaseModelID: "base", DatasetS3Path: "data.jsonl"})
		require.Error(t, err)
	})

	t.Run("uses_default_hyperparams_and_custom_output_path", func(t *testing.T) {
		bedrockClient := &fakeBedrockClient{}
		svc := &Service{bedrockClient: bedrockClient, trainingBucket: "bucket", roleARN: "arn:role", logger: zap.NewNop()}

		jobArn, jobName, err := svc.launchBedrockTraining(ctx, TrainingOptions{
			BaseModelID:   "base",
			DatasetS3Path: "data.jsonl",
			OutputS3Path:  "out/path",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, jobArn)
		assert.NotEmpty(t, jobName)
		require.Len(t, bedrockClient.calls, 1)
		assert.NotNil(t, bedrockClient.calls[0].HyperParameters)
		assert.Equal(t, "s3://bucket/data.jsonl", aws.ToString(bedrockClient.calls[0].TrainingDataConfig.S3Uri))
		assert.Equal(t, "s3://bucket/out/path", aws.ToString(bedrockClient.calls[0].OutputDataConfig.S3Uri))
	})
}

func TestService_TrainModel_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("dataset_prep_error_bubbles_up", func(t *testing.T) {
		repo := &fakeModerationRepo{samplesByID: map[string]*models.ModerationSample{}}
		svc := &Service{
			repo:           repo,
			s3Client:       &fakeS3Client{},
			bedrockClient:  &fakeBedrockClient{},
			trainingBucket: "bucket",
			roleARN:        "arn:role",
			logger:         zap.NewNop(),
		}
		_, err := svc.TrainModel(ctx, "t1", "u1", []string{"s1"}, TrainingOptions{BaseModelID: "base", DatasetS3Path: "data.jsonl"})
		require.Error(t, err)
	})

	t.Run("launch_error_bubbles_up", func(t *testing.T) {
		repo := &fakeModerationRepo{
			samplesByID: map[string]*models.ModerationSample{
				"s1": {ID: "s1", Label: "spam", Metadata: map[string]interface{}{"content": "hello"}},
			},
		}
		svc := &Service{
			repo:           repo,
			s3Client:       &fakeS3Client{},
			bedrockClient:  &fakeBedrockClient{},
			trainingBucket: "bucket",
			roleARN:        "",
			logger:         zap.NewNop(),
		}
		_, err := svc.TrainModel(ctx, "t1", "u1", []string{"s1"}, TrainingOptions{BaseModelID: "base", DatasetS3Path: "data.jsonl"})
		require.Error(t, err)
	})

	t.Run("success_even_if_tracking_records_fail", func(t *testing.T) {
		repo := &fakeModerationRepo{
			samplesByID: map[string]*models.ModerationSample{
				"s1": {ID: "s1", Label: "spam", Metadata: map[string]interface{}{"content": "hello"}},
			},
			createTrainingJobErr:  stderrors.New("ignored"),
			createPollRequestErr:  stderrors.New("ignored"),
		}
		bedrockClient := &fakeBedrockClient{out: &bedrock.CreateModelCustomizationJobOutput{JobArn: aws.String("arn:job")}}
		svc := &Service{
			repo:           repo,
			s3Client:       &fakeS3Client{},
			bedrockClient:  bedrockClient,
			trainingBucket: "bucket",
			roleARN:        "arn:role",
			logger:         zap.NewNop(),
		}

		result, err := svc.TrainModel(ctx, "t1", "u1", []string{"s1"}, TrainingOptions{BaseModelID: "base", DatasetS3Path: "data.jsonl"})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "SUBMITTED", result.Status)
		assert.Equal(t, "arn:job", result.JobID)
		assert.Equal(t, "data.jsonl", result.DatasetS3Key)
		require.Len(t, repo.trainingJobsCreated, 1)
		require.Len(t, repo.pollRequestsCreated, 1)
	})
}

func TestService_ScoreContent_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("no_active_model_errors", func(t *testing.T) {
		svc := &Service{repo: &fakeModerationRepo{activeModelErr: stderrors.New("boom")}, bedrockRuntime: &fakeBedrockRuntime{}, logger: zap.NewNop()}
		_, err := svc.ScoreContent(ctx, ScoreContentInput{Content: "hi"})
		require.Error(t, err)
	})

	t.Run("guardrail_block_returns_max_risk_and_tracks", func(t *testing.T) {
		repo := &fakeModerationRepo{
			activeModel: &models.ModerationModelVersion{VersionID: "v1", ModelARN: "arn:model"},
		}
		runtime := &fakeBedrockRuntime{err: stderrors.New("guardrail violation")}
		svc := &Service{
			repo:           repo,
			bedrockRuntime: runtime,
			logger:         zap.NewNop(),
			guardrailID:    "gr",
			guardrailVersion: "DRAFT",
		}

		result, err := svc.ScoreContent(ctx, ScoreContentInput{Content: "hi", ObjectID: "o1", ObjectType: "status", UseGuardrail: true})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.GuardrailBlocked)
		assert.Equal(t, 1.0, result.Score)
		require.Len(t, runtime.calls, 1)
		assert.Equal(t, "gr", aws.ToString(runtime.calls[0].GuardrailIdentifier))
		require.Len(t, repo.predictionsCreated, 1)
	})

	t.Run("invoke_model_other_error_bubbles", func(t *testing.T) {
		repo := &fakeModerationRepo{
			activeModel: &models.ModerationModelVersion{VersionID: "v1", ModelARN: "arn:model"},
		}
		runtime := &fakeBedrockRuntime{err: stderrors.New("boom")}
		svc := &Service{repo: repo, bedrockRuntime: runtime, logger: zap.NewNop()}
		_, err := svc.ScoreContent(ctx, ScoreContentInput{Content: "hi"})
		require.Error(t, err)
	})

	t.Run("invalid_json_response_errors", func(t *testing.T) {
		repo := &fakeModerationRepo{activeModel: &models.ModerationModelVersion{VersionID: "v1", ModelARN: "arn:model"}}
		runtime := &fakeBedrockRuntime{out: &bedrockruntime.InvokeModelOutput{Body: []byte("not-json"), ContentType: aws.String("application/json")}}
		svc := &Service{repo: repo, bedrockRuntime: runtime, logger: zap.NewNop()}
		_, err := svc.ScoreContent(ctx, ScoreContentInput{Content: "hi"})
		require.Error(t, err)
	})

	t.Run("success_parses_completion_and_tracks_prediction", func(t *testing.T) {
		repo := &fakeModerationRepo{activeModel: &models.ModerationModelVersion{VersionID: "v1", ModelARN: "arn:model"}}
		runtime := &fakeBedrockRuntime{out: &bedrockruntime.InvokeModelOutput{Body: []byte(`{"completion":" spam"}`), ContentType: aws.String("application/json")}}
		svc := &Service{repo: repo, bedrockRuntime: runtime, logger: zap.NewNop()}

		result, err := svc.ScoreContent(ctx, ScoreContentInput{Content: "hi", ObjectID: "o1", ObjectType: "status"})
		require.NoError(t, err)
		assert.Equal(t, 0.5, result.Score)
		require.Len(t, repo.predictionsCreated, 1)
		assert.Equal(t, "spam", repo.predictionsCreated[0].PredictedLabel)
	})

	t.Run("trackPrediction_skips_without_object_info", func(t *testing.T) {
		repo := &fakeModerationRepo{activeModel: &models.ModerationModelVersion{VersionID: "v1", ModelARN: "arn:model"}}
		runtime := &fakeBedrockRuntime{out: &bedrockruntime.InvokeModelOutput{Body: []byte(`{"completion":" clean"}`), ContentType: aws.String("application/json")}}
		svc := &Service{repo: repo, bedrockRuntime: runtime, logger: zap.NewNop()}

		_, err := svc.ScoreContent(ctx, ScoreContentInput{Content: "hi"})
		require.NoError(t, err)
		assert.Empty(t, repo.predictionsCreated)
	})
}

func TestService_GetEffectiveness_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns_cached_metric", func(t *testing.T) {
		expected := &models.ModerationEffectivenessMetric{PatternID: "p1", Period: "daily"}
		repo := &fakeModerationRepo{effectivenessMetric: expected}
		svc := &Service{repo: repo, logger: zap.NewNop()}

		got, err := svc.GetEffectiveness(ctx, "p1", "daily")
		require.NoError(t, err)
		require.Same(t, expected, got)
	})

	t.Run("computes_when_missing_and_saves_nonfatally", func(t *testing.T) {
		repo := &fakeModerationRepo{
			effectivenessMetricErr: stderrors.New("not found"),
			predictionsByModel: []*models.MLPrediction{
				{Reviewed: true, HumanLabel: "spam", PredictedLabel: "spam"},   // TP
				{Reviewed: true, HumanLabel: "safe", PredictedLabel: "spam"},   // FP
				{Reviewed: true, HumanLabel: "safe", PredictedLabel: "safe"},   // TN
				{Reviewed: true, HumanLabel: "spam", PredictedLabel: "safe"},   // FN
				{Reviewed: false, HumanLabel: "spam", PredictedLabel: "spam"},  // skipped
				{Reviewed: true, HumanLabel: "", PredictedLabel: "spam"},       // skipped
			},
			createMetricErr: stderrors.New("ignored"),
		}
		svc := &Service{repo: repo, logger: zap.NewNop()}

		metric, err := svc.GetEffectiveness(ctx, "p1", "daily")
		require.NoError(t, err)
		require.NotNil(t, metric)
		assert.Equal(t, 1, metric.TruePositives)
		assert.Equal(t, 1, metric.FalsePositives)
		assert.Equal(t, 1, metric.TrueNegatives)
		assert.Equal(t, 1, metric.FalseNegatives)
	})
}

func TestHelpers_round26_coverage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0.0, calculateRiskScore("safe"))
	assert.Equal(t, 0.5, calculateRiskScore("spam"))
	assert.Equal(t, 0.9, calculateRiskScore("violence"))
	assert.Equal(t, 1.0, calculateRiskScore("blocked"))
	assert.Equal(t, 0.3, calculateRiskScore("unknown"))

	assert.Equal(t, "hi", truncateString("hi", 10))
	assert.Equal(t, "0123...", truncateString("0123456", 4))
}
