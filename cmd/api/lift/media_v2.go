package lift

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleUploadMediaV2Lift handles POST /api/v2/media - Async media upload
func (h *Handler) HandleUploadMediaV2Lift(ctx *lift.Context) error {
	// Authenticate user
	claims, err := h.authenticateMediaRequest(ctx)
	if err != nil {
		return err
	}

	// Parse multipart form data
	reader, err := h.createMultipartReader(ctx)
	if err != nil {
		return err
	}

	// Parse and validate multipart data
	data, err := h.parseAndValidateMultipartData(ctx, reader)
	if err != nil {
		return err
	}

	// Upload files to S3 and create records
	mediaID, jobID, s3Key, thumbnailS3Key, err := h.uploadToS3(ctx, claims.Username, data)
	if err != nil {
		return err
	}

	// Create database records and trigger processing
	attachment, err := h.createMediaRecordsAndTriggerProcessing(ctx, claims.Username, mediaID, jobID, s3Key, thumbnailS3Key, data)
	if err != nil {
		return err
	}

	ctx.Status(http.StatusAccepted)
	return ctx.JSON(attachment)
}

// Helper functions for HandleUploadMediaV2Lift

// MediaUploadData holds parsed multipart data
type MediaUploadData struct {
	FileData      []byte
	ThumbnailData []byte
	MimeType      string
	Description   string
	Focus         string
}

// authenticateMediaRequest handles authentication for media upload
func (h *Handler) authenticateMediaRequest(ctx *lift.Context) (*auth.Claims, error) {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		return &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}, nil
	}

	// Extract token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return nil, ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return nil, ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return nil, ctx.JSON(map[string]string{
			"error": "insufficient scope",
		})
	}

	return claims, nil
}

// createMultipartReader creates and initializes a multipart reader
func (h *Handler) createMultipartReader(ctx *lift.Context) (*multipart.Reader, error) {
	bodyBytes := ctx.Request.Body

	// Handle potential base64 encoding (legacy support)
	if len(bodyBytes) > 0 {
		if decoded, err := base64.StdEncoding.DecodeString(string(bodyBytes)); err == nil {
			if bytes.Contains(decoded, []byte("boundary")) || bytes.Contains(decoded, []byte("Content-Disposition")) {
				bodyBytes = decoded
			}
		}
	}

	// Get content type header
	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}

	// Parse boundary from content type
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		h.logger.Error("invalid content type", zap.Error(err))
		ctx.Status(http.StatusBadRequest)
		return nil, ctx.JSON(map[string]string{
			"error": fmt.Sprintf("invalid content type: %v", err),
		})
	}

	boundary := params["boundary"]
	if boundary == "" {
		h.logger.Error("missing boundary in content type")
		ctx.Status(http.StatusBadRequest)
		return nil, ctx.JSON(map[string]string{
			"error": "missing boundary in content type",
		})
	}

	return multipart.NewReader(bytes.NewReader(bodyBytes), boundary), nil
}

// parseAndValidateMultipartData parses multipart data and validates it
func (h *Handler) parseAndValidateMultipartData(ctx *lift.Context, reader *multipart.Reader) (*MediaUploadData, error) {
	data := &MediaUploadData{}

	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		defer func() {
			if err := part.Close(); err != nil {
				h.logger.Warn("failed to close multipart", zap.Error(err))
			}
		}()

		if err := h.processMediaMultipartPart(part, data); err != nil {
			return nil, err
		}
	}

	return h.validateMediaData(ctx, data)
}

// processMediaMultipartPart processes a single multipart part for media upload
func (h *Handler) processMediaMultipartPart(part *multipart.Part, data *MediaUploadData) error {
	switch part.FormName() {
	case "file":
		return h.processFilePart(part, data)
	case "thumbnail":
		return h.processThumbnailPart(part, data)
	case "description":
		return h.processDescriptionPart(part, data)
	case "focus":
		return h.processFocusPart(part, data)
	}
	return nil
}

