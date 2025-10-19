package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type stubMediaService struct {
	lastCmd   *mediasvc.UploadMediaCommand
	result    *mediasvc.Result
	err       error
	callCount int
}

func (s *stubMediaService) UploadMedia(_ context.Context, cmd *mediasvc.UploadMediaCommand) (*mediasvc.Result, error) {
	s.callCount++
	s.lastCmd = cmd
	return s.result, s.err
}

func newTestMediaRecord() *models.Media {
	return &models.Media{
		MediaID:       "media123",
		UserID:        "alice",
		FileName:      "greeting.txt",
		ContentType:   "text/plain; charset=utf-8",
		FileSize:      int64(len("hello world")),
		Description:   "wave",
		Focus:         "0.00,0.00",
		CDNUrl:        "https://cdn.example.com/media123",
		IsNSFW:        true,
		SpoilerText:   "spoiler",
		MediaCategory: models.MediaCategoryDocument,
		CreatedAt:     time.Now().UTC(),
	}
}

func TestHandleUploadMediaSuccess(t *testing.T) {
	stub := &stubMediaService{result: &mediasvc.Result{Media: newTestMediaRecord()}}
	handler := NewSystemCommandHandler(nil, nil, stub, nil, zaptest.NewLogger(t))

	payload := map[string]interface{}{
		"file_data":    base64.StdEncoding.EncodeToString([]byte("hello world")),
		"file_name":    "greeting.txt",
		"description":  "wave",
		"focus":        map[string]interface{}{"x": 0.0, "y": 0.0},
		"sensitive":    true,
		"spoiler_text": "spoiler",
		"media_type":   "document",
	}

	conn := &streaming.ConnectionInfo{UserID: "alice", IsAuthenticated: true}
	cmd := &streaming.Command{ID: "cmd1", Payload: payload}

	resp, err := handler.handleUploadMedia(context.Background(), conn, cmd)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, 1, stub.callCount)
	if assert.NotNil(t, stub.lastCmd) {
		assert.Equal(t, "alice", stub.lastCmd.UserID)
		assert.Equal(t, "greeting.txt", stub.lastCmd.FileName)
		assert.Equal(t, "wave", stub.lastCmd.Description)
		assert.Equal(t, "text/plain; charset=utf-8", stub.lastCmd.ContentType)
		assert.Equal(t, []byte("hello world"), stub.lastCmd.FileData)
		assert.True(t, stub.lastCmd.Sensitive)
		assert.Equal(t, "spoiler", stub.lastCmd.SpoilerText)
		assert.Equal(t, models.MediaCategoryDocument, stub.lastCmd.MediaCategory)
	}
	mediaData := resp.Data["media"].(map[string]interface{})
	assert.Equal(t, "media123", mediaData["id"])
	assert.Equal(t, "https://cdn.example.com/media123", mediaData["url"])
	assert.Equal(t, true, mediaData["sensitive"])
	assert.Equal(t, "spoiler", mediaData["spoiler_text"])
	assert.Equal(t, "document", mediaData["media_category"])
}

func TestHandleUploadMediaInvalidBase64(t *testing.T) {
	stub := &stubMediaService{}
	handler := NewSystemCommandHandler(nil, nil, stub, nil, zaptest.NewLogger(t))

	payload := map[string]interface{}{"file_data": "@@@not-base64@@@"}
	conn := &streaming.ConnectionInfo{UserID: "alice", IsAuthenticated: true}
	cmd := &streaming.Command{ID: "cmd2", Payload: payload}

	resp, err := handler.handleUploadMedia(context.Background(), conn, cmd)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, 0, stub.callCount)
}

func TestHandleUploadMediaInvalidDescription(t *testing.T) {
	stub := &stubMediaService{}
	handler := NewSystemCommandHandler(nil, nil, stub, nil, zaptest.NewLogger(t))

	payload := map[string]interface{}{
		"file_data":   base64.StdEncoding.EncodeToString([]byte("hello world")),
		"description": strings.Repeat("a", 2000),
	}
	conn := &streaming.ConnectionInfo{UserID: "alice", IsAuthenticated: true}
	cmd := &streaming.Command{ID: "cmd3", Payload: payload}

	resp, err := handler.handleUploadMedia(context.Background(), conn, cmd)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, 0, stub.callCount)
}

func TestHandleUploadMediaServiceError(t *testing.T) {
	stub := &stubMediaService{err: errors.New("upload failed")}
	handler := NewSystemCommandHandler(nil, nil, stub, nil, zaptest.NewLogger(t))

	payload := map[string]interface{}{
		"file_data": base64.StdEncoding.EncodeToString([]byte("hello world")),
		"file_name": "greeting.txt",
	}
	conn := &streaming.ConnectionInfo{UserID: "alice", IsAuthenticated: true}
	cmd := &streaming.Command{ID: "cmd4", Payload: payload}

	resp, err := handler.handleUploadMedia(context.Background(), conn, cmd)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, 1, stub.callCount)
}

func TestEnsureFilenameForMimeFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mimeType string
		expected string
	}{
		{name: "avi", mimeType: "video/x-msvideo", expected: "upload.avi"},
		{name: "quicktime", mimeType: "video/quicktime", expected: "upload.mov"},
		{name: "mov", mimeType: "video/mov", expected: "upload.mov"},
		{name: "video_ogg", mimeType: "video/ogg", expected: "upload.ogv"},
		{name: "audio_webm", mimeType: "audio/webm", expected: "upload.webm"},
		{name: "audio_ogg", mimeType: "audio/ogg", expected: "upload.ogg"},
		{name: "audio_aac", mimeType: "audio/aac", expected: "upload.aac"},
		{name: "audio_flac", mimeType: "audio/flac", expected: "upload.flac"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := ensureFilenameForMime("upload", tc.mimeType)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}
