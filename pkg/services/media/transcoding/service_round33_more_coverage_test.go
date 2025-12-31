package transcoding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeMediaConvertClient struct {
	createJobOutput *mediaconvert.CreateJobOutput
	createJobErr    error

	getJobOutput *mediaconvert.GetJobOutput
	getJobErr    error

	cancelJobOutput *mediaconvert.CancelJobOutput
	cancelJobErr    error

	lastCreateInput *mediaconvert.CreateJobInput
	lastGetInput    *mediaconvert.GetJobInput
	lastCancelInput *mediaconvert.CancelJobInput
}

func (f *fakeMediaConvertClient) CreateJob(ctx context.Context, params *mediaconvert.CreateJobInput, optFns ...func(*mediaconvert.Options)) (*mediaconvert.CreateJobOutput, error) {
	f.lastCreateInput = params
	return f.createJobOutput, f.createJobErr
}

func (f *fakeMediaConvertClient) GetJob(ctx context.Context, params *mediaconvert.GetJobInput, optFns ...func(*mediaconvert.Options)) (*mediaconvert.GetJobOutput, error) {
	f.lastGetInput = params
	return f.getJobOutput, f.getJobErr
}

func (f *fakeMediaConvertClient) CancelJob(ctx context.Context, params *mediaconvert.CancelJobInput, optFns ...func(*mediaconvert.Options)) (*mediaconvert.CancelJobOutput, error) {
	f.lastCancelInput = params
	return f.cancelJobOutput, f.cancelJobErr
}

func TestNewService_SetsFieldsAndDefaults(t *testing.T) {
	t.Parallel()

	svc, err := NewService(
		aws.Config{Region: "us-east-1"},
		Config{
			Role:              "role-arn",
			DestinationBucket: "bucket",
			DestinationPrefix: "prefix",
			Queue:             "queue-arn",
		},
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, svc.client)
	require.NotNil(t, svc.logger)
	assert.Equal(t, "role-arn", svc.role)
	assert.Equal(t, "bucket", svc.destinationBucket)
	assert.Equal(t, "prefix", svc.destinationPrefix)
	assert.Equal(t, "queue-arn", svc.queue)
}

