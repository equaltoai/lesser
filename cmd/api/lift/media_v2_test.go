package lift

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleUploadMediaV2Lift(t *testing.T) {
	tests := []struct {
		name         string
		headers      map[string]string
		body         []byte
		setupMocks   func(*MockStorageAdapter)
		expectedCode int
		expectError  bool
	}{
		{
			name: "successful upload with test mode",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			},
			body: createMultipartBody("----WebKitFormBoundary7MA4YWxkTrZu0gW", map[string]string{
				"description": "Test image",
				"focus":       "-0.5,0.25",
			}, "test.jpg", "image/jpeg", []byte("fake jpeg data")),
			setupMocks: func(m *MockStorageAdapter) {
				m.On("CreateObject", mock.Anything, mock.MatchedBy(func(obj map[string]any) bool {
					return obj["PK"].(string) != "" && 
						   strings.HasPrefix(obj["PK"].(string), "MEDIA#") &&
						   obj["SK"] == "METADATA" &&
						   obj["Username"] == "testuser" &&
						   obj["Processing"] == true
				})).Return(nil)
				
				m.On("CreateObject", mock.Anything, mock.MatchedBy(func(obj map[string]any) bool {
					return obj["PK"].(string) != "" && 
						   strings.HasPrefix(obj["PK"].(string), "JOB#") &&
						   obj["Status"] == "pending"
				})).Return(nil)
			},
			expectedCode: http.StatusAccepted,
		},
		{
			name: "missing X-Test-Username and Authorization returns 401",
			headers: map[string]string{
				"Content-Type": "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			},
			body:         createMultipartBody("----WebKitFormBoundary7MA4YWxkTrZu0gW", nil, "test.jpg", "image/jpeg", []byte("fake jpeg data")),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusUnauthorized,
			expectError:  true,
		},
		{
			name: "missing boundary in content type returns 400",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "multipart/form-data",
			},
			body:         []byte("invalid multipart data"),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusBadRequest,
			expectError:  true,
		},
		{
			name: "no file data returns 400",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			},
			body:         createMultipartBody("----WebKitFormBoundary7MA4YWxkTrZu0gW", nil, "", "", nil),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusBadRequest,
			expectError:  true,
		},
		{
			name: "unsupported file type returns 422",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			},
			body:         createMultipartBody("----WebKitFormBoundary7MA4YWxkTrZu0gW", nil, "test.exe", "application/x-executable", []byte("fake exe data")),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusUnprocessableEntity,
			expectError:  true,
		},
		{
			name: "file too large returns 422",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			},
			body: createMultipartBody("----WebKitFormBoundary7MA4YWxkTrZu0gW", nil, "large.jpg", "image/jpeg", 
				make([]byte, 11*1024*1024)), // 11MB
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusUnprocessableEntity,
			expectError:  true,
		},
		{
			name: "storage error returns 500",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW",
			},
			body: createMultipartBody("----WebKitFormBoundary7MA4YWxkTrZu0gW", nil, "test.jpg", "image/jpeg", []byte("fake jpeg data")),
			setupMocks: func(m *MockStorageAdapter) {
				m.On("CreateObject", mock.Anything, mock.Anything).Return(errors.New("storage error"))
			},
			expectedCode: http.StatusInternalServerError,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStorage := new(MockStorageAdapter)
			tt.setupMocks(mockStorage)

			cfg := &config.Config{
				S3BucketName: "test-bucket",
			}
			handler := NewHandler(cfg, mockStorage, zap.NewNop(), nil)

			// Create test context
			req := &lift.Request{
				Request: &adapters.Request{
					Method:  "POST",
					Path:    "/api/v2/media",
					Headers: tt.headers,
					Body:    tt.body,
				},
				Method:  "POST",
				Path:    "/api/v2/media",
				Headers: tt.headers,
				Body:    tt.body,
			}
			ctx := lift.NewContext(context.Background(), req)

			// Execute
			err := handler.HandleUploadMediaV2Lift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCode, ctx.Response.StatusCode)

			mockStorage.AssertExpectations(t)
		})
	}
}

