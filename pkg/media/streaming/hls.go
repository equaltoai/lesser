package streaming

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
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
		resolution := info.Resolution      // Default resolution

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

// GenerateIFramePlaylist generates an I-frame only playlist for trick play with precise keyframe positioning
func (g *HLSGenerator) GenerateIFramePlaylist(mediaID string, quality Quality, metadata *MediaMetadata) string {
	var builder strings.Builder

	// I-frame playlists contain byte range requests to keyframes
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")
	builder.WriteString("#EXT-X-I-FRAMES-ONLY\n")
	builder.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	builder.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	// Get keyframe positions from metadata or calculate them
	keyframes := g.getKeyframePositions(mediaID, quality, metadata)

	if len(keyframes) == 0 {
		// Fallback to estimated keyframes if none provided
		keyframes = g.estimateKeyframePositions(metadata)
	}

	// Calculate target duration as the maximum interval between keyframes
	maxInterval := 0.0
	for i := 1; i < len(keyframes); i++ {
		interval := keyframes[i].PTS - keyframes[i-1].PTS
		if interval > maxInterval {
			maxInterval = interval
		}
	}

	// Set target duration (ceiling of max interval)
	targetDuration := int(maxInterval) + 1
	if targetDuration < 1 {
		targetDuration = g.config.SegmentDuration * 2
	}
	builder.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDuration))

	// Add I-frame entries with precise byte ranges
	for _, keyframe := range keyframes {
		// Each keyframe includes a byte range that captures just the I-frame data
		builder.WriteString(fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d\n", keyframe.ByteLength, keyframe.ByteOffset))
		builder.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", keyframe.Duration))
		builder.WriteString(keyframe.URI + "\n")
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

// Keyframe represents an I-frame position in the media stream
type Keyframe struct {
	PTS        float64 // Presentation timestamp in seconds
	ByteOffset int64   // Byte offset in the file
	ByteLength int64   // Length of the I-frame data in bytes
	Duration   float64 // Duration until next keyframe
	URI        string  // URI of the segment containing this I-frame
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

// getKeyframePositions retrieves actual keyframe positions from storage or media analysis
func (g *HLSGenerator) getKeyframePositions(mediaID string, quality Quality, metadata *MediaMetadata) []Keyframe {
	// Check if we have stored keyframe information
	if g.storage != nil {
		// Try to get keyframe data from storage
		keyframeData, err := g.storage.GetKeyframeData(mediaID, quality)
		if err == nil && keyframeData != nil {
			return g.parseKeyframeData(keyframeData, mediaID, quality)
		}
	}

	// Check if metadata contains keyframe information
	if metadata != nil && metadata.KeyframePositions != nil {
		return g.convertMetadataKeyframes(metadata.KeyframePositions, mediaID, quality)
	}

	return nil
}

// estimateKeyframePositions estimates keyframe positions based on GOP size and duration
func (g *HLSGenerator) estimateKeyframePositions(metadata *MediaMetadata) []Keyframe {
	if metadata == nil || metadata.Duration <= 0 {
		return nil
	}

	// Estimate GOP (Group of Pictures) size based on typical encoding settings
	// Default to 2-second GOPs for adaptive streaming
	gopDuration := 2.0

	// For higher quality levels, we might have longer GOPs
	switch metadata.VideoProfile {
	case "High":
		gopDuration = 3.0
	case "Main":
		gopDuration = 2.5
	}

	keyframes := []Keyframe{}
	numKeyframes := int(metadata.Duration / gopDuration)

	// Calculate estimated file size per keyframe
	// This is a rough estimate based on bitrate
	bytesPerSecond := int64(metadata.Bitrate * 125) // Convert kbps to bytes per second
	iframeSize := bytesPerSecond / 10               // I-frames are typically larger, estimate 10% of bitrate

	for i := 0; i <= numKeyframes; i++ {
		pts := float64(i) * gopDuration
		if pts > metadata.Duration {
			pts = metadata.Duration
		}

		// Calculate duration to next keyframe
		duration := gopDuration
		if pts+duration > metadata.Duration {
			duration = metadata.Duration - pts
		}

		// Estimate byte offset based on constant bitrate assumption
		byteOffset := int64(pts * float64(bytesPerSecond))

		// Determine which segment this keyframe belongs to
		segmentIndex := int(pts / float64(g.config.SegmentDuration))
		segmentURI := g.getSegmentURL(metadata.MediaID, Quality1080p, segmentIndex) // Default to 1080p for I-frames

		keyframe := Keyframe{
			PTS:        pts,
			ByteOffset: byteOffset,
			ByteLength: iframeSize,
			Duration:   duration,
			URI:        segmentURI,
		}

		keyframes = append(keyframes, keyframe)

		if pts >= metadata.Duration {
			break
		}
	}

	return keyframes
}

// parseKeyframeData parses keyframe data from storage format
// Supports multiple formats: JSON metadata, I-frame playlists, and binary stream analysis
func (g *HLSGenerator) parseKeyframeData(data []byte, mediaID string, quality Quality) []Keyframe {
	if len(data) == 0 {
		return []Keyframe{}
	}

	// Try to detect format and parse accordingly
	if keyframes := g.parseJSONKeyframeData(data, mediaID, quality); len(keyframes) > 0 {
		return keyframes
	}

	if keyframes := g.parseIFramePlaylist(data, mediaID, quality); len(keyframes) > 0 {
		return keyframes
	}

	// Try to parse as binary video stream data (last resort)
	if keyframes := g.parseVideoStreamKeyframes(data, mediaID, quality); len(keyframes) > 0 {
		return keyframes
	}

	return []Keyframe{}
}

// convertMetadataKeyframes converts metadata keyframe info to Keyframe structs
func (g *HLSGenerator) convertMetadataKeyframes(positions []float64, mediaID string, quality Quality) []Keyframe {
	keyframes := []Keyframe{}

	for i, pts := range positions {
		duration := 0.0
		if i < len(positions)-1 {
			duration = positions[i+1] - pts
		} else {
			// Last keyframe duration estimate
			duration = 2.0
		}

		segmentIndex := int(pts / float64(g.config.SegmentDuration))

		keyframe := Keyframe{
			PTS:        pts,
			ByteOffset: int64(pts * 100000), // Estimate
			ByteLength: 50000,               // Estimate
			Duration:   duration,
			URI:        g.getSegmentURL(mediaID, quality, segmentIndex),
		}

		keyframes = append(keyframes, keyframe)
	}

	return keyframes
}

// GenerateIFrameMasterPlaylist generates a master playlist with I-frame variants for trick play
func (g *HLSGenerator) GenerateIFrameMasterPlaylist(manifest *HLSManifest) string {
	var builder strings.Builder

	// Write HLS header
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")

	// Add I-frame variants for each quality
	for _, variant := range manifest.Variants {
		// I-frame playlists have higher bandwidth requirements for smooth scrubbing
		iframeBandwidth := variant.Bandwidth / 10 // Roughly 10% for I-frames only

		builder.WriteString(fmt.Sprintf("#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s,CODECS=\"%s\",URI=\"%s\"\n",
			iframeBandwidth,
			variant.Resolution,
			variant.Codecs,
			g.getIFramePlaylistURL(manifest.MediaID, variant.Quality),
		))
	}

	// Add regular variants
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

// getIFramePlaylistURL returns the URL for an I-frame playlist
func (g *HLSGenerator) getIFramePlaylistURL(mediaID string, quality Quality) string {
	return fmt.Sprintf("%s/media/%s/%s/iframe.m3u8", g.config.CDNBaseURL, mediaID, quality)
}

// KeyframeMetadata represents JSON keyframe metadata format
type KeyframeMetadata struct {
	MediaID   string          `json:"media_id"`
	Quality   string          `json:"quality"`
	Keyframes []KeyframeEntry `json:"keyframes"`
	GOP       int             `json:"gop_size,omitempty"`  // Group of Pictures size
	Framerate float64         `json:"framerate,omitempty"` // Video framerate
	Duration  float64         `json:"duration,omitempty"`  // Total duration in seconds
	Codec     string          `json:"codec,omitempty"`     // Video codec (H264, H265, etc.)
}

// KeyframeEntry represents a single keyframe in JSON format
type KeyframeEntry struct {
	PTS        float64 `json:"pts"`         // Presentation timestamp in seconds
	DTS        float64 `json:"dts"`         // Decode timestamp in seconds
	ByteOffset int64   `json:"byte_offset"` // Byte offset in the file
	ByteLength int64   `json:"byte_size"`   // Size of I-frame data in bytes
	FrameNum   int     `json:"frame_num"`   // Frame number
	Segment    int     `json:"segment"`     // Which HLS segment contains this keyframe
}

// parseJSONKeyframeData parses JSON-formatted keyframe metadata
func (g *HLSGenerator) parseJSONKeyframeData(data []byte, mediaID string, quality Quality) []Keyframe {
	// Check if data looks like JSON
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return []Keyframe{}
	}

	var metadata KeyframeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		// Try parsing as array of keyframe entries directly
		var entries []KeyframeEntry
		if err2 := json.Unmarshal(data, &entries); err2 != nil {
			return []Keyframe{}
		}
		metadata.Keyframes = entries
	}

	keyframes := make([]Keyframe, 0, len(metadata.Keyframes))
	for i, entry := range metadata.Keyframes {
		// Calculate duration to next keyframe
		duration := 0.0
		if i < len(metadata.Keyframes)-1 {
			duration = metadata.Keyframes[i+1].PTS - entry.PTS
		} else if metadata.GOP > 0 && metadata.Framerate > 0 {
			// Estimate duration based on GOP size and framerate
			duration = float64(metadata.GOP) / metadata.Framerate
		} else {
			// Default estimate
			duration = 2.0
		}

		// Determine segment URL
		segmentIndex := entry.Segment
		if segmentIndex <= 0 {
			// Calculate segment index from PTS
			segmentIndex = int(entry.PTS / float64(g.config.SegmentDuration))
		}

		keyframe := Keyframe{
			PTS:        entry.PTS,
			ByteOffset: entry.ByteOffset,
			ByteLength: entry.ByteLength,
			Duration:   duration,
			URI:        g.getSegmentURL(mediaID, quality, segmentIndex),
		}
		keyframes = append(keyframes, keyframe)
	}

	return keyframes
}

// parseIFramePlaylist parses HLS I-frame playlist format
func (g *HLSGenerator) parseIFramePlaylist(data []byte, _ string, _ Quality) []Keyframe {
	content := string(data)

	if !g.isValidIFramePlaylist(content) {
		return []Keyframe{}
	}

	lines := strings.Split(content, "\n")
	parser := g.newIFramePlaylistParser()
	keyframes := parser.parseLines(lines)

	g.fixKeyframeDurations(keyframes)
	return keyframes
}

// isValidIFramePlaylist checks if content is a valid HLS I-frame playlist
func (g *HLSGenerator) isValidIFramePlaylist(content string) bool {
	return strings.Contains(content, "#EXTM3U") && strings.Contains(content, "#EXT-X-I-FRAMES-ONLY")
}

// iFramePlaylistParser handles parsing of HLS I-frame playlist lines
type iFramePlaylistParser struct {
	currentPTS       float64
	currentByteRange int64
	currentOffset    int64
	currentURI       string
	extinfRegex      *regexp.Regexp
	byteRangeRegex   *regexp.Regexp
}

// newIFramePlaylistParser creates a new I-frame playlist parser
func (g *HLSGenerator) newIFramePlaylistParser() *iFramePlaylistParser {
	return &iFramePlaylistParser{
		extinfRegex:    regexp.MustCompile(`#EXTINF:([0-9.]+),`),
		byteRangeRegex: regexp.MustCompile(`#EXT-X-BYTERANGE:(\\d+)@(\\d+)`),
	}
}

// parseLines processes all lines in the playlist
func (p *iFramePlaylistParser) parseLines(lines []string) []Keyframe {
	var keyframes []Keyframe

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if keyframe := p.processLine(line, keyframes); keyframe != nil {
			keyframes = append(keyframes, *keyframe)
		}
	}

	return keyframes
}

// processLine handles a single line from the playlist
func (p *iFramePlaylistParser) processLine(line string, existingKeyframes []Keyframe) *Keyframe {
	switch {
	case strings.HasPrefix(line, "#EXTINF:"):
		p.parseExtInf(line)
		return nil
	case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
		p.parseByteRange(line)
		return nil
	case !strings.HasPrefix(line, "#") && line != "":
		return p.parseURILine(line, existingKeyframes)
	default:
		return nil
	}
}

// parseExtInf parses EXTINF line for duration information
func (p *iFramePlaylistParser) parseExtInf(line string) {
	matches := p.extinfRegex.FindStringSubmatch(line)
	if len(matches) > 1 {
		if duration, err := strconv.ParseFloat(matches[1], 64); err == nil {
			p.currentPTS += duration // Accumulate PTS
		}
	}
}

// parseByteRange parses EXT-X-BYTERANGE line for byte range information
func (p *iFramePlaylistParser) parseByteRange(line string) {
	matches := p.byteRangeRegex.FindStringSubmatch(line)
	if len(matches) > 2 {
		if length, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
			p.currentByteRange = length
		}
		if offset, err := strconv.ParseInt(matches[2], 10, 64); err == nil {
			p.currentOffset = offset
		}
	}
}