func TestService_SubmitJob_ReturnsServiceUnavailableOnCreateJobError(t *testing.T) {
	t.Parallel()

	createErr := errors.New("boom")
	client := &fakeMediaConvertClient{
		createJobErr: createErr,
	}

	svc := &Service{
		client:            client,
		logger:            zap.NewNop(),
		role:              "role-arn",
		destinationBucket: "bucket",
		destinationPrefix: "prefix",
		queue:             "queue-arn",
	}

	_, err := svc.SubmitJob(context.Background(), &TranscodeRequest{
		MediaID:       "media-1",
		UserID:        "user-1",
		Username:      "alice",
		SourceBucket:  "source-bucket",
		SourceKey:     "source.mp4",
		ContentType:   "video/mp4",
		Duration:      300,
		Width:         1920,
		Height:        1080,
		QualityLevels: []string{"720p"},
		GenerateHLS:   true,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceUnavailable)
	assert.ErrorIs(t, err, createErr)
}

func TestService_SubmitJob_SetsQueueAndReturnsResult(t *testing.T) {
	t.Parallel()

	client := &fakeMediaConvertClient{
		createJobOutput: &mediaconvert.CreateJobOutput{
			Job: &types.Job{
				Id:     aws.String("mc-job-1"),
				Status: types.JobStatusSubmitted,
			},
		},
	}

	svc := &Service{
		client:            client,
		logger:            zap.NewNop(),
		role:              "role-arn",
		destinationBucket: "bucket",
		destinationPrefix: "prefix",
		queue:             "queue-arn",
	}

	result, err := svc.SubmitJob(context.Background(), &TranscodeRequest{
		MediaID:       "media-1",
		UserID:        "user-1",
		Username:      "alice",
		SourceBucket:  "source-bucket",
		SourceKey:     "source.mp4",
		ContentType:   "video/mp4",
		Duration:      300,
		Width:         1920,
		Height:        1080,
		QualityLevels: []string{"720p"},
		GenerateHLS:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.JobID)
	assert.Equal(t, "mc-job-1", result.MediaConvertJobID)
	assert.Equal(t, "bucket", result.OutputBucket)
	assert.Equal(t, "prefix/media-1", result.OutputPrefix)

	require.NotNil(t, client.lastCreateInput)
	assert.Equal(t, "queue-arn", aws.ToString(client.lastCreateInput.Queue))
	assert.Equal(t, "role-arn", aws.ToString(client.lastCreateInput.Role))
	assert.NotNil(t, client.lastCreateInput.Settings)
	assert.Equal(t, "media-1", client.lastCreateInput.UserMetadata["media_id"])
	assert.Equal(t, "user-1", client.lastCreateInput.UserMetadata["user_id"])
	assert.Equal(t, "alice", client.lastCreateInput.UserMetadata["username"])
	assert.NotEmpty(t, client.lastCreateInput.UserMetadata["job_id"])
}

func TestService_GetJobStatus_ReturnsJobNotFoundOnError(t *testing.T) {
	t.Parallel()

	getErr := errors.New("nope")
	client := &fakeMediaConvertClient{
		getJobErr: getErr,
	}
	svc := &Service{client: client, logger: zap.NewNop()}

	_, err := svc.GetJobStatus(context.Background(), "mc-job-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJobNotFound)
	assert.ErrorIs(t, err, getErr)
}

func TestService_GetJobStatus_ParsesOutputsWhenComplete(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().Add(-1 * time.Hour).UTC()
	finishedAt := time.Now().UTC()

	client := &fakeMediaConvertClient{
		getJobOutput: &mediaconvert.GetJobOutput{
			Job: &types.Job{
				Id:                 aws.String("mc-job-1"),
				Status:             types.JobStatusComplete,
				JobPercentComplete: aws.Int32(100),
				CreatedAt:          aws.Time(createdAt),
				UserMetadata: map[string]string{
					"job_id": "job-1",
				},
				Timing: &types.Timing{
					FinishTime: aws.Time(finishedAt),
				},
				OutputGroupDetails: []types.OutputGroupDetail{
					{
						OutputDetails: []types.OutputDetail{
							{
								VideoDetails: &types.VideoDetail{
									WidthInPx:  aws.Int32(1280),
									HeightInPx: aws.Int32(720),
								},
							},
							{
								VideoDetails: nil,
							},
						},
					},
				},
			},
		},
	}

	svc := &Service{client: client, logger: zap.NewNop()}

	status, err := svc.GetJobStatus(context.Background(), "mc-job-1")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "mc-job-1", status.MediaConvertJobID)
	assert.Equal(t, "job-1", status.JobID)
	assert.Equal(t, 100, status.PercentComplete)
	assert.WithinDuration(t, createdAt, status.CreatedAt, time.Second)
	require.NotNil(t, status.CompletedAt)
	assert.WithinDuration(t, finishedAt, *status.CompletedAt, time.Second)

	require.Len(t, status.Outputs, 1)
	assert.Equal(t, 1280, status.Outputs[0].Width)
	assert.Equal(t, 720, status.Outputs[0].Height)
}

func TestService_CancelJob_ReturnsServiceUnavailableOnError(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("boom")
	client := &fakeMediaConvertClient{
		cancelJobErr: cancelErr,
	}
	svc := &Service{client: client, logger: zap.NewNop()}

	err := svc.CancelJob(context.Background(), "mc-job-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceUnavailable)
	assert.ErrorIs(t, err, cancelErr)
}

func TestService_CancelJob_SubmitsID(t *testing.T) {
	t.Parallel()

	client := &fakeMediaConvertClient{
		cancelJobOutput: &mediaconvert.CancelJobOutput{},
	}
	svc := &Service{client: client, logger: zap.NewNop()}

	require.NoError(t, svc.CancelJob(context.Background(), "mc-job-1"))
	require.NotNil(t, client.lastCancelInput)
	assert.Equal(t, "mc-job-1", aws.ToString(client.lastCancelInput.Id))
}

func TestSafeInt32_ClampsBounds(t *testing.T) {
	t.Parallel()

	assert.Equal(t, int32(2147483647), aws.ToInt32(safeInt32(2147483648)))
	assert.Equal(t, int32(-2147483648), aws.ToInt32(safeInt32(-2147483649)))
}
