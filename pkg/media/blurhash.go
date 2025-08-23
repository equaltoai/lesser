package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/bbrks/go-blurhash"
)

// GenerateBlurhash generates a blurhash for the given image
// componentX and componentY control the level of detail (typically 4x3 or 4x4)
func GenerateBlurhash(img image.Image, componentX, componentY int) (string, error) {
	// Resize image to a small size for faster processing
	// Blurhash works best with small images (20-100 pixels wide)
	resized := resizeForBlurhash(img, 32)

	// Generate the hash
	hash, err := blurhash.Encode(componentX, componentY, resized)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrBlurhashEncode, err)
	}

	return hash, nil
}

// GenerateBlurhashFromBytes generates a blurhash from image bytes
func GenerateBlurhashFromBytes(data []byte, mimeType string) (string, error) {
	// Decode the image
	var img image.Image
	var err error

	switch mimeType {
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "image/png":
		img, err = png.Decode(bytes.NewReader(data))
	default:
		// Try to decode with generic image decoder
		img, _, err = image.Decode(bytes.NewReader(data))
	}

	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrImageDecode, err)
	}

	// Use 4x3 components for a good balance of quality and size
	return GenerateBlurhash(img, 4, 3)
}

// DecodeBlurhash decodes a blurhash string into an image
func DecodeBlurhash(hash string, width, height int) (image.Image, error) {
	// Decode the hash
	img, err := blurhash.Decode(hash, width, height, 1)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBlurhashDecode, err)
	}

	return img, nil
}

// resizeForBlurhash resizes an image to a small size suitable for blurhash generation
func resizeForBlurhash(src image.Image, maxWidth int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate new dimensions maintaining aspect ratio
	newWidth := maxWidth
	newHeight := (height * maxWidth) / width
	if newHeight < 1 {
		newHeight = 1
	}

	// Create a new image
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Simple resize using nearest neighbor algorithm
	// For blurhash, quality doesn't matter much since it's going to be blurred anyway
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := x * width / newWidth
			srcY := y * height / newHeight
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

// GetDefaultBlurhash returns a default blurhash for error cases
func GetDefaultBlurhash() string {
	// A neutral gray blurhash
	return "L00000fQfQfQfQfQfQfQfQfQfQfQ"
}
