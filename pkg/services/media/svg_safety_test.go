package media

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSVGUpload(t *testing.T) {
	t.Parallel()

	t.Run("allows inert svg", func(t *testing.T) {
		err := ValidateSVGUpload("image/svg+xml; charset=utf-8", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10z"/></svg>`))
		require.NoError(t, err)
	})

	t.Run("ignores non svg media", func(t *testing.T) {
		err := ValidateSVGUpload("image/png", []byte(`<script>alert(1)</script>`))
		require.NoError(t, err)
	})

	t.Run("rejects script content", func(t *testing.T) {
		err := ValidateSVGUpload("image/svg+xml", []byte(`<svg><script>alert(1)</script></svg>`))
		require.True(t, errors.Is(err, ErrMediaUnsafeSVG))
	})

	t.Run("rejects event handlers", func(t *testing.T) {
		err := ValidateSVGUpload("image/svg+xml", []byte(`<svg onload="alert(1)"></svg>`))
		require.True(t, errors.Is(err, ErrMediaUnsafeSVG))
	})

	t.Run("rejects external references", func(t *testing.T) {
		err := ValidateSVGUpload("image/svg+xml", []byte(`<svg><a href="https://evil.example/x">x</a></svg>`))
		require.True(t, errors.Is(err, ErrMediaUnsafeSVG))
	})
}