// processFilePart processes the main file part
func (h *Handler) processFilePart(part *multipart.Part, data *MediaUploadData) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		h.logger.Error("failed to read file", zap.Error(err))
		return fmt.Errorf("failed to read file: %v", err)
	}
	data.FileData = buf.Bytes()

	// Get MIME type from header or detect it
	data.MimeType = part.Header.Get("Content-Type")
	if data.MimeType == "" {
		data.MimeType = http.DetectContentType(data.FileData)
	}

	return nil
}

// processThumbnailPart processes the thumbnail part
func (h *Handler) processThumbnailPart(part *multipart.Part, data *MediaUploadData) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		h.logger.Warn("failed to read thumbnail", zap.Error(err))
		return nil
	}
	data.ThumbnailData = buf.Bytes()
	return nil
}

// processDescriptionPart processes the description part
func (h *Handler) processDescriptionPart(part *multipart.Part, data *MediaUploadData) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		h.logger.Warn("failed to read description", zap.Error(err))
		return nil
	}
	data.Description = buf.String()
	return nil
}

// processFocusPart processes the focus part
func (h *Handler) processFocusPart(part *multipart.Part, data *MediaUploadData) error {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(part); err != nil {
		h.logger.Warn("failed to read focus", zap.Error(err))
		return nil
	}
	data.Focus = buf.String()
	return nil
}

// validateMediaData validates the parsed media data
func (h *Handler) validateMediaData(ctx *lift.Context, data *MediaUploadData) (*MediaUploadData, error) {
	if len(data.FileData) == 0 {
		h.logger.Error("no file data provided")
		ctx.Status(http.StatusBadRequest)
		return nil, ctx.JSON(map[string]string{
			"error": "no file data provided",
		})
	}

	// Validate file size (10MB limit for v2, can be configured)
	maxSize := int64(10 * 1024 * 1024)
	if int64(len(data.FileData)) > maxSize {
		h.logger.Error("file size exceeds limit",
			zap.Int("file_size", len(data.FileData)),
			zap.Int64("max_size", maxSize))
		ctx.Status(http.StatusUnprocessableEntity)
		return nil, ctx.JSON(map[string]string{
			"error": fmt.Sprintf("file size exceeds %dMB limit", maxSize/1024/1024),
		})
	}

	// Validate MIME type
	if !isAllowedMimeTypeLift(data.MimeType) {
		h.logger.Error("unsupported file type", zap.String("mime_type", data.MimeType))
		ctx.Status(http.StatusUnprocessableEntity)
		return nil, ctx.JSON(map[string]string{
			"error": fmt.Sprintf("unsupported file type: %s", data.MimeType),
		})
	}

	return data, nil
}

// uploadToS3 uploads files to S3 and returns identifiers
func (h *Handler) uploadToS3(ctx *lift.Context, username string, data *MediaUploadData) (string, string, string, string, error) {
	mediaID := uuid.New().String()
	jobID := uuid.New().String()

	// Generate S3 key for original file
	ext := getExtensionFromMimeTypeLift(data.MimeType)
	s3Key := fmt.Sprintf("media/%s/%s/original%s", username, mediaID, ext)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx.Context)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return "", "", "", "", ctx.JSON(map[string]string{
			"error": "failed to initialize S3 client",
		})
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload original file
	if err := h.uploadFileToS3(ctx, s3Client, s3Key, data.FileData, data.MimeType); err != nil {
		return "", "", "", "", err
	}

	// Upload thumbnail if provided
	var thumbnailS3Key string
	if len(data.ThumbnailData) > 0 {
		thumbnailS3Key = fmt.Sprintf("media/%s/%s/thumbnail%s", username, mediaID, ext)
		if err := h.uploadFileToS3(ctx, s3Client, thumbnailS3Key, data.ThumbnailData, data.MimeType); err != nil {
			h.logger.Warn("failed to upload thumbnail", zap.Error(err))
		}
	}

	return mediaID, jobID, s3Key, thumbnailS3Key, nil
}

