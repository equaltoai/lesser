package cost

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockS3API is a mock implementation of S3API
type MockS3API struct {
	mock.Mock
}

func (m *MockS3API) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.GetObjectOutput), args.Error(1)
}

func (m *MockS3API) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.PutObjectOutput), args.Error(1)
}

func (m *MockS3API) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.DeleteObjectOutput), args.Error(1)
}

func (m *MockS3API) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.HeadObjectOutput), args.Error(1)
}

func (m *MockS3API) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*s3.ListObjectsV2Output), args.Error(1)
}

func TestNewS3Wrapper(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)
	require.NotNil(t, wrapper)
	assert.Equal(t, mockClient, wrapper.client)
}

func TestS3Wrapper_GetObject(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.GetObjectInput{
		Bucket: aws.String("test-bucket"),
		Key:    aws.String("test-key"),
	}

	output := &s3.GetObjectOutput{
		ContentLength: aws.Int64(1024),
		Body:          io.NopCloser(strings.NewReader("test content")),
	}

	mockClient.On("GetObject", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.GetObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track GET and data transfer
	assert.Equal(t, int64(1), tracker.s3Gets.Load())
	assert.Equal(t, int64(1024), tracker.dataTransfer.Load())
}

func TestS3Wrapper_GetObject_NoContentLength(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.GetObjectInput{
		Bucket: aws.String("test-bucket"),
		Key:    aws.String("test-key"),
	}

	output := &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader("test content")),
	}

	mockClient.On("GetObject", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.GetObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track GET but no data transfer (nil content length)
	assert.Equal(t, int64(1), tracker.s3Gets.Load())
	assert.Equal(t, int64(0), tracker.dataTransfer.Load())
}

func TestS3Wrapper_PutObject(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.PutObjectInput{
		Bucket:        aws.String("test-bucket"),
		Key:           aws.String("test-key"),
		Body:          strings.NewReader("test content"),
		ContentLength: aws.Int64(12),
	}

	output := &s3.PutObjectOutput{}

	mockClient.On("PutObject", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.PutObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track PUT and data transfer
	assert.Equal(t, int64(1), tracker.s3Puts.Load())
	assert.Equal(t, int64(12), tracker.dataTransfer.Load())
}

func TestS3Wrapper_PutObject_NoContentLength(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.PutObjectInput{
		Bucket: aws.String("test-bucket"),
		Key:    aws.String("test-key"),
		Body:   strings.NewReader("test content"),
	}

	output := &s3.PutObjectOutput{}

	mockClient.On("PutObject", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.PutObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should track PUT but no data transfer
	assert.Equal(t, int64(1), tracker.s3Puts.Load())
	assert.Equal(t, int64(0), tracker.dataTransfer.Load())
}

func TestS3Wrapper_DeleteObject(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.DeleteObjectInput{
		Bucket: aws.String("test-bucket"),
		Key:    aws.String("test-key"),
	}

	output := &s3.DeleteObjectOutput{}

	mockClient.On("DeleteObject", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.DeleteObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Delete operations are free
	assert.Equal(t, int64(0), tracker.s3Gets.Load())
	assert.Equal(t, int64(0), tracker.s3Puts.Load())
}

func TestS3Wrapper_HeadObject(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.HeadObjectInput{
		Bucket: aws.String("test-bucket"),
		Key:    aws.String("test-key"),
	}

	output := &s3.HeadObjectOutput{
		ContentLength: aws.Int64(1024),
	}

	mockClient.On("HeadObject", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.HeadObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// HEAD counts as GET for pricing
	assert.Equal(t, int64(1), tracker.s3Gets.Load())
}

func TestS3Wrapper_ListObjectsV2(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String("test-bucket"),
	}

	output := &s3.ListObjectsV2Output{
		KeyCount: aws.Int32(10),
	}

	mockClient.On("ListObjectsV2", ctx, input, mock.Anything).Return(output, nil)

	result, err := wrapper.ListObjectsV2(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// LIST counts as GET for pricing
	assert.Equal(t, int64(1), tracker.s3Gets.Load())
}

func TestS3Wrapper_NoTracker(t *testing.T) {
	mockClient := &MockS3API{}
	wrapper := NewS3Wrapper(mockClient)

	// Context without tracker
	ctx := context.Background()

	input := &s3.GetObjectInput{
		Bucket: aws.String("test-bucket"),
		Key:    aws.String("test-key"),
	}

	output := &s3.GetObjectOutput{
		ContentLength: aws.Int64(1024),
		Body:          io.NopCloser(strings.NewReader("test content")),
	}

	mockClient.On("GetObject", ctx, input, mock.Anything).Return(output, nil)

	// Should not panic without tracker
	result, err := wrapper.GetObject(ctx, input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}
