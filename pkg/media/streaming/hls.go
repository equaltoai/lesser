package streaming

import (
	"fmt"
	"strings"
	"time"
)

// HLSGenerator handles HLS manifest generation
type HLSGenerator struct {
	config  *StreamingConfig
	storage MediaStorage
}

// NewHLSGenerator creates a new HLS generator
func NewHLSGenerator(config *StreamingConfig, storage MediaStorage) *HLSGenerator {
	return &HLSGenerator{
		config:  config,
		storage: storage,
	}
}

// GenerateMasterPlaylist generates the HLS master playlist
func (g *HLSGenerator) GenerateMasterPlaylist(mediaID string, metadata *MediaMetadata) (*HLSManifest, error) {
	manifest := &HLSManifest{
		MediaID:       mediaID,
		Duration:      metadata.Duration,
		MasterURL:     g.getMasterPlaylistURL(mediaID),
		GeneratedAt:   time.Now(),
		CacheDuration: g.config.ManifestCacheTTL,
		Variants:      []HLSVariant{},
	}

	// Add variants for each available quality
	for _, quality := range metadata.AvailableQualities {
		info := GetQualityInfo(quality)
		
		// Use metadata-specific settings if available, otherwise use defaults
		bandwidth := info.Bandwidth * 1000 // Convert to bps for HLS (default)
		resolution := info.Resolution        // Default resolution
		
		if codecInfo, exists := metadata.QualitySettings[quality]; exists {
			if codecInfo.Bandwidth > 0 {
				bandwidth = codecInfo.Bandwidth
			}
			if codecInfo.Width > 0 && codecInfo.Height > 0 {
				resolution = fmt.Sprintf("%dx%d", codecInfo.Width, codecInfo.Height)
			}
		}
		
		variant := HLSVariant{
			Quality:     quality,
			Bandwidth:   bandwidth,
			Resolution:  resolution,
			PlaylistURL: g.getVariantPlaylistURL(mediaID, quality),
			Codecs:      g.getCodecsWithMetadata(mediaID, quality),
		}
		manifest.Variants = append(manifest.Variants, variant)
	}

	return manifest, nil
}

// GenerateMasterPlaylistContent generates the actual M3U8 content for the master playlist
func (g *HLSGenerator) GenerateMasterPlaylistContent(manifest *HLSManifest) string {
	var builder strings.Builder

	// Write HLS header
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")

	// Write each variant
	for _, variant := range manifest.Variants {
		builder.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s,CODECS=\"%s\"\n",
			variant.Bandwidth,
			variant.Resolution,
			variant.Codecs,
		))
		builder.WriteString(variant.PlaylistURL + "\n")
	}

	return builder.String()
}

// GenerateVariantPlaylist generates a variant playlist for a specific quality
func (g *HLSGenerator) GenerateVariantPlaylist(mediaID string, quality Quality, metadata *MediaMetadata) string {
	var builder strings.Builder

	// Calculate segment count
	segmentDuration := float64(g.config.SegmentDuration)
	segmentCount := int(metadata.Duration / segmentDuration)
	if metadata.Duration > float64(segmentCount)*segmentDuration {
		segmentCount++
	}

	// Write HLS header
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")
	builder.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", g.config.SegmentDuration))
	builder.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	builder.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	// Write segments
	for i := 0; i < segmentCount; i++ {
		duration := segmentDuration
		// Last segment might be shorter
		if i == segmentCount-1 {
			remainingDuration := metadata.Duration - float64(i)*segmentDuration
			if remainingDuration < segmentDuration {
				duration = remainingDuration
			}
		}

		builder.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", duration))
		builder.WriteString(g.getSegmentURL(mediaID, quality, i) + "\n")
	}

	// End of playlist
	builder.WriteString("#EXT-X-ENDLIST\n")

	return builder.String()
}

