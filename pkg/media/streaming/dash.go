package streaming

import (
	"encoding/xml"
	"fmt"
	"time"
)

// DASHGenerator handles DASH manifest generation
type DASHGenerator struct {
	config  *StreamingConfig
	storage MediaStorage
}

// NewDASHGenerator creates a new DASH generator
func NewDASHGenerator(config *StreamingConfig, storage MediaStorage) *DASHGenerator {
	return &DASHGenerator{
		config:  config,
		storage: storage,
	}
}

// GenerateMPD generates a DASH Media Presentation Description
func (g *DASHGenerator) GenerateMPD(mediaID string, metadata *MediaMetadata) (*DASHManifest, error) {
	manifest := &DASHManifest{
		MediaID:         mediaID,
		Duration:        metadata.Duration,
		MinBufferTime:   float64(g.config.SegmentDuration),
		ManifestURL:     g.getManifestURL(mediaID),
		GeneratedAt:     time.Now(),
		CacheDuration:   g.config.ManifestCacheTTL,
		Representations: []DASHRepresentation{},
	}

	// Add representations for each available quality
	for i, quality := range metadata.AvailableQualities {
		info := GetQualityInfo(quality)
		representation := DASHRepresentation{
			ID:              fmt.Sprintf("video_%d", i),
			Quality:         quality,
			Bandwidth:       info.Bandwidth * 1000, // Convert to bps
			Width:           info.Width,
			Height:          info.Height,
			Codecs:          g.getCodecs(quality),
			BaseURL:         g.getBaseURL(mediaID, quality),
			SegmentTemplate: g.getSegmentTemplate(),
		}
		manifest.Representations = append(manifest.Representations, representation)
	}

	return manifest, nil
}

// GenerateMPDContent generates the actual MPD XML content
func (g *DASHGenerator) GenerateMPDContent(manifest *DASHManifest) (string, error) {
	mpd := &MPD{
		XMLName:                   xml.Name{Local: "MPD"},
		XMLNS:                     "urn:mpeg:dash:schema:mpd:2011",
		Type:                      "static",
		MediaPresentationDuration: g.formatDuration(manifest.Duration),
		MinBufferTime:             g.formatDuration(manifest.MinBufferTime),
		Profiles:                  "urn:mpeg:dash:profile:isoff-on-demand:2011",
		Periods: []Period{
			{
				ID:       "0",
				Duration: g.formatDuration(manifest.Duration),
				AdaptationSets: []AdaptationSet{
					g.createVideoAdaptationSet(manifest),
				},
			},
		},
	}

	// Marshal to XML
	output, err := xml.MarshalIndent(mpd, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal MPD: %w", err)
	}

	// Add XML declaration
	return xml.Header + string(output), nil
}

// createVideoAdaptationSet creates the video adaptation set
func (g *DASHGenerator) createVideoAdaptationSet(manifest *DASHManifest) AdaptationSet {
	adaptationSet := AdaptationSet{
		ID:               "0",
		MimeType:         "video/mp4",
		SegmentAlignment: true,
		StartWithSAP:     1,
		MaxWidth:         0,
		MaxHeight:        0,
		MaxFrameRate:     "30",
		Par:              "16:9",
		Representations:  []Representation{},
	}

	// Find max dimensions
	for _, rep := range manifest.Representations {
		if rep.Width > adaptationSet.MaxWidth {
			adaptationSet.MaxWidth = rep.Width
		}
		if rep.Height > adaptationSet.MaxHeight {
			adaptationSet.MaxHeight = rep.Height
		}
	}

	// Add representations
	for _, rep := range manifest.Representations {
		adaptationSet.Representations = append(adaptationSet.Representations, Representation{
			ID:        rep.ID,
			Bandwidth: rep.Bandwidth,
			Width:     rep.Width,
			Height:    rep.Height,
			Codecs:    rep.Codecs,
			BaseURL: BaseURL{
				Value: rep.BaseURL,
			},
			SegmentTemplate: SegmentTemplate{
				Media:          rep.SegmentTemplate,
				Initialization: "init-$RepresentationID$.mp4",
				StartNumber:    0,
				Duration:       g.config.SegmentDuration * 1000, // In milliseconds
				Timescale:      1000,
			},
		})
	}

	return adaptationSet
}