// parseURILine processes URI lines and creates keyframes
func (p *iFramePlaylistParser) parseURILine(line string, existingKeyframes []Keyframe) *Keyframe {
	p.currentURI = line

	if p.currentByteRange <= 0 {
		return nil
	}

	duration := p.calculateDuration(existingKeyframes)

	return &Keyframe{
		PTS:        p.currentPTS,
		ByteOffset: p.currentOffset,
		ByteLength: p.currentByteRange,
		Duration:   duration,
		URI:        p.currentURI,
	}
}

// calculateDuration determines the duration for the current keyframe
func (p *iFramePlaylistParser) calculateDuration(existingKeyframes []Keyframe) float64 {
	if len(existingKeyframes) > 0 {
		return p.currentPTS - existingKeyframes[len(existingKeyframes)-1].PTS
	}
	return 2.0 // Default duration
}

// parseVideoStreamKeyframes analyzes video stream data to find I-frames/keyframes
func (g *HLSGenerator) parseVideoStreamKeyframes(data []byte, mediaID string, quality Quality) []Keyframe {
	// This implements basic H.264/H.265 keyframe detection
	if len(data) < 1024 {
		return []Keyframe{}
	}

	// Try H.264 NAL unit parsing first
	if keyframes := g.parseH264Keyframes(data, mediaID, quality); len(keyframes) > 0 {
		return keyframes
	}

	// Try H.265 NAL unit parsing
	if keyframes := g.parseH265Keyframes(data, mediaID, quality); len(keyframes) > 0 {
		return keyframes
	}

	// Try MP4 atom-based parsing
	if keyframes := g.parseMP4Keyframes(data, mediaID, quality); len(keyframes) > 0 {
		return keyframes
	}

	return []Keyframe{}
}

