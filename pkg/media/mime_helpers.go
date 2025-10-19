package media

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"
)

var fallbackExtensions = map[string]string{
	// Images
	"image/jpeg":    ".jpg",
	"image/jpg":     ".jpg",
	"image/png":     ".png",
	"image/gif":     ".gif",
	"image/webp":    ".webp",
	"image/svg+xml": ".svg",
	"image/bmp":     ".bmp",
	"image/tiff":    ".tiff",

	// Videos
	"video/mp4":       ".mp4",
	"video/webm":      ".webm",
	"video/ogg":       ".ogv",
	"video/avi":       ".avi",
	"video/mov":       ".mov",
	"video/quicktime": ".mov",
	"video/x-msvideo": ".avi",

	// Audio
	"audio/mpeg":  ".mp3",
	"audio/mp3":   ".mp3",
	"audio/wav":   ".wav",
	"audio/x-wav": ".wav",
	"audio/ogg":   ".ogg",
	"audio/aac":   ".aac",
	"audio/flac":  ".flac",
	"audio/webm":  ".webm",

	// Generic
	"application/octet-stream": ".bin",
}

// EnsureFilenameHasExtension returns a filename with an extension that matches the supplied MIME type.
// If the filename already contains an extension it is returned as-is. When no extension is present,
// the helper attempts to infer one using mime.ExtensionsByType with a fallback map that covers the
// formats accepted by the media service.
func EnsureFilenameHasExtension(filename, contentType string) (string, error) {
	trimmed := strings.TrimSpace(filename)
	if trimmed == "" {
		return "", fmt.Errorf("filename cannot be blank")
	}

	if filepath.Ext(trimmed) != "" {
		return trimmed, nil
	}

	mimeType := normalizeMIMEType(contentType)
	if mimeType != "" {
		if fallback, ok := fallbackExtensions[mimeType]; ok && fallback != "" {
			return trimmed + fallback, nil
		}

		if exts, err := mime.ExtensionsByType(mimeType); err == nil {
			for _, ext := range exts {
				if ext != "" {
					return trimmed + strings.ToLower(ext), nil
				}
			}
		}
	}

	if fallback, ok := fallbackExtensions["application/octet-stream"]; ok && fallback != "" {
		return trimmed + fallback, nil
	}

	return "", fmt.Errorf("file extension required for MIME type %s", contentType)
}

func normalizeMIMEType(contentType string) string {
	value := strings.ToLower(strings.TrimSpace(contentType))
	if value == "" {
		return ""
	}

	if idx := strings.Index(value, ";"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}

	return value
}
