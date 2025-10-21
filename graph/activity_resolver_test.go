package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestActivityResolverObjectConvertsStatus(t *testing.T) {
	resolver := &Resolver{}
	ar := &activityResolver{Resolver: resolver}

	now := time.Now()
	status := &models.Status{
		StatusID:   "status-123",
		Content:    "Hello from Lesser",
		Visibility: VisibilityPublic,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	activity := &activitypub.Activity{
		Object: status,
	}

	obj, err := ar.Object(context.Background(), activity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj == nil {
		t.Fatal("expected object to be returned, got nil")
	}
	if obj.ID != status.StatusID {
		t.Fatalf("expected object ID %q, got %q", status.StatusID, obj.ID)
	}
	if obj.Content != status.Content {
		t.Fatalf("expected content %q, got %q", status.Content, obj.Content)
	}
	if obj.Visibility != model.VisibilityPublic {
		t.Fatalf("expected visibility %q, got %q", model.VisibilityPublic, obj.Visibility)
	}
}