// parseH264Keyframes finds H.264 I-frames by analyzing NAL units
func (g *HLSGenerator) parseH264Keyframes(data []byte, mediaID string, quality Quality) []Keyframe {
	keyframes := []Keyframe{}

	// H.264 NAL unit start codes: 0x000001 or 0x00000001
	startCode3 := []byte{0x00, 0x00, 0x01}
	startCode4 := []byte{0x00, 0x00, 0x00, 0x01}

	var currentPTS float64
	gopDuration := 2.0 // Assume 2-second GOP
	for i := 0; i < len(data)-4; i++ {
		var nalStart int
		var nalHeaderLen int

		// Check for 4-byte start code
		if bytes.Equal(data[i:i+4], startCode4) {
			nalStart = i + 4
			nalHeaderLen = 4
		} else if i < len(data)-3 && bytes.Equal(data[i:i+3], startCode3) {
			nalStart = i + 3
			nalHeaderLen = 3
		} else {
			continue
		}

		if nalStart >= len(data) {
			continue
		}

		// Parse NAL unit header
		nalHeader := data[nalStart]
		nalType := nalHeader & 0x1F // Lower 5 bits

		// H.264 NAL unit types:
		// 1 = Coded slice of a non-IDR picture
		// 5 = Coded slice of an IDR picture (I-frame)
		// 7 = Sequence parameter set
		// 8 = Picture parameter set
		if nalType == 5 { // IDR frame (I-frame)
			// Find next NAL unit to determine size
			nextNalPos := len(data)
			for j := nalStart + 1; j < len(data)-4; j++ {
				if bytes.Equal(data[j:j+4], startCode4) ||
					(j < len(data)-3 && bytes.Equal(data[j:j+3], startCode3)) {
					nextNalPos = j
					break
				}
			}

			byteLength := int64(nextNalPos - (nalStart - nalHeaderLen))

			// Calculate segment index
			segmentIndex := int(currentPTS / float64(g.config.SegmentDuration))

			keyframe := Keyframe{
				PTS:        currentPTS,
				ByteOffset: int64(nalStart - nalHeaderLen),
				ByteLength: byteLength,
				Duration:   gopDuration,
				URI:        g.getSegmentURL(mediaID, quality, segmentIndex),
			}
			keyframes = append(keyframes, keyframe)

			currentPTS += gopDuration
		}

		// Skip to after this NAL unit
		i = nalStart
	}

	return keyframes
}

