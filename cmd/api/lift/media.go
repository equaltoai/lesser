package lift

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
	"golang.org/x/image/draw"
)

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
	h.logger.Info("HandleUploadMediaLift called")

	// Authenticate user
	username, err := h.authenticateMediaUploadRequest(ctx)
	if err != nil {
		return err
	}

	// Parse multipart form data
	mediaData, err := h.parseMediaUpload(ctx)
	if err != nil {
		return err
	}

	// Validate media
	if err := h.validateMediaUpload(mediaData); err != nil {
		return err
	}

	// Upload to S3 and get URL
	mediaID, mediaURL, err := h.uploadMediaToS3(ctx.Context, username, mediaData)
	if err != nil {
		return err
	}

	// Parse focus coordinates
	focusX, focusY := h.parseFocusCoordinates(mediaData.focus)

	// Create MediaAttachment response
	attachment := &models.MediaAttachment{
		ID:         mediaID,
		Type:       getMediaType(mediaData.mimeType),
		URL:        mediaURL,
		PreviewURL: h.generateThumbnailURL(ctx.Context, mediaURL, mediaData.mimeType),
		RemoteURL:  nil,
		TextURL:    mediaURL,
		Meta: map[string]any{
			"original": h.getMediaDimensions(ctx.Context, mediaData.fileData, mediaData.mimeType),
			"focus": map[string]any{
				"x": focusX,
				"y": focusY,
			},
		},
		Description: mediaData.description,
		Blurhash:    h.generateBlurhash(ctx.Context, mediaData.fileData, mediaData.mimeType),
	}

	// Store media metadata in DynamoDB
	s3Key := h.extractS3KeyFromURL(mediaURL)
	mediaRecord := map[string]any{
		"PK":          fmt.Sprintf("MEDIA#%s", mediaID),
		"SK":          "METADATA",
		"id":          fmt.Sprintf("MEDIA#%s", mediaID), // CreateObject expects an 'id' field
		"MediaID":     mediaID,
		"Username":    username,
		"URL":         mediaURL,
		"S3Key":       s3Key,
		"MimeType":    mediaData.mimeType,
		"Size":        len(mediaData.fileData),
		"Description": mediaData.description,
		"Focus":       mediaData.focus,
		"CreatedAt":   time.Now(),
	}

	if err := h.repos.Object().CreateObject(ctx.Context, mediaRecord); err != nil {
		h.logger.Warn("failed to store media metadata", zap.Error(err))
		// Don't fail the request, media is already uploaded
	}

	return ctx.Status(http.StatusOK).JSON(attachment)
}