// GenerateLivePlaylist generates a live/sliding window playlist
func (g *HLSGenerator) GenerateLivePlaylist(mediaID string, quality Quality, startSegment, windowSize int) string {
	var builder strings.Builder

	// Write HLS header for live
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")
	builder.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", g.config.SegmentDuration))
	builder.WriteString("#EXT-X-PLAYLIST-TYPE:EVENT\n")
	builder.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", startSegment))

	// Write segments in the window
	for i := 0; i < windowSize; i++ {
		segmentIndex := startSegment + i
		builder.WriteString(fmt.Sprintf("#EXTINF:%.1f,\n", float64(g.config.SegmentDuration)))
		builder.WriteString(g.getSegmentURL(mediaID, quality, segmentIndex) + "\n")
	}

	// For live streams, we don't add ENDLIST

	return builder.String()
}

// GenerateIFramePlaylist generates an I-frame only playlist for trick play
func (g *HLSGenerator) GenerateIFramePlaylist(mediaID string, quality Quality, metadata *MediaMetadata) string {
	var builder strings.Builder

	// I-frame playlists contain byte range requests to keyframes
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")
	builder.WriteString("#EXT-X-I-FRAMES-ONLY\n")
	builder.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", g.config.SegmentDuration*3)) // I-frames are less frequent
	builder.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")

	// Add I-frame entries (simplified - in production this would need actual keyframe positions)
	iframeInterval := 3 // Assume keyframe every 3 seconds
	for i := 0; i < int(metadata.Duration); i += iframeInterval {
		builder.WriteString(fmt.Sprintf("#EXT-X-BYTERANGE:50000@%d\n", i*500000)) // Placeholder byte ranges
		builder.WriteString(fmt.Sprintf("#EXTINF:%.1f,\n", float64(iframeInterval)))
		builder.WriteString(g.getSegmentURL(mediaID, quality, i/g.config.SegmentDuration) + "\n")
	}

	builder.WriteString("#EXT-X-ENDLIST\n")

	return builder.String()
}

// Helper methods

func (g *HLSGenerator) getMasterPlaylistURL(mediaID string) string {
	return fmt.Sprintf("%s/media/%s/master.m3u8", g.config.CDNBaseURL, mediaID)
}

func (g *HLSGenerator) getVariantPlaylistURL(mediaID string, quality Quality) string {
	return fmt.Sprintf("%s/media/%s/%s/playlist.m3u8", g.config.CDNBaseURL, mediaID, quality)
}

func (g *HLSGenerator) getSegmentURL(mediaID string, quality Quality, index int) string {
	return fmt.Sprintf("%s/media/%s/%s/segment%03d.ts", g.config.CDNBaseURL, mediaID, quality, index)
}

func (g *HLSGenerator) getCodecs(quality Quality) string {
	// Attempt to get codec info from metadata if available
	// This method now supports both static fallback and dynamic metadata-based codec selection
	return g.getCodecsWithMetadata("", quality)
}

// getCodecsWithMetadata returns codec string using metadata if available, otherwise falls back to defaults
func (g *HLSGenerator) getCodecsWithMetadata(mediaID string, quality Quality) string {
	// Try to get metadata if mediaID is provided and storage is available
	if mediaID != "" && g.storage != nil {
		if metadata, err := g.storage.GetMediaMetadata(mediaID); err == nil {
			// Use quality-specific codec info if available
			if codecInfo, exists := metadata.QualitySettings[quality]; exists {
				if codecInfo.VideoCodec != "" && codecInfo.AudioCodec != "" {
					return fmt.Sprintf("%s,%s", codecInfo.VideoCodec, codecInfo.AudioCodec)
				}
			}
			
			// Use general codec info from metadata if available
			if metadata.VideoCodec != "" && metadata.AudioCodec != "" {
				return fmt.Sprintf("%s,%s", metadata.VideoCodec, metadata.AudioCodec)
			}
		}
	}
	
	// Fallback to quality-based defaults (preserving original behavior)
	switch quality {
	case Quality4K, Quality1080p:
		return "avc1.640028,mp4a.40.2" // H.264 High Profile + AAC
	case Quality720p, Quality480p:
		return "avc1.64001e,mp4a.40.2" // H.264 Main Profile + AAC
	default:
		return "avc1.42001e,mp4a.40.2" // H.264 Baseline Profile + AAC
	}
}

