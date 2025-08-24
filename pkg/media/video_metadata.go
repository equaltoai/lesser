package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"io"
)

// VideoMetadata contains extracted video metadata from MP4/MOV files
type VideoMetadata struct {
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	Duration         int     `json:"duration"`          // Duration in milliseconds
	DurationSeconds  float64 `json:"duration_seconds"`  // Duration in seconds (more precise)
	Timescale        uint32  `json:"timescale"`         // Movie timescale from mvhd
	CreationTime     uint32  `json:"creation_time"`     // Creation time from mvhd
	ModificationTime uint32  `json:"modification_time"` // Modification time from mvhd
	VideoCodec       string  `json:"video_codec"`       // Video codec identifier
	AudioCodec       string  `json:"audio_codec"`       // Audio codec identifier
	HasAudio         bool    `json:"has_audio"`         // Whether the file contains audio
	HasVideo         bool    `json:"has_video"`         // Whether the file contains video
	Bitrate          int64   `json:"bitrate"`           // Estimated bitrate in bits per second
	FrameRate        float64 `json:"frame_rate"`        // Estimated frame rate
	FileSize         int64   `json:"file_size"`         // File size in bytes
}

// MP4Atom represents an MP4/MOV atom (also known as box)
type MP4Atom struct {
	Size uint32
	Type [4]byte
	Data []byte
}

// VideoMetadataParser handles parsing of MP4/MOV video metadata
type VideoMetadataParser struct {
	data     []byte
	offset   int
	fileSize int64
}

// NewVideoMetadataParser creates a new parser for the given video data
func NewVideoMetadataParser(data []byte) *VideoMetadataParser {
	return &VideoMetadataParser{
		data:     data,
		offset:   0,
		fileSize: int64(len(data)),
	}
}

// ParseVideoMetadata extracts metadata from MP4/MOV video data
func ParseVideoMetadata(data []byte) (*VideoMetadata, error) {
	parser := NewVideoMetadataParser(data)
	metadata := &VideoMetadata{
		FileSize: int64(len(data)),
	}

	if len(data) < 8 {
		return parser.fallbackMetadata(metadata)
	}

	// Check for MP4/MOV file signature
	if !parser.isValidMP4() {
		return parser.fallbackMetadata(metadata)
	}

	// Parse atoms to extract metadata
	if err := parser.parseAtoms(metadata); err != nil {
		// If parsing fails, try to return what we found with fallbacks
		return parser.fallbackMetadata(metadata)
	}

	// Calculate derived values
	parser.calculateDerivedMetadata(metadata)

	return metadata, nil
}

// isValidMP4 checks if the data starts with a valid MP4/MOV signature
func (p *VideoMetadataParser) isValidMP4() bool {
	if len(p.data) < 8 {
		return false
	}

	// Check for 'ftyp' box which should be early in the file
	searchLen := len(p.data)
	if searchLen > 100 {
		searchLen = 100
	}
	ftypIndex := bytes.Index(p.data[:searchLen], []byte("ftyp"))
	if ftypIndex < 4 {
		return false
	}

	// Check for common MP4/MOV brand types
	brands := [][]byte{
		[]byte("isom"), // ISO Base Media
		[]byte("mp41"), // MP4 v1
		[]byte("mp42"), // MP4 v2
		[]byte("qt  "), // QuickTime
		[]byte("M4V "), // iTunes Video
		[]byte("M4A "), // iTunes Audio
		[]byte("3gp4"), // 3GPP
		[]byte("3gp5"), // 3GPP
	}

	for _, brand := range brands {
		endIndex := ftypIndex + 32
		if endIndex > len(p.data) {
			endIndex = len(p.data)
		}
		if bytes.Contains(p.data[ftypIndex:endIndex], brand) {
			return true
		}
	}

	return false
}

