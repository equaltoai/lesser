package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureFilenameHasExtensionFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filename    string
		contentType string
		expected    string
	}{
		{name: "quicktime", filename: "clip", contentType: "video/quicktime", expected: "clip.mov"},
		{name: "avi", filename: "clip", contentType: "video/x-msvideo", expected: "clip.avi"},
		{name: "audio_webm", filename: "recording", contentType: "audio/webm", expected: "recording.webm"},
		{name: "audio_ogg", filename: "track", contentType: "audio/ogg", expected: "track.ogg"},
		{name: "audio_aac", filename: "song", contentType: "audio/aac", expected: "song.aac"},
		{name: "audio_flac", filename: "sample", contentType: "audio/flac", expected: "sample.flac"},
		{name: "video_ogg", filename: "teaser", contentType: "video/ogg", expected: "teaser.ogv"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := EnsureFilenameHasExtension(tc.filename, tc.contentType)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestEnsureFilenameHasExtensionKeepsExistingExtension(t *testing.T) {
	t.Parallel()

	result, err := EnsureFilenameHasExtension("example.mp4", "video/mp4")
	require.NoError(t, err)
	require.Equal(t, "example.mp4", result)
}

func TestEnsureFilenameHasExtensionUnknownType(t *testing.T) {
	t.Parallel()

	result, err := EnsureFilenameHasExtension("upload", "application/unknown")
	require.NoError(t, err)
	require.Equal(t, "upload.bin", result)
}