// GenerateWebVTTPlaylist generates a playlist for subtitles/captions
func (g *HLSGenerator) GenerateWebVTTPlaylist(mediaID string, language string, segments int) string {
	var builder strings.Builder

	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")
	builder.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", g.config.SegmentDuration))
	builder.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")

	for i := 0; i < segments; i++ {
		builder.WriteString(fmt.Sprintf("#EXTINF:%.1f,\n", float64(g.config.SegmentDuration)))
		builder.WriteString(fmt.Sprintf("%s/media/%s/subtitles/%s/segment%03d.vtt\n",
			g.config.CDNBaseURL, mediaID, language, i))
	}

	builder.WriteString("#EXT-X-ENDLIST\n")

	return builder.String()
}

// GenerateMasterPlaylistWithSubtitles generates a master playlist including subtitle tracks
func (g *HLSGenerator) GenerateMasterPlaylistWithSubtitles(manifest *HLSManifest, subtitles []SubtitleTrack) string {
	var builder strings.Builder

	// Write HLS header
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")

	// Add subtitle tracks
	for _, subtitle := range subtitles {
		builder.WriteString(fmt.Sprintf("#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subs\",NAME=\"%s\",DEFAULT=%s,AUTOSELECT=%s,FORCED=%s,LANGUAGE=\"%s\",URI=\"%s\"\n",
			subtitle.Name,
			boolToYesNo(subtitle.Default),
			boolToYesNo(subtitle.AutoSelect),
			boolToYesNo(subtitle.Forced),
			subtitle.Language,
			subtitle.URI,
		))
	}

	// Write each variant with subtitle group
	for _, variant := range manifest.Variants {
		builder.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s,CODECS=\"%s\",SUBTITLES=\"subs\"\n",
			variant.Bandwidth,
			variant.Resolution,
			variant.Codecs,
		))
		builder.WriteString(variant.PlaylistURL + "\n")
	}

	return builder.String()
}

// SubtitleTrack represents a subtitle/caption track
type SubtitleTrack struct {
	Name       string
	Language   string
	URI        string
	Default    bool
	AutoSelect bool
	Forced     bool
}

func boolToYesNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// ValidatePlaylist performs basic validation on generated playlists
func (g *HLSGenerator) ValidatePlaylist(content string) error {
	lines := strings.Split(content, "\n")

	if len(lines) < 2 {
		return fmt.Errorf("playlist too short")
	}

	if !strings.HasPrefix(lines[0], "#EXTM3U") {
		return fmt.Errorf("missing #EXTM3U header")
	}

	// Check for required tags
	hasVersion := false
	hasTargetDuration := false

	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-VERSION:") {
			hasVersion = true
		}
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			hasTargetDuration = true
		}
	}

	if !hasVersion {
		return fmt.Errorf("missing #EXT-X-VERSION tag")
	}

	// Target duration is required for variant playlists
	if strings.Contains(content, "#EXTINF:") && !hasTargetDuration {
		return fmt.Errorf("missing #EXT-X-TARGETDURATION tag in variant playlist")
	}

	return nil
}

// GetSegmentDurationFromPlaylist extracts actual segment duration from a playlist
func GetSegmentDurationFromPlaylist(playlistContent string) (float64, error) {
	lines := strings.Split(playlistContent, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			var duration int
			_, err := fmt.Sscanf(line, "#EXT-X-TARGETDURATION:%d", &duration)
			if err != nil {
				return 0, err
			}
			return float64(duration), nil
		}
	}

	return 0, fmt.Errorf("target duration not found in playlist")
}