// Helper methods

func (g *DASHGenerator) getManifestURL(mediaID string) string {
	return fmt.Sprintf("%s/media/%s/manifest.mpd", g.config.CDNBaseURL, mediaID)
}

func (g *DASHGenerator) getBaseURL(mediaID string, quality Quality) string {
	return fmt.Sprintf("%s/media/%s/%s/", g.config.CDNBaseURL, mediaID, quality)
}

func (g *DASHGenerator) getSegmentTemplate() string {
	return "segment$Number$.m4s"
}

func (g *DASHGenerator) getCodecs(quality Quality) string {
	// Return appropriate codec string based on quality
	switch quality {
	case Quality4K, Quality1080p:
		return "avc1.640028" // H.264 High Profile
	case Quality720p, Quality480p:
		return "avc1.64001e" // H.264 Main Profile
	default:
		return "avc1.42001e" // H.264 Baseline Profile
	}
}

func (g *DASHGenerator) formatDuration(seconds float64) string {
	// Format duration as ISO 8601 duration (PT#S)
	return fmt.Sprintf("PT%.1fS", seconds)
}

func (g *DASHGenerator) getLiveManifestURL(mediaID string) string {
	return fmt.Sprintf("%s/live/%s/manifest.mpd", g.config.CDNBaseURL, mediaID)
}

func (g *DASHGenerator) getLiveBaseURL(mediaID string, quality Quality) string {
	return fmt.Sprintf("%s/live/%s/%s/", g.config.CDNBaseURL, mediaID, quality)
}

func (g *DASHGenerator) getLiveSegmentTemplate() string {
	// Use time-based template for live segments to support DVR functionality
	return "segment-$Time$.m4s"
}

func (g *DASHGenerator) formatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// getCodecsForLive returns codec string optimized for live streaming
func (g *DASHGenerator) getCodecsForLive(quality Quality, metadata *MediaMetadata) string {
	// Use quality-specific codec info if available
	if codecInfo, exists := metadata.QualitySettings[quality]; exists {
		if codecInfo.VideoCodec != "" {
			return codecInfo.VideoCodec + ",mp4a.40.2" // Add AAC audio
		}
	}
	
	// Use general metadata codec info
	if metadata.VideoCodec != "" {
		return metadata.VideoCodec + ",mp4a.40.2"
	}
	
	// Fallback to quality-based codec selection optimized for live streaming
	switch quality {
	case Quality4K:
		return "avc1.640032,mp4a.40.2" // H.264 High Profile Level 5.0 for 4K
	case Quality1080p:
		return "avc1.640028,mp4a.40.2" // H.264 High Profile Level 4.0 for 1080p
	case Quality720p:
		return "avc1.64001f,mp4a.40.2" // H.264 Main Profile Level 3.1 for 720p
	case Quality480p:
		return "avc1.64001e,mp4a.40.2" // H.264 Main Profile Level 3.0 for 480p
	default:
		return "avc1.42001e,mp4a.40.2" // H.264 Baseline Profile for lower qualities
	}
}

// DASH XML structures

// MPD represents a DASH Media Presentation Description document
type MPD struct {
	XMLName                   xml.Name `xml:"MPD"`
	XMLNS                     string   `xml:"xmlns,attr"`
	Type                      string   `xml:"type,attr"`
	MediaPresentationDuration string   `xml:"mediaPresentationDuration,attr"`
	MinBufferTime             string   `xml:"minBufferTime,attr"`
	Profiles                  string   `xml:"profiles,attr"`
	Periods                   []Period `xml:"Period"`
}

// Period represents a period within a DASH manifest
type Period struct {
	ID             string          `xml:"id,attr"`
	Start          string          `xml:"start,attr,omitempty"`
	Duration       string          `xml:"duration,attr,omitempty"`
	AdaptationSets []AdaptationSet `xml:"AdaptationSet"`
}

// AdaptationSet represents a set of interchangeable media components
type AdaptationSet struct {
	ID               string           `xml:"id,attr"`
	MimeType         string           `xml:"mimeType,attr"`
	SegmentAlignment bool             `xml:"segmentAlignment,attr"`
	StartWithSAP     int              `xml:"startWithSAP,attr"`
	MaxWidth         int              `xml:"maxWidth,attr"`
	MaxHeight        int              `xml:"maxHeight,attr"`
	MaxFrameRate     string           `xml:"maxFrameRate,attr"`
	Par              string           `xml:"par,attr"`
	Representations  []Representation `xml:"Representation"`
}

