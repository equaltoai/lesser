package advanced

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestImageAnalyzer_buildImageInput_NonS3UsesBytes(t *testing.T) {
	ia := NewImageAnalyzer(nil, zap.NewNop(), getTestConfig(), nil)
	ia.fetchImageBytes = func(_ context.Context, _ string) ([]byte, error) {
		return []byte{1, 2, 3}, nil
	}

	img, err := ia.buildImageInput(context.Background(), "https://example.com/image.png")
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Nil(t, img.S3Object)
	require.Len(t, img.Bytes, 3)
	assert.Equal(t, []byte{1, 2, 3}, img.Bytes)
}

func TestImageAnalyzer_buildImageInput_S3UsesS3Object(t *testing.T) {
	cfg := getTestConfig()
	cfg.S3Bucket = "test-bucket"
	ia := NewImageAnalyzer(nil, zap.NewNop(), cfg, nil)

	img, err := ia.buildImageInput(context.Background(), "https://test-bucket.s3.us-east-1.amazonaws.com/path/to/key.jpg?X-Amz-Signature=abc")
	require.NoError(t, err)
	require.NotNil(t, img)
	require.NotNil(t, img.S3Object)
	assert.Equal(t, "test-bucket", aws.ToString(img.S3Object.Bucket))
	assert.Equal(t, "path/to/key.jpg", aws.ToString(img.S3Object.Name))
	assert.Len(t, img.Bytes, 0)
}
