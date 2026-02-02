package common

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidationHelpers_ParseAndValidateBoolean(t *testing.T) {
	val, err := ParseAndValidateBoolean("")
	assert.NoError(t, err)
	assert.False(t, val)

	for _, in := range []string{StringTrue, "1", "yes", "on", " TRUE "} {
		val, err = ParseAndValidateBoolean(in)
		assert.NoError(t, err)
		assert.True(t, val)
	}

	for _, in := range []string{"false", "0", "no", "off"} {
		val, err = ParseAndValidateBoolean(in)
		assert.NoError(t, err)
		assert.False(t, val)
	}

	_, err = ParseAndValidateBoolean("maybe")
	assert.Error(t, err)
}

func TestValidationHelpers_RequiredAndEnumAndLengths(t *testing.T) {
	assert.Error(t, ValidateRequiredParam("x", ""))
	assert.NoError(t, ValidateRequiredParam("x", "ok"))

	err := ValidateMultipleRequiredParams(map[string]string{"a": "", "b": "ok", "c": ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "a")
	assert.Contains(t, err.Error(), "c")

	assert.Error(t, ValidateStringLength("f", "x", 2, 3))
	assert.Error(t, ValidateStringLength("f", "xxxx", 1, 3))
	assert.NoError(t, ValidateStringLength("f", "xx", 2, 3))

	assert.NoError(t, ValidateEnum("v", "", []string{"a"}))
	assert.NoError(t, ValidateEnum("v", "a", []string{"a", "b"}))
	assert.Error(t, ValidateEnum("v", "c", []string{"a", "b"}))
}

func TestValidationHelpers_ParseAndValidateIntWithBounds_AndLimits(t *testing.T) {
	got, err := ParseAndValidateIntWithBounds("n", "", 0, 10, 7)
	assert.NoError(t, err)
	assert.Equal(t, 7, got)

	_, err = ParseAndValidateIntWithBounds("n", "nope", 0, 10, 7)
	assert.Error(t, err)

	_, err = ParseAndValidateIntWithBounds("n", "0", 0, 10, 7)
	assert.Error(t, err)

	_, err = ParseAndValidateIntWithBounds("n", "11", 0, 10, 7)
	assert.Error(t, err)

	got, err = ParseAndValidateIntWithBounds("n", "10", 0, 10, 7)
	assert.NoError(t, err)
	assert.Equal(t, 10, got)

	got, err = ParseAndValidateAPILimit("", 10)
	assert.NoError(t, err)
	assert.Equal(t, 5, got) // maxLimit<=20 => default is max/2

	got, err = ParseTimelineLimit("")
	assert.NoError(t, err)
	assert.Equal(t, 20, got)
}

func TestValidationHelpers_SanitizeAndValidateString(t *testing.T) {
	out, err := ValidateAndSanitizeString("f", " \t hi \r\x00", 1, 10)
	assert.NoError(t, err)
	assert.Equal(t, "hi", out)

	_, err = ValidateAndSanitizeString("f", " \t ", 1, 10)
	assert.Error(t, err)
}

func TestValidationHelpers_IDsURLsAndTimestamps(t *testing.T) {
	assert.Error(t, ValidateUUID("id", ""))
	assert.Error(t, ValidateUUID("id", "nope"))
	assert.NoError(t, ValidateUUID("id", "550e8400-e29b-41d4-a716-446655440000"))

	assert.Error(t, ValidateNumericID("id", ""))
	assert.Error(t, ValidateNumericID("id", "abc"))
	assert.NoError(t, ValidateNumericID("id", "123"))

	assert.Error(t, ValidateAlphanumericID("id", ""))
	assert.Error(t, ValidateAlphanumericID("id", strings.Repeat("a", 101)))
	assert.Error(t, ValidateAlphanumericID("id", "bad space"))
	assert.NoError(t, ValidateAlphanumericID("id", "ok_123-ABC"))

	assert.NoError(t, ValidateTimestamp("", "ts"))
	assert.NoError(t, ValidateTimestamp(time.Now().UTC().Format(time.RFC3339), "ts"))
	assert.NoError(t, ValidateTimestamp("2024-01-01", "ts"))
	assert.Error(t, ValidateTimestamp("not-a-time", "ts"))

	assert.NoError(t, ValidateURL("", "u"))
	assert.Error(t, ValidateURL("ftp://example.com", "u"))
	assert.Error(t, ValidateURL("https://"+strings.Repeat("a", 2100), "u"))
	assert.NoError(t, ValidateURL("https://example.com/x", "u"))
}

func TestValidationHelpers_AccountIDsAndCursorAndSearchParsing(t *testing.T) {
	_, err := ValidateAccountIDsParameter("")
	assert.Error(t, err)

	ids, err := ValidateAccountIDsParameter("a, b,c")
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ids)

	_, err = ValidateAccountIDsParameter("a, ,c")
	assert.Error(t, err)

	assert.NoError(t, ValidateRepositoryCursor(""))
	assert.Error(t, ValidateRepositoryCursor(strings.Repeat("a", 501)))
	assert.Error(t, ValidateRepositoryCursor("not-base64"))

	validCursor := base64.StdEncoding.EncodeToString([]byte("cursor"))
	assert.NoError(t, ValidateRepositoryCursor(validCursor))

	got, err := ParseSearchOffset("")
	assert.NoError(t, err)
	assert.Equal(t, 0, got)

	got, err = ParseSearchOffset("5")
	assert.NoError(t, err)
	assert.Equal(t, 5, got)
}

