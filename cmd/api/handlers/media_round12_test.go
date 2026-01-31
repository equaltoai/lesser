package lift

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/media"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMediaHandlers(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{}

	mediaSvc := &MediaServiceStub{
		UploadMediaFunc: func(_ context.Context, cmd *media.UploadMediaCommand) (*media.Result, error) {
			require.Equal(t, "alice", cmd.UserID)
			require.Equal(t, "test.png", cmd.FileName)
			require.NotEmpty(t, cmd.FileData)
			return &media.Result{Media: &storagemodels.Media{
				MediaID:       "m1",
				ContentType:   cmd.ContentType,
				CDNUrl:        "https://cdn.example.com/m1",
				Description:   cmd.Description,
				Blurhash:      "hash",
				Width:         10,
				Height:        20,
				Duration:      0,
				IsNSFW:        cmd.Sensitive,
				SpoilerText:   cmd.SpoilerText,
				Focus:         cmd.Focus,
				MediaCategory: cmd.MediaCategory,
			}}, nil
		},
		GetMediaFunc: func(_ context.Context, query *media.GetMediaQuery) (*storagemodels.Media, error) {
			require.Equal(t, "m1", query.MediaID)
			require.Equal(t, "alice", query.ViewerID)
			return &storagemodels.Media{MediaID: "m1", ContentType: "image/png", CDNUrl: "https://cdn.example.com/m1", Width: 10, Height: 20}, nil
		},
		UpdateMediaFunc: func(_ context.Context, cmd *media.UpdateMediaCommand) (*media.UpdateResult, error) {
			require.Equal(t, "m1", cmd.MediaID)
			require.Equal(t, "alice", cmd.UserID)
			return &media.UpdateResult{Media: &storagemodels.Media{MediaID: "m1", ContentType: "image/png", CDNUrl: "https://cdn.example.com/m1", Width: 10, Height: 20}}, nil
		},
	}

	reg := &RegistryStub{MediaSvc: mediaSvc}
	h, _, _ := round11NewHandler(t, cfg, state, reg)

	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	t.Run("upload unauthorized", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", nil, nil, nil)
		handleWithAPIMiddleware(t, h.HandleUploadMediaLift, ctx)
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("upload insufficient scope", func(t *testing.T) {
		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, nil)
		handleWithAPIMiddleware(t, h.HandleUploadMediaLift, ctx)
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("upload success", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.SetBoundary("----WebKitFormBoundaryTestBoundary"))

		filePart, err := writer.CreateFormFile("file", "test.png")
		require.NoError(t, err)
		_, err = filePart.Write([]byte("pngdata"))
		require.NoError(t, err)
		require.NoError(t, writer.WriteField("description", "desc"))
		require.NoError(t, writer.WriteField("focus", "0.0,0.0"))
		require.NoError(t, writer.WriteField("sensitive", "true"))
		require.NoError(t, writer.WriteField("spoiler_text", "spoiler"))
		require.NoError(t, writer.WriteField("media_type", "image"))
		require.NoError(t, writer.Close())

		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  writer.FormDataContentType(),
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, body.Bytes())

		require.NoError(t, h.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("upload parse error", func(t *testing.T) {
		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  "multipart/form-data; boundary=----WebKitFormBoundaryTestBoundary",
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, nil)
		require.NoError(t, h.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("upload base64 decoding error", func(t *testing.T) {
		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  "multipart/form-data; boundary=----WebKitFormBoundaryTestBoundary",
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, []byte("not-multipart"))
		require.NoError(t, h.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("upload missing boundary", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.SetBoundary("----WebKitFormBoundaryTestBoundary"))
		_, err := writer.CreateFormFile("file", "test.png")
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  "multipart/form-data",
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, body.Bytes())
		require.NoError(t, h.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("upload no file data", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.SetBoundary("----WebKitFormBoundaryTestBoundary"))
		require.NoError(t, writer.WriteField("description", "desc"))
		require.NoError(t, writer.Close())

		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  writer.FormDataContentType(),
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, body.Bytes())
		require.NoError(t, h.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("upload invalid focus validation", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.SetBoundary("----WebKitFormBoundaryTestBoundary"))

		filePart, err := writer.CreateFormFile("file", "test.png")
		require.NoError(t, err)
		_, err = filePart.Write([]byte("pngdata"))
		require.NoError(t, err)
		require.NoError(t, writer.WriteField("focus", "bad"))
		require.NoError(t, writer.Close())

		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  writer.FormDataContentType(),
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, body.Bytes())
		require.NoError(t, h.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("upload service failure returns 500", func(t *testing.T) {
		mediaSvcFail := &MediaServiceStub{
			UploadMediaFunc: func(context.Context, *media.UploadMediaCommand) (*media.Result, error) {
				return nil, errors.New("upload failed")
			},
		}
		hFail, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{MediaSvc: mediaSvcFail})

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.SetBoundary("----WebKitFormBoundaryTestBoundary"))
		filePart, err := writer.CreateFormFile("file", "test.png")
		require.NoError(t, err)
		_, err = filePart.Write([]byte("pngdata"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		headers := map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  writer.FormDataContentType(),
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/media", headers, nil, body.Bytes())
		require.NoError(t, hFail.HandleUploadMediaLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get success", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/media/m1", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "m1")
		require.NoError(t, h.HandleGetMediaLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/media/m1", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "m1")
		handleWithAPIMiddleware(t, h.HandleGetMediaLift, ctx)
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get invalid id", func(t *testing.T) {
		invalidID := strings.Repeat("a", 501)
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/media/"+invalidID, headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", invalidID)
		require.NoError(t, h.HandleGetMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get not found", func(t *testing.T) {
		mediaSvc2 := &MediaServiceStub{
			GetMediaFunc: func(_ context.Context, _ *media.GetMediaQuery) (*storagemodels.Media, error) {
				return nil, errors.New("media not found")
			},
		}
		h2, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{MediaSvc: mediaSvc2})

		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/media/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "missing")
		require.NoError(t, h2.HandleGetMediaLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("update unauthorized", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/media/m1", nil, nil, []byte("{}"))
		ctx.SetParam("id", "m1")
		handleWithAPIMiddleware(t, h.HandleUpdateMediaLift, ctx)
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("update invalid id", func(t *testing.T) {
		invalidID := strings.Repeat("a", 501)
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/media/"+invalidID, headers, nil, apimodels.UpdateMediaRequest{Description: "ok"})
		require.NoError(t, err)
		ctx.SetParam("id", invalidID)
		require.NoError(t, h.HandleUpdateMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update invalid json", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/media/m1", headers, nil, []byte("{"))
		ctx.SetParam("id", "m1")
		require.NoError(t, h.HandleUpdateMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update invalid focus", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/media/m1", headers, nil, apimodels.UpdateMediaRequest{Focus: "bad"})
		require.NoError(t, err)
		ctx.SetParam("id", "m1")
		require.NoError(t, h.HandleUpdateMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update invalid description", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/media/m1", headers, nil, apimodels.UpdateMediaRequest{Description: strings.Repeat("x", 2001)})
		require.NoError(t, err)
		ctx.SetParam("id", "m1")
		require.NoError(t, h.HandleUpdateMediaLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update success", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/media/m1", headers, nil, apimodels.UpdateMediaRequest{Description: "ok"})
		require.NoError(t, err)
		ctx.SetParam("id", "m1")
		require.NoError(t, h.HandleUpdateMediaLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("update service failure returns 500", func(t *testing.T) {
		mediaSvcFail := &MediaServiceStub{
			UpdateMediaFunc: func(context.Context, *media.UpdateMediaCommand) (*media.UpdateResult, error) {
				return nil, errors.New("update failed")
			},
		}
		hFail, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{MediaSvc: mediaSvcFail})

		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/media/m1", headers, nil, apimodels.UpdateMediaRequest{Description: "ok"})
		require.NoError(t, err)
		ctx.SetParam("id", "m1")
		require.NoError(t, hFail.HandleUpdateMediaLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}
