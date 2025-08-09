package streaming

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// S3MediaStorage implements MediaStorage using S3 for files and DynamORM for metadata
type S3MediaStorage struct {
	client               *s3.Client
	bucket               string
	region               string
	db                   core.DB  // DynamORM client for metadata storage
	metaCache            sync.Map // Cache for metadata
	cloudfrontDomain     string
	cloudfrontKeyPairID  string
	cloudfrontPrivateKey *rsa.PrivateKey
	urlSigner            *sign.URLSigner
}

// NewS3MediaStorage creates a new S3-based media storage with DynamORM for metadata
func NewS3MediaStorage(client *s3.Client, bucket, region string, db core.DB) *S3MediaStorage {
	storage := &S3MediaStorage{
		client: client,
		bucket: bucket,
		region: region,
		db:     db,
	}

	// Initialize CloudFront if environment variables are set
	if err := storage.initializeCloudFront(); err != nil {
		common.Logger().Warn("CloudFront initialization failed, falling back to S3 URLs",
			zap.Error(err))
	}

	return storage
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

	// Fetch from DynamoDB using DynamORM
	ctx := context.Background()
	var metadataModel models.MediaMetadata

	err := s.db.WithContext(ctx).Model(&models.MediaMetadata{}).
		Where("PK", "=", fmt.Sprintf("MEDIA#%s", mediaID)).
		Where("SK", "=", "METADATA").
		First(&metadataModel)

	if err != nil {
		// Check for "not found" error pattern in DynamORM
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "item not found") {
			return nil, fmt.Errorf("media metadata not found: %s", mediaID)
		}
		return nil, fmt.Errorf("get metadata from DynamoDB: %w", err)
	}

	// Convert from DynamORM model to streaming MediaMetadata
	metadata := &MediaMetadata{
		MediaID:            metadataModel.MediaID,
		OriginalURL:        metadataModel.OriginalURL,
		Duration:           metadataModel.Duration,
		Width:              metadataModel.Width,
		Height:             metadataModel.Height,
		Bitrate:            metadataModel.Bitrate,
		FileSize:           metadataModel.FileSize,
		ProcessedAt:        metadataModel.ProcessedAt,
		AvailableQualities: convertQualities(metadataModel.AvailableQualities),
		Status:             ProcessingStatus(metadataModel.Status),
		VideoCodec:         metadataModel.VideoCodec,
		AudioCodec:         metadataModel.AudioCodec,
		VideoProfile:       metadataModel.VideoProfile,
		VideoLevel:         metadataModel.VideoLevel,
		QualitySettings:    convertQualitySettings(metadataModel.QualitySettings),
	}

	// Cache the metadata
	s.metaCache.Store(mediaID, &cachedMetadata{
		metadata: metadata,
		cachedAt: time.Now(),
	})

	return metadata, nil
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

	// Convert from streaming MediaMetadata to DynamORM model
	metadataModel := &models.MediaMetadata{
		MediaID:            metadata.MediaID,
		OriginalURL:        metadata.OriginalURL,
		Duration:           metadata.Duration,
		Width:              metadata.Width,
		Height:             metadata.Height,
		Bitrate:            metadata.Bitrate,
		FileSize:           metadata.FileSize,
		ProcessedAt:        metadata.ProcessedAt,
		AvailableQualities: convertQualitiesFromStreaming(metadata.AvailableQualities),
		Status:             string(metadata.Status),
		VideoCodec:         metadata.VideoCodec,
		AudioCodec:         metadata.AudioCodec,
		VideoProfile:       metadata.VideoProfile,
		VideoLevel:         metadata.VideoLevel,
		QualitySettings:    convertQualitySettingsFromStreaming(metadata.QualitySettings),
	}

	// Set the keys
	metadataModel.UpdateKeys()

	// Try to update first, if not found then create
	err := s.db.WithContext(ctx).Model(metadataModel).
		Where("PK", "=", fmt.Sprintf("MEDIA#%s", mediaID)).
		Where("SK", "=", "METADATA").
		Update()

	if err != nil {
		// Check for "not found" error pattern in DynamORM
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "item not found") {
			// Record doesn't exist, create it
			err = s.db.WithContext(ctx).Model(metadataModel).Create()
			if err != nil {
				return fmt.Errorf("create metadata in DynamoDB: %w", err)
			}
		} else {
			return fmt.Errorf("update metadata in DynamoDB: %w", err)
		}
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
	// Use CloudFront if configured, otherwise fall back to S3
	if s.urlSigner != nil && s.cloudfrontDomain != "" {
		return s.generateCloudFrontURL(s3Key)
	}

	// Fallback to S3 direct URL
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

	// Create placeholder objects for each quality directory in S3
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

	// Create initial metadata in DynamoDB
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

