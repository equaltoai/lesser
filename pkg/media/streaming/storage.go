package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// S3MediaStorage implements MediaStorage using S3
type S3MediaStorage struct {
	client    *s3.Client
	bucket    string
	region    string
	metaCache sync.Map // Cache for metadata
}

// NewS3MediaStorage creates a new S3-based media storage
func NewS3MediaStorage(client *s3.Client, bucket, region string) *S3MediaStorage {
	return &S3MediaStorage{
		client: client,
		bucket: bucket,
		region: region,
	}
}

// GetManifestPath returns the S3 path for a manifest file
func (s *S3MediaStorage) GetManifestPath(mediaID string, format MediaFormat, quality Quality) string {
	switch format {
	case FormatHLS:
		if quality == "" {
			return fmt.Sprintf("media/%s/master.m3u8", mediaID)
		}
		return fmt.Sprintf("media/%s/%s/playlist.m3u8", mediaID, quality)
	case FormatDASH:
		return fmt.Sprintf("media/%s/manifest.mpd", mediaID)
	default:
		return ""
	}
}

// GetSegmentPath returns the S3 path for a segment file
func (s *S3MediaStorage) GetSegmentPath(mediaID string, quality Quality, segmentIndex int) string {
	return fmt.Sprintf("media/%s/%s/segment%03d.ts", mediaID, quality, segmentIndex)
}

// GetMediaMetadata retrieves metadata for a media item
func (s *S3MediaStorage) GetMediaMetadata(mediaID string) (*MediaMetadata, error) {
	// Check cache first
	if cached, ok := s.metaCache.Load(mediaID); ok {
		if meta, ok := cached.(*cachedMetadata); ok && time.Since(meta.cachedAt) < 5*time.Minute {
			return meta.metadata, nil
		}
	}

	// Fetch from S3
	ctx := context.Background()
	metadataKey := fmt.Sprintf("media/%s/metadata.json", mediaID)

	getInput := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(metadataKey),
	}

	result, err := s.client.GetObject(ctx, getInput)
	if err != nil {
		return nil, fmt.Errorf("get metadata from S3: %w", err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			log.Printf("Warning: failed to close S3 object body: %v", closeErr)
		}
	}()

	// Parse metadata
	var metadata MediaMetadata
	decoder := json.NewDecoder(result.Body)
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}

	// Cache the metadata
	s.metaCache.Store(mediaID, &cachedMetadata{
		metadata: &metadata,
		cachedAt: time.Now(),
	})

	return &metadata, nil
}

// ManifestExists checks if a manifest file exists
func (s *S3MediaStorage) ManifestExists(mediaID string, format MediaFormat) (bool, error) {
	ctx := context.Background()
	manifestPath := s.GetManifestPath(mediaID, format, "")

	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(manifestPath),
	}

	_, err := s.client.HeadObject(ctx, headInput)
	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, fmt.Errorf("check manifest exists: %w", err)
	}

	return true, nil
}

// SaveManifest saves a manifest to S3
func (s *S3MediaStorage) SaveManifest(mediaID string, format MediaFormat, quality Quality, content string) error {
	ctx := context.Background()
	manifestPath := s.GetManifestPath(mediaID, format, quality)

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(manifestPath),
		Body:         strings.NewReader(content),
		ContentType:  aws.String(s.getManifestContentType(format)),
		CacheControl: aws.String("max-age=300"), // Cache for 5 minutes
	}

	_, err := s.client.PutObject(ctx, putInput)
	if err != nil {
		return fmt.Errorf("save manifest to S3: %w", err)
	}

	return nil
}

// GetSegmentInfo retrieves information about a segment
func (s *S3MediaStorage) GetSegmentInfo(mediaID string, quality Quality, segmentIndex int) (*Segment, error) {
	ctx := context.Background()
	segmentPath := s.GetSegmentPath(mediaID, quality, segmentIndex)

	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(segmentPath),
	}

	result, err := s.client.HeadObject(ctx, headInput)
	if err != nil {
		return nil, fmt.Errorf("get segment info: %w", err)
	}

	segmentSize := int64(0)
	if result.ContentLength != nil {
		segmentSize = *result.ContentLength
	}

	segment := &Segment{
		Index:    segmentIndex,
		Duration: 0, // Would need to be parsed from segment metadata
		URL:      s.getSegmentURL(segmentPath),
		Size:     segmentSize,
	}

	return segment, nil
}

