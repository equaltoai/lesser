package media

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAtomExtended(buf *bytes.Buffer, atomType string, data []byte) {
	binary.Write(buf, binary.BigEndian, uint32(1))
	buf.WriteString(atomType)
	binary.Write(buf, binary.BigEndian, uint64(len(data)+16))
	buf.Write(data)
}

func buildStsdData(t *testing.T, codec string) []byte {
	t.Helper()
	require.Len(t, codec, 4)

	data := make([]byte, 24)
	binary.BigEndian.PutUint32(data[4:8], 1) // entry count
	binary.BigEndian.PutUint32(data[8:12], 16)
	copy(data[12:16], []byte(codec))
	return data
}

func buildTrakV0Atom(t *testing.T, duration uint32, width uint32, height uint32, handlerType string, codec string) []byte {
	t.Helper()
	require.Len(t, handlerType, 4)

	var trakBuf bytes.Buffer

	tkhdData := make([]byte, 84)
	tkhdData[0] = 0    // version 0
	tkhdData[3] = 0x07 // flags: track enabled, in movie, in preview
	binary.BigEndian.PutUint32(tkhdData[20:24], duration)
	binary.BigEndian.PutUint32(tkhdData[76:80], width<<16)
	binary.BigEndian.PutUint32(tkhdData[80:84], height<<16)
	writeAtom(&trakBuf, "tkhd", tkhdData)

	var mdiaBuf bytes.Buffer
	hdlrData := make([]byte, 32)
	copy(hdlrData[8:12], []byte(handlerType))
	writeAtom(&mdiaBuf, "hdlr", hdlrData)

	var stblBuf bytes.Buffer
	writeAtom(&stblBuf, "stsd", buildStsdData(t, codec))

	var minfBuf bytes.Buffer
	writeAtom(&minfBuf, "stbl", stblBuf.Bytes())
	writeAtom(&mdiaBuf, "minf", minfBuf.Bytes())

	writeAtom(&trakBuf, "mdia", mdiaBuf.Bytes())
	return trakBuf.Bytes()
}

func createMockMP4DataWithTracks(t *testing.T, tracks ...[]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	writeAtom(&buf, "ftyp", []byte("isom\x00\x00\x00\x01isom"))

	var moovBuf bytes.Buffer
	mvhdData := make([]byte, 32)
	mvhdData[0] = 0
	binary.BigEndian.PutUint32(mvhdData[12:16], 1000)
	binary.BigEndian.PutUint32(mvhdData[16:20], 30000)
	writeAtom(&moovBuf, "mvhd", mvhdData)

	for _, trak := range tracks {
		writeAtom(&moovBuf, "trak", trak)
	}

	writeAtom(&buf, "moov", moovBuf.Bytes())
	return buf.Bytes()
}

func TestParseVideoMetadata_WithCodecsAndNestedAtoms(t *testing.T) {
	videoTrack := buildTrakV0Atom(t, 30000, 1920, 1080, "vide", "avc1")
	audioTrack := buildTrakV0Atom(t, 30000, 0, 0, "soun", "mp4a")
	data := createMockMP4DataWithTracks(t, videoTrack, audioTrack)

	metadata, err := ParseVideoMetadata(data)
	require.NoError(t, err)

	assert.Equal(t, 1920, metadata.Width)
	assert.Equal(t, 1080, metadata.Height)
	assert.Equal(t, "avc1", metadata.VideoCodec)
	assert.Equal(t, "mp4a", metadata.AudioCodec)
	assert.True(t, metadata.HasVideo)
	assert.True(t, metadata.HasAudio)
	assert.Equal(t, 30.0, metadata.FrameRate)
}

func TestParseVideoMetadata_FrameRateUsesSDDefault(t *testing.T) {
	videoTrack := buildTrakV0Atom(t, 30000, 640, 480, "vide", "avc1")
	data := createMockMP4DataWithTracks(t, videoTrack)

	metadata, err := ParseVideoMetadata(data)
	require.NoError(t, err)

	assert.Equal(t, 24.0, metadata.FrameRate)
}

func TestParseVideoMetadata_MoovMissingFallsBack(t *testing.T) {
	var buf bytes.Buffer
	writeAtom(&buf, "ftyp", []byte("isom\x00\x00\x00\x01isom"))

	metadata, err := ParseVideoMetadata(buf.Bytes())
	require.ErrorIs(t, err, ErrVideoMetadataParsingFailed)
	require.NotNil(t, metadata)
}

