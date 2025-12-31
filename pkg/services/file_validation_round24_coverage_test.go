package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultImportValidationConfig_Round24(t *testing.T) {
	cfg := DefaultImportValidationConfig()
	require.Greater(t, cfg.MaxSizeBytes, int64(0))
	require.NotEmpty(t, cfg.AllowedTypes)
	require.Contains(t, cfg.RequiredFormats, FormatJSON)
	require.Contains(t, cfg.RequiredFormats, FormatCSV)
	require.True(t, cfg.EnableVirusScan)
	require.True(t, cfg.ValidateContent)
}

func TestNewFileValidationService_Round24(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	svc, err := NewFileValidationService(zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, svc.s3Client)
	require.True(t, svc.virusCheck)
}

func TestFileValidationService_ValidateFile_SizeAndTypeAndFormat_Round24(t *testing.T) {
	fv := &FileValidationService{logger: zap.NewNop(), virusCheck: true}

	t.Run("empty_file_sets_error", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		res, err := fv.ValidateFile(context.Background(), nil, cfg)
		require.NoError(t, err)
		require.False(t, res.Valid)
		require.Contains(t, res.Errors, ErrFileEmpty.Error())
	})

	t.Run("size_limit_sets_error", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.MaxSizeBytes = 1
		res, err := fv.ValidateFile(context.Background(), []byte("too big"), cfg)
		require.NoError(t, err)
		require.False(t, res.Valid)
		require.NotEmpty(t, res.Errors)
		require.True(t, strings.Contains(res.Errors[0], ErrFileSizeExceedsLimit.Error()))
	})

	t.Run("content_type_not_allowed_sets_error", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.AllowedTypes = []string{"application/json"}
		res, err := fv.ValidateFile(context.Background(), []byte("a,b\n1,2"), cfg)
		require.NoError(t, err)
		require.False(t, res.Valid)
		require.True(t, sliceContainsPrefix(res.Errors, ErrContentTypeNotAllowed.Error()))
	})

	t.Run("format_not_supported_sets_error", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.AllowedTypes = []string{"text/csv"}
		cfg.RequiredFormats = []string{FormatJSON}
		res, err := fv.ValidateFile(context.Background(), []byte("a,b\n1,2"), cfg)
		require.NoError(t, err)
		require.False(t, res.Valid)
		require.True(t, sliceContainsPrefix(res.Errors, ErrFormatNotSupported.Error()))
	})
}

func TestFileValidationService_ValidateFile_JSONAndCSVContent_Round24(t *testing.T) {
	fv := &FileValidationService{logger: zap.NewNop(), virusCheck: true}

	t.Run("json_object_empty_warning", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.EnableVirusScan = false

		res, err := fv.ValidateFile(context.Background(), []byte("{}"), cfg)
		require.NoError(t, err)
		require.True(t, res.Valid)
		require.Equal(t, FormatJSON, res.DetectedFormat)
		require.Contains(t, res.Warnings, ErrJSONObjectEmpty.Error())
	})

	t.Run("json_array_item_warning", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.EnableVirusScan = false

		res, err := fv.ValidateFile(context.Background(), []byte(`[1, {"ok": true}]`), cfg)
		require.NoError(t, err)
		require.True(t, res.Valid)
		require.Equal(t, FormatJSON, res.DetectedFormat)
		require.True(t, sliceContainsPrefix(res.Warnings, "array item 0 is not an object"))
	})

	t.Run("json_primitive_warning", func(t *testing.T) {
		res := &FileValidationResult{
			Valid:    true,
			Warnings: []string{},
			Metadata: map[string]any{},
		}
		require.NoError(t, fv.validateJSONImportStructure([]byte(`"hello"`), res))
		require.True(t, res.Valid)
		require.Contains(t, res.Warnings, ErrJSONNotObjectOrArray.Error())
	})

	t.Run("deep_json_is_rejected", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.EnableVirusScan = false

		depth := 60
		payload := strings.Repeat("[", depth) + strings.Repeat("]", depth)
		res, err := fv.ValidateFile(context.Background(), []byte(payload), cfg)
		require.NoError(t, err)
		require.False(t, res.Valid)
		require.True(t, sliceContainsPrefix(res.Errors, ErrJSONStructureTooDeep.Error()))
		require.NotNil(t, res.Metadata["json_depth"])
	})

	t.Run("csv_inconsistent_rows_rejected", func(t *testing.T) {
		cfg := DefaultImportValidationConfig()
		cfg.AllowedTypes = []string{"text/csv"}
		cfg.RequiredFormats = []string{FormatCSV}
		cfg.EnableVirusScan = false

		res, err := fv.ValidateFile(context.Background(), []byte("a,b\n1\n2\n3"), cfg)
		require.NoError(t, err)
		require.False(t, res.Valid)
		require.Contains(t, res.Errors, ErrCSVTooManyInconsistentRows.Error())
		require.Contains(t, res.Warnings, ErrCSVHeaderMissingImportFields.Error())
	})
}

func TestFileValidationService_ValidateFile_VirusScan_Round24(t *testing.T) {
	fv := &FileValidationService{logger: zap.NewNop(), virusCheck: true}
	cfg := DefaultImportValidationConfig()
	cfg.AllowedTypes = []string{"text/csv"}
	cfg.RequiredFormats = []string{FormatCSV}
	cfg.ValidateContent = false

	data := append([]byte("a,b\n<script>\n1,2\n"), bytesRepeat(0x01, 50)...)
	res, err := fv.ValidateFile(context.Background(), data, cfg)
	require.NoError(t, err)
	require.False(t, res.Valid)
	require.True(t, sliceContainsPrefix(res.Warnings, ErrSuspiciousContentDetected.Error()))
	require.True(t, sliceContainsPrefix(res.Errors, ErrFileTooMuchBinaryContent.Error()))
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func sliceContainsPrefix(items []string, prefix string) bool {
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}