func TestValidationHelpers_QueryValidationAndPreferences(t *testing.T) {
	assert.Error(t, ValidateQueryLimit(-1, 10, "x"))
	assert.Error(t, ValidateQueryLimit(11, 10, "x"))
	assert.NoError(t, ValidateQueryLimit(10, 10, "x"))

	assert.Error(t, ValidateQueryOffset(-1, 10))
	assert.Error(t, ValidateQueryOffset(11, 10))
	assert.NoError(t, ValidateQueryOffset(10, 10))

	assert.Error(t, ValidateQueryFilters(map[string]interface{}{"": "x"}))
	assert.Error(t, ValidateQueryFilters(map[string]interface{}{"k": nil}))
	assert.Error(t, ValidateQueryFilters(map[string]interface{}{"k": strings.Repeat("a", 501)}))
	assert.NoError(t, ValidateQueryFilters(map[string]interface{}{"k": "ok"}))

	assert.Error(t, ValidateSortParameters("bad", "asc", []string{"a", "b"}))
	assert.Error(t, ValidateSortParameters("a", "up", []string{"a", "b"}))
	assert.NoError(t, ValidateSortParameters("a", "desc", []string{"a", "b"}))

	assert.Error(t, ValidateRepositoryRelationship("", "b", "x"))
	assert.Error(t, ValidateRepositoryRelationship("a", "", "x"))
	assert.Error(t, ValidateRepositoryRelationship("a", "a", "x"))
	assert.NoError(t, ValidateRepositoryRelationship("a", "a", "self_reference"))

	assert.Error(t, ValidatePreferenceValue("", "x"))
	assert.Error(t, ValidatePreferenceValue("k", nil))
	assert.Error(t, ValidatePreferenceValue("k", strings.Repeat("a", 1001)))
	assert.NoError(t, ValidatePreferenceValue("k", map[string]interface{}{"nested": true}))
}

func TestValidationHelpers_SlicesEntitiesAndContent(t *testing.T) {
	assert.Error(t, ValidateSliceNotEmpty("s", "not-a-slice"))
	assert.Error(t, ValidateSliceNotEmpty("s", []string{}))
	assert.NoError(t, ValidateSliceNotEmpty("s", []string{"x"}))

	assert.Error(t, ValidateSliceLength("s", "not-a-slice", 1))
	assert.Error(t, ValidateSliceLength("s", []int{1, 2, 3}, 2))
	assert.NoError(t, ValidateSliceLength("s", []int{1, 2}, 2))

	assert.Error(t, ValidateMediaEntity("", "f", 1))
	assert.Error(t, ValidateMediaEntity("m1", "", 1))
	assert.Error(t, ValidateMediaEntity("m1", "f", 0))
	assert.Error(t, ValidateMediaEntity("m1", "f", 101*1024*1024))
	assert.NoError(t, ValidateMediaEntity("m1", "f", 1))

	assert.Error(t, ValidateContentOrAttachments("", nil))
	assert.NoError(t, ValidateContentOrAttachments("x", nil))
	assert.NoError(t, ValidateContentOrAttachments("", []string{"a"}))
}

func TestGetEnvInt(t *testing.T) {
	const key = "LESSER_TEST_ENV_INT"
	t.Cleanup(func() { _ = os.Unsetenv(key) })

	assert.Equal(t, 7, GetEnvInt(key, 7))

	_ = os.Setenv(key, "12")
	assert.Equal(t, 12, GetEnvInt(key, 7))

	_ = os.Setenv(key, "nope")
	assert.Equal(t, 7, GetEnvInt(key, 7))
}