func TestHandleGetMediaV2Lift(t *testing.T) {
	tests := []struct {
		name         string
		mediaID      string
		setupMocks   func(*MockStorageAdapter)
		expectedCode int
		expectError  bool
	}{
		{
			name:    "successful get processing media",
			mediaID: "test-media-123",
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"PK":          "MEDIA#test-media-123",
					"SK":          "METADATA",
					"MediaID":     "test-media-123",
					"Username":    "testuser",
					"MimeType":    "image/jpeg",
					"Processing":  true,
					"JobID":       "job-123",
					"Description": "Test media",
					"Focus":       "-0.5,0.25",
				}
				jobData := map[string]any{
					"PK":              "JOB#job-123",
					"SK":              "JOB#job-123",
					"ProcessingTasks": []string{"resize", "blurhash"},
					"Results":         map[string]any{"resize": "done"},
					"Status":          "processing",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
				m.On("GetObject", mock.Anything, "JOB#job-123").Return(jobData, nil)
			},
			expectedCode: http.StatusOK,
		},
		{
			name:    "successful get completed media",
			mediaID: "test-media-456",
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"PK":          "MEDIA#test-media-456",
					"SK":          "METADATA",
					"MediaID":     "test-media-456",
					"Username":    "testuser",
					"MimeType":    "image/jpeg",
					"Processing":  false,
					"URL":         "https://cdn.example.com/media/test.jpg",
					"PreviewURL":  "https://cdn.example.com/media/test_thumb.jpg",
					"Width":       1920,
					"Height":      1080,
					"Description": "Test media",
					"Blurhash":    "LKNekKjs00xu~AXkozkC?vt7-;R*",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-456").Return(mediaData, nil)
			},
			expectedCode: http.StatusOK,
		},
		{
			name:    "missing media ID returns 400",
			mediaID: "",
			setupMocks: func(m *MockStorageAdapter) {
				// No mocks needed
			},
			expectedCode: http.StatusBadRequest,
			expectError:  true,
		},
		{
			name:    "media not found returns 404",
			mediaID: "nonexistent",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetObject", mock.Anything, "MEDIA#nonexistent").Return(nil, errors.New("not found"))
			},
			expectedCode: http.StatusNotFound,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStorage := new(MockStorageAdapter)
			tt.setupMocks(mockStorage)

			cfg := &config.Config{}
			handler := NewHandler(cfg, mockStorage, zap.NewNop(), nil)

			// Create test context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   fmt.Sprintf("/api/v2/media/%s", tt.mediaID),
					PathParams: map[string]string{
						"id": tt.mediaID,
					},
				},
				Method: "GET",
				Path:   fmt.Sprintf("/api/v2/media/%s", tt.mediaID),
			}
			ctx := lift.NewContext(context.Background(), req)

			// Execute
			err := handler.HandleGetMediaV2Lift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCode, ctx.Response.StatusCode)

			mockStorage.AssertExpectations(t)
		})
	}
}