// parseAtoms parses MP4 atoms to extract metadata
func (p *VideoMetadataParser) parseAtoms(metadata *VideoMetadata) error {
	p.offset = 0
	var foundMoov bool

	for p.offset < len(p.data)-8 {
		atom, err := p.readAtom()
		if err != nil {
			if foundMoov {
				// We found moov, so partial success is OK
				break
			}
			return err
		}

		switch string(atom.Type[:]) {
		case "moov":
			foundMoov = true
			if err := p.parseMoovAtom(atom.Data, metadata); err != nil {
				return fmt.Errorf("%w: %w", ErrMoovAtomParseFailed, err)
			}
		case "mdat":
			// Skip media data atom
			continue
		case "ftyp":
			// Skip file type atom
			continue
		default:
			// Skip unknown atoms
			continue
		}
	}

	if !foundMoov {
		return ErrMoovAtomNotFound
	}

	return nil
}

// readAtom reads the next atom from the current offset
func (p *VideoMetadataParser) readAtom() (*MP4Atom, error) {
	if p.offset+8 > len(p.data) {
		return nil, io.EOF
	}

	// Read atom size (4 bytes, big-endian)
	size := binary.BigEndian.Uint32(p.data[p.offset : p.offset+4])

	// Read atom type (4 bytes)
	var atomType [4]byte
	copy(atomType[:], p.data[p.offset+4:p.offset+8])

	// Handle extended size (size == 1)
	headerSize := 8
	if size == 1 {
		if p.offset+16 > len(p.data) {
			return nil, ErrExtendedSizeIncomplete
		}
		// Extended size is next 8 bytes
		size64 := binary.BigEndian.Uint64(p.data[p.offset+8 : p.offset+16])
		if size64 > 0xFFFFFFFF {
			// Size too large for uint32
			return nil, fmt.Errorf("%w: %d", ErrAtomSizeTooLarge, size64)
		}
		size = uint32(size64) // #nosec G115 - checked for overflow above
		headerSize = 16
	}

	// Validate size
	if int(size) < headerSize || headerSize < 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidAtomSize, size)
	}

	dataSize := int(size) - headerSize
	if p.offset+int(size) > len(p.data) {
		return nil, fmt.Errorf("%w: size=%d, available=%d", ErrAtomExtendsFile, size, len(p.data)-p.offset)
	}

	// Read atom data
	data := make([]byte, dataSize)
	copy(data, p.data[p.offset+headerSize:p.offset+int(size)])

	// Move to next atom
	p.offset += int(size)

	return &MP4Atom{
		Size: size,
		Type: atomType,
		Data: data,
	}, nil
}

// parseMoovAtom parses the movie atom which contains metadata
func (p *VideoMetadataParser) parseMoovAtom(data []byte, metadata *VideoMetadata) error {
	parser := &VideoMetadataParser{data: data, offset: 0}

	for parser.offset < len(data)-8 {
		atom, err := parser.readAtom()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch string(atom.Type[:]) {
		case "mvhd":
			if err := parseMvhdAtom(atom.Data, metadata); err != nil {
				return fmt.Errorf("%w: %w", ErrMvhdAtomParseFailed, err)
			}
		case "trak":
			if err := p.parseTrakAtom(atom.Data, metadata); err != nil {
				// Don't fail if one track fails, just log and continue
				continue
			}
		}
	}

	return nil
}

