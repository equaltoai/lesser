package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
		defer part.Close()

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

	// Build response
	url := mediaData["URL"].(string)
	description := ""
	if desc, ok := mediaData["Description"].(string); ok {
		description = desc
	}

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
		Description: description,
		Blurhash:    "",
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
	if err := json.Unmarshal([]byte(request.Body), &updateReq); err != nil {
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
