package lift

import (
	"bytes"
	"fmt"
	"mime/multipart"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// MediaUploadRequest holds parsed multipart media upload data
type MediaUploadRequest struct {
	FileName    string
	MimeType    string
	FileData    []byte
	Description string
	Focus       string
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
func (h *Handler) HandleUploadMediaLift(ctx *lift.Context) error {
	// Authenticate user with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Parse multipart form data
	mediaData, err := h.parseMediaUpload(ctx)
	if err != nil {
		h.logger.Error("failed to parse media upload", zap.Error(err))
		return common.RespondBadRequest(ctx, "failed to parse media upload")
	}

	// Validate media parameters
	params := map[string]interface{}{
		"file":        mediaData.FileName,
		"description": mediaData.Description,
		"focus":       mediaData.Focus,
	}
	if err := common.ValidateMediaParams(params); err != nil {
		h.logger.Error("media validation failed", zap.Error(err))
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Call Media service
	result, err := h.registry.Media().UploadMedia(ctx.Context, &media.UploadMediaCommand{
		UserID:      claims.Username,
		FileName:    mediaData.FileName,
		ContentType: mediaData.MimeType,
		FileData:    mediaData.FileData,
		Description: mediaData.Description,
		Focus:       mediaData.Focus,
	})
	if err != nil {
		h.logger.Error("failed to upload media", zap.String("user", claims.Username), zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to upload media")
	}

	return ctx.JSON(result.Media)
}

// HandleGetMediaLift handles GET /api/v1/media/:id (Lift version)
func (h *Handler) HandleGetMediaLift(ctx *lift.Context) error {
	// Authenticate user with read scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Extract media ID
	mediaID := ctx.Param("id")
	if err := common.ValidateEntityID(mediaID, "media"); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Call Media service
	mediaResult, err := h.registry.Media().GetMedia(ctx.Context, &media.GetMediaQuery{
		MediaID:  mediaID,
		ViewerID: claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to get media", zap.String("media_id", mediaID), zap.Error(err))
		return common.RespondNotFound(ctx, "media")
	}

	return ctx.JSON(mediaResult)
}

// HandleUpdateMediaLift handles PUT /api/v1/media/:id (Lift version)
func (h *Handler) HandleUpdateMediaLift(ctx *lift.Context) error {
	// Authenticate user with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Extract media ID
	mediaID := ctx.Param("id")
	if err := common.ValidateEntityID(mediaID, "media"); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Parse update request
	var req struct {
		Description string `json:"description"`
		Focus       string `json:"focus"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
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
	result, err := h.registry.Media().UpdateMedia(ctx.Context, &media.UpdateMediaCommand{
		MediaID:     mediaID,
		UserID:      claims.Username,
		Description: req.Description,
		Focus:       req.Focus,
	})
	if err != nil {
		h.logger.Error("failed to update media", zap.String("media_id", mediaID), zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to update media")
	}

	return ctx.JSON(result.Media)
}

// parseMediaUpload parses multipart form data for media uploads
func (h *Handler) parseMediaUpload(ctx *lift.Context) (*MediaUploadRequest, error) {
	var mediaData MediaUploadRequest

	// Get raw body from request
	bodyBytes := ctx.Request.Body
	if err := common.ValidateSliceNotEmpty("request_body", bodyBytes); err != nil {
		return nil, fmt.Errorf("empty request body")
	}

	// Handle base64 decoding for API Gateway
	bodyBytes, err := h.handleBase64Decoding(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse request body: %w", err)
	}

	// Parse multipart form
	boundary, err := h.extractBoundary(ctx.Header("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("failed to extract boundary: %w", err)
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
		return nil, fmt.Errorf("no file data found in request")
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
	}

	return nil
}
