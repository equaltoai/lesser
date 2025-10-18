package media

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateBlurhash(t *testing.T) {
	tests := []struct {
		name       string
		img        image.Image
		componentX int
		componentY int
		wantErr    bool
	}{
		{
			name:       "Simple solid color image",
			img:        createSolidColorImage(100, 100, color.RGBA{255, 0, 0, 255}),
			componentX: 4,
			componentY: 3,
			wantErr:    false,
		},
		{
			name:       "Gradient image",
			img:        createGradientImage(100, 100),
			componentX: 4,
			componentY: 4,
			wantErr:    false,
		},
		{
			name:       "Small image",
			img:        createSolidColorImage(10, 10, color.RGBA{0, 255, 0, 255}),
			componentX: 2,
			componentY: 2,
			wantErr:    false,
		},
		{
			name:       "Large image",
			img:        createSolidColorImage(1000, 1000, color.RGBA{0, 0, 255, 255}),
			componentX: 6,
			componentY: 4,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := GenerateBlurhash(tt.img, tt.componentX, tt.componentY)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, hash)
				// Blurhash should be a valid string
				assert.Greater(t, len(hash), 0)
				assert.Less(t, len(hash), 200) // Blurhash strings are typically < 200 chars
			}
		})
	}
}

func TestGenerateBlurhashFromBytes(t *testing.T) {
	// Create a test image
	img := createSolidColorImage(100, 100, color.RGBA{255, 128, 0, 255})

	tests := []struct {
		name     string
		data     []byte
		mimeType string
		wantErr  bool
	}{
		{
			name:     "Valid JPEG",
			data:     imageToJPEGBytes(img),
			mimeType: "image/jpeg",
			wantErr:  false,
		},
		{
			name:     "Valid PNG",
			data:     imageToPNGBytes(img),
			mimeType: "image/png",
			wantErr:  false,
		},
		{
			name:     "Invalid image data",
			data:     []byte("not an image"),
			mimeType: "image/jpeg",
			wantErr:  true,
		},
		{
			name:     "Empty data",
			data:     []byte{},
			mimeType: "image/jpeg",
			wantErr:  true,
		},
		{
			name:     "Unknown mime type with valid PNG data",
			data:     imageToPNGBytes(img),
			mimeType: "image/unknown",
			wantErr:  false, // Should fall back to generic decoder
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := GenerateBlurhashFromBytes(tt.data, tt.mimeType)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, hash)
			}
		})
	}
}

func TestDecodeBlurhash(t *testing.T) {
	// Generate a blurhash from a known image
	testImg := createGradientImage(100, 100)
	hash, err := GenerateBlurhash(testImg, 4, 3)
	require.NoError(t, err)

	tests := []struct {
		name    string
		hash    string
		width   int
		height  int
		wantErr bool
	}{
		{
			name:    "Valid blurhash",
			hash:    hash,
			width:   100,
			height:  100,
			wantErr: false,
		},
		{
			name:    "Default blurhash",
			hash:    GetDefaultBlurhash(),
			width:   50,
			height:  50,
			wantErr: false,
		},
		{
			name:    "Invalid blurhash",
			hash:    "invalid",
			width:   100,
			height:  100,
			wantErr: true,
		},
		{
			name:    "Too short blurhash",
			hash:    "L",
			width:   100,
			height:  100,
			wantErr: true,
		},
		{
			name:    "Different dimensions",
			hash:    hash,
			width:   200,
			height:  150,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := DecodeBlurhash(tt.hash, tt.width, tt.height)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, img)
				bounds := img.Bounds()
				assert.Equal(t, tt.width, bounds.Dx())
				assert.Equal(t, tt.height, bounds.Dy())
			}
		})
	}
}