// parseH265Keyframes finds H.265/HEVC I-frames by analyzing NAL units
func (g *HLSGenerator) parseH265Keyframes(data []byte, mediaID string, quality Quality) []Keyframe {
	keyframes := []Keyframe{}

	// H.265 uses similar NAL unit structure but different type values
	startCode3 := []byte{0x00, 0x00, 0x01}
	startCode4 := []byte{0x00, 0x00, 0x00, 0x01}

	var currentPTS float64
	gopDuration := 2.0

	for i := 0; i < len(data)-4; i++ {
		var nalStart int
		var nalHeaderLen int

		// Check for start codes
		if bytes.Equal(data[i:i+4], startCode4) {
			nalStart = i + 4
			nalHeaderLen = 4
		} else if i < len(data)-3 && bytes.Equal(data[i:i+3], startCode3) {
			nalStart = i + 3
			nalHeaderLen = 3
		} else {
			continue
		}

		if nalStart >= len(data) {
			continue
		}

		// H.265 has 2-byte NAL unit header
		if nalStart+1 >= len(data) {
			continue
		}

		nalHeader := binary.BigEndian.Uint16(data[nalStart : nalStart+2])
		nalType := (nalHeader >> 9) & 0x3F // Bits 15-9

		// H.265 NAL unit types:
		// 19-20 = IDR frames (I-frames)
		// 16-18 = BLA (Broken Link Access) frames
		if nalType >= 16 && nalType <= 20 { // I-frame types
			// Find next NAL unit
			nextNalPos := len(data)
			for j := nalStart + 2; j < len(data)-4; j++ {
				if bytes.Equal(data[j:j+4], startCode4) ||
					(j < len(data)-3 && bytes.Equal(data[j:j+3], startCode3)) {
					nextNalPos = j
					break
				}
			}

			byteLength := int64(nextNalPos - (nalStart - nalHeaderLen))
			segmentIndex := int(currentPTS / float64(g.config.SegmentDuration))

			keyframe := Keyframe{
				PTS:        currentPTS,
				ByteOffset: int64(nalStart - nalHeaderLen),
				ByteLength: byteLength,
				Duration:   gopDuration,
				URI:        g.getSegmentURL(mediaID, quality, segmentIndex),
			}
			keyframes = append(keyframes, keyframe)

			currentPTS += gopDuration
		}

		// Skip to after this NAL unit
		i = nalStart + 1
	}

	return keyframes
}

