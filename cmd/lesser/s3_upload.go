package main

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type s3PutObjectAPI interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

type s3BucketAPI interface {
	s3.ListObjectsV2APIClient
	DeleteObjects(context.Context, *s3.DeleteObjectsInput, ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

type s3BucketUploaderAPI interface {
	s3BucketAPI
	s3PutObjectAPI
}

var (
	putObjectStringFn     = putObjectString
	uploadDirWithPrefixFn = uploadDirWithPrefix
)

func replaceBucketWithDirPrefix(ctx context.Context, client s3BucketUploaderAPI, bucket string, prefix string, dir string) error {
	if strings.TrimSpace(bucket) == "" {
		return fmt.Errorf("bucket is empty")
	}

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", dir)
	}

	if err := deleteBucketPrefix(ctx, client, bucket, prefix); err != nil {
		return err
	}

	return uploadDirWithPrefixFn(ctx, client, bucket, prefix, dir)
}

type localFile struct {
	RelativePath string
	FullPath     string
}

func listFiles(root string) ([]localFile, error) {
	var files []localFile

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "/")

		files = append(files, localFile{
			RelativePath: rel,
			FullPath:     path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files, nil
}

func deleteAllObjects(ctx context.Context, client s3BucketAPI, bucket string) error {
	return deleteBucketPrefix(ctx, client, bucket, "")
}

func deleteBucketPrefix(ctx context.Context, client s3BucketAPI, bucket string, prefix string) error {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(strings.TrimSpace(prefix)),
	})

	var keys []s3types.ObjectIdentifier
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list bucket %s: %w", bucket, err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if strings.TrimSpace(key) == "" {
				continue
			}
			keys = append(keys, s3types.ObjectIdentifier{Key: aws.String(key)})
		}
	}

	for len(keys) > 0 {
		batch := keys
		if len(batch) > 1000 {
			batch = batch[:1000]
		}

		_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &s3types.Delete{
				Objects: batch,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("delete bucket %s: %w", bucket, err)
		}
		keys = keys[len(batch):]
	}

	return nil
}

func uploadDirWithPrefix(ctx context.Context, client s3PutObjectAPI, bucket string, prefix string, dir string) error {
	if strings.TrimSpace(bucket) == "" {
		return fmt.Errorf("bucket is empty")
	}

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", dir)
	}

	files, err := listFiles(dir)
	if err != nil {
		return err
	}

	cleanPrefix := strings.Trim(strings.TrimSpace(prefix), "/")
	for _, file := range files {
		key := filepath.ToSlash(file.RelativePath)
		if cleanPrefix != "" {
			key = cleanPrefix + "/" + key
		}
		contentType := contentTypeForKey(key)
		cacheControl := cacheControlForKey(key)

		f, err := os.Open(file.FullPath) //nolint:gosec // file path derived from repo root
		if err != nil {
			return fmt.Errorf("open %s: %w", file.FullPath, err)
		}
		_, putErr := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:       aws.String(bucket),
			Key:          aws.String(key),
			Body:         f,
			ContentType:  aws.String(contentType),
			CacheControl: aws.String(cacheControl),
		})
		_ = f.Close()
		if putErr != nil {
			return fmt.Errorf("upload %s to s3://%s/%s: %w", file.RelativePath, bucket, key, putErr)
		}
	}

	fmt.Println("  s3: uploaded", len(files), "file(s)")
	return nil
}

func putObjectString(ctx context.Context, client s3PutObjectAPI, bucket, key, content, contentType, cacheControl string) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		Body:         strings.NewReader(content),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String(cacheControl),
	})
	if err != nil {
		return fmt.Errorf("put s3://%s/%s: %w", bucket, key, err)
	}
	fmt.Println("  s3: uploaded", key)
	return nil
}

func contentTypeForKey(key string) string {
	ext := strings.ToLower(filepath.Ext(key))
	if ext != "" {
		if typ := mime.TypeByExtension(ext); typ != "" {
			return typ
		}
	}
	if strings.HasSuffix(key, ".svg") {
		return "image/svg+xml"
	}
	return "application/octet-stream"
}

func cacheControlForKey(key string) string {
	switch {
	case strings.HasPrefix(key, "_assets/"), strings.HasPrefix(key, "l/_assets/"):
		return "public, max-age=31536000, immutable"
	case strings.HasSuffix(key, ".html"):
		return "public, max-age=60"
	default:
		return "public, max-age=3600"
	}
}
