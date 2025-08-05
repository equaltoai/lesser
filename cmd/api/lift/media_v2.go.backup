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

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleUploadMediaV2Lift handles POST /api/v2/media - Async media upload
func (h *Handler) HandleUploadMediaV2Lift(ctx *lift.Context) error {
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

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "insufficient scope",
		})
	}

	// Parse multipart form data
	var bodyBytes []byte
	
	// Get raw body data
	bodyBytes = ctx.Request.Body

	// Handle potential base64 encoding (legacy support)
	if len(bodyBytes) > 0 {
		// Try to decode as base64 if it looks like base64
		if decoded, err := base64.StdEncoding.DecodeString(string(bodyBytes)); err == nil {
			// Check if the decoded data looks like multipart form data
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
		return ctx.JSON(map[string]string{
			"error": fmt.Sprintf("invalid content type: %v", err),
		})
	}

	boundary := params["boundary"]
	if boundary == "" {
		h.logger.Error("missing boundary in content type")
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "missing boundary in content type",
		})
	}

	// Create multipart reader
	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)

	// Read the file part
	var fileData []byte
	var thumbnailData []byte
	var mimeType string
	var description string
	var focus string

	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		defer func() {
			if err := part.Close(); err != nil {
				// Log error but don't fail the request
				h.logger.Warn("failed to close multipart", zap.Error(err))
			}
		}()

		switch part.FormName() {
		case "file":
			// Read file data
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(part); err != nil {
				h.logger.Error("failed to read file", zap.Error(err))
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]string{
					"error": fmt.Sprintf("failed to read file: %v", err),
				})
			}
			fileData = buf.Bytes()

			// Get MIME type from header or detect it
			mimeType = part.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = http.DetectContentType(fileData)
			}

		case "thumbnail":
			// Read thumbnail data (optional)
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(part); err != nil {
				h.logger.Warn("failed to read thumbnail", zap.Error(err))
			}
			thumbnailData = buf.Bytes()

		case "description":
			// Read description
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(part); err != nil {
				h.logger.Warn("failed to read description", zap.Error(err))
			}
			description = buf.String()

		case "focus":
			// Read focus point
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(part); err != nil {
				h.logger.Warn("failed to read focus", zap.Error(err))
			}
			focus = buf.String()
		}
	}

	if len(fileData) == 0 {
		h.logger.Error("no file data provided")
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "no file data provided",
		})
	}

	// Validate file size (10MB limit for v2, can be configured)
	maxSize := int64(10 * 1024 * 1024)
	if int64(len(fileData)) > maxSize {
		h.logger.Error("file size exceeds limit", 
			zap.Int("file_size", len(fileData)),
			zap.Int64("max_size", maxSize))
		ctx.Status(http.StatusUnprocessableEntity)
		return ctx.JSON(map[string]string{
			"error": fmt.Sprintf("file size exceeds %dMB limit", maxSize/1024/1024),
		})
	}

	// Validate MIME type
	if !isAllowedMimeTypeLift(mimeType) {
		h.logger.Error("unsupported file type", zap.String("mime_type", mimeType))
		ctx.Status(http.StatusUnprocessableEntity)
		return ctx.JSON(map[string]string{
			"error": fmt.Sprintf("unsupported file type: %s", mimeType),
		})
	}

	// Generate unique ID for the media
	mediaID := uuid.New().String()
	jobID := uuid.New().String()

	// Generate S3 key for original file
	ext := getExtensionFromMimeTypeLift(mimeType)
	s3Key := fmt.Sprintf("media/%s/%s/original%s", claims.Username, mediaID, ext)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx.Context)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to initialize S3 client",
		})
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload original file to S3
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

	_, err = s3Client.PutObject(ctx.Context, putInput)
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

	// Upload thumbnail if provided
	var thumbnailS3Key string
	if len(thumbnailData) > 0 {
		thumbnailS3Key = fmt.Sprintf("media/%s/%s/thumbnail%s", claims.Username, mediaID, ext)
		thumbnailInput := &s3.PutObjectInput{
			Bucket:       aws.String(bucketName),
			Key:          aws.String(thumbnailS3Key),
			Body:         bytes.NewReader(thumbnailData),
			ContentType:  aws.String(mimeType),
			ACL:          types.ObjectCannedACLPrivate,
			CacheControl: aws.String("public, max-age=31536000, immutable"),
		}
		_, err = s3Client.PutObject(ctx.Context, thumbnailInput)
		if err != nil {
			h.logger.Warn("failed to upload thumbnail", zap.Error(err))
		}
	}

	// Create media record
	now := time.Now()
	mediaRecord := map[string]any{
		"PK":          fmt.Sprintf("MEDIA#%s", mediaID),
		"SK":          "METADATA",
		"id":          fmt.Sprintf("MEDIA#%s", mediaID),
		"MediaID":     mediaID,
		"Username":    claims.Username,
		"S3Key":       s3Key,
		"MimeType":    mimeType,
		"Size":        len(fileData),
		"Description": description,
		"Focus":       focus,
		"Processing":  true, // Mark as processing
		"JobID":       jobID,
		"CreatedAt":   now,
		"UpdatedAt":   now,
	}

	if err := h.store.CreateObject(ctx.Context, mediaRecord); err != nil {
		h.logger.Error("failed to store media metadata", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to create media record",
		})
	}

	// Create processing job
	processingTasks := []string{}
	mediaType := getMediaTypeLift(mimeType)

	switch mediaType {
	case "image", "gifv":
		processingTasks = append(processingTasks, "resize", "blurhash", "dimensions", "exif")
	case "video":
		processingTasks = append(processingTasks, "thumbnail", "dimensions", "duration", "transcode")
	case "audio":
		processingTasks = append(processingTasks, "waveform", "duration", "metadata")
	}

	jobRecord := map[string]any{
		"PK":              fmt.Sprintf("JOB#%s", jobID),
		"SK":              fmt.Sprintf("JOB#%s", jobID),
		"id":              fmt.Sprintf("JOB#%s", jobID),
		"GSI1PK":          fmt.Sprintf("USER#%s", claims.Username),
		"GSI1SK":          fmt.Sprintf("CREATED#%s", now.Format(time.RFC3339)),
		"GSI2PK":          "STATUS#pending",
		"GSI2SK":          fmt.Sprintf("CREATED#%s", now.Format(time.RFC3339)),
		"JobID":           jobID,
		"MediaID":         mediaID,
		"Username":        claims.Username,
		"S3Key":           s3Key,
		"ThumbnailS3Key":  thumbnailS3Key,
		"MimeType":        mimeType,
		"Status":          "pending",
		"RetryCount":      0,
		"ProcessingTasks": processingTasks,
		"Results":         map[string]any{},
		"CreatedAt":       now,
		"UpdatedAt":       now,
		"TTL":             now.Add(7 * 24 * time.Hour).Unix(), // 7 days TTL
	}

	if err := h.store.CreateObject(ctx.Context, jobRecord); err != nil {
		h.logger.Error("failed to create processing job", zap.Error(err))
		// Don't fail the request, media is uploaded
	}

	// Trigger media processor Lambda via SQS
	if err := h.triggerMediaProcessorLift(ctx, jobID, mediaID, s3Key, mimeType); err != nil {
		h.logger.Error("failed to trigger media processor", zap.Error(err))
		// Don't fail the request, processing can be retried later
	}

	// Parse focus if provided
	var focusX, focusY float64
	if focus != "" {
		fmt.Sscanf(focus, "%f,%f", &focusX, &focusY)
	}

	// Create MediaAttachment response (processing version)
	attachment := &models.MediaAttachment{
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
		Description: description,
		Blurhash:    "", // Will be generated during processing
	}

	ctx.Status(http.StatusAccepted)
	return ctx.JSON(attachment)
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
	obj, err := h.store.GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
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
	obj, err := h.store.GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
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

	if err := h.store.UpdateObject(ctx.Context, mediaData); err != nil {
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
	return "unknown"
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
	obj, err := h.store.GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
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
	jobObj, err := h.store.GetObject(ctx.Context, fmt.Sprintf("JOB#%s", jobID))
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