package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-xray-sdk-go/v2/xray"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestXRayTracer_Disabled(t *testing.T) {
	logger := zap.NewNop()
	t.Setenv("_X_AMZN_TRACE_ID", "")

	tracer := NewXRayTracer("svc", "test", logger)
	require.False(t, tracer.IsEnabled())

	cfg := tracer.InstrumentAWSConfig(aws.Config{})
	assert.Empty(t, cfg.APIOptions)

	base := &dynamodb.Client{}
	assert.Same(t, base, tracer.InstrumentDynamoDBClient(base))
	s3Client := &s3.Client{}
	assert.Same(t, s3Client, tracer.InstrumentS3Client(s3Client))
	sqsClient := &sqs.Client{}
	assert.Same(t, sqsClient, tracer.InstrumentSQSClient(sqsClient))

	ctx := context.Background()
	require.NoError(t, tracer.AddAnnotation(ctx, "k", "v"))
	require.NoError(t, tracer.AddMetadata(ctx, "ns", map[string]interface{}{"k": "v"}))
	tracer.RecordError(ctx, errors.New("boom"))
	assert.Equal(t, "", tracer.GetTraceID(ctx))

	// Trace wrappers should simply delegate to the provided functions.
	expectedErr := errors.New("dynamo boom")
	require.ErrorIs(t, tracer.TraceDynamoDBOperation(ctx, "GetItem", "table", func(context.Context) error {
		return expectedErr
	}), expectedErr)

	require.NoError(t, tracer.TraceMediaProcessing(ctx, "resize", "image", 10, func(context.Context) error {
		return nil
	}))

	ok, err := tracer.TraceAuthOperation(ctx, "login", "jwt", func(context.Context) (bool, error) {
		return true, nil
	})
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestXRayTracer_Enabled(t *testing.T) {
	logger := zap.NewNop()
	t.Setenv("_X_AMZN_TRACE_ID", "trace")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	tracer := NewXRayTracer("svc", "test", logger)
	require.True(t, tracer.IsEnabled())

	cfg := tracer.InstrumentAWSConfig(aws.Config{})
	assert.NotEmpty(t, cfg.APIOptions)

	// Instrumented AWS SDK clients (success path).
	require.NotNil(t, tracer.InstrumentDynamoDBClient(dynamodb.NewFromConfig(aws.Config{Region: "us-east-1"})))
	require.NotNil(t, tracer.InstrumentS3Client(s3.NewFromConfig(aws.Config{Region: "us-east-1"})))
	require.NotNil(t, tracer.InstrumentSQSClient(sqs.NewFromConfig(aws.Config{Region: "us-east-1"})))

	// Use an active X-Ray segment for the enabled-path helpers.
	ctx, seg := xray.BeginSegment(context.Background(), "test")
	defer seg.Close(nil)

	require.NoError(t, tracer.AddAnnotation(ctx, "k", "v"))
	require.NoError(t, tracer.AddMetadata(ctx, "ns", map[string]interface{}{"k": "v"}))
	tracer.RecordError(ctx, errors.New("boom"))
	tracer.RecordError(ctx, nil)
	assert.NotEmpty(t, tracer.GetTraceID(ctx))
	assert.Equal(t, "", tracer.GetTraceID(context.Background()))

	require.Error(t, tracer.TraceDynamoDBOperation(ctx, "GetItem", "table", func(context.Context) error {
		return errors.New("dynamo boom")
	}))

	require.NoError(t, tracer.TraceS3Operation(ctx, "PutObject", "bucket", "key", func(context.Context) error {
		return nil
	}))
	require.Error(t, tracer.TraceS3Operation(ctx, "PutObject", "bucket", "key", func(context.Context) error {
		return errors.New("s3 boom")
	}))

	require.NoError(t, tracer.TraceSQSOperation(ctx, "SendMessage", "queue", 2, func(context.Context) error {
		return nil
	}))
	require.Error(t, tracer.TraceSQSOperation(ctx, "SendMessage", "queue", 2, func(context.Context) error {
		return errors.New("sqs boom")
	}))

	require.Error(t, tracer.TraceFederationCall(ctx, "example.com", "Fetch", "Follow", func(context.Context) error {
		return errors.New("federation boom")
	}))

	require.NoError(t, tracer.TraceGraphQLResolver(ctx, "Resolver", "field", func(context.Context) error {
		return nil
	}))
	require.Error(t, tracer.TraceGraphQLResolver(ctx, "Resolver", "field", func(context.Context) error {
		return errors.New("graphql boom")
	}))

	require.NoError(t, tracer.TraceMediaProcessing(ctx, "resize", "image", 10, func(context.Context) error {
		return nil
	}))

	require.NoError(t, tracer.TraceStreamProcessor(ctx, "processor", 3, func(context.Context) error {
		time.Sleep(2 * time.Millisecond)
		return nil
	}))
	require.Error(t, tracer.TraceStreamProcessor(ctx, "processor", 3, func(context.Context) error {
		time.Sleep(2 * time.Millisecond)
		return errors.New("stream boom")
	}))

	ok, err := tracer.TraceAuthOperation(ctx, "login", "jwt", func(context.Context) (bool, error) {
		return false, errors.New("auth boom")
	})
	require.Error(t, err)
	assert.False(t, ok)

	require.Error(t, tracer.TraceCostOperation(ctx, "dynamodb", 123, func(context.Context) error {
		return errors.New("cost boom")
	}))
	require.NoError(t, tracer.TraceCostOperation(ctx, "dynamodb", 123, func(context.Context) error {
		return nil
	}))
}

func TestXRayTracer_TraceLiftHandler_UsesLambdaContext(t *testing.T) {
	logger := zap.NewNop()
	t.Setenv("_X_AMZN_TRACE_ID", "trace")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	tracer := NewXRayTracer("svc", "test", logger)
	require.True(t, tracer.IsEnabled())

	lc := &lambdacontext.LambdaContext{
		AwsRequestID:       "aws-req",
		InvokedFunctionArn: "arn:aws:lambda:us-east-1:123:function:test",
	}
	base := lambdacontext.NewContext(context.Background(), lc)

	liftCtx := lift.NewContext(base, &lift.Request{
		Method:  "GET",
		Path:    "/inbox",
		Headers: map[string]string{"X-Test": "1"},
	})
	liftCtx.RequestID = "req"
	liftCtx.SetTenantID("tenant")

	wrapped := tracer.TraceLiftHandler("handler", func(*lift.Context) error {
		return errors.New("handler boom")
	})
	require.Error(t, wrapped(liftCtx))
}

func TestXRayTracer_InstrumentClient_ConfigErrorFallsBack(t *testing.T) {
	logger := zap.NewNop()
	t.Setenv("_X_AMZN_TRACE_ID", "trace")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	tracer := NewXRayTracer("svc", "test", logger)
	require.True(t, tracer.IsEnabled())

	dynamo := dynamodb.NewFromConfig(aws.Config{Region: "us-east-1"})
	out := tracer.InstrumentDynamoDBClient(dynamo)
	require.NotNil(t, out)
}
