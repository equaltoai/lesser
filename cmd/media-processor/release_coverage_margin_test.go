package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReleaseCoverageMargin_VideoMetadataDrivesDurationValidation(t *testing.T) {
	t.Parallel()

	data := releaseCoverageMP4(t, 1920, 1080, 30_000)
	mp := &MediaProcessor{logger: zap.NewNop(), mediaRepo: &fakeMediaRepo{}}

	width, height, durationMillis := mp.extractVideoMetadata(data)
	require.Equal(t, 1920, width)
	require.Equal(t, 1080, height)
	require.Equal(t, 30_000, durationMillis)

	durationSeconds, err := mp.getVideoMetadata(data, "video/mp4")
	require.NoError(t, err)
	require.Equal(t, 30, durationSeconds)

	tests := []struct {
		name        string
		maxDuration int
		wantErr     bool
	}{
		{name: "within configured duration", maxDuration: 31},
		{name: "exceeds configured duration", maxDuration: 29, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mp.validateFileForUser(data, "video/mp4", &MediaConfig{
				MaxVideoDuration:       tt.maxDuration,
				VideoProcessingEnabled: true,
			}, "alice", "media-1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestReleaseCoverageMargin_MediaUserResolutionAndCostFallbacks(t *testing.T) {
	t.Parallel()

	repo := &fakeMediaRepo{userByUsername: map[string]*models.UserMediaConfig{
		"alice": {UserID: "user-123", Username: "alice"},
	}}
	mp := &MediaProcessor{logger: zap.NewNop(), mediaRepo: repo}

	userID, err := mp.resolveUsernameToUserID(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, "user-123", userID)
	require.Equal(t, int64(100), mp.estimateProcessingCost([]byte("payload"), "application/octet-stream"))

	require.NoError(t, mp.validateFileForUser(
		[]byte("ID3\x04\x00\x00\x00\x00\x00\x00"),
		"audio/mpeg",
		&MediaConfig{AudioProcessingEnabled: true},
		"alice",
		"media-audio",
	))
}

func releaseCoverageMP4(t *testing.T, width, height, durationMillis uint32) []byte {
	t.Helper()

	var file bytes.Buffer
	writeReleaseCoverageAtom(t, &file, "ftyp", []byte("mp42\x00\x00\x00\x00mp42isom"))

	var movie bytes.Buffer
	movieHeader := make([]byte, 32)
	binary.BigEndian.PutUint32(movieHeader[12:16], 1000)
	binary.BigEndian.PutUint32(movieHeader[16:20], durationMillis)
	writeReleaseCoverageAtom(t, &movie, "mvhd", movieHeader)

	var track bytes.Buffer
	trackHeader := make([]byte, 84)
	trackHeader[3] = 0x07
	binary.BigEndian.PutUint32(trackHeader[20:24], durationMillis)
	binary.BigEndian.PutUint32(trackHeader[76:80], width<<16)
	binary.BigEndian.PutUint32(trackHeader[80:84], height<<16)
	writeReleaseCoverageAtom(t, &track, "tkhd", trackHeader)

	var media bytes.Buffer
	handler := make([]byte, 32)
	copy(handler[8:12], "vide")
	writeReleaseCoverageAtom(t, &media, "hdlr", handler)
	writeReleaseCoverageAtom(t, &track, "mdia", media.Bytes())
	writeReleaseCoverageAtom(t, &movie, "trak", track.Bytes())
	writeReleaseCoverageAtom(t, &file, "moov", movie.Bytes())

	return file.Bytes()
}

func writeReleaseCoverageAtom(t *testing.T, dst *bytes.Buffer, atomType string, data []byte) {
	t.Helper()
	require.NoError(t, binary.Write(dst, binary.BigEndian, uint32(len(data)+8)))
	_, err := dst.WriteString(atomType)
	require.NoError(t, err)
	_, err = dst.Write(data)
	require.NoError(t, err)
}