// Representation represents a single encoded version of the content
type Representation struct {
	ID              string      `xml:"id,attr"`
	Bandwidth       int         `xml:"bandwidth,attr"`
	Width           int         `xml:"width,attr"`
	Height          int         `xml:"height,attr"`
	Codecs          string      `xml:"codecs,attr"`
	BaseURL         BaseURL     `xml:"BaseURL"`
	SegmentTemplate interface{} `xml:"SegmentTemplate"`
}

// BaseURL represents the base URL for media segments
type BaseURL struct {
	Value string `xml:",chardata"`
}

// SegmentTemplate defines how media segments are addressed
type SegmentTemplate struct {
	Media          string `xml:"media,attr"`
	Initialization string `xml:"initialization,attr"`
	StartNumber    int    `xml:"startNumber,attr"`
	Duration       int    `xml:"duration,attr"`
	Timescale      int    `xml:"timescale,attr"`
}

// GenerateLiveMPD generates a comprehensive live DASH manifest for real-time streaming
func (g *DASHGenerator) GenerateLiveMPD(mediaID string, windowSize int) (*DASHManifest, error) {
	// Retrieve live stream metadata
	metadata, err := g.storage.GetMediaMetadata(mediaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media metadata: %w", err)
	}

	// Validate live stream
	if !metadata.IsLive {
		return nil, fmt.Errorf("media %s is not a live stream", mediaID)
	}

	// Calculate timing parameters for live streaming
	now := time.Now()
	availabilityStartTime := metadata.StartTime
	if availabilityStartTime.IsZero() {
		// Default to 30 minutes before current time for live streams
		availabilityStartTime = now.Add(-30 * time.Minute)
	}

	// Calculate the time shift buffer depth (DVR window)
	// This controls how far back viewers can seek in the live stream
	timeShiftBufferDepth := float64(windowSize)
	if timeShiftBufferDepth == 0 {
		timeShiftBufferDepth = 1800 // Default 30 minutes DVR
	}
	if timeShiftBufferDepth > 7200 { // Cap at 2 hours for performance
		timeShiftBufferDepth = 7200
	}

	// Calculate suggested presentation delay (live edge buffer)
	// This prevents viewers from getting too close to the live edge
	suggestedPresentationDelay := float64(g.config.SegmentDuration * 4) // 4 segments buffer
	if suggestedPresentationDelay < 12.0 { // Minimum 12 seconds for stability
		suggestedPresentationDelay = 12.0
	}

	// Minimum update period for manifest refreshes
	minimumUpdatePeriod := float64(g.config.SegmentDuration)
	if minimumUpdatePeriod < 1.0 { // At least 1 second between updates
		minimumUpdatePeriod = 1.0
	}

	manifest := &DASHManifest{
		MediaID:                    mediaID,
		Duration:                   0, // Live streams have no fixed duration
		MinBufferTime:              float64(g.config.SegmentDuration),
		ManifestURL:                g.getLiveManifestURL(mediaID),
		GeneratedAt:                now,
		CacheDuration:              time.Duration(minimumUpdatePeriod/2) * time.Second, // Half the update period
		IsLive:                     true,
		AvailabilityStartTime:      availabilityStartTime,
		PublishTime:                now,
		TimeShiftBufferDepth:       timeShiftBufferDepth,
		SuggestedPresentationDelay: suggestedPresentationDelay,
		MinimumUpdatePeriod:        minimumUpdatePeriod,
		Representations:            []DASHRepresentation{},
	}

	// Add representations for each available quality with live-specific settings
	for i, quality := range metadata.AvailableQualities {
		info := GetQualityInfo(quality)
		
		// Use quality-specific codec settings if available
		bandwidth := info.Bandwidth * 1000 // Convert to bps
		if codecInfo, exists := metadata.QualitySettings[quality]; exists && codecInfo.Bandwidth > 0 {
			bandwidth = codecInfo.Bandwidth
		}
		
		representation := DASHRepresentation{
			ID:              fmt.Sprintf("video_%d", i),
			Quality:         quality,
			Bandwidth:       bandwidth,
			Width:           info.Width,
			Height:          info.Height,
			Codecs:          g.getCodecsForLive(quality, metadata),
			BaseURL:         g.getLiveBaseURL(mediaID, quality),
			SegmentTemplate: g.getLiveSegmentTemplate(),
		}
		manifest.Representations = append(manifest.Representations, representation)
	}

	return manifest, nil
}

