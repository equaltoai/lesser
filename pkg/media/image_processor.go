package media

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/disintegration/imaging"
	"golang.org/x/image/webp"
)

// ImageSize represents a target image size
type ImageSize struct {
	Name      string
	MaxWidth  int
	MaxHeight int
	Quality   int // JPEG quality (1-100)
}

// StandardImageSizes defines the standard sizes for Mastodon compatibility
var StandardImageSizes = []ImageSize{
	{Name: "small", MaxWidth: 400, MaxHeight: 400, Quality: 80},
	{Name: "medium", MaxWidth: 800, MaxHeight: 800, Quality: 85},
	{Name: "large", MaxWidth: 1920, MaxHeight: 1080, Quality: 90},
}

// ProcessedImage contains the result of processing an image
type ProcessedImage struct {
	Data     []byte
	Width    int
	Height   int
	Format   string
	Blurhash string
}

// ProcessImage processes an image to multiple sizes and generates blurhash
func ProcessImage(data []byte, mimeType string) (map[string]*ProcessedImage, error) {
	// Decode the original image
	img, format, err := decodeImage(bytes.NewReader(data), mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Generate blurhash from original
	blurhash, err := GenerateBlurhash(img, 4, 3)
	if err != nil {
		// Use default blurhash on error
		blurhash = GetDefaultBlurhash()
	}

	results := make(map[string]*ProcessedImage)

	// Store original
	bounds := img.Bounds()
	results["original"] = &ProcessedImage{
		Data:     data,
		Width:    bounds.Dx(),
		Height:   bounds.Dy(),
		Format:   format,
		Blurhash: blurhash,
	}

	// Process each size
	for _, size := range StandardImageSizes {
		processed, err := resizeImage(img, size, format)
		if err != nil {
			continue // Skip failed sizes
		}
		processed.Blurhash = blurhash
		results[size.Name] = processed
	}

	return results, nil
}

// decodeImage decodes an image from a reader
func decodeImage(r io.Reader, mimeType string) (image.Image, string, error) {
	switch mimeType {
	case "image/jpeg":
		img, err := jpeg.Decode(r)
		return img, FormatJPEG, err
	case "image/png":
		img, err := png.Decode(r)
		return img, "png", err
	case "image/gif":
		img, err := gif.Decode(r)
		return img, "gif", err
	case "image/webp":
		img, err := webp.Decode(r)
		return img, "webp", err
	default:
		// Try generic decoder
		img, format, err := image.Decode(r)
		return img, format, err
	}
}

// resizeImage resizes an image to fit within the specified dimensions
func resizeImage(img image.Image, size ImageSize, format string) (*ProcessedImage, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Skip if image is already smaller than target
	if width <= size.MaxWidth && height <= size.MaxHeight {
		return encodeImage(img, format, size.Quality)
	}

	// Calculate new dimensions maintaining aspect ratio
	newWidth, newHeight := calculateDimensions(width, height, size.MaxWidth, size.MaxHeight)

	// Resize using Lanczos3 for high quality
	resized := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)

	return encodeImage(resized, format, size.Quality)
}

// calculateDimensions calculates new dimensions maintaining aspect ratio
func calculateDimensions(origWidth, origHeight, maxWidth, maxHeight int) (int, int) {
	if origWidth <= maxWidth && origHeight <= maxHeight {
		return origWidth, origHeight
	}

	widthRatio := float64(maxWidth) / float64(origWidth)
	heightRatio := float64(maxHeight) / float64(origHeight)

	ratio := widthRatio
	if heightRatio < widthRatio {
		ratio = heightRatio
	}

	newWidth := int(float64(origWidth) * ratio)
	newHeight := int(float64(origHeight) * ratio)

	return newWidth, newHeight
}

// encodeImage encodes an image to bytes
func encodeImage(img image.Image, format string, quality int) (*ProcessedImage, error) {
	var buf bytes.Buffer
	var err error

	bounds := img.Bounds()

	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "png":
		err = png.Encode(&buf, img)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	default:
		// Default to JPEG for unknown formats
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
		format = "jpeg"
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}

	return &ProcessedImage{
		Data:   buf.Bytes(),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
		Format: format,
	}, nil
}

// StripEXIF removes EXIF data from an image
func StripEXIF(data []byte, mimeType string) ([]byte, error) {
	// Decode and re-encode the image to strip metadata
	img, format, err := decodeImage(bytes.NewReader(data), mimeType)
	if err != nil {
		return nil, err
	}

	// Re-encode without metadata
	result, err := encodeImage(img, format, 95)
	if err != nil {
		return nil, err
	}

	return result.Data, nil
}