func TestParseVideoMetadata_PartialAfterMoovIsOK(t *testing.T) {
	videoTrack := buildTrakV0Atom(t, 30000, 1920, 1080, "vide", "avc1")
	data := createMockMP4DataWithTracks(t, videoTrack)

	// Add a truncated atom so parseAtoms hits an error after moov was found.
	data = append(data, []byte{
		0x00, 0x00, 0x00, 0x10,
		'f', 'r', 'e', 'e',
		0x01, 0x02, 0x03, 0x04,
	}...)

	metadata, err := ParseVideoMetadata(data)
	require.NoError(t, err)
	assert.Equal(t, 1920, metadata.Width)
	assert.Equal(t, 1080, metadata.Height)
}

func TestParseVideoMetadata_SkipsFailedTracks(t *testing.T) {
	var brokenTrak bytes.Buffer
	writeAtom(&brokenTrak, "tkhd", []byte{0x00})

	goodVideoTrack := buildTrakV0Atom(t, 30000, 1920, 1080, "vide", "avc1")
	data := createMockMP4DataWithTracks(t, brokenTrak.Bytes(), goodVideoTrack)

	metadata, err := ParseVideoMetadata(data)
	require.NoError(t, err)
	assert.Equal(t, 1920, metadata.Width)
	assert.Equal(t, 1080, metadata.Height)
}

func TestVideoMetadataParser_parseMinfAtom_IgnoresStblErrors(t *testing.T) {
	var minfBuf bytes.Buffer
	writeAtom(&minfBuf, "stbl", []byte{
		0x00, 0x00, 0x00, 0x10, // size=16
		's', 't', 's', 'd', // type
	})

	p := &VideoMetadataParser{}
	metadata := &VideoMetadata{}

	err := p.parseMinfAtom(minfBuf.Bytes(), metadata)
	require.NoError(t, err)
	assert.Empty(t, metadata.VideoCodec)
	assert.Empty(t, metadata.AudioCodec)
}

func TestVideoMetadataParser_parseStblAtom_IgnoresStsdErrors(t *testing.T) {
	var stblBuf bytes.Buffer
	writeAtom(&stblBuf, "stsd", []byte{0x00})

	p := &VideoMetadataParser{}
	metadata := &VideoMetadata{}

	err := p.parseStblAtom(stblBuf.Bytes(), metadata)
	require.NoError(t, err)
	assert.Empty(t, metadata.VideoCodec)
	assert.Empty(t, metadata.AudioCodec)
}

func TestParseStsdAtom_ErrorsAndEntryCountZero(t *testing.T) {
	metadata := &VideoMetadata{}

	require.ErrorIs(t, parseStsdAtom([]byte{0x00}, metadata), ErrStsdAtomTooSmall)

	zeroEntries := make([]byte, 16)
	require.NoError(t, parseStsdAtom(zeroEntries, metadata))

	oneEntryIncomplete := make([]byte, 20)
	binary.BigEndian.PutUint32(oneEntryIncomplete[4:8], 1)
	require.ErrorIs(t, parseStsdAtom(oneEntryIncomplete, metadata), ErrStsdEntryIncomplete)

	audioEntry := buildStsdData(t, "mp4a")
	require.NoError(t, parseStsdAtom(audioEntry, metadata))
	assert.Equal(t, "mp4a", metadata.AudioCodec)
	assert.True(t, metadata.HasAudio)
}

func TestParseMvhdAtom_Version1_ClampsAndComputesDuration(t *testing.T) {
	mvhdData := make([]byte, 44)
	mvhdData[0] = 1
	binary.BigEndian.PutUint64(mvhdData[4:12], 0x1_0000_0000)  // clamp creation time
	binary.BigEndian.PutUint64(mvhdData[12:20], 0x1_0000_0000) // clamp modification time
	binary.BigEndian.PutUint32(mvhdData[20:24], 1000)
	binary.BigEndian.PutUint64(mvhdData[24:32], 5000)

	metadata := &VideoMetadata{}
	require.NoError(t, parseMvhdAtom(mvhdData, metadata))

	assert.Equal(t, uint32(1000), metadata.Timescale)
	assert.Equal(t, uint32(0xFFFFFFFF), metadata.CreationTime)
	assert.Equal(t, uint32(0xFFFFFFFF), metadata.ModificationTime)
	assert.Equal(t, 5000, metadata.Duration)
	assert.Equal(t, 5.0, metadata.DurationSeconds)
}

