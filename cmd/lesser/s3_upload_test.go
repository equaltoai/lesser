package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
)

func TestListFiles_SortsAndRelativizes(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a", "c.txt"), []byte("c"), 0o644))

	files, err := listFiles(root)
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "a/c.txt", files[0].RelativePath)
	require.Equal(t, "b.txt", files[1].RelativePath)
}

func TestContentTypeAndCacheControlHelpers(t *testing.T) {
	require.Contains(t, contentTypeForKey("index.html"), "text/html")
	require.Equal(t, "application/octet-stream", contentTypeForKey("file.unknownext"))

	require.Equal(t, "public, max-age=31536000, immutable", cacheControlForKey("_assets/app.js"))
	require.Equal(t, "public, max-age=60", cacheControlForKey("index.html"))
	require.Equal(t, "public, max-age=3600", cacheControlForKey("image.png"))
}

func TestReplaceBucketWithDir_Validation(t *testing.T) {
	err := replaceBucketWithDir(context.Background(), &s3.Client{}, "", t.TempDir())
	require.Error(t, err)

	err = replaceBucketWithDir(context.Background(), &s3.Client{}, "bucket", filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)

	file := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	err = replaceBucketWithDir(context.Background(), &s3.Client{}, "bucket", file)
	require.Error(t, err)
}

type fakeSmithyAPIError struct {
	code string
}

func (e fakeSmithyAPIError) Error() string {
	return e.code
}

func (e fakeSmithyAPIError) ErrorCode() string {
	return e.code
}

func (e fakeSmithyAPIError) ErrorMessage() string {
	return e.code
}

