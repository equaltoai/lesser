package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
)

type articleResolver struct{ *Resolver }

// RenderedHTML returns the canonical sanitized presentation form of the stored
// Article source, composing the article's persisted editorial media.
func (r *articleResolver) RenderedHTML(ctx context.Context, obj *model.Article) (*string, error) {
	if obj == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	format := obj.RawContentFormat
	if strings.TrimSpace(format) == "" {
		format = string(obj.ContentFormat)
	}
	rendered, err := cmsrender.RenderArticleContentWithMedia(obj.Content, format, cmsGraphArticleRenderMedia(obj.RawEditorialMedia))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &rendered.HTML, nil
}

// cmsGraphArticleRenderMedia maps a stored article's persisted editorial
// bindings onto the canonical renderer's descriptors. Only inline media
// composes into published article HTML (the hero lives on featuredImage and
// composes only in previews); a binding without a minted URL is a fail-closed
// skip: publish only mints digest-verified assets.
func cmsGraphArticleRenderMedia(media []storageModels.ArticleEditorialMedia) []cmsrender.ArticleMedia {
	if len(media) == 0 {
		return nil
	}
	out := make([]cmsrender.ArticleMedia, 0, len(media))
	for _, m := range media {
		if m.Role != storageModels.EditorialMediaRoleInline {
			continue
		}
		render := m.RenderMedia()
		if strings.TrimSpace(render.URL) == "" {
			continue
		}
		out = append(out, render)
	}
	return out
}
