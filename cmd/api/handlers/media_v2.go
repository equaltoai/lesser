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
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// HandleMediaUploadV2 handles POST /api/v2/media - Async media upload
func (h *Handler) HandleMediaUploadV2(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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
		return common.BadRequest(errors.New("no file data provided")), nil
	}

	// Validate file size (10MB limit for v2, can be configured)
	maxSize := int64(10 * 1024 * 1024)
	if int64(len(fileData)) > maxSize {
		return common.UnprocessableEntity(fmt.Errorf("file size exceeds %dMB limit", maxSize/1024/1024)), nil
	}

	// Validate MIME type
	if !isAllowedMimeType(mimeType) {
		return common.UnprocessableEntity(fmt.Errorf("unsupported file type: %s", mimeType)), nil
	}

	// Generate unique ID for the media
	mediaID := uuid.New().String()
	jobID := uuid.New().String()

	// Generate S3 key for original file
	ext := getExtensionFromMimeType(mimeType)
	s3Key := fmt.Sprintf("media/%s/%s/original%s", claims.Username, mediaID, ext)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		return common.InternalServerError(errors.New("failed to initialize S3 client")), nil
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload original file to S3
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		return common.InternalServerError(errors.New("S3 bucket not configured")), nil
	}

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(bucketName),
		Key:          aws.String(s3Key),
		Body:         bytes.NewReader(fileData),
		ContentType:  aws.String(mimeType),
		ACL:          types.ObjectCannedACLPrivate,
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
		_, err = s3Client.PutObject(ctx, thumbnailInput)
		if err != nil {
			h.logger.Warn("failed to upload thumbnail", zap.Error(err))
		}
	}

	// Create media record
	now := time.Now()
	mediaRecord := map[string]interface{}{
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

	if err := h.store.CreateObject(ctx, mediaRecord); err != nil {
		h.logger.Error("failed to store media metadata", zap.Error(err))
		return common.InternalServerError(errors.New("failed to create media record")), nil
	}

	// Create processing job
	processingTasks := []string{}
	mediaType := getMediaType(mimeType)

	switch mediaType {
	case "image", "gifv":
		processingTasks = append(processingTasks, "resize", "blurhash", "dimensions", "exif")
	case "video":
		processingTasks = append(processingTasks, "thumbnail", "dimensions", "duration", "transcode")
	case "audio":
		processingTasks = append(processingTasks, "waveform", "duration", "metadata")
	}

	jobRecord := map[string]interface{}{
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
		"Results":         map[string]interface{}{},
		"CreatedAt":       now,
		"UpdatedAt":       now,
		"TTL":             now.Add(7 * 24 * time.Hour).Unix(), // 7 days TTL
	}

	if err := h.store.CreateObject(ctx, jobRecord); err != nil {
		h.logger.Error("failed to create processing job", zap.Error(err))
		// Don't fail the request, media is uploaded
	}

	// TODO: Trigger media processor Lambda
	// This would normally be done via SQS or EventBridge

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
		Meta: map[string]interface{}{
			"processing": true,
			"focus": map[string]interface{}{
				"x": focusX,
				"y": focusY,
			},
		},
		Description: description,
		Blurhash:    "", // Will be generated during processing
	}

	return common.Accepted(attachment), nil
}
