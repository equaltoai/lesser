package handlers

import (
	"bytes"
	"errors"
	"mime/multipart"
	"strconv"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/media"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

// MediaUploadRequest holds parsed multipart media upload data
type MediaUploadRequest struct {
	FileName    string
	MimeType    string
	FileData    []byte
	Description string
	Focus       string
	Sensitive   bool
	SpoilerText string
	MediaType   string
}

// Media type constants
const (
	MediaTypeImage   = "image"
	MediaTypeVideo   = "video"
	MediaTypeAudio   = "audio"
	MediaTypeGifv    = "gifv"
	MediaTypeUnknown = "unknown"
)

// MIME type constants
const (
	MimeTypeImageGif  = "image/gif"
	MimeTypeImageJpeg = "image/jpeg"
	MimeTypeImagePng  = "image/png"
	MimeTypeImageWebp = "image/webp"
)

// HandleUploadMediaLift handles POST /api/v1/media (Lift version)
func (h *Handler) HandleUploadMediaLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse multipart form data
	mediaData, err := h.parseMediaUpload(ctx)
	if err != nil {
		h.logger.Error("failed to parse media upload", zap.Error(err))
		return common.RespondBadRequest(ctx, "failed to parse media upload")
	}

	// Validate media parameters
	params := map[string]interface{}{
		"file":         mediaData.FileName,
		"description":  mediaData.Description,
		"focus":        mediaData.Focus,
		"spoiler_text": mediaData.SpoilerText,
		"sensitive":    mediaData.Sensitive,
		"media_type":   mediaData.MediaType,
	}
	if err := common.ValidateMediaParams(params); err != nil {
		h.logger.Error("media validation failed", zap.Error(err))
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Call Media service
	result, err := h.registry.Media().UploadMedia(ctx.Context(), &media.UploadMediaCommand{
		UserID:        claims.Username,
		FileName:      mediaData.FileName,
		ContentType:   mediaData.MimeType,
		FileData:      mediaData.FileData,
		Description:   mediaData.Description,
		Focus:         mediaData.Focus,
		Sensitive:     mediaData.Sensitive,
		SpoilerText:   strings.TrimSpace(mediaData.SpoilerText),
		MediaCategory: storageModels.MediaCategory(strings.TrimSpace(mediaData.MediaType)),
	})
	if err != nil {
		h.logger.Error("failed to upload media", zap.String("user", claims.Username), zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to upload media")
	}

	return okJSON(h.convertMediaToAPI(result.Media))
}

// HandleGetMediaLift handles GET /api/v1/media/:id (Lift version)
func (h *Handler) HandleGetMediaLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with read scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Extract media ID
	mediaID := ctx.Param("id")
	if err := common.ValidateEntityID(mediaID, "media"); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Call Media service
	mediaResult, err := h.registry.Media().GetMedia(ctx.Context(), &media.GetMediaQuery{
		MediaID:  mediaID,
		ViewerID: claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to get media", zap.String("media_id", mediaID), zap.Error(err))
		return common.RespondNotFound(ctx, "media")
	}

	return okJSON(h.convertMediaToAPI(mediaResult))
}

// HandleUpdateMediaLift handles PUT /api/v1/media/:id (Lift version)
func (h *Handler) HandleUpdateMediaLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Extract media ID
	mediaID := ctx.Param("id")
	if err := common.ValidateEntityID(mediaID, "media"); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Parse update request
	var req apimodels.UpdateMediaRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Validate media update parameters
	if req.Description != "" {
		if err := common.ValidateMediaDescription(req.Description); err != nil {
			h.logger.Error("media description validation failed", zap.Error(err))
			return common.RespondBadRequest(ctx, err.Error())
		}
	}
	if req.Focus != "" {
		if err := common.ValidateMediaFocus(req.Focus); err != nil {
			h.logger.Error("media focus validation failed", zap.Error(err))
			return common.RespondBadRequest(ctx, err.Error())
		}
	}

	// Call Media service
	result, err := h.registry.Media().UpdateMedia(ctx.Context(), &media.UpdateMediaCommand{
		MediaID:     mediaID,
		UserID:      claims.Username,
		Description: req.Description,
		Focus:       req.Focus,
	})
	if err != nil {
		h.logger.Error("failed to update media", zap.String("media_id", mediaID), zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to update media")
	}

	return okJSON(h.convertMediaToAPI(result.Media))
}

// parseMediaUpload parses multipart form data for media uploads
func (h *Handler) parseMediaUpload(ctx *apptheory.Context) (*MediaUploadRequest, error) {
	var mediaData MediaUploadRequest

	// Get raw body from request
	bodyBytes := ctx.Request.Body
	if err := common.ValidateSliceNotEmpty("request_body", bodyBytes); err != nil {
		return nil, emptyRequestBody()
	}

	// Handle base64 decoding for API Gateway
	bodyBytes, err := h.handleBase64Decoding(bodyBytes)
	if err != nil {
		return nil, errors.Join(unableToParseRequestBody(), err)
	}

	// Parse multipart form
	boundary, err := h.extractBoundary(headerValue(ctx, "Content-Type"))
	if err != nil {
		return nil, errors.Join(failedToExtractBoundary(), err)
	}

	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)

	// Process each part
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}

		if err := h.processMediaPart(part, &mediaData); err != nil {
			h.logger.Warn("failed to process multipart part", zap.Error(err))
		}

		if err := part.Close(); err != nil {
			h.logger.Warn("failed to close multipart reader", zap.Error(err))
		}
	}

	// Validate required file data
	if err := common.ValidateSliceNotEmpty("mediaData.FileData", mediaData.FileData); err != nil {
		return nil, noFileDataFoundInRequest()
	}

	return &mediaData, nil
}

// processMediaPart processes a single multipart form part for media upload
func (h *Handler) processMediaPart(part *multipart.Part, mediaData *MediaUploadRequest) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		return err
	}

	switch part.FormName() {
	case "file":
		if part.FileName() != "" {
			mediaData.FileData = buf.Bytes()
			mediaData.FileName = part.FileName()
			mediaData.MimeType = part.Header.Get("Content-Type")
		}
	case "description":
		mediaData.Description = buf.String()
	case "focus":
		mediaData.Focus = buf.String()
	case "sensitive":
		value := strings.TrimSpace(buf.String())
		if parsed, err := strconv.ParseBool(value); err == nil {
			mediaData.Sensitive = parsed
		}
	case "spoiler_text", "spoilerText":
		mediaData.SpoilerText = buf.String()
	case "media_type", "mediaType":
		mediaData.MediaType = buf.String()
	}

	return nil
}