// uploadFileToS3 uploads a single file to S3
func (h *Handler) uploadFileToS3(ctx *lift.Context, s3Client *s3.Client, s3Key string, fileData []byte, mimeType string) error {
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		h.logger.Error("S3 bucket not configured")
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "S3 bucket not configured",
		})
	}

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(bucketName),
		Key:          aws.String(s3Key),
		Body:         bytes.NewReader(fileData),
		ContentType:  aws.String(mimeType),
		ACL:          types.ObjectCannedACLPrivate,
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	_, err := s3Client.PutObject(ctx.Context, putInput)
	if err != nil {
		h.logger.Error("failed to upload to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to upload media",
		})
	}

	return nil
}

// createMediaRecordsAndTriggerProcessing creates database records and triggers async processing
func (h *Handler) createMediaRecordsAndTriggerProcessing(ctx *lift.Context, username, mediaID, jobID, s3Key, thumbnailS3Key string, data *MediaUploadData) (*models.MediaAttachment, error) {
	now := time.Now()

	// Create media record
	if err := h.createMediaRecord(ctx, mediaID, username, s3Key, data, jobID, now); err != nil {
		return nil, err
	}

	// Create processing job
	if err := h.createProcessingJob(ctx, jobID, mediaID, username, s3Key, thumbnailS3Key, data.MimeType, now); err != nil {
		h.logger.Error("failed to create processing job", zap.Error(err))
		// Don't fail the request, media is uploaded
	}

	// Trigger media processor
	if err := h.triggerMediaProcessorLift(ctx, jobID, mediaID, s3Key, data.MimeType); err != nil {
		h.logger.Error("failed to trigger media processor", zap.Error(err))
		// Don't fail the request, processing can be retried later
	}

	return h.buildMediaAttachmentResponse(mediaID, data), nil
}

// createMediaRecord creates the media metadata record
func (h *Handler) createMediaRecord(ctx *lift.Context, mediaID, username, s3Key string, data *MediaUploadData, jobID string, now time.Time) error {
	mediaRecord := map[string]any{
		"PK":          fmt.Sprintf("MEDIA#%s", mediaID),
		"SK":          "METADATA",
		"id":          fmt.Sprintf("MEDIA#%s", mediaID),
		"MediaID":     mediaID,
		"Username":    username,
		"S3Key":       s3Key,
		"MimeType":    data.MimeType,
		"Size":        len(data.FileData),
		"Description": data.Description,
		"Focus":       data.Focus,
		"Processing":  true,
		"JobID":       jobID,
		"CreatedAt":   now,
		"UpdatedAt":   now,
	}

	if err := h.repos.Object().CreateObject(ctx.Context, mediaRecord); err != nil {
		h.logger.Error("failed to store media metadata", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to create media record",
		})
	}

	return nil
}

// createProcessingJob creates the async processing job
func (h *Handler) createProcessingJob(ctx *lift.Context, jobID, mediaID, username, s3Key, thumbnailS3Key, mimeType string, now time.Time) error {
	processingTasks := h.determineProcessingTasks(mimeType)

	jobRecord := map[string]any{
		"PK":              fmt.Sprintf("JOB#%s", jobID),
		"SK":              fmt.Sprintf("JOB#%s", jobID),
		"id":              fmt.Sprintf("JOB#%s", jobID),
		"GSI1PK":          fmt.Sprintf("USER#%s", username),
		"GSI1SK":          fmt.Sprintf("CREATED#%s", now.Format(time.RFC3339)),
		"GSI2PK":          "STATUS#pending",
		"GSI2SK":          fmt.Sprintf("CREATED#%s", now.Format(time.RFC3339)),
		"JobID":           jobID,
		"MediaID":         mediaID,
		"Username":        username,
		"S3Key":           s3Key,
		"ThumbnailS3Key":  thumbnailS3Key,
		"MimeType":        mimeType,
		"Status":          "pending",
		"RetryCount":      0,
		"ProcessingTasks": processingTasks,
		"Results":         map[string]any{},
		"CreatedAt":       now,
		"UpdatedAt":       now,
		"TTL":             now.Add(7 * 24 * time.Hour).Unix(),
	}

	return h.repos.Object().CreateObject(ctx.Context, jobRecord)
}