// parseMP4Keyframes extracts keyframe information from MP4 container format
func (g *HLSGenerator) parseMP4Keyframes(data []byte, mediaID string, quality Quality) []Keyframe {
	stssData, entryCount := g.parseStssAtom(data)
	if stssData == nil {
		return []Keyframe{}
	}

	sampleTimes := g.parseSampleTimes(data)
	chunkOffsets := g.parseChunkOffsets(data)

	keyframes := g.buildKeyframes(stssData, entryCount, sampleTimes, chunkOffsets, data, mediaID, quality)
	g.fixKeyframeDurations(keyframes)

	return keyframes
}

// parseStssAtom extracts sync sample atom data
func (g *HLSGenerator) parseStssAtom(data []byte) ([]byte, uint32) {
	stssIndex := bytes.Index(data, []byte("stss"))
	if stssIndex == -1 || stssIndex < 4 {
		return nil, 0
	}

	atomStart := stssIndex - 4
	if atomStart+8 > len(data) {
		return nil, 0
	}

	atomSize := binary.BigEndian.Uint32(data[atomStart : atomStart+4])
	if atomSize == 0 || int(atomSize) > len(data)-atomStart {
		return nil, 0
	}

	stssData := data[atomStart+8 : atomStart+int(atomSize)]
	if len(stssData) < 8 {
		return nil, 0
	}

	entryCount := binary.BigEndian.Uint32(stssData[4:8])
	if entryCount == 0 || entryCount > 10000 {
		return nil, 0
	}

	expectedSize := 8 + int(entryCount)*4
	if len(stssData) < expectedSize {
		return nil, 0
	}

	return stssData, entryCount
}

