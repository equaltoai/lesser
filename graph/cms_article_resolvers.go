package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/cmsrender"
)

type articleResolver struct{ *Resolver }

// RenderedHTML returns the canonical sanitized presentation form of the stored Article source.
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
	rendered, err := cmsrender.RenderArticleContent(obj.Content, format)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &rendered.HTML, nil
}
