package cost

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API defines the subset of S3 operations we use
type S3API interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// S3CostWrapper wraps an S3 client to track costs
type S3CostWrapper struct {
	client S3API
}

// NewS3Wrapper creates a new cost-tracking S3 wrapper
func NewS3Wrapper(client S3API) *S3CostWrapper {
	return &S3CostWrapper{client: client}
}

// GetObject tracks the cost of an S3 GetObject operation
func (w *S3CostWrapper) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	// Track GET operation
	TrackS3GetContext(ctx, 1)

	output, err := w.client.GetObject(ctx, params, optFns...)

	// Track data transfer if successful
	if err == nil && output != nil && output.ContentLength != nil {
		TrackDataTransferContext(ctx, *output.ContentLength)
	}

	return output, err
}

// PutObject tracks the cost of an S3 PutObject operation
func (w *S3CostWrapper) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	// Track PUT operation
	TrackS3PutContext(ctx, 1)

	// Track data transfer for upload (if content length is known)
	if params.ContentLength != nil {
		TrackDataTransferContext(ctx, *params.ContentLength)
	}

	return w.client.PutObject(ctx, params, optFns...)
}

// DeleteObject tracks the cost of an S3 DeleteObject operation
func (w *S3CostWrapper) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	// Delete operations are free but we track them for completeness
	return w.client.DeleteObject(ctx, params, optFns...)
}

// HeadObject tracks the cost of an S3 HeadObject operation
func (w *S3CostWrapper) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	// HEAD requests count as GET for pricing
	TrackS3GetContext(ctx, 1)

	return w.client.HeadObject(ctx, params, optFns...)
}

// ListObjectsV2 tracks the cost of an S3 ListObjectsV2 operation
func (w *S3CostWrapper) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	// LIST requests count as GET for pricing
	TrackS3GetContext(ctx, 1)

	return w.client.ListObjectsV2(ctx, params, optFns...)
}