// determineProcessingTasks determines what processing tasks are needed based on media type
func (h *Handler) determineProcessingTasks(mimeType string) []string {
	mediaType := getMediaTypeLift(mimeType)
	switch mediaType {
	case "image", "gifv":
		return []string{"resize", "blurhash", "dimensions", "exif"}
	case "video":
		return []string{"thumbnail", "dimensions", "duration", "transcode"}
	case "audio":
		return []string{"waveform", "duration", "metadata"}
	default:
		return []string{}
	}
}

// buildMediaAttachmentResponse builds the response MediaAttachment object
func (h *Handler) buildMediaAttachmentResponse(mediaID string, data *MediaUploadData) *models.MediaAttachment {
	// Parse focus if provided
	var focusX, focusY float64
	if data.Focus != "" {
		if _, err := fmt.Sscanf(data.Focus, "%f,%f", &focusX, &focusY); err != nil {
			// Invalid focus format, leave as zero values
			h.logger.Debug("invalid focus format", zap.String("focus", data.Focus), zap.Error(err))
		}
	}

	mediaType := getMediaTypeLift(data.MimeType)

	return &models.MediaAttachment{
		ID:         mediaID,
		Type:       mediaType,
		URL:        "", // Empty until processed
		PreviewURL: "", // Empty until processed
		RemoteURL:  nil,
		TextURL:    "", // Will be populated after processing
		Meta: map[string]any{
			"processing": true,
			"focus": map[string]any{
				"x": focusX,
				"y": focusY,
			},
		},
		Description: data.Description,
		Blurhash:    "", // Will be generated during processing
	}
}

// HandleGetMediaV2Lift handles GET /api/v2/media/:id
func (h *Handler) HandleGetMediaV2Lift(ctx *lift.Context) error {
	// Extract media ID
	mediaID := ctx.Param("id")
	if mediaID == "" {
		h.logger.Error("missing media ID")
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "missing media ID",
		})
	}

	// Get media metadata from DynamoDB
	obj, err := h.repos.Object().GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		h.logger.Error("media not found", zap.String("media_id", mediaID), zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": fmt.Sprintf("media not found: %s", mediaID),
		})
	}

	// Convert to MediaAttachment
	mediaData, ok := obj.(map[string]any)
	if !ok {
		h.logger.Error("invalid media data", zap.String("media_id", mediaID))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "invalid media data",
		})
	}

	// Check if media is still processing (for v2 uploads)
	processing, _ := mediaData["Processing"].(bool)
	if processing {
		// Get processing progress
		isProcessing, progress, _ := h.GetMediaProcessingStatusLift(ctx, mediaID)
		attachment := &models.MediaAttachment{
			ID:         mediaID,
			Type:       getMediaTypeLift(getStringFromMediaDataLift(mediaData, "MimeType")),
			URL:        "", // Empty while processing
			PreviewURL: "", // Empty while processing
			RemoteURL:  nil,
			TextURL:    "",
			Meta: map[string]any{
				"processing": isProcessing,
				"progress":   progress,
			},
			Description: getStringFromMediaDataLift(mediaData, "Description"),
			Blurhash:    "",
		}

		// Add focus if present
		if focus := getStringFromMediaDataLift(mediaData, "Focus"); focus != "" {
			var focusX, focusY float64
			if n, err := fmt.Sscanf(focus, "%f,%f", &focusX, &focusY); err == nil && n == 2 {
				attachment.Meta["focus"] = map[string]any{
					"x": focusX,
					"y": focusY,
				}
			}
		}

		ctx.Status(http.StatusOK)
		return ctx.JSON(attachment)
	}

	// Media processing is complete, return full attachment
	url := getStringFromMediaDataLift(mediaData, "URL")
	previewURL := getStringFromMediaDataLift(mediaData, "PreviewURL", url)

	attachment := &models.MediaAttachment{
		ID:         mediaID,
		Type:       getMediaTypeLift(getStringFromMediaDataLift(mediaData, "MimeType")),
		URL:        url,
		PreviewURL: previewURL,
		RemoteURL:  nil,
		TextURL:    url,
		Meta: map[string]any{
			"original": map[string]any{
				"width":  getIntFromMediaDataLift(mediaData, "Width"),
				"height": getIntFromMediaDataLift(mediaData, "Height"),
				"size":   fmt.Sprintf("%dx%d", getIntFromMediaDataLift(mediaData, "Width"), getIntFromMediaDataLift(mediaData, "Height")),
				"aspect": calculateAspectRatioLift(getIntFromMediaDataLift(mediaData, "Width"), getIntFromMediaDataLift(mediaData, "Height")),
			},
		},
		Description: getStringFromMediaDataLift(mediaData, "Description"),
		Blurhash:    getStringFromMediaDataLift(mediaData, "Blurhash"),
	}

	// Add focus if present
	if focus := getStringFromMediaDataLift(mediaData, "Focus"); focus != "" {
		var focusX, focusY float64
		if n, err := fmt.Sscanf(focus, "%f,%f", &focusX, &focusY); err == nil && n == 2 {
			attachment.Meta["focus"] = map[string]any{
				"x": focusX,
				"y": focusY,
			}
		}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(attachment)
}