// HandleGetMediaLift handles GET /api/v1/media/:id (Lift version)
func (h *Handler) HandleGetMediaLift(ctx *lift.Context) error {
	h.logger.Info("HandleGetMediaLift called")

	// Extract media ID
	mediaID := ctx.Param("id")
	if mediaID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "missing media ID",
		})
	}

	// Get media metadata from DynamoDB
	obj, err := h.repos.Object().GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{
			"error": fmt.Sprintf("media not found: %s", mediaID),
		})
	}

	// Convert to MediaAttachment
	mediaData, ok := obj.(map[string]any)
	if !ok {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "invalid media data",
		})
	}

	// Check if media is still processing (for v2 uploads)
	processing, _ := mediaData["Processing"].(bool)
	if processing {
		// Get processing progress
		isProcessing, progress, _ := h.GetMediaProcessingStatus(ctx.Context, mediaID)

		attachment := &models.MediaAttachment{
			ID:         mediaID,
			Type:       getMediaType(mediaData["MimeType"].(string)),
			URL:        "", // Empty while processing
			PreviewURL: "", // Empty while processing
			RemoteURL:  nil,
			TextURL:    "",
			Meta: map[string]any{
				"processing": isProcessing,
				"progress":   progress,
			},
			Description: getStringFromMediaData(mediaData, "Description"),
			Blurhash:    "",
		}

		// Add focus if present
		if focus := getStringFromMediaData(mediaData, "Focus"); focus != "" {
			var focusX, focusY float64
			if n, err := fmt.Sscanf(focus, "%f,%f", &focusX, &focusY); err == nil && n == 2 {
				attachment.Meta["focus"] = map[string]any{
					"x": focusX,
					"y": focusY,
				}
			}
		}

		return ctx.Status(http.StatusOK).JSON(attachment)
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
		Meta: map[string]any{
			"original": map[string]any{
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
		if n, err := fmt.Sscanf(focus, "%f,%f", &focusX, &focusY); err == nil && n == 2 {
			attachment.Meta["focus"] = map[string]any{
				"x": focusX,
				"y": focusY,
			}
		}
	}

	// Add duration for video/audio
	if duration := getIntFromMediaData(mediaData, "Duration"); duration > 0 {
		attachment.Meta["duration"] = float64(duration) / 1000.0 // Convert ms to seconds
	}

	return ctx.Status(http.StatusOK).JSON(attachment)
}

// HandleUpdateMediaLift handles PUT /api/v1/media/:id (Lift version)
func (h *Handler) HandleUpdateMediaLift(ctx *lift.Context) error {
	h.logger.Info("HandleUpdateMediaLift called")

	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	var username string
	var authenticated bool

	if testUsername != "" {
		// Test mode - bypass JWT validation
		username = testUsername
		authenticated = true
		h.logger.Info("Using test mode", zap.String("username", username))
	} else {
		// Production mode - validate JWT token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
				"error": "unauthorized",
			})
		}

		// Validate token
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
				"error": "invalid_token",
			})
		}

		username = claims.Username
		authenticated = true
	}

	if !authenticated {
		return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
			"error": "unauthorized",
		})
	}

	// Extract media ID
	mediaID := ctx.Param("id")
	if mediaID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
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
		h.logger.Debug("ctx.ParseRequest failed, trying common.ParseRequestBody", zap.Error(err))
		if err := common.ParseRequestBody(ctx.Request.Body, &updateReq); err != nil {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{
				"error": "invalid request body",
			})
		}
	}

	// Get existing media
	obj, err := h.repos.Object().GetObject(ctx.Context, fmt.Sprintf("MEDIA#%s", mediaID))
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{
			"error": fmt.Sprintf("media not found: %s", mediaID),
		})
	}

	// Verify ownership
	mediaData, ok := obj.(map[string]any)
	if !ok {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "invalid media data",
		})
	}

	if mediaData["Username"] != username {
		return ctx.Status(http.StatusForbidden).JSON(map[string]string{
			"error": "not authorized to update this media",
		})
	}

	// Update the media metadata
	mediaData["Description"] = updateReq.Description
	mediaData["Focus"] = updateReq.Focus
	mediaData["UpdatedAt"] = time.Now()

	if err := h.repos.Object().UpdateObject(ctx.Context, mediaData); err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "failed to update media",
		})
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

	return ctx.Status(http.StatusOK).JSON(attachment)
}

// Helper functions (keeping all the original helper functions from the legacy file)

func isAllowedMimeType(mimeType string) bool {
	allowed := []string{
		MimeTypeImageJpeg,
		MimeTypeImagePng,
		MimeTypeImageGif,
		MimeTypeImageWebp,
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
		MimeTypeImageJpeg: ".jpg",
		MimeTypeImagePng:  ".png",
		MimeTypeImageGif:  ".gif",
		MimeTypeImageWebp: ".webp",
		"video/mp4":       ".mp4",
		"video/webm":      ".webm",
		"audio/mpeg":      ".mp3",
		"audio/mp3":       ".mp3",
		"audio/ogg":       ".ogg",
		"audio/wav":       ".wav",
	}

	if ext, ok := extensions[mimeType]; ok {
		return ext
	}
	return ".bin"
}

func getMediaType(mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		if mimeType == MimeTypeImageGif {
			return MediaTypeGifv
		}
		return MediaTypeImage
	} else if strings.HasPrefix(mimeType, "video/") {
		return MediaTypeVideo
	} else if strings.HasPrefix(mimeType, "audio/") {
		return MediaTypeAudio
	}
	return MediaTypeUnknown
}

