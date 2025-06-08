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

// DASH XML structures

type MPD struct {
	XMLName                   xml.Name `xml:"MPD"`
	XMLNS                     string   `xml:"xmlns,attr"`
	Type                      string   `xml:"type,attr"`
	MediaPresentationDuration string   `xml:"mediaPresentationDuration,attr"`
	MinBufferTime             string   `xml:"minBufferTime,attr"`
	Profiles                  string   `xml:"profiles,attr"`
	Periods                   []Period `xml:"Period"`
}

type Period struct {
	ID             string          `xml:"id,attr"`
	Duration       string          `xml:"duration,attr,omitempty"`
	AdaptationSets []AdaptationSet `xml:"AdaptationSet"`
}

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

type Representation struct {
	ID              string          `xml:"id,attr"`
	Bandwidth       int             `xml:"bandwidth,attr"`
	Width           int             `xml:"width,attr"`
	Height          int             `xml:"height,attr"`
	Codecs          string          `xml:"codecs,attr"`
	BaseURL         BaseURL         `xml:"BaseURL"`
	SegmentTemplate SegmentTemplate `xml:"SegmentTemplate"`
}

type BaseURL struct {
	Value string `xml:",chardata"`
}

type SegmentTemplate struct {
	Media          string `xml:"media,attr"`
	Initialization string `xml:"initialization,attr"`
	StartNumber    int    `xml:"startNumber,attr"`
	Duration       int    `xml:"duration,attr"`
	Timescale      int    `xml:"timescale,attr"`
}

// GenerateLiveMPD generates a live DASH manifest
func (g *DASHGenerator) GenerateLiveMPD(mediaID string, windowSize int) (*DASHManifest, error) {
	// For live streaming, we would need to implement:
	// - Dynamic manifest updates
	// - Sliding window of segments
	// - Availability start time
	// - Suggested presentation delay

	// This is a simplified placeholder
	return nil, fmt.Errorf("live DASH not yet implemented")
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
	// This would add subtitle adaptation sets to the MPD
	// Implementation would include WebVTT or TTML subtitle tracks
	return g.GenerateMPDContent(manifest)
}