// ListSegments lists all segments for a quality level
func (s *S3MediaStorage) ListSegments(mediaID string, quality Quality) ([]*Segment, error) {
	ctx := context.Background()
	prefix := fmt.Sprintf("media/%s/%s/segment", mediaID, quality)

	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	var segments []*Segment
	paginator := s3.NewListObjectsV2Paginator(s.client, listInput)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list segments: %w", err)
		}

		for _, obj := range page.Contents {
			// Extract segment index from filename
			segmentIndex := s.extractSegmentIndex(*obj.Key)
			if segmentIndex >= 0 {
				objSize := int64(0)
				if obj.Size != nil {
					objSize = *obj.Size
				}

				segment := &Segment{
					Index:    segmentIndex,
					Duration: 0, // Would need segment duration info
					URL:      s.getSegmentURL(*obj.Key),
					Size:     objSize,
				}
				segments = append(segments, segment)
			}
		}
	}

	return segments, nil
}

// UpdateMediaMetadata updates the metadata for a media item
func (s *S3MediaStorage) UpdateMediaMetadata(mediaID string, metadata *MediaMetadata) error {
	ctx := context.Background()
	metadataKey := fmt.Sprintf("media/%s/metadata.json", mediaID)

	// Marshal metadata to JSON
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(metadataKey),
		Body:         strings.NewReader(string(data)),
		ContentType:  aws.String("application/json"),
		CacheControl: aws.String("max-age=300"),
	}

	_, err = s.client.PutObject(ctx, putInput)
	if err != nil {
		return fmt.Errorf("update metadata in S3: %w", err)
	}

	// Invalidate cache
	s.metaCache.Delete(mediaID)

	return nil
}

// Helper methods

type cachedMetadata struct {
	metadata *MediaMetadata
	cachedAt time.Time
}

func (s *S3MediaStorage) getManifestContentType(format MediaFormat) string {
	switch format {
	case FormatHLS:
		return "application/vnd.apple.mpegurl"
	case FormatDASH:
		return "application/dash+xml"
	default:
		return "text/plain"
	}
}

func (s *S3MediaStorage) getSegmentURL(s3Key string) string {
	// In production, this would generate a CloudFront URL
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, s3Key)
}

func (s *S3MediaStorage) extractSegmentIndex(key string) int {
	// Extract segment index from filename like "segment001.ts"
	parts := strings.Split(path.Base(key), ".")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "segment") {
		return -1
	}

	var index int
	if _, err := fmt.Sscanf(parts[0], "segment%03d", &index); err != nil {
		common.Logger().Warn("failed to parse segment index",
			zap.String("key", key),
			zap.Error(err))
		return -1
	}
	return index
}

// CreateMediaStructure creates the S3 structure for a new media item
func (s *S3MediaStorage) CreateMediaStructure(mediaID string, qualities []Quality) error {
	ctx := context.Background()

	// Create placeholder objects for each quality directory
	for _, quality := range qualities {
		key := fmt.Sprintf("media/%s/%s/.placeholder", mediaID, quality)
		putInput := &s3.PutObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(key),
			Body:   strings.NewReader(""),
		}

		_, err := s.client.PutObject(ctx, putInput)
		if err != nil {
			return fmt.Errorf("create structure for quality %s: %w", quality, err)
		}
	}

	// Create initial metadata
	metadata := &MediaMetadata{
		MediaID:            mediaID,
		Status:             StatusPending,
		ProcessedAt:        time.Now(),
		AvailableQualities: qualities,
	}

	return s.UpdateMediaMetadata(mediaID, metadata)
}

// GetPresignedUploadURL generates a presigned URL for uploading media
func (s *S3MediaStorage) GetPresignedUploadURL(mediaID string, filename string) (string, error) {
	ctx := context.Background()
	key := fmt.Sprintf("media/%s/original/%s", mediaID, filename)

	presignClient := s3.NewPresignClient(s.client)
	request, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 1 * time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("generate presigned upload URL: %w", err)
	}

	return request.URL, nil
}
