package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageProcessor_decodeImage_SupportedTypes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})

	var jpegBuf bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90}))
	decoded, format, err := decodeImage(bytes.NewReader(jpegBuf.Bytes()), "image/jpeg")
	require.NoError(t, err)
	require.NotNil(t, decoded)
	assert.Equal(t, FormatJPEG, format)

	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, img))
	decoded, format, err = decodeImage(bytes.NewReader(pngBuf.Bytes()), "image/png")
	require.NoError(t, err)
	require.NotNil(t, decoded)
	assert.Equal(t, "png", format)

	var gifBuf bytes.Buffer
	require.NoError(t, gif.Encode(&gifBuf, img, nil))
	decoded, format, err = decodeImage(bytes.NewReader(gifBuf.Bytes()), "image/gif")
	require.NoError(t, err)
	require.NotNil(t, decoded)
	assert.Equal(t, "gif", format)

	// Unknown mime type falls back to generic decoder.
	decoded, format, err = decodeImage(bytes.NewReader(pngBuf.Bytes()), "application/octet-stream")
	require.NoError(t, err)
	require.NotNil(t, decoded)
	assert.Equal(t, "png", format)

	// WebP branch covered via error path.
	_, format, err = decodeImage(bytes.NewReader([]byte("not-webp")), "image/webp")
	require.Error(t, err)
	assert.Equal(t, "webp", format)
}

func TestImageProcessor_encodeImage_FormatsAndDefaults(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	out, err := encodeImage(img, "png", 90)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "png", out.Format)
	assert.NotEmpty(t, out.Data)

	out, err = encodeImage(img, "gif", 90)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "gif", out.Format)
	assert.NotEmpty(t, out.Data)

	out, err = encodeImage(img, "unknown", 90)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "jpeg", out.Format)
	assert.NotEmpty(t, out.Data)
}

func TestImageProcessor_resizeImage_SkipsWhenAlreadySmall(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	out, err := resizeImage(img, ImageSize{Name: "big", MaxWidth: 500, MaxHeight: 500, Quality: 80}, "jpeg")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, 100, out.Width)
	assert.Equal(t, 100, out.Height)
}

func TestImageProcessor_calculateDimensions_ReturnsOriginalWhenFits(t *testing.T) {
	w, h := calculateDimensions(100, 200, 500, 500)
	assert.Equal(t, 100, w)
	assert.Equal(t, 200, h)
}

func TestImageProcessor_StripEXIF_ReencodesAndErrorsOnBadData(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	var jpegBuf bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90}))

	stripped, err := StripEXIF(jpegBuf.Bytes(), "image/jpeg")
	require.NoError(t, err)
	assert.NotEmpty(t, stripped)

	decoded, _, err := image.Decode(bytes.NewReader(stripped))
	require.NoError(t, err)
	require.NotNil(t, decoded)

	_, err = StripEXIF([]byte("not an image"), "image/jpeg")
	require.Error(t, err)
}

func TestImageProcessor_ProcessImage_InvalidDataReturnsError(t *testing.T) {
	_, err := ProcessImage([]byte("not an image"), "image/jpeg")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImageDecodeProcess)
}