func TestParseTkhdAtom_Version1_SetsDimensionsAndUpdatesDuration(t *testing.T) {
	tkhdData := make([]byte, 96)
	tkhdData[0] = 1
	binary.BigEndian.PutUint64(tkhdData[28:36], 40000) // duration (timescale units)
	binary.BigEndian.PutUint32(tkhdData[88:92], 1280<<16)
	binary.BigEndian.PutUint32(tkhdData[92:96], 720<<16)

	metadata := &VideoMetadata{Timescale: 1000}
	require.NoError(t, parseTkhdAtom(tkhdData, metadata))

	assert.Equal(t, 1280, metadata.Width)
	assert.Equal(t, 720, metadata.Height)
	assert.True(t, metadata.HasVideo)
	assert.Equal(t, 40000, metadata.Duration)
}

func TestVideoMetadataParser_readAtom_ExtendedSizeAndErrors(t *testing.T) {
	t.Run("extended size success", func(t *testing.T) {
		var buf bytes.Buffer
		writeAtomExtended(&buf, "free", []byte{0x01, 0x02, 0x03, 0x04})

		p := NewVideoMetadataParser(buf.Bytes())
		atom, err := p.readAtom()
		require.NoError(t, err)
		require.NotNil(t, atom)
		assert.Equal(t, "free", string(atom.Type[:]))
		assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, atom.Data)
	})

	t.Run("incomplete extended size", func(t *testing.T) {
		data := []byte{
			0x00, 0x00, 0x00, 0x01,
			'f', 'r', 'e', 'e',
			0x00, 0x00, 0x00, 0x10,
		}
		p := NewVideoMetadataParser(data)
		_, err := p.readAtom()
		require.ErrorIs(t, err, ErrExtendedSizeIncomplete)
	})

	t.Run("extended size too large", func(t *testing.T) {
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, uint32(1))
		buf.WriteString("free")
		binary.Write(&buf, binary.BigEndian, uint64(0x1_0000_0000))

		p := NewVideoMetadataParser(buf.Bytes())
		_, err := p.readAtom()
		require.ErrorIs(t, err, ErrAtomSizeTooLarge)
	})

	t.Run("invalid atom size", func(t *testing.T) {
		data := []byte{
			0x00, 0x00, 0x00, 0x04,
			'f', 'r', 'e', 'e',
		}
		p := NewVideoMetadataParser(data)
		_, err := p.readAtom()
		require.ErrorIs(t, err, ErrInvalidAtomSize)
	})

	t.Run("atom extends file", func(t *testing.T) {
		data := []byte{
			0x00, 0x00, 0x00, 0x10,
			'f', 'r', 'e', 'e',
			0x01, 0x02, 0x03, 0x04,
		}
		p := NewVideoMetadataParser(data)
		_, err := p.readAtom()
		require.ErrorIs(t, err, ErrAtomExtendsFile)
	})

	t.Run("io EOF", func(t *testing.T) {
		p := NewVideoMetadataParser([]byte{0x00, 0x00, 0x00})
		_, err := p.readAtom()
		require.ErrorIs(t, err, io.EOF)
	})
}

func TestExportedCodecHelpers(t *testing.T) {
	assert.True(t, IsVideoCodecExported("avc1"))
	assert.False(t, IsVideoCodecExported("zzzz"))
	assert.True(t, IsAudioCodecExported("mp4a"))
	assert.False(t, IsAudioCodecExported("zzzz"))
}

func TestVideoMetadataParser_fallbackMetadata_UsesSizeBasedDefaults(t *testing.T) {
	t.Run("720p medium file", func(t *testing.T) {
		parser := &VideoMetadataParser{}
		metadata := &VideoMetadata{
			FileSize: 60 * 1024 * 1024,
		}

		got, err := parser.fallbackMetadata(metadata)
		require.ErrorIs(t, err, ErrVideoMetadataParsingFailed)
		assert.Equal(t, 1280, got.Width)
		assert.Equal(t, 720, got.Height)
		assert.Equal(t, "avc1", got.VideoCodec)
	})

	t.Run("1080p large file with duration default", func(t *testing.T) {
		parser := &VideoMetadataParser{}
		metadata := &VideoMetadata{
			FileSize:     1 * 1024 * 1024 * 1024,
			VideoCodec:   "hvc1",
			AudioCodec:   "mp4a",
			HasAudio:     true,
			HasVideo:     true,
			Duration:     0,
			FrameRate:    0,
			Bitrate:      0,
			Timescale:    0,
			Width:        0,
			Height:       0,
			CreationTime: 0,
		}

		got, err := parser.fallbackMetadata(metadata)
		require.ErrorIs(t, err, ErrVideoMetadataParsingFailed)
		assert.Equal(t, 1920, got.Width)
		assert.Equal(t, 1080, got.Height)
		assert.Equal(t, 30000, got.Duration)
		assert.Equal(t, 30.0, got.DurationSeconds)
		assert.Equal(t, "hvc1", got.VideoCodec)
	})
}