// ValidateMPD validates a generated MPD
func (g *DASHGenerator) ValidateMPD(content string) error {
	var mpd MPD
	err := xml.Unmarshal([]byte(content), &mpd)
	if err != nil {
		return fmt.Errorf("invalid MPD XML: %w", err)
	}

	// Basic validation
	if mpd.Type == "" {
		return fmt.Errorf("missing type attribute")
	}

	if len(mpd.Periods) == 0 {
		return fmt.Errorf("no periods defined")
	}

	for _, period := range mpd.Periods {
		if len(period.AdaptationSets) == 0 {
			return fmt.Errorf("period %s has no adaptation sets", period.ID)
		}

		for _, adaptationSet := range period.AdaptationSets {
			if len(adaptationSet.Representations) == 0 {
				return fmt.Errorf("adaptation set %s has no representations", adaptationSet.ID)
			}
		}
	}

	return nil
}

// GenerateAudioAdaptationSet creates an audio adaptation set
func (g *DASHGenerator) GenerateAudioAdaptationSet(audioTracks []AudioTrack) AdaptationSet {
	adaptationSet := AdaptationSet{
		ID:               "1",
		MimeType:         "audio/mp4",
		SegmentAlignment: true,
		StartWithSAP:     1,
		Representations:  []Representation{},
	}

	for i, track := range audioTracks {
		representation := Representation{
			ID:        fmt.Sprintf("audio_%d", i),
			Bandwidth: track.Bitrate * 1000,
			Codecs:    track.Codec,
			BaseURL: BaseURL{
				Value: track.BaseURL,
			},
			SegmentTemplate: SegmentTemplate{
				Media:          "segment$Number$.m4a",
				Initialization: "init.mp4",
				StartNumber:    0,
				Duration:       g.config.SegmentDuration * 1000,
				Timescale:      1000,
			},
		}
		adaptationSet.Representations = append(adaptationSet.Representations, representation)
	}

	return adaptationSet
}

// AudioTrack represents an audio track
type AudioTrack struct {
	Language string
	Bitrate  int
	Codec    string
	BaseURL  string
}

// GenerateMPDWithSubtitles generates an MPD with subtitle tracks
func (g *DASHGenerator) GenerateMPDWithSubtitles(manifest *DASHManifest, subtitles []SubtitleTrack) (string, error) {
	// For live streams, use the live MPD generator
	if manifest.IsLive {
		return g.GenerateLiveMPDContentWithSubtitles(manifest, subtitles)
	}
	
	// For VOD, add subtitle adaptation sets to the standard MPD
	return g.generateVODMPDWithSubtitles(manifest, subtitles)
}

// SubtitleTrack is defined in hls.go

// LiveMPD represents a live Media Presentation Description for DASH streaming
type LiveMPD struct {
	XMLName                    xml.Name `xml:"MPD"`
	XMLNS                      string   `xml:"xmlns,attr"`
	Type                       string   `xml:"type,attr"`
	AvailabilityStartTime      string   `xml:"availabilityStartTime,attr"`
	PublishTime                string   `xml:"publishTime,attr"`
	MinimumUpdatePeriod        string   `xml:"minimumUpdatePeriod,attr,omitempty"`
	TimeShiftBufferDepth       string   `xml:"timeShiftBufferDepth,attr,omitempty"`
	SuggestedPresentationDelay string   `xml:"suggestedPresentationDelay,attr,omitempty"`
	MinBufferTime              string   `xml:"minBufferTime,attr"`
	Profiles                   string   `xml:"profiles,attr"`
	Periods                    []Period `xml:"Period"`
}

