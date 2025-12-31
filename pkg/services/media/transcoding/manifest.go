// Package transcoding provides HLS/DASH manifest generation
package transcoding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// ManifestService handles HLS and DASH manifest operations
type ManifestService struct {
	s3Client  s3API
	logger    *zap.Logger
	bucket    string
	cdnDomain string
}

type s3API interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// ManifestConfig holds configuration for manifest service
type ManifestConfig struct {
	Bucket    string // S3 bucket for manifests
	CDNDomain string // CloudFront domain (optional)
}

// ManifestInfo contains information about generated manifests
type ManifestInfo struct {
	MediaID         string
	HLSMasterURL    string
	DASHManifestURL string
	Variants        []VariantInfo
	ThumbnailURLs   []string
	GeneratedAt     time.Time
}

// VariantInfo contains information about a specific quality variant
type VariantInfo struct {
	Quality        string
	Width          int
	Height         int
	Bitrate        int
	Codec          string
	HLSPlaylistURL string
	DASHSegmentURL string
}

var (
	// ErrManifestNotFound is returned when a manifest is not found
	ErrManifestNotFound = errors.New("manifest not found")
	// ErrManifestGenerationFailed is returned when manifest generation fails
	ErrManifestGenerationFailed = errors.New("manifest generation failed")
)

// NewManifestService creates a new manifest service
func NewManifestService(s3Client s3API, config ManifestConfig, logger *zap.Logger) *ManifestService {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &ManifestService{
		s3Client:  s3Client,
		logger:    logger,
		bucket:    config.Bucket,
		cdnDomain: config.CDNDomain,
	}
}

// GetManifestInfo retrieves manifest information for a media item
func (m *ManifestService) GetManifestInfo(ctx context.Context, mediaID string, outputPrefix string) (*ManifestInfo, error) {
	m.logger.Debug("getting manifest info",
		zap.String("media_id", mediaID),
		zap.String("output_prefix", outputPrefix))

	info := &ManifestInfo{
		MediaID:     mediaID,
		GeneratedAt: time.Now(),
		Variants:    []VariantInfo{},
	}

	// Check for HLS master playlist
	hlsMasterKey := fmt.Sprintf("%s/hls/master.m3u8", outputPrefix)
	if exists, err := m.checkS3ObjectExists(ctx, hlsMasterKey); err == nil && exists {
		info.HLSMasterURL = m.buildURL(hlsMasterKey)

		// Get HLS variants
		variants, err := m.getHLSVariants(ctx, outputPrefix)
		if err != nil {
			m.logger.Warn("failed to get HLS variants",
				zap.String("media_id", mediaID),
				zap.Error(err))
		} else {
			info.Variants = append(info.Variants, variants...)
		}
	}

	// Check for DASH manifest
	dashManifestKey := fmt.Sprintf("%s/dash/manifest.mpd", outputPrefix)
	if exists, err := m.checkS3ObjectExists(ctx, dashManifestKey); err == nil && exists {
		info.DASHManifestURL = m.buildURL(dashManifestKey)
	}

	// Get thumbnail URLs
	thumbnails, err := m.getThumbnails(ctx, outputPrefix)
	if err != nil {
		m.logger.Warn("failed to get thumbnails",
			zap.String("media_id", mediaID),
			zap.Error(err))
	} else {
		info.ThumbnailURLs = thumbnails
	}

	// If no manifests found, return error
	if info.HLSMasterURL == "" && info.DASHManifestURL == "" {
		return nil, ErrManifestNotFound
	}

	return info, nil
}

// GenerateHLSMasterPlaylist generates an HLS master playlist from variants
func (m *ManifestService) GenerateHLSMasterPlaylist(ctx context.Context, mediaID string, outputPrefix string, variants []VariantInfo) error {
	m.logger.Info("generating HLS master playlist",
		zap.String("media_id", mediaID),
		zap.Int("variant_count", len(variants)))

	// Build HLS master playlist content
	var content strings.Builder
	content.WriteString("#EXTM3U\n")
	content.WriteString("#EXT-X-VERSION:3\n\n")

	for _, variant := range variants {
		// Write stream info
		content.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"%s\"\n",
			variant.Bitrate,
			variant.Width,
			variant.Height,
			variant.Codec))

		// Write variant playlist filename
		content.WriteString(fmt.Sprintf("%s.m3u8\n", variant.Quality))
	}

	// Upload master playlist to S3
	key := fmt.Sprintf("%s/hls/master.m3u8", outputPrefix)
	err := m.uploadToS3(ctx, key, content.String(), "application/vnd.apple.mpegurl")
	if err != nil {
		m.logger.Error("failed to upload HLS master playlist",
			zap.String("media_id", mediaID),
			zap.Error(err))
		return errors.Join(ErrManifestGenerationFailed, err)
	}

	m.logger.Info("HLS master playlist generated",
		zap.String("media_id", mediaID),
		zap.String("key", key))

	return nil
}

