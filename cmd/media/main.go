package main

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

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

var (
	cfg      *config.Config
	store    storage.Storage
	logger   *zap.Logger
	s3Client *s3.Client
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}
	s3Client = s3.NewFromConfig(awsCfg)
}

// MediaUploadResponse represents the response after successful media upload
type MediaUploadResponse struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // image, video, gifv, audio, unknown
	URL         string                 `json:"url"`
	PreviewURL  string                 `json:"preview_url"`
	RemoteURL   *string                `json:"remote_url,omitempty"`
	TextURL     string                 `json:"text_url"`
	Meta        map[string]interface{} `json:"meta"`
	Description string                 `json:"description,omitempty"`
	Blurhash    *string                `json:"blurhash,omitempty"`
}

func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(cfg.JWTSecret, store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse multipart form data
	// Lambda provides base64 encoded body for binary content
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
				logger.Warn("failed to read description", zap.Error(err))
			}
			description = buf.String()
		}
	}

	if len(fileData) == 0 {
		return common.BadRequest(errors.New("no file data provided")), nil
	}

	// Validate file size (10MB limit)
	if len(fileData) > 10*1024*1024 {
		return common.UnprocessableEntity(errors.New("file size exceeds 10MB limit")), nil
	}

	// Validate MIME type
	if !isAllowedMimeType(mimeType) {
		return common.UnprocessableEntity(fmt.Errorf("unsupported file type: %s", mimeType)), nil
	}

	// Generate unique filename
	mediaID := fmt.Sprintf("%s-%d-%s", claims.Username, time.Now().Unix(), generateRandomString(8))
	ext := getExtensionFromMimeType(mimeType)
	s3Key := fmt.Sprintf("media/%s/%s%s", claims.Username, mediaID, ext)

	// Upload to S3
	bucketName := cfg.S3BucketName
	if bucketName == "" {
		bucketName = fmt.Sprintf("lesser-media-%s", strings.ReplaceAll(cfg.Domain, ".", "-"))
	}

	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(fileData),
		ContentType: aws.String(mimeType),
		// Make publicly readable
		ACL: types.ObjectCannedACLPublicRead,
		// Add cache control
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	_, err = s3Client.PutObject(ctx, putInput)
	if err != nil {
		logger.Error("failed to upload to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to upload media")), nil
	}

	// Build media URL
	mediaURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, s3Key)
	// Check if we have a CloudFront domain or custom CDN
	cdnDomain := os.Getenv("CDN_DOMAIN")
	if cdnDomain != "" {
		mediaURL = fmt.Sprintf("https://%s/%s", cdnDomain, s3Key)
	}

	// Determine media type
	mediaType := getMediaType(mimeType)

	// Build response
	resp := MediaUploadResponse{
		ID:         mediaID,
		Type:       mediaType,
		URL:        mediaURL,
		PreviewURL: mediaURL, // TODO: Generate thumbnails for videos
		TextURL:    mediaURL,
		Meta: map[string]interface{}{
			"original": map[string]interface{}{
				"size": fmt.Sprintf("%dx%d", 0, 0), // TODO: Get actual dimensions
			},
		},
		Description: description,
	}

	// Store media metadata in DynamoDB for future reference
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
		"CreatedAt":   time.Now().Format(time.RFC3339),
	}

	// Store in DynamoDB (we'll use a generic CreateObject for now)
	if err := store.CreateObject(ctx, mediaRecord); err != nil {
		logger.Warn("failed to store media metadata", zap.Error(err))
		// Don't fail the request, media is already uploaded
	}

	respBody, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusCreated,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(respBody),
	}, nil
}

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

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func main() {
	lambda.Start(handler)
}
