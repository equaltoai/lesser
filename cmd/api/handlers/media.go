package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

// HandleMediaUpload handles POST /api/v1/media
func (h *Handler) HandleMediaUpload(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse multipart form data
	var bodyBytes []byte
	if request.IsBase64Encoded {
		bodyBytes, err = base64.StdEncoding.DecodeString(request.Body)
		if err != nil {
			return common.BadRequest(fmt.Errorf("failed to decode base64 body: %w", err)), nil
		}
	} else {
		bodyBytes = []byte(request.Body)
	}

	// Get content type header
	contentType := request.Headers["Content-Type"]
	if contentType == "" {
		contentType = request.Headers["content-type"]
	}

	// Parse boundary from content type
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return common.BadRequest(fmt.Errorf("invalid content type: %w", err)), nil
	}

	boundary := params["boundary"]
	if boundary == "" {
		return common.BadRequest(errors.New("missing boundary in content type")), nil
	}

	// Create multipart reader
	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)

	// Read the file part
	var fileData []byte
	var mimeType string
	var description string
	var focus string // x,y coordinates for focal point

	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		defer func() {
			if err := part.Close(); err != nil {
				// Log error but don't fail the request
			}
		}()

		switch part.FormName() {
		case "file":
			// Read file data
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(part); err != nil {
				return common.BadRequest(fmt.Errorf("failed to read file: %w", err)), nil
			}
			fileData = buf.Bytes()

			// Get MIME type from header or detect it
			mimeType = part.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = http.DetectContentType(fileData)
			}

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
		return common.BadRequest(errors.New("no file data provided")), nil
	}

	// Validate file size (10MB limit for now, can be configured)
	maxSize := int64(10 * 1024 * 1024)
	if int64(len(fileData)) > maxSize {
		return common.UnprocessableEntity(fmt.Errorf("file size exceeds %dMB limit", maxSize/1024/1024)), nil
	}

	// Validate MIME type
	if !isAllowedMimeType(mimeType) {
		return common.UnprocessableEntity(fmt.Errorf("unsupported file type: %s", mimeType)), nil
	}

	// Generate unique ID for the media
	mediaID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Generate S3 key
	ext := getExtensionFromMimeType(mimeType)
	s3Key := fmt.Sprintf("media/%s/%s%s", claims.Username, mediaID, ext)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		return common.InternalServerError(errors.New("failed to initialize S3 client")), nil
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload to S3
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		return common.InternalServerError(errors.New("S3 bucket not configured")), nil
	}

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(bucketName),
		Key:          aws.String(s3Key),
		Body:         bytes.NewReader(fileData),
		ContentType:  aws.String(mimeType),
		ACL:          types.ObjectCannedACLPrivate, // Use CloudFront for access
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	_, err = s3Client.PutObject(ctx, putInput)
	if err != nil {
		h.logger.Error("failed to upload to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		return common.InternalServerError(errors.New("failed to upload media")), nil
	}

	// Build media URL (using CDN if configured)
	cdnDomain := os.Getenv("CDN_DOMAIN")
	var mediaURL string
	if cdnDomain != "" {
		mediaURL = fmt.Sprintf("https://%s/%s", cdnDomain, s3Key)
	} else {
		// Fallback to S3 URL if no CDN
		mediaURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, s3Key)
	}

	// Parse focus if provided
	var focusX, focusY float64
	if focus != "" {
		fmt.Sscanf(focus, "%f,%f", &focusX, &focusY)
	}

	// Create MediaAttachment response
	attachment := &models.MediaAttachment{
		ID:         mediaID,
		Type:       getMediaType(mimeType),
		URL:        mediaURL,
		PreviewURL: mediaURL, // TODO: Generate thumbnails
		RemoteURL:  nil,
		TextURL:    mediaURL,
		Meta: map[string]interface{}{
			"original": map[string]interface{}{
				"width":  0, // TODO: Get actual dimensions
				"height": 0,
				"size":   "0x0",
				"aspect": 1.0,
			},
			"focus": map[string]interface{}{
				"x": focusX,
				"y": focusY,
			},
		},
		Description: description,
		Blurhash:    "", // TODO: Generate blurhash
	}

	// Store media metadata in DynamoDB
	mediaRecord := map[string]interface{}{
		"PK":          fmt.Sprintf("MEDIA#%s", mediaID),
		"SK":          "METADATA",
		"id":          fmt.Sprintf("MEDIA#%s", mediaID), // CreateObject expects an 'id' field
		"MediaID":     mediaID,
		"Username":    claims.Username,
		"URL":         mediaURL,
		"S3Key":       s3Key,
		"MimeType":    mimeType,
		"Size":        len(fileData),
		"Description": description,
		"Focus":       focus,
		"CreatedAt":   time.Now(),
	}

	if err := h.store.CreateObject(ctx, mediaRecord); err != nil {
		h.logger.Warn("failed to store media metadata", zap.Error(err))
		// Don't fail the request, media is already uploaded
	}

	return common.OK(attachment), nil
}