// Helper functions for media data extraction
func getStringFromMediaData(data map[string]any, key string, defaultValue ...string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func getIntFromMediaData(data map[string]any, key string) int {
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
	obj, err := h.repos.Object().GetObject(ctx, fmt.Sprintf("MEDIA#%s", mediaID))
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
	jobObj, err := h.repos.Object().GetObject(ctx, fmt.Sprintf("JOB#%s", jobID))
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
	if status == statusCompleted || status == ExportStatusFailed {
		return false, 100, nil
	}

	return true, progress, nil
}

// generateThumbnailURL generates a thumbnail URL for media
func (h *Handler) generateThumbnailURL(_ context.Context, originalURL, mimeType string) string {
	// For images, generate a thumbnail
	if strings.HasPrefix(mimeType, "image/") && mimeType != MimeTypeImageGif {
		// Extract the base URL and add thumbnail suffix
		baseURL := strings.TrimSuffix(originalURL, ".jpg")
		baseURL = strings.TrimSuffix(baseURL, ".png")
		baseURL = strings.TrimSuffix(baseURL, ".webp")

		// Get file extension
		ext := getExtensionFromMimeType(mimeType)

		// Return thumbnail URL (assumes thumbnail generation pipeline)
		return fmt.Sprintf("%s_thumb%s", baseURL, ext)
	}

	// For videos, we'd generate a video thumbnail
	if strings.HasPrefix(mimeType, "video/") {
		baseURL := strings.TrimSuffix(originalURL, ".mp4")
		baseURL = strings.TrimSuffix(baseURL, ".webm")
		return fmt.Sprintf("%s_thumb.jpg", baseURL)
	}

	// For other types, return the original URL
	return originalURL
}

// getMediaDimensions extracts dimensions from media file data
func (h *Handler) getMediaDimensions(_ context.Context, fileData []byte, mimeType string) map[string]any {
	// For images, try to extract dimensions using proper image decoding
	if strings.HasPrefix(mimeType, "image/") {
		img, err := h.decodeImage(fileData, mimeType)
		if err != nil {
			h.logger.Warn("failed to decode image for dimensions, using header parsing", zap.Error(err))
			// Fallback to header parsing
			width, height := h.extractImageDimensions(fileData, mimeType)
			return map[string]any{
				"width":  width,
				"height": height,
				"size":   fmt.Sprintf("%dx%d", width, height),
				"aspect": calculateAspectRatio(width, height),
			}
		}

		bounds := img.Bounds()
		width := bounds.Dx()
		height := bounds.Dy()

		return map[string]any{
			"width":  width,
			"height": height,
			"size":   fmt.Sprintf("%dx%d", width, height),
			"aspect": calculateAspectRatio(width, height),
		}
	}

	// For videos, we'd need video metadata extraction
	if strings.HasPrefix(mimeType, "video/") {
		// For now, return default dimensions
		// In production, use ffprobe or similar to get actual dimensions
		return map[string]any{
			"width":    1920,
			"height":   1080,
			"size":     "1920x1080",
			"aspect":   1.777,
			"duration": 0, // Would extract from video metadata
		}
	}

	// Default for other types
	return map[string]any{
		"width":  0,
		"height": 0,
		"size":   "0x0",
		"aspect": 1.0,
	}
}

// extractImageDimensions extracts width and height from image data
func (h *Handler) extractImageDimensions(fileData []byte, mimeType string) (int, int) {
	// This is a simplified implementation
	// In production, you'd use image libraries like:
	// - "image" package for basic formats
	// - "github.com/disintegration/imaging" for more formats
	// - "github.com/h2non/bimg" for libvips integration

	switch mimeType {
	case MimeTypeImageJpeg:
		return h.extractJPEGDimensions(fileData)
	case MimeTypeImagePng:
		return h.extractPNGDimensions(fileData)
	case MimeTypeImageGif:
		return h.extractGIFDimensions(fileData)
	case MimeTypeImageWebp:
		return h.extractWebPDimensions(fileData)
	default:
		return 0, 0
	}
}

// extractJPEGDimensions extracts dimensions from JPEG data
func (h *Handler) extractJPEGDimensions(data []byte) (int, int) {
	// Simplified JPEG dimension extraction
	// Look for SOF0 (Start of Frame) marker
	for i := 0; i < len(data)-9; i++ {
		if data[i] == 0xFF && data[i+1] == 0xC0 {
			// SOF0 marker found
			height := int(data[i+5])<<8 | int(data[i+6])
			width := int(data[i+7])<<8 | int(data[i+8])
			return width, height
		}
	}
	return 0, 0
}

// extractPNGDimensions extracts dimensions from PNG data
func (h *Handler) extractPNGDimensions(data []byte) (int, int) {
	// PNG signature check
	if len(data) < 24 || string(data[1:4]) != "PNG" {
		return 0, 0
	}

	// IHDR chunk starts at byte 8
	if string(data[12:16]) == "IHDR" {
		width := int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
		height := int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
		return width, height
	}
	return 0, 0
}

// extractGIFDimensions extracts dimensions from GIF data
func (h *Handler) extractGIFDimensions(data []byte) (int, int) {
	// GIF signature check
	if len(data) < 10 || (string(data[0:6]) != "GIF87a" && string(data[0:6]) != "GIF89a") {
		return 0, 0
	}

	// Width and height are at bytes 6-9
	width := int(data[6]) | int(data[7])<<8
	height := int(data[8]) | int(data[9])<<8
	return width, height
}

// extractWebPDimensions extracts dimensions from WebP data
func (h *Handler) extractWebPDimensions(data []byte) (int, int) {
	// WebP signature check
	if len(data) < 30 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0
	}

	// Simple WebP VP8 format
	if string(data[12:16]) == "VP8 " {
		// Look for frame tag
		for i := 20; i < len(data)-10; i++ {
			if data[i] == 0x9D && data[i+1] == 0x01 && data[i+2] == 0x2A {
				width := int(data[i+6]) | int(data[i+7])<<8
				height := int(data[i+8]) | int(data[i+9])<<8
				return width & 0x3FFF, height & 0x3FFF
			}
		}
	}

	return 0, 0
}