// HandleUpdateMediaV2Lift handles PUT /api/v2/media/:id
func (h *Handler) HandleUpdateMediaV2Lift(ctx *lift.Context) error {
	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	if testUsername != "" {
		// Test mode - create mock claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   []string{auth.ScopeWrite},
		}
	} else {
		// Extract token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required",
			})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		var err error
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}
	}

	// Extract media ID
	mediaID := ctx.Param("id")
	if mediaID == "" {
		h.logger.Error("missing media ID")
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "missing media ID",
		})
	}

	// Parse request body
	var updateReq struct {
		Description string `json:"description"`
		Focus       string `json:"focus"`
	}

	// Try ctx.ParseRequest first, then fallback to common.ParseRequestBody for test mode
	if err := ctx.ParseRequest(&updateReq); err != nil {
		// Fallback for test mode
		if err := common.ParseRequestBody(ctx.Request.Body, &updateReq); err != nil {
			h.logger.Error("failed to parse request body", zap.Error(err))
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": err.Error(),
			})
		}
	}

	// Get existing media
	obj, err := h.repos.Object().GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		h.logger.Error("media not found", zap.String("media_id", mediaID), zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": fmt.Sprintf("media not found: %s", mediaID),
		})
	}

	// Verify ownership
	mediaData, ok := obj.(map[string]any)
	if !ok {
		h.logger.Error("invalid media data", zap.String("media_id", mediaID))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "invalid media data",
		})
	}

	if mediaData["Username"] != claims.Username {
		h.logger.Error("not authorized to update media",
			zap.String("media_id", mediaID),
			zap.String("owner", fmt.Sprintf("%v", mediaData["Username"])),
			zap.String("user", claims.Username))
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "not authorized to update this media",
		})
	}

	// Update the media metadata
	mediaData["Description"] = updateReq.Description
	mediaData["Focus"] = updateReq.Focus
	mediaData["UpdatedAt"] = time.Now()

	if err := h.repos.Object().UpdateObject(ctx.Context, mediaData); err != nil {
		h.logger.Error("failed to update media", zap.String("media_id", mediaID), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": fmt.Sprintf("failed to update media: %v", err),
		})
	}

	// Build response
	url := getStringFromMediaDataLift(mediaData, "URL")
	attachment := &models.MediaAttachment{
		ID:         mediaID,
		Type:       getMediaTypeLift(getStringFromMediaDataLift(mediaData, "MimeType")),
		URL:        url,
		PreviewURL: url,
		RemoteURL:  nil,
		TextURL:    url,
		Meta: map[string]any{
			"original": map[string]any{
				"width":  0,
				"height": 0,
				"size":   "0x0",
				"aspect": 1.0,
			},
		},
		Description: updateReq.Description,
		Blurhash:    "",
	}

	// Parse focus if provided
	if updateReq.Focus != "" {
		var focusX, focusY float64
		if n, err := fmt.Sscanf(updateReq.Focus, "%f,%f", &focusX, &focusY); err == nil && n == 2 {
			attachment.Meta["focus"] = map[string]any{
				"x": focusX,
				"y": focusY,
			}
		}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(attachment)
}