func (e fakeSmithyAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

type fakeS3HeadClient struct {
	err error
}

func (f *fakeS3HeadClient) HeadObject(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &s3.HeadObjectOutput{}, nil
}

func TestS3ObjectExists_HandlesNotFoundAndErrors(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		ok, err := s3ObjectExists(context.Background(), &fakeS3HeadClient{}, "b", "k")
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("smithy NotFound", func(t *testing.T) {
		ok, err := s3ObjectExists(context.Background(), &fakeS3HeadClient{err: fakeSmithyAPIError{code: "NotFound"}}, "b", "k")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("string contains NotFound", func(t *testing.T) {
		ok, err := s3ObjectExists(context.Background(), &fakeS3HeadClient{err: errors.New("NotFound")}, "b", "k")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("other error", func(t *testing.T) {
		boom := errors.New("boom")
		ok, err := s3ObjectExists(context.Background(), &fakeS3HeadClient{err: boom}, "b", "k")
		require.ErrorIs(t, err, boom)
		require.False(t, ok)
	})
}

type fakeS3PutClient struct {
	err   error
	input *s3.PutObjectInput
}

func (f *fakeS3PutClient) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &s3.PutObjectOutput{}, nil
}

func TestPutObjectString_WrapsErrorAndSendsContent(t *testing.T) {
	t.Run("error wraps", func(t *testing.T) {
		boom := errors.New("boom")
		err := putObjectString(context.Background(), &fakeS3PutClient{err: boom}, "bucket", "key", "content", "text/plain", "max-age=0")
		require.ErrorIs(t, err, boom)
		require.Contains(t, err.Error(), "put s3://bucket/key")
	})

	t.Run("success sets fields", func(t *testing.T) {
		fake := &fakeS3PutClient{}
		require.NoError(t, putObjectString(context.Background(), fake, "bucket", "key", "hello", "text/plain", "max-age=0"))
		require.Equal(t, "bucket", aws.ToString(fake.input.Bucket))
		require.Equal(t, "key", aws.ToString(fake.input.Key))
		require.Equal(t, "text/plain", aws.ToString(fake.input.ContentType))
		require.Equal(t, "max-age=0", aws.ToString(fake.input.CacheControl))
		bodyBytes, err := io.ReadAll(fake.input.Body)
		require.NoError(t, err)
		require.Equal(t, "hello", string(bodyBytes))
	})
}

type fakeS3BucketClient struct {
	listOutputs []*s3.ListObjectsV2Output
	listErr     error

	deleteErr     error
	deleteCalls   int
	deleteBatches [][]s3types.ObjectIdentifier
}

func (f *fakeS3BucketClient) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listOutputs) == 0 {
		return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
	}
	out := f.listOutputs[0]
	f.listOutputs = f.listOutputs[1:]
	return out, nil
}

func (f *fakeS3BucketClient) DeleteObjects(_ context.Context, input *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	f.deleteCalls++
	if input.Delete != nil {
		f.deleteBatches = append(f.deleteBatches, input.Delete.Objects)
	}
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func TestDeleteAllObjects_PaginatesAndBatches(t *testing.T) {
	t.Run("list error", func(t *testing.T) {
		fake := &fakeS3BucketClient{listErr: errors.New("list failed")}
		err := deleteAllObjects(context.Background(), fake, "bucket")
		require.Error(t, err)
		require.Contains(t, err.Error(), "list bucket bucket")
	})

	t.Run("delete error", func(t *testing.T) {
		fake := &fakeS3BucketClient{
			listOutputs: []*s3.ListObjectsV2Output{{
				Contents:    []s3types.Object{{Key: aws.String("a")}},
				IsTruncated: aws.Bool(false),
			}},
			deleteErr: errors.New("delete failed"),
		}
		err := deleteAllObjects(context.Background(), fake, "bucket")
		require.Error(t, err)
		require.Contains(t, err.Error(), "delete bucket bucket")
	})

	t.Run("batches >1000 and skips blank keys", func(t *testing.T) {
		var objects []s3types.Object
		objects = append(objects, s3types.Object{Key: aws.String("  ")})
		for i := 0; i < 1001; i++ {
			objects = append(objects, s3types.Object{Key: aws.String("k" + string(rune('a'+(i%26))))})
		}

		fake := &fakeS3BucketClient{
			listOutputs: []*s3.ListObjectsV2Output{{
				Contents:    objects,
				IsTruncated: aws.Bool(false),
			}},
		}
		require.NoError(t, deleteAllObjects(context.Background(), fake, "bucket"))
		require.Equal(t, 2, fake.deleteCalls)
		require.Len(t, fake.deleteBatches[0], 1000)
		require.Len(t, fake.deleteBatches[1], 1)
	})
}

type fakeS3UploaderClient struct {
	fakeS3BucketClient

	putCalls int
	putKeys  []string
	putErr   error
}

func (f *fakeS3UploaderClient) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalls++
	f.putKeys = append(f.putKeys, aws.ToString(input.Key))
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func TestReplaceBucketWithDir_UploadsFilesAndWrapsErrors(t *testing.T) {
	t.Run("success uploads", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("hi"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "_assets"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "_assets", "app.js"), []byte("x"), 0o644))

		fake := &fakeS3UploaderClient{
			fakeS3BucketClient: fakeS3BucketClient{
				listOutputs: []*s3.ListObjectsV2Output{{IsTruncated: aws.Bool(false)}},
			},
		}
		require.NoError(t, replaceBucketWithDir(context.Background(), fake, "bucket", root))
		require.Equal(t, 2, fake.putCalls)
		require.Equal(t, []string{"_assets/app.js", "index.html"}, fake.putKeys)
	})

	t.Run("put error wraps", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("hi"), 0o644))

		fake := &fakeS3UploaderClient{
			fakeS3BucketClient: fakeS3BucketClient{
				listOutputs: []*s3.ListObjectsV2Output{{IsTruncated: aws.Bool(false)}},
			},
			putErr: errors.New("put failed"),
		}
		err := replaceBucketWithDir(context.Background(), fake, "bucket", root)
		require.Error(t, err)
		require.Contains(t, err.Error(), "upload index.html to s3://bucket/index.html")
	})
}
