package handlers

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/media"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

// avatarUploadRequest holds the parsed multipart file part of an avatar upload.
type avatarUploadRequest struct {
	FileName    string
	ContentType string
	FileData    []byte
}

// HandleUploadAvatarLift handles POST /api/v1/accounts/avatar
func (h *Handler) HandleUploadAvatarLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	upload, err := h.parseAvatarUpload(ctx)
	if err != nil {
		if errors.Is(err, media.ErrAvatarRequired) {
			return common.RespondUnprocessableEntity(ctx, "avatar file is required")
		}
		h.logger.Error("failed to parse avatar upload", zap.Error(err))
		return common.RespondBadRequest(ctx, "failed to parse avatar upload")
	}
	if int64(len(upload.FileData)) > media.AvatarMaxUploadBytes {
		return common.SendError(ctx, http.StatusRequestEntityTooLarge, "avatar file is too large")
	}

	stored, err := h.registry.Media().StoreAvatar(ctx.Context(), &media.StoreAvatarCommand{
		UserID:      claims.Username,
		FileName:    upload.FileName,
		ContentType: upload.ContentType,
		FileData:    upload.FileData,
	})
	if err != nil {
		return h.respondAvatarUploadError(ctx, claims.Username, err)
	}

	result, err := h.registry.Accounts().SetAvatar(ctx.Context(), &accounts.SetAvatarCommand{
		Username:    claims.Username,
		AvatarID:    stored.AvatarID,
		ContentType: stored.ContentType,
	})
	if err != nil {
		h.logger.Error("failed to attach avatar to account", zap.String("user", claims.Username), zap.Error(err))
		h.deleteOrphanedAvatar(ctx, stored.AvatarID)
		return common.RespondFailedToUpdate(ctx, "profile")
	}

	// Best-effort cleanup of the object this account previously recorded. The id
	// comes from the authenticated principal's own row, so a re-upload can only
	// ever delete the caller's own previous object.
	h.deleteOrphanedAvatar(ctx, result.PreviousAvatarID)

	mastodonAccount, err := h.mastodonAccountFromStorageAccountWithStatusCount(ctx.Context(), result.Account)
	if err != nil {
		h.logger.Error("failed to transform avatar response", zap.Error(err))
		return common.RespondFailedToUpdate(ctx, "profile")
	}

	return h.respondOK(ctx, mastodonAccount)
}

// HandleClearAvatarLift handles DELETE /api/v1/accounts/avatar
func (h *Handler) HandleClearAvatarLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	result, err := h.registry.Accounts().ClearAvatar(ctx.Context(), &accounts.ClearAvatarCommand{
		Username: claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to clear avatar", zap.String("user", claims.Username), zap.Error(err))
		return common.RespondFailedToDelete(ctx, "avatar")
	}

	// Best-effort cleanup of the now-orphaned stored object. The id is the one the
	// authenticated principal's own row recorded when the avatar was set, so a
	// clear can never address an object belonging to another account. Rows with
	// no recorded id delete nothing.
	h.deleteOrphanedAvatar(ctx, result.PreviousAvatarID)

	mastodonAccount, err := h.mastodonAccountFromStorageAccountWithStatusCount(ctx.Context(), result.Account)
	if err != nil {
		h.logger.Error("failed to transform avatar response", zap.Error(err))
		return common.RespondFailedToDelete(ctx, "avatar")
	}

	return h.respondOK(ctx, mastodonAccount)
}