// parseMvhdAtom parses the movie header atom
func parseMvhdAtom(data []byte, metadata *VideoMetadata) error {
	if len(data) < 32 {
		return ErrMvhdAtomTooSmall
	}

	// Version (1 byte) + flags (3 bytes)
	version := data[0]

	var timescale, duration uint32
	var creationTime, modificationTime uint32

	switch version {
	case 0:
		// Version 0: 32-bit values
		if len(data) < 32 {
			return ErrMvhdV0AtomIncomplete
		}
		creationTime = binary.BigEndian.Uint32(data[4:8])
		modificationTime = binary.BigEndian.Uint32(data[8:12])
		timescale = binary.BigEndian.Uint32(data[12:16])
		duration = binary.BigEndian.Uint32(data[16:20])
	case 1:
		// Version 1: 64-bit values
		if len(data) < 44 {
			return ErrMvhdV1AtomIncomplete
		}
		// Handle 64-bit creation time with overflow check
		time64 := binary.BigEndian.Uint64(data[4:12])
		if time64 > 0xFFFFFFFF {
			// Time too large for uint32, clamp it
			creationTime = 0xFFFFFFFF
		} else {
			creationTime = uint32(time64) // #nosec G115 - checked for overflow above
		}
		// Handle 64-bit modification time with overflow check
		modTime64 := binary.BigEndian.Uint64(data[12:20])
		if modTime64 > 0xFFFFFFFF {
			modificationTime = 0xFFFFFFFF
		} else {
			modificationTime = uint32(modTime64) // #nosec G115 - checked for overflow above
		}
		timescale = binary.BigEndian.Uint32(data[20:24])
		// Handle 64-bit duration with overflow check
		dur64 := binary.BigEndian.Uint64(data[24:32])
		if dur64 > 0xFFFFFFFF {
			duration = 0xFFFFFFFF
		} else {
			duration = uint32(dur64) // #nosec G115 - checked for overflow above
		}
	default:
		return fmt.Errorf("%w: %d", ErrUnsupportedMvhdVersion, version)
	}

	metadata.Timescale = timescale
	metadata.CreationTime = creationTime
	metadata.ModificationTime = modificationTime

	// Calculate duration in seconds and milliseconds
	if timescale > 0 {
		metadata.DurationSeconds = float64(duration) / float64(timescale)
		metadata.Duration = int(metadata.DurationSeconds * 1000) // Convert to milliseconds
	}

	return nil
}

// parseTrakAtom parses a track atom which contains track-specific metadata
func (p *VideoMetadataParser) parseTrakAtom(data []byte, metadata *VideoMetadata) error {
	parser := &VideoMetadataParser{data: data, offset: 0}

	for parser.offset < len(data)-8 {
		atom, err := parser.readAtom()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch string(atom.Type[:]) {
		case "tkhd":
			if err := parseTkhdAtom(atom.Data, metadata); err != nil {
				return fmt.Errorf("%w: %w", ErrTkhdAtomParseFailed, err)
			}
		case "mdia":
			if err := p.parseMdiaAtom(atom.Data, metadata); err != nil {
				// Don't fail for media parsing errors
				continue
			}
		}
	}

	return nil
}

// parseTkhdAtom parses the track header atom (this is the main focus of the requirement)
func parseTkhdAtom(data []byte, metadata *VideoMetadata) error {
	if len(data) < 32 {
		return ErrTkhdAtomTooSmall
	}

	// Version (1 byte) + flags (3 bytes)
	version := data[0]

	var width, height uint32
	var trackDuration uint32

	switch version {
	case 0:
		// Version 0: 32-bit values
		if len(data) < 84 {
			return ErrTkhdV0AtomIncomplete
		}

		// Skip creation/modification time (8 bytes)
		// Track ID at offset 12
		// Reserved at offset 16
		trackDuration = binary.BigEndian.Uint32(data[20:24])

		// Width and height are at the end of the atom (fixed point 16.16)
		width = binary.BigEndian.Uint32(data[76:80]) >> 16  // Convert from 16.16 fixed point
		height = binary.BigEndian.Uint32(data[80:84]) >> 16 // Convert from 16.16 fixed point

	case 1:
		// Version 1: 64-bit values
		if len(data) < 96 {
			return ErrTkhdV1AtomIncomplete
		}

		// Skip 64-bit creation/modification time (16 bytes)
		// Track ID at offset 20
		// Reserved at offset 24
		// Handle 64-bit duration with overflow check
		dur64 := binary.BigEndian.Uint64(data[28:36])
		if dur64 > 0xFFFFFFFF {
			trackDuration = 0xFFFFFFFF
		} else {
			trackDuration = uint32(dur64) // #nosec G115 - checked for overflow above
		}

		// Width and height are at the end of the atom (fixed point 16.16)
		width = binary.BigEndian.Uint32(data[88:92]) >> 16  // Convert from 16.16 fixed point
		height = binary.BigEndian.Uint32(data[92:96]) >> 16 // Convert from 16.16 fixed point

	default:
		return fmt.Errorf("%w: %d", ErrUnsupportedTkhdVersion, version)
	}

	// Only update dimensions if they seem reasonable and we haven't found video dimensions yet
	if width > 0 && height > 0 && width <= 8192 && height <= 8192 {
		if metadata.Width == 0 || metadata.Height == 0 {
			metadata.Width = int(width)
			metadata.Height = int(height)
			metadata.HasVideo = true
		}
	}

	// Update duration if this track is longer (for files with multiple tracks)
	if trackDuration > 0 && metadata.Timescale > 0 {
		trackDurationMs := int(float64(trackDuration) / float64(metadata.Timescale) * 1000)
		if trackDurationMs > metadata.Duration {
			metadata.Duration = trackDurationMs
			metadata.DurationSeconds = float64(trackDurationMs) / 1000
		}
	}

	return nil
}