// HandleGetMedia handles GET /api/v1/media/:id
func (h *Handler) HandleGetMedia(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract media ID
	mediaID := request.PathParameters["id"]
	if mediaID == "" {
		return common.BadRequest(errors.New("missing media ID")), nil
	}

	// Get media metadata from DynamoDB
	obj, err := h.store.GetObject(ctx, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		return common.NotFound(fmt.Errorf("media not found: %s", mediaID)), nil
	}

	// Convert to MediaAttachment
	mediaData, ok := obj.(map[string]interface{})
	if !ok {
		return common.InternalServerError(errors.New("invalid media data")), nil
	}

	// Check if media is still processing (for v2 uploads)
	processing, _ := mediaData["Processing"].(bool)
	if processing {
		// Get processing progress
		isProcessing, progress, _ := h.GetMediaProcessingStatus(ctx, mediaID)

		attachment := &models.MediaAttachment{
			ID:         mediaID,
			Type:       getMediaType(mediaData["MimeType"].(string)),
			URL:        "", // Empty while processing
			PreviewURL: "", // Empty while processing
			RemoteURL:  nil,
			TextURL:    "",
			Meta: map[string]interface{}{
				"processing": isProcessing,
				"progress":   progress,
			},
			Description: getStringFromMediaData(mediaData, "Description"),
			Blurhash:    "",
		}

		// Add focus if present
		if focus := getStringFromMediaData(mediaData, "Focus"); focus != "" {
			var focusX, focusY float64
			fmt.Sscanf(focus, "%f,%f", &focusX, &focusY)
			attachment.Meta["focus"] = map[string]interface{}{
				"x": focusX,
				"y": focusY,
			}
		}

		return common.OK(attachment), nil
	}

	// Build response for processed media
	url := getStringFromMediaData(mediaData, "URL")
	attachment := &models.MediaAttachment{
		ID:         mediaID,
		Type:       getMediaType(mediaData["MimeType"].(string)),
		URL:        url,
		PreviewURL: getStringFromMediaData(mediaData, "PreviewURL", url), // Fallback to main URL
		RemoteURL:  nil,
		TextURL:    url,
		Meta: map[string]interface{}{
			"original": map[string]interface{}{
				"width":  getIntFromMediaData(mediaData, "Width"),
				"height": getIntFromMediaData(mediaData, "Height"),
				"size":   fmt.Sprintf("%dx%d", getIntFromMediaData(mediaData, "Width"), getIntFromMediaData(mediaData, "Height")),
				"aspect": calculateAspectRatio(getIntFromMediaData(mediaData, "Width"), getIntFromMediaData(mediaData, "Height")),
			},
		},
		Description: getStringFromMediaData(mediaData, "Description"),
		Blurhash:    getStringFromMediaData(mediaData, "Blurhash"),
	}

	// Add focus if present
	if focus := getStringFromMediaData(mediaData, "Focus"); focus != "" {
		var focusX, focusY float64
		fmt.Sscanf(focus, "%f,%f", &focusX, &focusY)
		attachment.Meta["focus"] = map[string]interface{}{
			"x": focusX,
			"y": focusY,
		}
	}

	// Add duration for video/audio
	if duration := getIntFromMediaData(mediaData, "Duration"); duration > 0 {
		attachment.Meta["duration"] = float64(duration) / 1000.0 // Convert ms to seconds
	}

	return common.OK(attachment), nil
}