// initializeCloudFront sets up CloudFront signing if environment variables are configured
func (s *S3MediaStorage) initializeCloudFront() error {
	// Get CloudFront configuration from environment
	cfDomain := os.Getenv("CLOUDFRONT_DISTRIBUTION_DOMAIN")
	cfKeyPairID := os.Getenv("CLOUDFRONT_KEY_PAIR_ID")
	cfPrivateKeyPath := os.Getenv("CLOUDFRONT_PRIVATE_KEY_PATH")
	cfPrivateKeyContent := os.Getenv("CLOUDFRONT_PRIVATE_KEY")

	// Check if CloudFront is configured
	if cfDomain == "" || cfKeyPairID == "" {
		return fmt.Errorf("CloudFront not configured: missing domain or key pair ID")
	}

	// Load private key
	var privateKeyPEM []byte
	var err error

	if cfPrivateKeyPath != "" {
		// Validate the file path to prevent directory traversal
		cleanPath := filepath.Clean(cfPrivateKeyPath)
		if !filepath.IsAbs(cleanPath) || strings.Contains(cleanPath, "..") {
			return fmt.Errorf("invalid CloudFront private key path: %s", cfPrivateKeyPath)
		}
		// Load from file path
		privateKeyPEM, err = os.ReadFile(cleanPath)
		if err != nil {
			return fmt.Errorf("failed to read CloudFront private key file: %w", err)
		}
	} else if cfPrivateKeyContent != "" {
		// Use key content directly
		privateKeyPEM = []byte(cfPrivateKeyContent)
	} else {
		return fmt.Errorf("CloudFront private key not provided")
	}

	// Parse PEM block
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return fmt.Errorf("invalid RSA private key PEM")
	}

	// Parse RSA private key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse RSA private key: %w", err)
	}

	// Create URL signer
	urlSigner := sign.NewURLSigner(cfKeyPairID, privateKey)

	// Store configuration
	s.cloudfrontDomain = cfDomain
	s.cloudfrontKeyPairID = cfKeyPairID
	s.cloudfrontPrivateKey = privateKey
	s.urlSigner = urlSigner

	common.Logger().Info("CloudFront URL signing enabled",
		zap.String("domain", cfDomain),
		zap.String("key_pair_id", cfKeyPairID))

	return nil
}

// generateCloudFrontURL creates a signed CloudFront URL for the given S3 key
func (s *S3MediaStorage) generateCloudFrontURL(s3Key string) string {
	if s.urlSigner == nil {
		// Fallback to S3 if signer not available
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, s3Key)
	}

	// Create the CloudFront URL
	rawURL := fmt.Sprintf("https://%s/%s", s.cloudfrontDomain, s3Key)

	// Set expiration to 1 hour from now
	expiresAt := time.Now().Add(time.Hour)

	// Sign the URL
	signedURL, err := s.urlSigner.Sign(rawURL, expiresAt)
	if err != nil {
		common.Logger().Error("Failed to sign CloudFront URL, falling back to S3",
			zap.String("url", rawURL),
			zap.Error(err))
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, s3Key)
	}

	return signedURL
}

// GetCloudFrontDomain returns the configured CloudFront domain
func (s *S3MediaStorage) GetCloudFrontDomain() string {
	return s.cloudfrontDomain
}