// parseSampleTimes extracts timing information from stts atom
func (g *HLSGenerator) parseSampleTimes(data []byte) []uint32 {
	sttsIndex := bytes.Index(data, []byte("stts"))
	if sttsIndex < 4 || sttsIndex+4 >= len(data) {
		return nil
	}

	sttsAtomStart := sttsIndex - 4
	sttsSize := binary.BigEndian.Uint32(data[sttsAtomStart : sttsAtomStart+4])
	if sttsSize == 0 || int(sttsSize) > len(data)-sttsAtomStart {
		return nil
	}

	sttsData := data[sttsAtomStart+8 : sttsAtomStart+int(sttsSize)]
	if len(sttsData) < 8 {
		return nil
	}

	sttsEntries := binary.BigEndian.Uint32(sttsData[4:8])
	if sttsEntries == 0 || sttsEntries > 1000 || len(sttsData) < int(8+sttsEntries*8) {
		return nil
	}

	return g.extractSampleTimes(sttsData, sttsEntries)
}

// extractSampleTimes processes time-to-sample entries
func (g *HLSGenerator) extractSampleTimes(sttsData []byte, sttsEntries uint32) []uint32 {
	var sampleTimes []uint32
	currentTime := uint32(0)

	for i := uint32(0); i < sttsEntries; i++ {
		offset := 8 + i*8
		sampleCount := binary.BigEndian.Uint32(sttsData[offset : offset+4])
		sampleDuration := binary.BigEndian.Uint32(sttsData[offset+4 : offset+8])

		for j := uint32(0); j < sampleCount; j++ {
			sampleTimes = append(sampleTimes, currentTime)
			currentTime += sampleDuration
		}
	}

	return sampleTimes
}

// parseChunkOffsets extracts chunk offset information from stco atom
func (g *HLSGenerator) parseChunkOffsets(data []byte) []uint32 {
	stcoIndex := bytes.Index(data, []byte("stco"))
	if stcoIndex < 4 || stcoIndex+4 >= len(data) {
		return nil
	}

	stcoAtomStart := stcoIndex - 4
	stcoSize := binary.BigEndian.Uint32(data[stcoAtomStart : stcoAtomStart+4])
	if stcoSize == 0 || int(stcoSize) > len(data)-stcoAtomStart {
		return nil
	}

	stcoData := data[stcoAtomStart+8 : stcoAtomStart+int(stcoSize)]
	if len(stcoData) < 8 {
		return nil
	}

	stcoEntries := binary.BigEndian.Uint32(stcoData[4:8])
	if stcoEntries == 0 || stcoEntries > 10000 || len(stcoData) < int(8+stcoEntries*4) {
		return nil
	}

	return g.extractChunkOffsets(stcoData, stcoEntries)
}

