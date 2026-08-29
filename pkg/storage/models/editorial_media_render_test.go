package models

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/stretchr/testify/require"
)

func TestArticleEditorialMediaRenderMedia(t *testing.T) {
	position := 2
	m := ArticleEditorialMedia{
		MediaID:        "m1",
		Role:           EditorialMediaRoleInline,
		InlinePosition: &position,
		Caption:        " caption ",
		CreditLine:     "credit",
		AltText:        "alt",
		URL:            " https://cdn.example.test/a.png ",
		ContentType:    "image/png",
		ContentHash:    "sha256:abc",
		Width:          640,
		Height:         480,
	}
	render := m.RenderMedia()
	require.Equal(t, cmsrender.ArticleMediaRoleInline, render.Role)
	require.Equal(t, 2, render.InlinePosition)
	require.Equal(t, "https://cdn.example.test/a.png", render.URL)
	require.Equal(t, "caption", render.Caption)
	require.Equal(t, "credit", render.CreditLine)
	require.Equal(t, 640, render.Width)
	require.Equal(t, 480, render.Height)
	require.Equal(t, "image/png", render.ContentType)

	// Zero and nil positions clamp to the leading slot; roles pass through.
	nilPosition := ArticleEditorialMedia{Role: EditorialMediaRoleHero, URL: "https://cdn.example.test/h.png"}
	require.Equal(t, 0, nilPosition.RenderMedia().InlinePosition)
	require.Equal(t, cmsrender.ArticleMediaRoleHero, nilPosition.RenderMedia().Role)
}

func TestArticleRenderMediaListComposesInlineOnly(t *testing.T) {
	position := 1
	article := &Article{
		EditorialMedia: []ArticleEditorialMedia{
			{MediaID: "hero", Role: EditorialMediaRoleHero, URL: "https://cdn.example.test/hero.png"},
			{MediaID: "inline", Role: EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/inline.png"},
			{MediaID: "card", Role: EditorialMediaRoleSocialCard, URL: "https://cdn.example.test/card.png"},
			{MediaID: "empty", Role: EditorialMediaRoleInline, InlinePosition: &position, URL: " "},
		},
	}
	media := article.RenderMediaList()
	require.Len(t, media, 1)
	require.Equal(t, cmsrender.ArticleMediaRoleInline, media[0].Role)
	require.Equal(t, "https://cdn.example.test/inline.png", media[0].URL)

	// No inline bindings (or no article) yields no media.
	require.Nil(t, (&Article{}).RenderMediaList())
	require.Nil(t, (*Article)(nil).RenderMediaList())
}

func TestArticleAPAttachmentsSkipsSocialAndEmptyServing(t *testing.T) {
	article := &Article{
		EditorialMedia: []ArticleEditorialMedia{
			{MediaID: "hero", Role: EditorialMediaRoleHero, URL: "https://cdn.example.test/hero.png", ContentType: "image/png", AltText: "hero", Width: 1200, Height: 630},
			{MediaID: "inline", Role: EditorialMediaRoleInline, URL: "https://cdn.example.test/inline.png", ContentType: "image/jpeg", AltText: "inline"},
			{MediaID: "card", Role: EditorialMediaRoleSocialCard, URL: "https://cdn.example.test/card.png", ContentType: "image/png"},
			{MediaID: "empty", Role: EditorialMediaRoleInline, URL: " "},
		},
	}
	attachments := article.APAttachments()
	require.Len(t, attachments, 2)
	require.Equal(t, "Document", attachments[0].Type)
	require.Equal(t, "https://cdn.example.test/hero.png", attachments[0].URL)
	require.Equal(t, "hero", attachments[0].Name)
	require.Equal(t, "image/png", attachments[0].MediaType)
	require.Equal(t, 1200, attachments[0].Width)
	require.Equal(t, 630, attachments[0].Height)
	require.Equal(t, "https://cdn.example.test/inline.png", attachments[1].URL)
}