// triggerMediaProcessorLift sends a message to SQS to trigger media processing
func (h *Handler) triggerMediaProcessorLift(ctx *lift.Context, jobID, mediaID, s3Key, mimeType string) error {
	// Initialize SQS client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx.Context)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	sqsClient := sqs.NewFromConfig(awsCfg)

	// Create message payload
	message := map[string]any{
		"jobID":     jobID,
		"mediaID":   mediaID,
		"s3Key":     s3Key,
		"bucket":    h.cfg.S3BucketName,
		"mimeType":  mimeType,
		"timestamp": time.Now().Unix(),
	}

	messageBody, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Get SQS queue URL from environment or config
	queueURL := h.cfg.MediaProcessorQueueURL
	if queueURL == "" {
		// Default queue name pattern
		queueURL = fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/media-processor-queue",
			awsCfg.Region, h.cfg.AWSAccountID)
	}

	// Send message to SQS
	_, err = sqsClient.SendMessage(ctx.Context, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
			"JobType": {
				DataType:    aws.String("String"),
				StringValue: aws.String("media-processing"),
			},
			"MediaType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(getMediaTypeLift(mimeType)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send SQS message: %w", err)
	}

	h.logger.Info("media processing job queued",
		zap.String("job_id", jobID),
		zap.String("media_id", mediaID),
		zap.String("queue_url", queueURL))

	return nil
}

// Helper functions

func isAllowedMimeTypeLift(mimeType string) bool {
	allowed := []string{
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"video/mp4",
		"video/webm",
		"audio/mpeg",
		"audio/mp3",
		"audio/ogg",
		"audio/wav",
	}

	for _, t := range allowed {
		if t == mimeType {
			return true
		}
	}
	return false
}

func getExtensionFromMimeTypeLift(mimeType string) string {
	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
		"video/mp4":  ".mp4",
		"video/webm": ".webm",
		"audio/mpeg": ".mp3",
		"audio/mp3":  ".mp3",
		"audio/ogg":  ".ogg",
		"audio/wav":  ".wav",
	}

	if ext, ok := extensions[mimeType]; ok {
		return ext
	}
	return ".bin"
}

func getMediaTypeLift(mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		if mimeType == "image/gif" {
			return "gifv"
		}
		return "image"
	} else if strings.HasPrefix(mimeType, "video/") {
		return "video"
	} else if strings.HasPrefix(mimeType, "audio/") {
		return "audio"
	}
	return MediaTypeUnknown
}

// Helper functions for media data extraction
func getStringFromMediaDataLift(data map[string]any, key string, defaultValue ...string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func getIntFromMediaDataLift(data map[string]any, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	if val, ok := data[key].(int); ok {
		return val
	}
	return 0
}

func calculateAspectRatioLift(width, height int) float64 {
	if height == 0 {
		return 1.0
	}
	return float64(width) / float64(height)
}

// GetMediaProcessingStatusLift checks if a media item is still processing
func (h *Handler) GetMediaProcessingStatusLift(ctx *lift.Context, mediaID string) (bool, int, error) {
	// Get media record
	obj, err := h.repos.Object().GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		return false, 0, err
	}

	mediaData, ok := obj.(map[string]any)
	if !ok {
		return false, 0, errors.New("invalid media data")
	}

	// Check if still processing
	processing, _ := mediaData["Processing"].(bool)
	if !processing {
		return false, 100, nil
	}

	// Get job ID and check job status
	jobID, ok := mediaData["JobID"].(string)
	if !ok {
		return true, 0, nil
	}

	// Get job record
	jobObj, err := h.repos.Object().GetObject(ctx.Context, fmt.Sprintf("JOB#%s", jobID))
	if err != nil {
		// Job not found, assume still processing
		return true, 0, nil
	}

	jobData, ok := jobObj.(map[string]any)
	if !ok {
		return true, 0, nil
	}

	// Calculate progress based on completed tasks
	tasks, _ := jobData["ProcessingTasks"].([]string)
	results, _ := jobData["Results"].(map[string]any)

	if len(tasks) == 0 {
		return true, 0, nil
	}

	completedTasks := len(results)
	progress := (completedTasks * 100) / len(tasks)

	status, _ := jobData["Status"].(string)
	if status == "completed" || status == "failed" {
		return false, 100, nil
	}

	return true, progress, nil
}