// extractChunkOffsets processes chunk offset entries
func (g *HLSGenerator) extractChunkOffsets(stcoData []byte, stcoEntries uint32) []uint32 {
	var chunkOffsets []uint32

	for i := uint32(0); i < stcoEntries; i++ {
		offset := 8 + i*4
		chunkOffset := binary.BigEndian.Uint32(stcoData[offset : offset+4])
		chunkOffsets = append(chunkOffsets, chunkOffset)
	}

	return chunkOffsets
}

// buildKeyframes creates keyframe objects from parsed data
func (g *HLSGenerator) buildKeyframes(stssData []byte, entryCount uint32, sampleTimes, chunkOffsets []uint32, data []byte, mediaID string, quality Quality) []Keyframe {
	var keyframes []Keyframe
	timescale := uint32(25) // Default to 25 fps

	for i := uint32(0); i < entryCount; i++ {
		keyframe := g.createKeyframe(stssData, i, sampleTimes, chunkOffsets, data, mediaID, quality, timescale, entryCount)
		keyframes = append(keyframes, keyframe)
	}

	return keyframes
}

// createKeyframe creates a single keyframe from sample data
func (g *HLSGenerator) createKeyframe(stssData []byte, index uint32, sampleTimes, chunkOffsets []uint32, data []byte, mediaID string, quality Quality, timescale, entryCount uint32) Keyframe {
	offset := 8 + index*4
	sampleNum := binary.BigEndian.Uint32(stssData[offset : offset+4])
	sampleIndex := sampleNum - 1

	pts := g.calculatePTS(sampleIndex, sampleTimes, timescale)
	byteOffset := g.calculateByteOffset(sampleIndex, chunkOffsets, data, entryCount)
	estimatedSize := g.estimateKeyframeSize(data, entryCount)
	segmentIndex := int(pts / float64(g.config.SegmentDuration))

	return Keyframe{
		PTS:        pts,
		ByteOffset: byteOffset,
		ByteLength: estimatedSize,
		Duration:   2.0, // Will be corrected later
		URI:        g.getSegmentURL(mediaID, quality, segmentIndex),
	}
}

// calculatePTS computes presentation timestamp for a sample
func (g *HLSGenerator) calculatePTS(sampleIndex uint32, sampleTimes []uint32, timescale uint32) float64 {
	if int(sampleIndex) < len(sampleTimes) {
		return float64(sampleTimes[sampleIndex]) / float64(timescale)
	}
	return float64(sampleIndex) * 2.0 // Assume 2-second GOP
}

// calculateByteOffset computes byte offset for a sample
func (g *HLSGenerator) calculateByteOffset(sampleIndex uint32, chunkOffsets []uint32, data []byte, entryCount uint32) int64 {
	if len(chunkOffsets) > 0 {
		chunkIndex := int(sampleIndex) % len(chunkOffsets)
		return int64(chunkOffsets[chunkIndex])
	}
	return int64(float64(len(data)) * float64(sampleIndex) / float64(entryCount))
}

// estimateKeyframeSize estimates the size of a keyframe
func (g *HLSGenerator) estimateKeyframeSize(data []byte, entryCount uint32) int64 {
	return int64(math.Max(float64(len(data))/float64(entryCount)*2, 4096))
}

// fixKeyframeDurations corrects duration values between keyframes
func (g *HLSGenerator) fixKeyframeDurations(keyframes []Keyframe) {
	for i := 0; i < len(keyframes)-1; i++ {
		keyframes[i].Duration = keyframes[i+1].PTS - keyframes[i].PTS
	}
}