func TestResizeForBlurhash(t *testing.T) {
	tests := []struct {
		name       string
		srcWidth   int
		srcHeight  int
		maxWidth   int
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "Square image",
			srcWidth:   100,
			srcHeight:  100,
			maxWidth:   32,
			wantWidth:  32,
			wantHeight: 32,
		},
		{
			name:       "Landscape image",
			srcWidth:   200,
			srcHeight:  100,
			maxWidth:   32,
			wantWidth:  32,
			wantHeight: 16,
		},
		{
			name:       "Portrait image",
			srcWidth:   100,
			srcHeight:  200,
			maxWidth:   32,
			wantWidth:  32,
			wantHeight: 64,
		},
		{
			name:       "Very wide image",
			srcWidth:   1000,
			srcHeight:  10,
			maxWidth:   32,
			wantWidth:  32,
			wantHeight: 1, // Minimum height
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := createSolidColorImage(tt.srcWidth, tt.srcHeight, color.RGBA{128, 128, 128, 255})
			result := resizeForBlurhash(src, tt.maxWidth)

			bounds := result.Bounds()
			assert.Equal(t, tt.wantWidth, bounds.Dx())
			assert.Equal(t, tt.wantHeight, bounds.Dy())
		})
	}
}

func TestGetDefaultBlurhash(t *testing.T) {
	hash := GetDefaultBlurhash()

	assert.NotEmpty(t, hash)
	assert.Equal(t, "L00000fQfQfQfQfQfQfQfQfQfQfQ", hash)

	// Verify it can be decoded
	img, err := DecodeBlurhash(hash, 32, 32)
	assert.NoError(t, err)
	assert.NotNil(t, img)
}

func TestProcessImageWithBlurhash(t *testing.T) {
	// Create a test image
	img := createGradientImage(800, 600)
	jpegData := imageToJPEGBytes(img)

	// Process the image
	results, err := ProcessImage(jpegData, "image/jpeg")
	require.NoError(t, err)

	// Check that all sizes have blurhash
	assert.Contains(t, results, "original")
	assert.Contains(t, results, "small")
	assert.Contains(t, results, "medium")
	assert.Contains(t, results, "large")

	// Verify blurhash is generated for each size
	for name, processed := range results {
		assert.NotEmpty(t, processed.Blurhash, "Blurhash should be generated for %s", name)
		assert.Greater(t, len(processed.Blurhash), 10, "Blurhash should be a valid string for %s", name)

		// All sizes should have the same blurhash (generated from original)
		if name != "original" {
			assert.Equal(t, results["original"].Blurhash, processed.Blurhash)
		}
	}
}

func TestBlurhashConsistency(t *testing.T) {
	// Test that the same image produces the same blurhash
	img := createGradientImage(100, 100)

	hash1, err := GenerateBlurhash(img, 4, 3)
	require.NoError(t, err)

	hash2, err := GenerateBlurhash(img, 4, 3)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2, "Same image should produce same blurhash")

	// Different component values should produce different hashes
	hash3, err := GenerateBlurhash(img, 5, 4)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash3, "Different components should produce different blurhash")
}

// Helper functions for testing

func createSolidColorImage(width, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return img
}

func createGradientImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create a gradient from red to blue
			r := uint8((255 * x) / width)
			b := uint8((255 * y) / height)
			img.Set(x, y, color.RGBA{r, 0, b, 255})
		}
	}
	return img
}

func imageToJPEGBytes(img image.Image) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func imageToPNGBytes(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// Benchmark tests

func BenchmarkGenerateBlurhash(b *testing.B) {
	sizes := []struct {
		name   string
		width  int
		height int
	}{
		{"Small_100x100", 100, 100},
		{"Medium_500x500", 500, 500},
		{"Large_1000x1000", 1000, 1000},
		{"HD_1920x1080", 1920, 1080},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			img := createGradientImage(size.width, size.height)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = GenerateBlurhash(img, 4, 3)
			}
		})
	}
}

func BenchmarkDecodeBlurhash(b *testing.B) {
	// Generate a test hash
	img := createGradientImage(100, 100)
	hash, _ := GenerateBlurhash(img, 4, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeBlurhash(hash, 100, 100)
	}
}