// LiveSegmentTemplate represents a segment template for live DASH streaming
type LiveSegmentTemplate struct {
	Media                  string  `xml:"media,attr"`
	Initialization         string  `xml:"initialization,attr"`
	Duration               int     `xml:"duration,attr,omitempty"`
	Timescale              int     `xml:"timescale,attr"`
	AvailabilityTimeOffset float64 `xml:"availabilityTimeOffset,attr,omitempty"`
	PresentationTimeOffset int64   `xml:"presentationTimeOffset,attr,omitempty"`
	StartNumber            int     `xml:"startNumber,attr,omitempty"`
}

// GenerateLiveMPDContent generates the live DASH MPD XML content
func (g *DASHGenerator) GenerateLiveMPDContent(manifest *DASHManifest) (string, error) {
	mpd := &LiveMPD{
		XMLName:                    xml.Name{Local: "MPD"},
		XMLNS:                      "urn:mpeg:dash:schema:mpd:2011",
		Type:                       "dynamic",
		AvailabilityStartTime:      g.formatTimestamp(manifest.AvailabilityStartTime),
		PublishTime:                g.formatTimestamp(manifest.PublishTime),
		MinimumUpdatePeriod:        g.formatDuration(manifest.MinimumUpdatePeriod),
		TimeShiftBufferDepth:       g.formatDuration(manifest.TimeShiftBufferDepth),
		SuggestedPresentationDelay: g.formatDuration(manifest.SuggestedPresentationDelay),
		MinBufferTime:              g.formatDuration(manifest.MinBufferTime),
		Profiles:                   "urn:mpeg:dash:profile:isoff-live:2011",
		Periods: []Period{
			{
				ID:    "0",
				Start: "PT0S",
				AdaptationSets: []AdaptationSet{
					g.createLiveVideoAdaptationSet(manifest),
				},
			},
		},
	}

	// Marshal to XML
	output, err := xml.MarshalIndent(mpd, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal live MPD: %w", err)
	}

	// Add XML declaration
	return xml.Header + string(output), nil
}

// createLiveVideoAdaptationSet creates the video adaptation set for live streaming with advanced features
func (g *DASHGenerator) createLiveVideoAdaptationSet(manifest *DASHManifest) AdaptationSet {
	adaptationSet := AdaptationSet{
		ID:               "0",
		MimeType:         "video/mp4",
		SegmentAlignment: true,
		StartWithSAP:     1, // Start with Stream Access Point (keyframe)
		MaxWidth:         0,
		MaxHeight:        0,
		MaxFrameRate:     "30",
		Par:              "16:9",
		Representations:  []Representation{},
	}

	// Find max dimensions and frame rate across all representations
	maxFrameRate := 0.0
	for _, rep := range manifest.Representations {
		if rep.Width > adaptationSet.MaxWidth {
			adaptationSet.MaxWidth = rep.Width
		}
		if rep.Height > adaptationSet.MaxHeight {
			adaptationSet.MaxHeight = rep.Height
		}
		
		// Try to detect frame rate from quality level
		if rep.Quality == Quality4K && maxFrameRate < 60 {
			maxFrameRate = 60
		} else if maxFrameRate < 30 {
			maxFrameRate = 30
		}
	}
	
	if maxFrameRate > 0 {
		adaptationSet.MaxFrameRate = fmt.Sprintf("%.0f", maxFrameRate)
	}

	// Calculate availability time offset for live edge management
	// This controls when segments become available relative to their media time
	_ = manifest.SuggestedPresentationDelay // Available for future use
	
	// Calculate presentation time offset for synchronized playback  
	// This aligns the media timeline with wall clock time
	_ = int64(time.Since(manifest.AvailabilityStartTime).Seconds() * 1000) // Available for future use

	// Add representations with enhanced live-specific segment templates
	for _, rep := range manifest.Representations {
		// Create segment template based on stream characteristics
		segmentTemplate := g.createLiveSegmentTemplate(manifest, rep)
		
		adaptationSet.Representations = append(adaptationSet.Representations, Representation{
			ID:        rep.ID,
			Bandwidth: rep.Bandwidth,
			Width:     rep.Width,
			Height:    rep.Height,
			Codecs:    rep.Codecs,
			BaseURL: BaseURL{
				Value: rep.BaseURL,
			},
			SegmentTemplate: segmentTemplate,
		})
	}

	return adaptationSet
}