// HandleUpdateMedia handles PUT /api/v1/media/:id
func (h *Handler) HandleUpdateMedia(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Extract media ID
	mediaID := request.PathParameters["id"]
	if mediaID == "" {
		return common.BadRequest(errors.New("missing media ID")), nil
	}

	// Parse request body
	var updateReq struct {
		Description string `json:"description"`
		Focus       string `json:"focus"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &updateReq); err != nil {
		return common.BadRequest(err), nil
	}

	// Get existing media
	obj, err := h.store.GetObject(ctx, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		return common.NotFound(fmt.Errorf("media not found: %s", mediaID)), nil
	}

	// Verify ownership
	mediaData, ok := obj.(map[string]interface{})
	if !ok {
		return common.InternalServerError(errors.New("invalid media data")), nil
	}

	if mediaData["Username"] != claims.Username {
		return common.Forbidden(errors.New("not authorized to update this media")), nil
	}

	// Update the media metadata
	mediaData["Description"] = updateReq.Description
	mediaData["Focus"] = updateReq.Focus
	mediaData["UpdatedAt"] = time.Now()

	if err := h.store.UpdateObject(ctx, mediaData); err != nil {
		return common.InternalServerError(fmt.Errorf("failed to update media: %w", err)), nil
	}

	// Build response
	url := mediaData["URL"].(string)
	attachment := &models.MediaAttachment{
		ID:         mediaID,
		Type:       getMediaType(mediaData["MimeType"].(string)),
		URL:        url,
		PreviewURL: url,
		RemoteURL:  nil,
		TextURL:    url,
		Meta: map[string]interface{}{
			"original": map[string]interface{}{
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
		fmt.Sscanf(updateReq.Focus, "%f,%f", &focusX, &focusY)
		attachment.Meta["focus"] = map[string]interface{}{
			"x": focusX,
			"y": focusY,
		}
	}

	return common.OK(attachment), nil
}

// Helper functions

func isAllowedMimeType(mimeType string) bool {
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

func getExtensionFromMimeType(mimeType string) string {
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

func getMediaType(mimeType string) string {
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
func getStringFromMediaData(data map[string]interface{}, key string, defaultValue ...string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func getIntFromMediaData(data map[string]interface{}, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	if val, ok := data[key].(int); ok {
		return val
	}
	return 0
}

func calculateAspectRatio(width, height int) float64 {
	if height == 0 {
		return 1.0
	}
	return float64(width) / float64(height)
}

// GetMediaProcessingStatus checks if a media item is still processing
func (h *Handler) GetMediaProcessingStatus(ctx context.Context, mediaID string) (bool, int, error) {
	// Get media record
	obj, err := h.store.GetObject(ctx, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		return false, 0, err
	}

	mediaData, ok := obj.(map[string]interface{})
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
	jobObj, err := h.store.GetObject(ctx, fmt.Sprintf("JOB#%s", jobID))
	if err != nil {
		// Job not found, assume still processing
		return true, 0, nil
	}

	jobData, ok := jobObj.(map[string]interface{})
	if !ok {
		return true, 0, nil
	}

	// Calculate progress based on completed tasks
	tasks, _ := jobData["ProcessingTasks"].([]string)
	results, _ := jobData["Results"].(map[string]interface{})

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