// GenerateDASHManifest generates a DASH manifest from variants
func (m *ManifestService) GenerateDASHManifest(ctx context.Context, mediaID string, outputPrefix string, variants []VariantInfo, duration int) error {
	m.logger.Info("generating DASH manifest",
		zap.String("media_id", mediaID),
		zap.Int("variant_count", len(variants)))

	// Build DASH manifest content (simplified MPD)
	var content strings.Builder
	content.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	content.WriteString("<MPD xmlns=\"urn:mpeg:dash:schema:mpd:2011\" ")
	content.WriteString("type=\"static\" ")
	content.WriteString(fmt.Sprintf("mediaPresentationDuration=\"PT%dS\" ", duration))
	content.WriteString("minBufferTime=\"PT2S\" ")
	content.WriteString("profiles=\"urn:mpeg:dash:profile:isoff-main:2011\">\n")

	content.WriteString("  <Period>\n")

	// Add video adaptation set
	content.WriteString("    <AdaptationSet mimeType=\"video/mp4\" contentType=\"video\">\n")
	for _, variant := range variants {
		content.WriteString(fmt.Sprintf("      <Representation id=\"%s\" bandwidth=\"%d\" width=\"%d\" height=\"%d\" codecs=\"%s\">\n",
			variant.Quality,
			variant.Bitrate,
			variant.Width,
			variant.Height,
			variant.Codec))
		content.WriteString(fmt.Sprintf("        <BaseURL>%s.mp4</BaseURL>\n", variant.Quality))
		content.WriteString("      </Representation>\n")
	}
	content.WriteString("    </AdaptationSet>\n")

	content.WriteString("  </Period>\n")
	content.WriteString("</MPD>\n")

	// Upload DASH manifest to S3
	key := fmt.Sprintf("%s/dash/manifest.mpd", outputPrefix)
	err := m.uploadToS3(ctx, key, content.String(), "application/dash+xml")
	if err != nil {
		m.logger.Error("failed to upload DASH manifest",
			zap.String("media_id", mediaID),
			zap.Error(err))
		return errors.Join(ErrManifestGenerationFailed, err)
	}

	m.logger.Info("DASH manifest generated",
		zap.String("media_id", mediaID),
		zap.String("key", key))

	return nil
}

// PreloadManifests downloads manifests to warm up CDN cache
func (m *ManifestService) PreloadManifests(ctx context.Context, mediaIDs []string) error {
	m.logger.Info("preloading manifests", zap.Int("count", len(mediaIDs)))

	for _, mediaID := range mediaIDs {
		// Try to get manifest info to trigger CDN cache
		_, err := m.GetManifestInfo(ctx, mediaID, mediaID)
		if err != nil {
			m.logger.Warn("failed to preload manifest",
				zap.String("media_id", mediaID),
				zap.Error(err))
			// Continue with other media IDs
			continue
		}
	}

	return nil
}

// getHLSVariants discovers HLS variant playlists
func (m *ManifestService) getHLSVariants(ctx context.Context, outputPrefix string) ([]VariantInfo, error) {
	// List objects with HLS prefix
	prefix := fmt.Sprintf("%s/hls/", outputPrefix)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(m.bucket),
		Prefix: aws.String(prefix),
	}

	result, err := m.s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	variants := []VariantInfo{}
	for _, obj := range result.Contents {
		key := aws.ToString(obj.Key)

		// Skip master playlist
		if strings.HasSuffix(key, "master.m3u8") {
			continue
		}

		// Only process variant playlists
		if !strings.HasSuffix(key, ".m3u8") {
			continue
		}

		// Extract quality from filename (e.g., "720p.m3u8")
		parts := strings.Split(key, "/")
		filename := parts[len(parts)-1]
		quality := strings.TrimSuffix(filename, ".m3u8")

		// Get quality params
		width, height, bitrate := m.getQualityParams(quality)

		variant := VariantInfo{
			Quality:        quality,
			Width:          width,
			Height:         height,
			Bitrate:        bitrate,
			Codec:          "avc1.42E01E,mp4a.40.2",
			HLSPlaylistURL: m.buildURL(key),
		}

		variants = append(variants, variant)
	}

	return variants, nil
}

// getThumbnails discovers thumbnail URLs
func (m *ManifestService) getThumbnails(ctx context.Context, outputPrefix string) ([]string, error) {
	prefix := fmt.Sprintf("%s/thumbnails/", outputPrefix)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(m.bucket),
		Prefix: aws.String(prefix),
	}

	result, err := m.s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	thumbnails := []string{}
	for _, obj := range result.Contents {
		key := aws.ToString(obj.Key)
		if strings.HasSuffix(key, ".jpg") || strings.HasSuffix(key, ".png") {
			thumbnails = append(thumbnails, m.buildURL(key))
		}
	}

	return thumbnails, nil
}

// checkS3ObjectExists checks if an S3 object exists
func (m *ManifestService) checkS3ObjectExists(ctx context.Context, key string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(m.bucket),
		Key:    aws.String(key),
	}

	_, err := m.s3Client.HeadObject(ctx, input)
	if err != nil {
		// Object doesn't exist
		return false, nil
	}

	return true, nil
}

// uploadToS3 uploads content to S3
func (m *ManifestService) uploadToS3(ctx context.Context, key, content, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(m.bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(content),
		ContentType: aws.String(contentType),
	}

	_, err := m.s3Client.PutObject(ctx, input)
	return err
}

// buildURL builds a URL for an S3 object (using CDN if configured)
func (m *ManifestService) buildURL(key string) string {
	if m.cdnDomain != "" {
		return fmt.Sprintf("https://%s/%s", m.cdnDomain, key)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", m.bucket, key)
}

// getQualityParams returns width, height, and bitrate for a quality level
func (m *ManifestService) getQualityParams(quality string) (width, height, bitrate int) {
	switch quality {
	case "2160p", "4k":
		return 3840, 2160, 15000000
	case "1080p":
		return 1920, 1080, 5000000
	case "720p":
		return 1280, 720, 3000000
	case "480p":
		return 854, 480, 1500000
	case "360p":
		return 640, 360, 800000
	case "240p":
		return 426, 240, 400000
	default:
		return 1280, 720, 3000000
	}
}