// createLiveSegmentTemplate creates an optimized segment template for live streaming
func (g *DASHGenerator) createLiveSegmentTemplate(manifest *DASHManifest, rep DASHRepresentation) LiveSegmentTemplate {
	// Calculate presentation time offset for live streams
	// This ensures proper synchronization across different qualities
	presentationTimeOffset := int64(time.Since(manifest.AvailabilityStartTime).Seconds() * 1000)
	
	// For live streams, we use time-based addressing for DVR functionality
	// This allows clients to seek to specific wall clock times
	segmentTemplate := LiveSegmentTemplate{
		Media:                  rep.SegmentTemplate,
		Initialization:         fmt.Sprintf("init-%s.mp4", rep.ID),
		Duration:               g.config.SegmentDuration * 1000, // Convert to milliseconds
		Timescale:              1000, // 1000 units per second for precise timing
		AvailabilityTimeOffset: manifest.SuggestedPresentationDelay,
		PresentationTimeOffset: presentationTimeOffset,
	}
	
	return segmentTemplate
}

// GenerateLiveMPDContentWithSubtitles generates live DASH MPD content with subtitle support
func (g *DASHGenerator) GenerateLiveMPDContentWithSubtitles(manifest *DASHManifest, subtitles []SubtitleTrack) (string, error) {
	mpd := &LiveMPD{
		XMLName:                    xml.Name{Local: "MPD"},
		XMLNS:                      "urn:mpeg:dash:schema:mpd:2011",
		Type:                       "dynamic",
		AvailabilityStartTime:      g.formatTimestamp(manifest.AvailabilityStartTime),
		PublishTime:                g.formatTimestamp(manifest.PublishTime),
		MinimumUpdatePeriod:        g.formatDuration(manifest.MinimumUpdatePeriod),
		TimeShiftBufferDepth:       g.formatDuration(manifest.TimeShiftBufferDepth),
		SuggestedPresentationDelay: g.formatDuration(manifest.SuggestedPresentationDelay),
		MinBufferTime:              g.formatDuration(manifest.MinBufferTime),
		Profiles:                   "urn:mpeg:dash:profile:isoff-live:2011",
		Periods: []Period{
			{
				ID:    "0",
				Start: "PT0S",
				AdaptationSets: []AdaptationSet{
					g.createLiveVideoAdaptationSet(manifest),
				},
			},
		},
	}
	
	// Add subtitle adaptation sets for live streams
	if len(subtitles) > 0 {
		subtitleAdaptationSet := g.createLiveSubtitleAdaptationSet(subtitles, manifest.MediaID)
		mpd.Periods[0].AdaptationSets = append(mpd.Periods[0].AdaptationSets, subtitleAdaptationSet)
	}

	// Marshal to XML
	output, err := xml.MarshalIndent(mpd, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal live MPD with subtitles: %w", err)
	}

	// Add XML declaration
	return xml.Header + string(output), nil
}

// generateVODMPDWithSubtitles generates VOD DASH MPD content with subtitle support
func (g *DASHGenerator) generateVODMPDWithSubtitles(manifest *DASHManifest, subtitles []SubtitleTrack) (string, error) {
	mpd := &MPD{
		XMLName:                   xml.Name{Local: "MPD"},
		XMLNS:                     "urn:mpeg:dash:schema:mpd:2011",
		Type:                      "static",
		MediaPresentationDuration: g.formatDuration(manifest.Duration),
		MinBufferTime:             g.formatDuration(manifest.MinBufferTime),
		Profiles:                  "urn:mpeg:dash:profile:isoff-on-demand:2011",
		Periods: []Period{
			{
				ID:       "0",
				Duration: g.formatDuration(manifest.Duration),
				AdaptationSets: []AdaptationSet{
					g.createVideoAdaptationSet(manifest),
				},
			},
		},
	}
	
	// Add subtitle adaptation sets
	if len(subtitles) > 0 {
		subtitleAdaptationSet := g.createSubtitleAdaptationSet(subtitles, manifest.MediaID)
		mpd.Periods[0].AdaptationSets = append(mpd.Periods[0].AdaptationSets, subtitleAdaptationSet)
	}

	// Marshal to XML
	output, err := xml.MarshalIndent(mpd, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal VOD MPD with subtitles: %w", err)
	}

	// Add XML declaration
	return xml.Header + string(output), nil
}