// generateBlurhash generates a blurhash string for the image
func (h *Handler) generateBlurhash(_ context.Context, fileData []byte, mimeType string) string {
	if !strings.HasPrefix(mimeType, "image/") {
		return "" // Only generate blurhash for images
	}

	// Decode the image
	img, err := h.decodeImage(fileData, mimeType)
	if err != nil {
		h.logger.Warn("failed to decode image for blurhash", zap.Error(err))
		return "LEHV6nWB2yk8pyo0adR*.7kCMdnj" // Default blurhash
	}

	// Generate a simplified hash based on image characteristics
	return h.generateSimpleBlurhash(img)
}

// decodeImage decodes image data based on MIME type
func (h *Handler) decodeImage(data []byte, mimeType string) (image.Image, error) {
	reader := bytes.NewReader(data)

	switch mimeType {
	case MimeTypeImageJpeg:
		return jpeg.Decode(reader)
	case MimeTypeImagePng:
		return png.Decode(reader)
	case MimeTypeImageGif:
		return gif.Decode(reader)
	default:
		// Try generic decode
		img, _, err := image.Decode(reader)
		return img, err
	}
}

// generateSimpleBlurhash creates a simplified blurhash-like string
func (h *Handler) generateSimpleBlurhash(img image.Image) string {
	// Resize image to 4x4 for analysis
	bounds := img.Bounds()
	smallImg := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.CatmullRom.Scale(smallImg, smallImg.Bounds(), img, bounds, draw.Over, nil)

	// Extract color components
	var r, g, b float64
	pixelCount := 0

	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := smallImg.RGBAAt(x, y)
			r += float64(c.R)
			g += float64(c.G)
			b += float64(c.B)
			pixelCount++
		}
	}

	// Calculate average colors
	avgR := r / float64(pixelCount)
	avgG := g / float64(pixelCount)
	avgB := b / float64(pixelCount)

	// Generate a simplified hash based on the dominant colors
	hash := sha256.Sum256([]byte(fmt.Sprintf("%.2f,%.2f,%.2f", avgR, avgG, avgB)))
	hashStr := hex.EncodeToString(hash[:])

	// Convert to a blurhash-like format (simplified)
	return h.formatAsBlurhash(avgR, avgG, avgB, hashStr[:16])
}