// parseMdiaAtom parses the media atom
func (p *VideoMetadataParser) parseMdiaAtom(data []byte, metadata *VideoMetadata) error {
	parser := &VideoMetadataParser{data: data, offset: 0}

	for parser.offset < len(data)-8 {
		atom, err := parser.readAtom()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch string(atom.Type[:]) {
		case "minf":
			if err := p.parseMinfAtom(atom.Data, metadata); err != nil {
				continue
			}
		case "hdlr":
			if err := parseHdlrAtom(atom.Data, metadata); err != nil {
				continue
			}
		}
	}

	return nil
}

// parseMinfAtom parses the media information atom
func (p *VideoMetadataParser) parseMinfAtom(data []byte, metadata *VideoMetadata) error {
	parser := &VideoMetadataParser{data: data, offset: 0}

	for parser.offset < len(data)-8 {
		atom, err := parser.readAtom()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch string(atom.Type[:]) {
		case "stbl":
			if err := p.parseStblAtom(atom.Data, metadata); err != nil {
				continue
			}
		}
	}

	return nil
}

// parseStblAtom parses the sample table atom
func (p *VideoMetadataParser) parseStblAtom(data []byte, metadata *VideoMetadata) error {
	parser := &VideoMetadataParser{data: data, offset: 0}

	for parser.offset < len(data)-8 {
		atom, err := parser.readAtom()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch string(atom.Type[:]) {
		case "stsd":
			if err := parseStsdAtom(atom.Data, metadata); err != nil {
				continue
			}
		}
	}

	return nil
}

// parseHdlrAtom parses the handler reference atom to determine track type
func parseHdlrAtom(data []byte, metadata *VideoMetadata) error {
	if len(data) < 24 {
		return ErrHdlrAtomTooSmall
	}

	// Handler type is at offset 8-12
	handlerType := string(data[8:12])

	switch handlerType {
	case "vide":
		metadata.HasVideo = true
	case "soun":
		metadata.HasAudio = true
	}

	return nil
}

// parseStsdAtom parses the sample description atom to get codec information
func parseStsdAtom(data []byte, metadata *VideoMetadata) error {
	if len(data) < 16 {
		return ErrStsdAtomTooSmall
	}

	// Skip version/flags (4 bytes) and entry count (4 bytes)
	entryCount := binary.BigEndian.Uint32(data[4:8])
	if entryCount == 0 {
		return nil
	}

	// First sample description starts at offset 8
	if len(data) < 24 {
		return ErrStsdEntryIncomplete
	}

	// Sample description size (4 bytes) + codec type (4 bytes)
	codecBytes := data[12:16]
	codec := string(codecBytes)

	// Determine if this is video or audio codec
	switch {
	case isVideoCodec(codec):
		metadata.VideoCodec = codec
		metadata.HasVideo = true
	case isAudioCodec(codec):
		metadata.AudioCodec = codec
		metadata.HasAudio = true
	}

	return nil
}