// createLiveSubtitleAdaptationSet creates subtitle adaptation set for live streams
func (g *DASHGenerator) createLiveSubtitleAdaptationSet(subtitles []SubtitleTrack, mediaID string) AdaptationSet {
	adaptationSet := AdaptationSet{
		ID:               "subs",
		MimeType:         "text/vtt", // Default to WebVTT for live streams
		SegmentAlignment: true,
		Representations:  []Representation{},
	}
	
	for i, subtitle := range subtitles {
		// Generate DASH-specific URLs and templates from HLS subtitle info
		baseURL := fmt.Sprintf("%s/live/%s/subtitles/%s/", g.config.CDNBaseURL, mediaID, subtitle.Language)
		segmentTemplate := "segment-$Time$.vtt"
		
		representation := Representation{
			ID:        fmt.Sprintf("subtitle_%d", i),
			Bandwidth: 1000, // Low bandwidth for subtitles
			Codecs:    "wvtt", // WebVTT codec
			BaseURL: BaseURL{
				Value: baseURL,
			},
			SegmentTemplate: LiveSegmentTemplate{
				Media:          segmentTemplate,
				Initialization: fmt.Sprintf("subtitle_init_%s.vtt", subtitle.Language),
				Duration:       g.config.SegmentDuration * 1000,
				Timescale:      1000,
			},
		}
		adaptationSet.Representations = append(adaptationSet.Representations, representation)
	}
	
	return adaptationSet
}

// createSubtitleAdaptationSet creates subtitle adaptation set for VOD
func (g *DASHGenerator) createSubtitleAdaptationSet(subtitles []SubtitleTrack, mediaID string) AdaptationSet {
	adaptationSet := AdaptationSet{
		ID:               "subs",
		MimeType:         "text/vtt",
		SegmentAlignment: true,
		Representations:  []Representation{},
	}
	
	for i, subtitle := range subtitles {
		// Generate DASH-specific URLs and templates from HLS subtitle info
		baseURL := fmt.Sprintf("%s/media/%s/subtitles/%s/", g.config.CDNBaseURL, mediaID, subtitle.Language)
		segmentTemplate := "segment$Number$.vtt"
		
		representation := Representation{
			ID:        fmt.Sprintf("subtitle_%d", i),
			Bandwidth: 1000,
			Codecs:    "wvtt",
			BaseURL: BaseURL{
				Value: baseURL,
			},
			SegmentTemplate: SegmentTemplate{
				Media:          segmentTemplate,
				Initialization: fmt.Sprintf("subtitle_init_%s.vtt", subtitle.Language),
				StartNumber:    0,
				Duration:       g.config.SegmentDuration * 1000,
				Timescale:      1000,
			},
		}
		adaptationSet.Representations = append(adaptationSet.Representations, representation)
	}
	
	return adaptationSet
}

// ValidateLiveManifest validates a live DASH manifest for compliance
func (g *DASHGenerator) ValidateLiveManifest(content string) error {
	var mpd LiveMPD
	err := xml.Unmarshal([]byte(content), &mpd)
	if err != nil {
		return fmt.Errorf("invalid live MPD XML: %w", err)
	}
	
	// Validate live-specific requirements
	if mpd.Type != "dynamic" {
		return fmt.Errorf("live MPD must have type='dynamic'")
	}
	
	if mpd.AvailabilityStartTime == "" {
		return fmt.Errorf("live MPD must have availabilityStartTime")
	}
	
	if mpd.PublishTime == "" {
		return fmt.Errorf("live MPD should have publishTime")
	}
	
	// Validate periods
	if len(mpd.Periods) == 0 {
		return fmt.Errorf("no periods defined in live MPD")
	}
	
	for _, period := range mpd.Periods {
		if len(period.AdaptationSets) == 0 {
			return fmt.Errorf("period %s has no adaptation sets", period.ID)
		}
		
		for _, adaptationSet := range period.AdaptationSets {
			if len(adaptationSet.Representations) == 0 {
				return fmt.Errorf("adaptation set %s has no representations", adaptationSet.ID)
			}
		}
	}
	
	return nil
}