// HandleGetAvatarLift handles GET /api/v1/avatars/{id}. It is public and serves
// only lesser-stored avatar objects addressed by a validated opaque id.
func (h *Handler) HandleGetAvatarLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	id := avatarIDParam(ctx)

	data, contentType, err := h.registry.Media().GetAvatar(ctx.Context(), id)
	if err != nil {
		switch {
		// A stored object that is missing, or whose stored state is not a
		// servable avatar (drifted content type), is reported as absent. Serving
		// it is impossible, so the response is the same 404 body as a missing id
		// rather than a 500.
		case errors.Is(err, media.ErrAvatarInvalidID),
			errors.Is(err, media.ErrAvatarNotFound),
			errors.Is(err, media.ErrAvatarContentTypeNotAllowed):
			return common.RespondNotFound(ctx, "avatar")
		case errors.Is(err, media.ErrAvatarStoreUnavailable):
			h.logger.Error("avatar store unavailable", zap.Error(err))
			return common.RespondServiceUnavailable(ctx, "avatar")
		default:
			h.logger.Error("failed to read avatar", zap.String("avatar_id", id), zap.Error(err))
			return common.RespondInternalServerError(ctx, "failed to read avatar")
		}
	}

	return &apptheory.Response{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"content-type":           {contentType},
			"cache-control":          {media.AvatarCacheControl},
			"content-length":         {strconv.Itoa(len(data))},
			"x-content-type-options": {"nosniff"},
		},
		Body:     data,
		IsBase64: true,
	}, nil
}

func avatarIDParam(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	return ctx.Param("id")
}

func (h *Handler) respondAvatarUploadError(ctx *apptheory.Context, username string, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, media.ErrAvatarTooLarge):
		return common.SendError(ctx, http.StatusRequestEntityTooLarge, "avatar file is too large")
	case errors.Is(err, media.ErrAvatarRequired):
		return common.RespondUnprocessableEntity(ctx, "avatar file is required")
	case errors.Is(err, media.ErrAvatarContentTypeNotAllowed):
		return common.RespondUnprocessableEntity(ctx, "unsupported avatar image type")
	default:
		// Covers media.ErrAvatarStoreUnavailable and every unexpected failure.
		h.logger.Error("failed to store avatar", zap.String("user", username), zap.Error(err))
		return common.RespondFailedToUpdate(ctx, "profile")
	}
}

func (h *Handler) deleteOrphanedAvatar(ctx *apptheory.Context, id string) {
	if id == "" {
		return
	}
	if err := h.registry.Media().DeleteAvatar(ctx.Context(), id); err != nil {
		h.logger.Warn("failed to delete stored avatar object", zap.String("avatar_id", id), zap.Error(err))
	}
}

// parseAvatarUpload parses the multipart form data of an avatar upload. The
// file part may be named "file" or "avatar"; exactly one non-empty file part is
// required.
func (h *Handler) parseAvatarUpload(ctx *apptheory.Context) (*avatarUploadRequest, error) {
	bodyBytes := ctx.Request.Body
	if err := common.ValidateSliceNotEmpty("request_body", bodyBytes); err != nil {
		return nil, errors.Join(emptyRequestBody(), media.ErrAvatarRequired)
	}

	bodyBytes, err := h.handleBase64Decoding(bodyBytes)
	if err != nil {
		return nil, errors.Join(unableToParseRequestBody(), err)
	}

	boundary, err := h.extractBoundary(headerValue(ctx, "Content-Type"))
	if err != nil {
		return nil, errors.Join(failedToExtractBoundary(), err)
	}

	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)

	var upload avatarUploadRequest
	fileParts := 0
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}

		if isAvatarFilePart(part.FormName()) {
			fileParts++
		}
		if err := readAvatarPart(part, &upload, fileParts); err != nil {
			h.logger.Warn("failed to process multipart part", zap.Error(err))
		}

		if err := part.Close(); err != nil {
			h.logger.Warn("failed to close multipart reader", zap.Error(err))
		}
	}

	if fileParts != 1 || len(upload.FileData) == 0 {
		return nil, errors.Join(noFileDataFoundInRequest(), media.ErrAvatarRequired)
	}

	return &upload, nil
}

func isAvatarFilePart(name string) bool {
	return name == "file" || name == "avatar"
}

// readAvatarPart copies a single file part into the upload request. Parts after
// the first file part are ignored so an ambiguous request cannot smuggle a
// different payload past validation.
func readAvatarPart(part *multipart.Part, upload *avatarUploadRequest, fileParts int) error {
	if !isAvatarFilePart(part.FormName()) || fileParts > 1 || upload == nil {
		return nil
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		return err
	}

	upload.FileData = buf.Bytes()
	upload.FileName = part.FileName()
	upload.ContentType = part.Header.Get("Content-Type")
	return nil
}