// isVideoCodec checks if the codec string represents a video codec
func isVideoCodec(codec string) bool {
	videoCodecs := []string{
		"avc1", "avc3", "hev1", "hvc1", "mp4v", "s263", "h263", "dvh1", "dvhe",
		"av01", "vp08", "vp09", "jpeg", "mjp2",
	}

	for _, vc := range videoCodecs {
		if codec == vc {
			return true
		}
	}
	return false
}

// isAudioCodec checks if the codec string represents an audio codec
func isAudioCodec(codec string) bool {
	audioCodecs := []string{
		"mp4a", "ac-3", "ec-3", "dtsc", "dtse", "dtsh", "dtsl", "opus", "fLaC",
		"alac", "sowt", "twos", "lpcm",
	}

	for _, ac := range audioCodecs {
		if codec == ac {
			return true
		}
	}
	return false
}

// calculateDerivedMetadata calculates derived values like bitrate and frame rate
func (p *VideoMetadataParser) calculateDerivedMetadata(metadata *VideoMetadata) {
	// Calculate estimated bitrate
	if metadata.DurationSeconds > 0 {
		metadata.Bitrate = int64(float64(metadata.FileSize*8) / metadata.DurationSeconds)
	}

	// Estimate frame rate (very rough approximation)
	if metadata.DurationSeconds > 0 && metadata.HasVideo {
		// Assume 24-30 fps for most content
		metadata.FrameRate = 25.0 // Default assumption

		// Adjust based on common standards
		if metadata.Height >= 1080 {
			metadata.FrameRate = 30.0 // HD+ content often 30fps
		} else if metadata.Height <= 480 {
			metadata.FrameRate = 24.0 // SD content often 24fps
		}
	}
}

// fallbackMetadata provides reasonable defaults when parsing fails
// Returns metadata with sensible defaults, but with an error indicating fallback was used
func (p *VideoMetadataParser) fallbackMetadata(metadata *VideoMetadata) (*VideoMetadata, error) {
	// Set reasonable defaults based on file size and common resolutions
	if metadata.Width == 0 || metadata.Height == 0 {
		sizeMB := metadata.FileSize / (1024 * 1024)

		switch {
		case sizeMB > 100: // Large file, likely HD or higher
			metadata.Width = 1920
			metadata.Height = 1080
		case sizeMB > 50: // Medium file, likely 720p
			metadata.Width = 1280
			metadata.Height = 720
		default: // Small file, likely SD
			metadata.Width = 854
			metadata.Height = 480
		}
	}

	// Set default duration if not found (very rough estimate based on file size)
	if metadata.Duration == 0 {
		// Assume 1 Mbps average bitrate for estimation
		estimatedSeconds := float64(metadata.FileSize) / (1024 * 1024 / 8) // MB -> seconds at 1 Mbps
		if estimatedSeconds > 0 && estimatedSeconds < 7200 {               // Reasonable range (0-2 hours)
			metadata.Duration = int(estimatedSeconds * 1000)
			metadata.DurationSeconds = estimatedSeconds
		} else {
			metadata.Duration = 30000 // 30 seconds default
			metadata.DurationSeconds = 30.0
		}
	}

	// Set defaults for other fields
	if err := common.ValidateRequiredParam("metadata.VideoCodec", metadata.VideoCodec); err != nil {
		metadata.VideoCodec = "avc1" // H.264 is most common
	}

	metadata.HasVideo = true
	metadata.HasAudio = true // Assume most videos have audio

	// Calculate estimated bitrate
	if metadata.DurationSeconds > 0 {
		metadata.Bitrate = int64(float64(metadata.FileSize*8) / metadata.DurationSeconds)
	}

	metadata.FrameRate = 25.0 // Default frame rate

	// Return populated fallback metadata with error indicating parsing failed
	// Callers should use the returned metadata even when error is non-nil
	return metadata, ErrVideoMetadataParsingFailed
}

// IsVideoCodecExported is an exported version of isVideoCodec for testing/demo purposes
func IsVideoCodecExported(codec string) bool {
	return isVideoCodec(codec)
}

// IsAudioCodecExported is an exported version of isAudioCodec for testing/demo purposes
func IsAudioCodecExported(codec string) bool {
	return isAudioCodec(codec)
}