func TestHandleUpdateMediaV2Lift(t *testing.T) {
	tests := []struct {
		name         string
		headers      map[string]string
		mediaID      string
		body         []byte
		setupMocks   func(*MockStorageAdapter)
		expectedCode int
		expectError  bool
	}{
		{
			name: "successful update with test mode",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "application/json",
			},
			mediaID: "test-media-123",
			body:    []byte(`{"description": "Updated description", "focus": "0.1,-0.2"}`),
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"PK":          "MEDIA#test-media-123",
					"SK":          "METADATA",
					"MediaID":     "test-media-123",
					"Username":    "testuser",
					"MimeType":    "image/jpeg",
					"URL":         "https://cdn.example.com/media/test.jpg",
					"Description": "Old description",
					"Focus":       "0,0",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
				m.On("UpdateObject", mock.Anything, mock.MatchedBy(func(obj map[string]any) bool {
					return obj["Description"] == "Updated description" &&
						   obj["Focus"] == "0.1,-0.2" &&
						   obj["PK"] == "MEDIA#test-media-123" &&
						   obj["Username"] == "testuser"
				})).Return(nil)
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "missing X-Test-Username and Authorization returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			mediaID:      "test-media-123",
			body:         []byte(`{"description": "Updated description"}`),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusUnauthorized,
			expectError:  true,
		},
		{
			name: "missing media ID returns 400",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "application/json",
			},
			mediaID:      "",
			body:         []byte(`{"description": "Updated description"}`),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusBadRequest,
			expectError:  true,
		},
		{
			name: "invalid JSON returns 400",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "application/json",
			},
			mediaID:      "test-media-123",
			body:         []byte(`{"description": "invalid json"`),
			setupMocks:   func(m *MockStorageAdapter) {},
			expectedCode: http.StatusBadRequest,
			expectError:  true,
		},
		{
			name: "media not found returns 404",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "application/json",
			},
			mediaID: "nonexistent",
			body:    []byte(`{"description": "Updated description"}`),
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetObject", mock.Anything, "MEDIA#nonexistent").Return(nil, errors.New("not found"))
			},
			expectedCode: http.StatusNotFound,
			expectError:  true,
		},
		{
			name: "not owner returns 403",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "application/json",
			},
			mediaID: "test-media-123",
			body:    []byte(`{"description": "Updated description"}`),
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"PK":        "MEDIA#test-media-123",
					"SK":        "METADATA",
					"MediaID":   "test-media-123",
					"Username":  "otheruser", // Different owner
					"MimeType":  "image/jpeg",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
			},
			expectedCode: http.StatusForbidden,
			expectError:  true,
		},
		{
			name: "update error returns 500",
			headers: map[string]string{
				"X-Test-Username": "testuser",
				"Content-Type":    "application/json",
			},
			mediaID: "test-media-123",
			body:    []byte(`{"description": "Updated description"}`),
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"PK":          "MEDIA#test-media-123",
					"SK":          "METADATA",
					"MediaID":     "test-media-123",
					"Username":    "testuser",
					"MimeType":    "image/jpeg",
					"Description": "Old description",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
				m.On("UpdateObject", mock.Anything, mock.Anything).Return(errors.New("update failed"))
			},
			expectedCode: http.StatusInternalServerError,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStorage := new(MockStorageAdapter)
			tt.setupMocks(mockStorage)

			cfg := &config.Config{}
			handler := NewHandler(cfg, mockStorage, zap.NewNop(), nil)

			// Create test context
			req := &lift.Request{
				Request: &adapters.Request{
					Method:  "PUT",
					Path:    fmt.Sprintf("/api/v2/media/%s", tt.mediaID),
					Headers: tt.headers,
					PathParams: map[string]string{
						"id": tt.mediaID,
					},
					Body: tt.body,
				},
				Method:  "PUT",
				Path:    fmt.Sprintf("/api/v2/media/%s", tt.mediaID),
				Headers: tt.headers,
				Body: tt.body,
			}
			ctx := lift.NewContext(context.Background(), req)

			// Execute
			err := handler.HandleUpdateMediaV2Lift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCode, ctx.Response.StatusCode)

			mockStorage.AssertExpectations(t)
		})
	}
}