// IsCloudFrontEnabled returns true if CloudFront is properly configured
func (s *S3MediaStorage) IsCloudFrontEnabled() bool {
	return s.urlSigner != nil && s.cloudfrontDomain != ""
}

// Helper conversion functions for types between streaming and DynamORM models

// convertQualities converts from DynamORM model string slice to streaming Quality slice
func convertQualities(qualities []string) []Quality {
	result := make([]Quality, len(qualities))
	for i, q := range qualities {
		result[i] = Quality(q)
	}
	return result
}

// convertQualitiesFromStreaming converts from streaming Quality slice to string slice
func convertQualitiesFromStreaming(qualities []Quality) []string {
	result := make([]string, len(qualities))
	for i, q := range qualities {
		result[i] = string(q)
	}
	return result
}

// convertQualitySettings converts from DynamORM model QualityCodecInfo to streaming QualityCodecInfo
func convertQualitySettings(settings map[string]models.QualityCodecInfo) map[Quality]QualityCodecInfo {
	if settings == nil {
		return nil
	}
	result := make(map[Quality]QualityCodecInfo)
	for k, v := range settings {
		result[Quality(k)] = QualityCodecInfo{
			VideoCodec: v.VideoCodec,
			AudioCodec: v.AudioCodec,
			Bandwidth:  v.Bandwidth,
			Width:      v.Width,
			Height:     v.Height,
		}
	}
	return result
}

// convertQualitySettingsFromStreaming converts from streaming QualityCodecInfo to DynamORM model QualityCodecInfo
func convertQualitySettingsFromStreaming(settings map[Quality]QualityCodecInfo) map[string]models.QualityCodecInfo {
	if settings == nil {
		return nil
	}
	result := make(map[string]models.QualityCodecInfo)
	for k, v := range settings {
		result[string(k)] = models.QualityCodecInfo{
			VideoCodec: v.VideoCodec,
			AudioCodec: v.AudioCodec,
			Bandwidth:  v.Bandwidth,
			Width:      v.Width,
			Height:     v.Height,
		}
	}
	return result
}

// GetKeyframeData retrieves keyframe/I-frame data for a media item at a specific quality level
func (s *S3MediaStorage) GetKeyframeData(mediaID string, quality Quality) ([]byte, error) {
	ctx := context.Background()
	
	// Construct the S3 key for keyframe data
	// Keyframe data is typically stored alongside the media segments
	keyframeKey := fmt.Sprintf("media/%s/%s/keyframes.json", mediaID, quality)
	
	// First, check if explicit keyframe data exists
	getInput := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(keyframeKey),
	}
	
	result, err := s.client.GetObject(ctx, getInput)
	if err != nil {
		// If keyframe data doesn't exist, check for I-frame playlist
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NotFound") {
			// Try to get I-frame playlist instead
			iframeKey := fmt.Sprintf("media/%s/%s/iframe.m3u8", mediaID, quality)
			iframeInput := &s3.GetObjectInput{
				Bucket: aws.String(s.bucket),
				Key:    aws.String(iframeKey),
			}
			
			iframeResult, iframeErr := s.client.GetObject(ctx, iframeInput)
			if iframeErr != nil {
				// If neither exists, return nil (no keyframe data available)
				if strings.Contains(iframeErr.Error(), "NoSuchKey") || strings.Contains(iframeErr.Error(), "NotFound") {
					return nil, nil
				}
				return nil, fmt.Errorf("failed to get keyframe data: %w", iframeErr)
			}
			defer func() { _ = iframeResult.Body.Close() }()
			
			// Read I-frame playlist data
			iframeData, readErr := io.ReadAll(iframeResult.Body)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read I-frame playlist: %w", readErr)
			}
			
			return iframeData, nil
		}
		return nil, fmt.Errorf("failed to get keyframe data: %w", err)
	}
	defer func() { _ = result.Body.Close() }()
	
	// Read keyframe data
	keyframeData, readErr := io.ReadAll(result.Body)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read keyframe data: %w", readErr)
	}
	
	return keyframeData, nil
}