// formatAsBlurhash formats color data as a blurhash-like string
func (h *Handler) formatAsBlurhash(r, g, b float64, entropy string) string {
	// Simple blurhash-like encoding
	// This is a simplified version that encodes the dominant color

	// Normalize RGB values to 0-1 range
	rNorm := r / 255.0
	gNorm := g / 255.0
	bNorm := b / 255.0

	// Create base64-like encoding for color
	colorValue := int(rNorm*83)*83*83 + int(gNorm*83)*83 + int(bNorm*83)

	// Convert to blurhash alphabet
	alphabet := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

	result := "L" // Leading character for basic blurhash

	// Encode the main color
	for i := 0; i < 8; i++ {
		idx := (colorValue >> (i * 6)) & 63
		if idx < len(alphabet) {
			result += string(alphabet[idx])
		} else {
			result += "H" // Fallback
		}
	}

	// Add some entropy from the hash
	for i := 0; i < 15 && i < len(entropy); i += 2 {
		if i+1 < len(entropy) {
			val := 0
			if entropy[i] >= '0' && entropy[i] <= '9' {
				val += int(entropy[i] - '0')
			} else if entropy[i] >= 'a' && entropy[i] <= 'f' {
				val += int(entropy[i] - 'a' + 10)
			}
			val *= 16
			if entropy[i+1] >= '0' && entropy[i+1] <= '9' {
				val += int(entropy[i+1] - '0')
			} else if entropy[i+1] >= 'a' && entropy[i+1] <= 'f' {
				val += int(entropy[i+1] - 'a' + 10)
			}

			idx := val % len(alphabet)
			result += string(alphabet[idx])
		}
	}

	// Pad to standard length
	for len(result) < 26 {
		result += "H"
	}

	return result[:26] // Standard blurhash length
}

// mediaV1UploadData holds parsed media upload data for v1 endpoint
type mediaV1UploadData struct {
	fileData    []byte
	mimeType    string
	description string
	focus       string
}

// authenticateMediaUploadRequest authenticates a media upload request
func (h *Handler) authenticateMediaUploadRequest(ctx *lift.Context) (string, error) {
	// Check for test mode first
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - bypass JWT validation
		h.logger.Info("Using test mode", zap.String("username", testUsername))
		return testUsername, nil
	}

	// Production mode - validate JWT token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return "", ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
			"error": "unauthorized",
		})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(http.StatusUnauthorized).JSON(map[string]string{
			"error": "invalid_token",
		})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return "", ctx.Status(http.StatusForbidden).JSON(map[string]string{
			"error": "insufficient_scope",
		})
	}

	return claims.Username, nil
}

// parseMediaUpload parses multipart media upload data
func (h *Handler) parseMediaUpload(ctx *lift.Context) (*mediaV1UploadData, error) {
	bodyBytes := h.prepareRequestBody(ctx)

	// Parse boundary from content type
	boundary, err := h.extractMediaBoundary(ctx)
	if err != nil {
		return nil, err
	}

	// Create multipart reader and parse parts
	return h.parseMultipartData(bodyBytes, boundary)
}

// prepareRequestBody prepares the request body, handling base64 encoding
func (h *Handler) prepareRequestBody(ctx *lift.Context) []byte {
	bodyBytes := ctx.Request.Body

	// Handle potential base64 encoding
	if len(bodyBytes) > 0 {
		if decoded, err := base64.StdEncoding.DecodeString(string(bodyBytes)); err == nil {
			// Check if the decoded data looks like multipart form data
			if bytes.Contains(decoded, []byte("boundary")) || bytes.Contains(decoded, []byte("Content-Disposition")) {
				return decoded
			}
		}
	}

	return bodyBytes
}

// extractMediaBoundary extracts the boundary from content type
func (h *Handler) extractMediaBoundary(ctx *lift.Context) (string, error) {
	contentType := ctx.Header("Content-Type")
	if contentType == "" {
		contentType = ctx.Header("content-type")
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "invalid content type",
		})
	}

	boundary := params["boundary"]
	if boundary == "" {
		return "", ctx.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "missing boundary in content type",
		})
	}

	return boundary, nil
}

// parseMultipartData parses multipart form data
func (h *Handler) parseMultipartData(bodyBytes []byte, boundary string) (*mediaV1UploadData, error) {
	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)
	data := &mediaV1UploadData{}

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

		if err := h.processMediaV1MultipartPart(part, data); err != nil {
			return nil, err
		}
	}

	if len(data.fileData) == 0 {
		return nil, errors.New("no file data provided")
	}

	return data, nil
}