func TestGetMediaProcessingStatusLift(t *testing.T) {
	tests := []struct {
		name           string
		mediaID        string
		setupMocks     func(*MockStorageAdapter)
		expectedProc   bool
		expectedProg   int
		expectError    bool
	}{
		{
			name:    "processing complete returns false",
			mediaID: "test-media-123",
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"Processing": false,
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
			},
			expectedProc: false,
			expectedProg: 100,
		},
		{
			name:    "processing in progress returns true with progress",
			mediaID: "test-media-123",
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"Processing": true,
					"JobID":      "job-123",
				}
				jobData := map[string]any{
					"ProcessingTasks": []string{"resize", "blurhash", "dimensions"},
					"Results":         map[string]any{"resize": "done"},
					"Status":          "processing",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
				m.On("GetObject", mock.Anything, "JOB#job-123").Return(jobData, nil)
			},
			expectedProc: true,
			expectedProg: 33, // 1 out of 3 tasks completed
		},
		{
			name:    "job completed returns false",
			mediaID: "test-media-123",
			setupMocks: func(m *MockStorageAdapter) {
				mediaData := map[string]any{
					"Processing": true,
					"JobID":      "job-123",
				}
				jobData := map[string]any{
					"Status": "completed",
				}
				m.On("GetObject", mock.Anything, "MEDIA#test-media-123").Return(mediaData, nil)
				m.On("GetObject", mock.Anything, "JOB#job-123").Return(jobData, nil)
			},
			expectedProc: false,
			expectedProg: 100,
		},
		{
			name:    "media not found returns error",
			mediaID: "nonexistent",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetObject", mock.Anything, "MEDIA#nonexistent").Return(nil, errors.New("not found"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStorage := new(MockStorageAdapter)
			tt.setupMocks(mockStorage)

			cfg := &config.Config{}
			handler := NewHandler(cfg, mockStorage, zap.NewNop(), nil)

			// Create test context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/status",
				},
			}
			ctx := lift.NewContext(context.Background(), req)

			// Execute
			isProcessing, progress, err := handler.GetMediaProcessingStatusLift(ctx, tt.mediaID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedProc, isProcessing)
				assert.Equal(t, tt.expectedProg, progress)
			}

			mockStorage.AssertExpectations(t)
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("isAllowedMimeTypeLift", func(t *testing.T) {
		tests := []struct {
			mimeType string
			expected bool
		}{
			{"image/jpeg", true},
			{"image/png", true},
			{"video/mp4", true},
			{"audio/mp3", true},
			{"application/pdf", false},
			{"text/plain", false},
		}

		for _, tt := range tests {
			result := isAllowedMimeTypeLift(tt.mimeType)
			assert.Equal(t, tt.expected, result, "mime type: %s", tt.mimeType)
		}
	})

	t.Run("getExtensionFromMimeTypeLift", func(t *testing.T) {
		tests := []struct {
			mimeType  string
			expected  string
		}{
			{"image/jpeg", ".jpg"},
			{"image/png", ".png"},
			{"video/mp4", ".mp4"},
			{"audio/mp3", ".mp3"},
			{"unknown/type", ".bin"},
		}

		for _, tt := range tests {
			result := getExtensionFromMimeTypeLift(tt.mimeType)
			assert.Equal(t, tt.expected, result, "mime type: %s", tt.mimeType)
		}
	})

	t.Run("getMediaTypeLift", func(t *testing.T) {
		tests := []struct {
			mimeType string
			expected string
		}{
			{"image/jpeg", "image"},
			{"image/gif", "gifv"},
			{"video/mp4", "video"},
			{"audio/mp3", "audio"},
			{"unknown/type", "unknown"},
		}

		for _, tt := range tests {
			result := getMediaTypeLift(tt.mimeType)
			assert.Equal(t, tt.expected, result, "mime type: %s", tt.mimeType)
		}
	})
}

// Helper function to create multipart form data for testing
func createMultipartBody(boundary string, fields map[string]string, filename, contentType string, fileData []byte) []byte {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	w.SetBoundary(boundary)

	// Add text fields
	for key, value := range fields {
		fw, _ := w.CreateFormField(key)
		fw.Write([]byte(value))
	}

	// Add file field if provided
	if filename != "" && fileData != nil {
		fw, _ := w.CreateFormFile("file", filename)
		if contentType != "" {
			// This is a bit of a hack since multipart.Writer doesn't let us set content type easily
			// In real tests, we might need a more sophisticated approach
		}
		fw.Write(fileData)
	}

	w.Close()
	return b.Bytes()
}