package main

import (
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestActivityProcessor_ExtractLanguage_AdditionalBranches(t *testing.T) {
	ap := &ActivityProcessor{}

	// Summary hint present but invalid should fall back to content detection.
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{Summary: "[lang:english] hola"},
		Content:    " el la de ",
	}
	require.Equal(t, "es", ap.extractLanguage(note))

	// Summary hint missing closing bracket should also fall back.
	note = &activitypub.Note{
		BaseObject: activitypub.BaseObject{Summary: "[lang:es hola"},
		Content:    " le la de ",
	}
	require.Equal(t, "fr", ap.extractLanguage(note))

	// Non-Latin scripts.
	require.Equal(t, "zh", ap.extractLanguage(&activitypub.Note{Content: strings.Repeat("\u9FFF", 6)}))
	require.Equal(t, "ko", ap.extractLanguage(&activitypub.Note{Content: "안녕하세요"}))
	require.Equal(t, "ar", ap.extractLanguage(&activitypub.Note{Content: "مرحبا"}))
	require.Equal(t, "ru", ap.extractLanguage(&activitypub.Note{Content: "Привет"}))

	// Latin-script pattern detection.
	require.Equal(t, "es", ap.extractLanguage(&activitypub.Note{Content: " el la de "}))
	require.Equal(t, "fr", ap.extractLanguage(&activitypub.Note{Content: " le la de "}))
	require.Equal(t, "de", ap.extractLanguage(&activitypub.Note{Content: " der die und "}))
	require.Equal(t, "pt", ap.extractLanguage(&activitypub.Note{Content: " o a de "}))
	require.Equal(t, "it", ap.extractLanguage(&activitypub.Note{Content: " il la di "}))
}