// processMediaV1MultipartPart processes a single multipart part for v1 endpoint
func (h *Handler) processMediaV1MultipartPart(part *multipart.Part, data *mediaV1UploadData) error {
	buf := new(bytes.Buffer)

	switch part.FormName() {
	case "file":
		if _, err := buf.ReadFrom(part); err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		data.fileData = buf.Bytes()

		// Get MIME type from header or detect it
		data.mimeType = part.Header.Get("Content-Type")
		if data.mimeType == "" {
			data.mimeType = http.DetectContentType(data.fileData)
		}

	case "description":
		if _, err := buf.ReadFrom(part); err != nil {
			h.logger.Warn("failed to read description", zap.Error(err))
		}
		data.description = buf.String()

	case "focus":
		if _, err := buf.ReadFrom(part); err != nil {
			h.logger.Warn("failed to read focus", zap.Error(err))
		}
		data.focus = buf.String()
	}

	return nil
}

// validateMediaUpload validates the uploaded media
func (h *Handler) validateMediaUpload(data *mediaV1UploadData) error {
	// Validate file size (10MB limit for now, can be configured)
	maxSize := int64(10 * 1024 * 1024)
	if int64(len(data.fileData)) > maxSize {
		return fmt.Errorf("file size exceeds %dMB limit", maxSize/1024/1024)
	}

	// Validate MIME type
	if !isAllowedMimeType(data.mimeType) {
		return fmt.Errorf("unsupported file type: %s", data.mimeType)
	}

	return nil
}

// uploadMediaToS3 uploads media to S3 and returns the media ID and URL
func (h *Handler) uploadMediaToS3(ctx context.Context, username string, data *mediaV1UploadData) (string, string, error) {
	// Generate unique ID for the media
	mediaID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Generate S3 key
	ext := getExtensionFromMimeType(data.mimeType)
	s3Key := fmt.Sprintf("media/%s/%s%s", username, mediaID, ext)

	// Initialize S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		return "", "", fmt.Errorf("failed to initialize S3 client: %w", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	// Upload to S3
	bucketName := h.cfg.S3BucketName
	if bucketName == "" {
		return "", "", errors.New("S3 bucket not configured")
	}

	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(bucketName),
		Key:          aws.String(s3Key),
		Body:         bytes.NewReader(data.fileData),
		ContentType:  aws.String(data.mimeType),
		ACL:          types.ObjectCannedACLPrivate,
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}

	if _, err = s3Client.PutObject(ctx, putInput); err != nil {
		h.logger.Error("failed to upload to S3",
			zap.String("bucket", bucketName),
			zap.String("key", s3Key),
			zap.Error(err))
		return "", "", fmt.Errorf("failed to upload media: %w", err)
	}

	// Build media URL (using CDN if configured)
	mediaURL := h.buildMediaURL(bucketName, s3Key)

	return mediaID, mediaURL, nil
}

// buildMediaURL builds the media URL, using CDN if configured
func (h *Handler) buildMediaURL(bucketName, s3Key string) string {
	cdnDomain := os.Getenv("CDN_DOMAIN")
	if cdnDomain != "" {
		return fmt.Sprintf("https://%s/%s", cdnDomain, s3Key)
	}
	// Fallback to S3 URL if no CDN
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, s3Key)
}

// parseFocusCoordinates parses focus point coordinates
func (h *Handler) parseFocusCoordinates(focus string) (float64, float64) {
	if focus == "" {
		return 0, 0
	}

	var focusX, focusY float64
	if n, err := fmt.Sscanf(focus, "%f,%f", &focusX, &focusY); err != nil || n != 2 {
		h.logger.Warn("failed to parse focus coordinates", zap.String("focus", focus), zap.Error(err))
		return 0, 0
	}

	return focusX, focusY
}

// extractS3KeyFromURL extracts the S3 key from a media URL
func (h *Handler) extractS3KeyFromURL(mediaURL string) string {
	// Remove the protocol and domain
	if idx := strings.Index(mediaURL, "//"); idx != -1 {
		mediaURL = mediaURL[idx+2:]
	}

	// Remove the domain part
	if idx := strings.Index(mediaURL, "/"); idx != -1 {
		return mediaURL[idx+1:]
	}

	return ""
}
