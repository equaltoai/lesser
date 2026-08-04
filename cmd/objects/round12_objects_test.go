package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	dynamormSchema "github.com/theory-cloud/tabletheory/v3/pkg/schema"
	pkgtypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
	"go.uber.org/zap"
)

type fakeObjectRepo struct {
	obj        any
	err        error
	gotID      string
	tombstoned bool
	tombErr    error
	tombstone  *storageModels.Tombstone
}

func (f *fakeObjectRepo) GetObject(_ context.Context, id string) (any, error) {
	f.gotID = id
	if f.err != nil {
		return nil, f.err
	}
	return f.obj, nil
}

func (f *fakeObjectRepo) GetTombstone(_ context.Context, id string) (*storageModels.Tombstone, error) {
	f.gotID = id
	if f.tombErr != nil {
		return nil, f.tombErr
	}
	if f.tombstone == nil {
		return nil, errors.New("not found")
	}
	return f.tombstone, nil
}

func (f *fakeObjectRepo) IsTombstoned(_ context.Context, id string) (bool, error) {
	f.gotID = id
	if f.tombErr != nil {
		return false, f.tombErr
	}
	return f.tombstoned, nil
}

type fakeAuthorizedFetch struct {
	enabled   bool
	verifyErr error
	actor     *activitypub.Actor
	calls     int
}

func (f *fakeAuthorizedFetch) IsAuthorizedFetchEnabled(context.Context) bool { return f.enabled }

func (f *fakeAuthorizedFetch) VerifyAuthorizedFetch(context.Context, *http.Request) (*activitypub.Actor, error) {
	f.calls++
	return f.actor, f.verifyErr
}

type fakeRelationshipChecker struct {
	following map[string]bool
	err       error
	calls     int
}

func (f *fakeRelationshipChecker) IsFollowing(_ context.Context, followerUsername, targetActorID string) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	return f.following[followerUsername+"|"+targetActorID], nil
}

type fakeInstanceRepo struct {
	state *storageModels.InstanceState
	err   error
}

func (f *fakeInstanceRepo) GetInstanceState(context.Context) (*storageModels.InstanceState, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.state, nil
}

func TestHandleGetObject_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	t.Run("missing object id", func(t *testing.T) {
		h := &Handler{}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{Method: http.MethodGet, Path: "/objects/"},
			Params:  map[string]string{},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 422, resp.Status)
	})

	t.Run("locked when instance state lookup fails", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{err: errors.New("db down")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("locked when instance is locked", func(t *testing.T) {
		h := &Handler{
			instanceRepo: &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: true}},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("authorized fetch conversion failure returns 400", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: "bad method",
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
	})

	t.Run("authorized fetch invalid signature returns 403", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("bad signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept":    {"application/activity+json"},
					"host":      {"example.com"},
					"signature": {"sig"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 403, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401 when accept is missing", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"host": {"example.com"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
	})

	t.Run("authorized fetch missing signature returns 401 for accept */*", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/123",
				Headers: map[string][]string{
					"accept": {"*/*"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
	})
}

func TestHandleGetObject_FetchResponses_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	instanceRepo := &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}}

	t.Run("not found error returns 404", func(t *testing.T) {
		objRepo := &fakeObjectRepo{err: errors.New("not found")}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
		require.Contains(t, objRepo.gotID, "https://example.com/objects/123")
	})

	t.Run("internal fetch error returns 500", func(t *testing.T) {
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{err: errors.New("boom")},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/https://example.com/objects/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "https://example.com/objects/123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("html response for browsers", func(t *testing.T) {
		now := time.Now().UTC()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/objects/123",
				Type:      "Note",
				Summary:   "cw",
				Sensitive: true,
				Published: &now,
				Updated:   &now,
				To:        []string{activitypub.PublicAddress},
			},
			Content:      "Hello <b>world</b>",
			AttributedTo: "https://example.com/users/alice",
			Attachment: []activitypub.Attachment{
				{Type: "Image", URL: "https://example.com/img.png", Name: "img"},
			},
			Tag: []activitypub.Tag{
				{Type: "Hashtag", Href: "https://example.com/tags/cats", Name: "#cats"},
			},
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"text/html; charset=utf-8"}, resp.Headers["content-type"])
		body := string(resp.Body)
		require.Contains(t, body, "<!DOCTYPE html>")
		require.Contains(t, body, "Content Warning")
		require.Contains(t, body, "@alice")
	})

	t.Run("authorized fetch enabled suppresses HTML for non-public objects", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/objects/123",
				Type: "Note",
			},
			Content:      "secret",
			AttributedTo: "https://example.com/users/alice",
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 404, resp.Status)
	})

	t.Run("authorized fetch enabled still allows HTML for public objects", func(t *testing.T) {
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/objects/123",
				Type: "Note",
				To:   []string{activitypub.PublicAddress},
			},
			Content:      "hello",
			AttributedTo: "https://example.com/users/alice",
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"text/html; charset=utf-8"}, resp.Headers["content-type"])
		require.Contains(t, string(resp.Body), "<!DOCTYPE html>")
	})

	t.Run("activitypub json response", func(t *testing.T) {
		objRepo := &fakeObjectRepo{obj: map[string]any{
			"id":   "x",
			"type": "Note",
			"to":   []any{activitypub.PublicAddress},
			"bto":  []any{"https://remote.example/users/hidden"},
			"bcc":  []any{"https://remote.example/users/also-hidden"},
		}}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "123"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotContains(t, body, "bto")
		require.NotContains(t, body, "bcc")
	})

	t.Run("canonical article route resolves article url and negotiates activitypub json", func(t *testing.T) {
		now := time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC)
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/articles/hello-world",
					Type:      activitypub.ArticleType,
					Summary:   "A short summary",
					Published: &now,
					Updated:   &now,
					To:        []string{activitypub.PublicAddress},
					BTo:       []string{"https://remote.example/users/hidden"},
					BCC:       []string{"https://remote.example/users/also-hidden"},
				},
				Content:      "<p>Article body</p>",
				AttributedTo: "https://example.com/users/alice",
				Attachment: []activitypub.Attachment{
					{Type: "Image", MediaType: "image/png", URL: "https://example.com/media/cover.png", Name: "cover"},
				},
				Tag: []activitypub.Tag{
					{Type: "Hashtag", Href: "https://example.com/tags/cms", Name: "#cms"},
				},
			},
			Name: "Hello World",
		}

		objRepo := &fakeObjectRepo{obj: article}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/articles/hello-world",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"slug": "hello-world"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])
		require.Equal(t, "https://example.com/articles/hello-world", objRepo.gotID)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "https://example.com/articles/hello-world", body["id"])
		require.Equal(t, activitypub.ArticleType, body["type"])
		require.Equal(t, "Hello World", body["name"])
		require.Equal(t, "A short summary", body["summary"])
		require.Equal(t, "<p>Article body</p>", body["content"])
		require.Equal(t, "https://example.com/users/alice", body["attributedTo"])
		require.NotContains(t, body, "bto")
		require.NotContains(t, body, "bcc")
		require.NotEmpty(t, body["attachment"])
		require.NotEmpty(t, body["tag"])
	})

	t.Run("canonical article route returns html for public article", func(t *testing.T) {
		now := time.Now().UTC()
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/articles/hello-world",
					Type:      activitypub.ArticleType,
					Summary:   "A summary",
					Published: &now,
					To:        []string{activitypub.PublicAddress},
				},
				Content:      `<h2>Rendered</h2><script>alert(1)</script><p onclick="evil()">safe body</p>`,
				AttributedTo: "https://example.com/users/alice",
			},
			Name: "Hello World",
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: article},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/articles/hello-world",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"slug": "hello-world"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, []string{"text/html; charset=utf-8"}, resp.Headers["content-type"])
		require.Contains(t, string(resp.Body), "Hello World")
		require.Contains(t, string(resp.Body), "A summary")
		require.Contains(t, string(resp.Body), "<h2>Rendered</h2>")
		require.Contains(t, string(resp.Body), "<p>safe body</p>")
		require.NotContains(t, string(resp.Body), "<script")
		require.NotContains(t, string(resp.Body), "onclick")
	})

	t.Run("canonical article route suppresses html for non-public article", func(t *testing.T) {
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/articles/private-article",
					Type: activitypub.ArticleType,
					To:   []string{"https://example.com/users/alice/followers"},
				},
				Content:      "followers only",
				AttributedTo: "https://example.com/users/alice",
			},
			Name: "Private Article",
		}

		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: article},
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/articles/private-article",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"slug": "private-article"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("canonical status route resolves published status url", func(t *testing.T) {
		now := time.Now().UTC()
		note := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://example.com/users/alice/statuses/123",
				Type:      activitypub.NoteType,
				Published: &now,
				To:        []string{activitypub.PublicAddress},
			},
			Content:      "hello from canonical route",
			AttributedTo: "https://example.com/users/alice",
		}

		objRepo := &fakeObjectRepo{obj: note}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice/statuses/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{
				"username": "alice",
				"id":       "123",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 200, resp.Status)
		require.Equal(t, "https://example.com/users/alice/statuses/123", objRepo.gotID)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "https://example.com/users/alice/statuses/123", body["id"])
	})

	t.Run("canonical status route returns authorized fetch error when no hidden tombstone exists", func(t *testing.T) {
		objRepo := &fakeObjectRepo{obj: map[string]any{"id": "https://example.com/users/alice/statuses/123", "type": "Note"}}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true, verifyErr: errors.New("missing signature")},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/123",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{
				"username": "alice",
				"id":       "123",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
		require.Equal(t, "https://example.com/users/alice/statuses/123", objRepo.gotID)
	})

	t.Run("canonical status route returns tombstone when object was deleted", func(t *testing.T) {
		deletedAt := time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC)
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           "https://example.com/users/alice/statuses/123",
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice/statuses/123",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{
				"username": "alice",
				"id":       "123",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Tombstone", body["type"])
		require.Equal(t, "https://example.com/users/alice/statuses/123", body["id"])
		require.Equal(t, activitypub.NoteType, body["formerType"])
	})

	t.Run("canonical article route returns article tombstone when deleted", func(t *testing.T) {
		deletedAt := time.Date(2026, time.May, 19, 14, 30, 0, 0, time.UTC)
		articleID := "https://example.com/articles/deleted-article"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           articleID,
				FormerType:   activitypub.ArticleType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/articles/deleted-article",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"slug": "deleted-article"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])
		require.Equal(t, articleID, objRepo.gotID)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Tombstone", body["type"])
		require.Equal(t, articleID, body["id"])
		require.Equal(t, activitypub.ArticleType, body["formerType"])
		require.Equal(t, deletedAt.Format(time.RFC3339), body["deleted"])
	})

	t.Run("canonical article tombstone wins over stale object row", func(t *testing.T) {
		deletedAt := time.Date(2026, time.May, 19, 14, 35, 0, 0, time.UTC)
		articleID := "https://example.com/articles/stale-deleted-article"
		objRepo := &fakeObjectRepo{
			obj: &activitypub.Article{
				Note: activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:   articleID,
						Type: activitypub.ArticleType,
						To:   []string{activitypub.PublicAddress},
					},
					Content:      "<p>stale row</p>",
					AttributedTo: "https://example.com/users/alice",
				},
				Name: "Stale Deleted Article",
			},
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           articleID,
				FormerType:   activitypub.ArticleType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/articles/stale-deleted-article",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"slug": "stale-deleted-article"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Tombstone", body["type"])
		require.Equal(t, articleID, body["id"])
		require.Equal(t, activitypub.ArticleType, body["formerType"])
	})

	t.Run("legacy object article route returns article tombstone when deleted", func(t *testing.T) {
		deletedAt := time.Date(2026, time.May, 19, 14, 45, 0, 0, time.UTC)
		legacyID := "https://example.com/objects/legacy-article-id"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           legacyID,
				FormerType:   activitypub.ArticleType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/legacy-article-id",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "legacy-article-id"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
		require.Equal(t, legacyID, objRepo.gotID)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Tombstone", body["type"])
		require.Equal(t, legacyID, body["id"])
		require.Equal(t, activitypub.ArticleType, body["formerType"])
	})
}

func TestExtractObjectData_Round12(t *testing.T) {
	h := &Handler{}

	data := h.extractObjectData(123)
	require.Equal(t, "Object", data.objectType)

	now := time.Now().UTC()
	data = h.extractObjectData(map[string]any{
		"type":         "Article",
		"id":           "https://example.com/objects/1",
		"content":      "<p>content</p>",
		"name":         "Title",
		"summary":      "Summary",
		"published":    now.Format(time.RFC3339),
		"updated":      now,
		"sensitive":    true,
		"attributedTo": "https://example.com/users/alice",
		"attachment":   []map[string]any{{"type": "Image", "url": "https://example.com/img.png", "name": "img"}},
		"tag":          []map[string]any{{"type": "Hashtag", "href": "https://example.com/tags/cats", "name": "#cats"}},
	})
	require.Equal(t, "Article", data.objectType)
	require.Equal(t, "Title", data.name)
	require.True(t, data.sensitive)
	require.Len(t, data.attachments, 1)
	require.Len(t, data.tags, 1)

	now = time.Now().UTC()
	data = h.extractObjectData(&activitypub.Article{
		Note: activitypub.Note{
			BaseObject: activitypub.BaseObject{
				Type:      activitypub.ArticleType,
				ID:        "https://example.com/articles/1",
				Summary:   "article summary",
				Published: &now,
				To:        []string{activitypub.PublicAddress},
			},
			Content:      "<p>article body</p>",
			AttributedTo: "https://example.com/users/alice",
			Attachment: []activitypub.Attachment{
				{Type: "Image", URL: "https://example.com/cover.png"},
			},
			Tag: []activitypub.Tag{
				{Type: "Hashtag", Name: "#cms", Href: "https://example.com/tags/cms"},
			},
		},
		Name: "Article Title",
	})
	require.Equal(t, activitypub.ArticleType, data.objectType)
	require.Equal(t, "Article Title", data.name)
	require.Equal(t, "<p>article body</p>", data.content)
	require.Equal(t, "article summary", data.summary)
	require.Len(t, data.attachments, 1)
	require.Len(t, data.tags, 1)

	data = h.extractObjectData(&activitypub.Article{
		Note: activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/articles/untyped"},
		},
		Name: "Untyped Article",
	})
	require.Equal(t, activitypub.ArticleType, data.objectType)
	require.Equal(t, "Untyped Article", data.name)
}

type extendedMockDB struct {
	inner *dynamormMocks.MockDB
}

var _ dynamormCore.ExtendedDB = (*extendedMockDB)(nil)

func (db *extendedMockDB) Model(model any) dynamormCore.Query {
	return db.inner.Model(model)
}

func (db *extendedMockDB) Migrate() error { return nil }

func (db *extendedMockDB) AutoMigrate(models ...any) error { return nil }

func (db *extendedMockDB) Close() error { return nil }

func (db *extendedMockDB) WithContext(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) AutoMigrateWithOptions(_ any, _ ...dynamormSchema.AutoMigrateOption) error {
	return nil
}

func (db *extendedMockDB) RegisterTypeConverter(_ reflect.Type, _ pkgtypes.CustomConverter) error {
	return nil
}

func (db *extendedMockDB) CreateTable(_ any, _ ...dynamormSchema.TableOption) error { return nil }

func (db *extendedMockDB) EnsureTable(_ any) error { return nil }

func (db *extendedMockDB) DeleteTable(_ any) error { return nil }

func (db *extendedMockDB) DescribeTable(_ any) (any, error) { return nil, nil }

func (db *extendedMockDB) WithLambdaTimeout(_ context.Context) dynamormCore.DB { return db }

func (db *extendedMockDB) WithLambdaTimeoutBuffer(_ time.Duration) dynamormCore.DB { return db }

func (db *extendedMockDB) Transact() dynamormCore.TransactionBuilder { return nil }

func (db *extendedMockDB) TransactWrite(_ context.Context, fn func(dynamormCore.TransactionBuilder) error) error {
	return fn(nil)
}

func TestNewHandler_MainAndInit_Round12(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	origRepos := repos
	origLambdaCtx := lambdaCtx
	origMust := mustInitializeLambdaFn
	origDefaults := initializeWithDefaultsFn
	origStart := lambdaStartFn
	origNewHandler := newHandlerFn
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
		repos = origRepos
		lambdaCtx = origLambdaCtx
		mustInitializeLambdaFn = origMust
		initializeWithDefaultsFn = origDefaults
		lambdaStartFn = origStart
		newHandlerFn = origNewHandler
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	innerDB := new(dynamormMocks.MockDB)
	db := &extendedMockDB{inner: innerDB}
	repoFactory, err := factory.NewRepositoryFactory(db, "test-table", zap.NewNop())
	require.NoError(t, err)
	repos = repoFactory

	t.Run("NewHandler wires dependencies", func(t *testing.T) {
		h := NewHandler()
		require.NotNil(t, h.objectRepo)
		require.NotNil(t, h.authorizedFetchService)
		require.NotNil(t, h.instanceRepo)
		require.NotNil(t, h.relationshipRepo)
	})

	t.Run("initializeObjects + main register routes", func(t *testing.T) {
		mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
			return &common.LambdaContext{
				Config:    cfg,
				Logger:    nil,
				Repos:     repos,
				StartTime: time.Now(),
			}
		}
		initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }

		initializeObjects()
		require.NotNil(t, lambdaCtx)
		require.NotNil(t, repos)

		fakeRepo := &fakeObjectRepo{obj: map[string]any{"id": "x", "type": "Note", "to": []any{activitypub.PublicAddress}}}
		newHandlerFn = func() *Handler {
			return &Handler{
				objectRepo:             fakeRepo,
				authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
				instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
			}
		}

		called := false
		lambdaStartFn = func(handler any) {
			called = true
			fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
			require.True(t, ok)

			event := events.APIGatewayV2HTTPRequest{
				Version:  "2.0",
				RouteKey: "GET /objects/123",
				RawPath:  "/objects/123",
				Headers:  map[string]string{"accept": "application/activity+json"},
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					RequestID: "req",
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: "GET",
						Path:   "/objects/123",
					},
				},
			}

			raw, err := json.Marshal(event)
			require.NoError(t, err)
			resp, err := fn(context.Background(), raw)
			require.NoError(t, err)
			lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
			require.True(t, ok)
			require.Equal(t, 200, lambdaResp.StatusCode)
			require.Equal(t, "application/activity+json", lambdaResp.Headers["content-type"])
		}

		main()
		require.True(t, called)
		require.Contains(t, fakeRepo.gotID, "https://example.com/objects/123")
	})

	t.Run("build app serves canonical status routes", func(t *testing.T) {
		fakeRepo := &fakeObjectRepo{obj: map[string]any{
			"id":   "https://example.com/users/alice/statuses/123",
			"type": "Note",
			"to":   []any{activitypub.PublicAddress},
		}}
		app := buildApp(&Handler{
			objectRepo:             fakeRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}, zap.NewNop())

		event := events.APIGatewayV2HTTPRequest{
			Version:  "2.0",
			RouteKey: "GET /users/alice/statuses/123",
			RawPath:  "/users/alice/statuses/123",
			Headers:  map[string]string{"accept": "application/activity+json"},
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				RequestID: "req-canonical",
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: "GET",
					Path:   "/users/alice/statuses/123",
				},
			},
		}

		raw, err := json.Marshal(event)
		require.NoError(t, err)

		resp, err := app.HandleLambda(context.Background(), raw)
		require.NoError(t, err)
		lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		require.Equal(t, 200, lambdaResp.StatusCode)
		require.Equal(t, "https://example.com/users/alice/statuses/123", fakeRepo.gotID)
		require.Equal(t, "application/activity+json", lambdaResp.Headers["content-type"])
	})

	t.Run("build app serves canonical article federation probe", func(t *testing.T) {
		now := time.Date(2026, time.May, 19, 15, 0, 0, 0, time.UTC)
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/articles/probe-article",
					Type:      activitypub.ArticleType,
					Summary:   "Probe summary",
					Published: &now,
					Updated:   &now,
					To:        []string{activitypub.PublicAddress},
				},
				Content:      "<p>probe body</p>",
				AttributedTo: "https://example.com/users/alice",
			},
			Name: "Probe Article",
		}
		fakeRepo := &fakeObjectRepo{obj: article}
		app := buildApp(&Handler{
			objectRepo:             fakeRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
			instanceRepo:           &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}},
		}, zap.NewNop())

		event := events.APIGatewayV2HTTPRequest{
			Version:  "2.0",
			RouteKey: "GET /articles/probe-article",
			RawPath:  "/articles/probe-article",
			Headers:  map[string]string{"accept": "application/activity+json"},
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				RequestID: "req-article-probe",
				HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
					Method: "GET",
					Path:   "/articles/probe-article",
				},
			},
		}

		raw, err := json.Marshal(event)
		require.NoError(t, err)

		resp, err := app.HandleLambda(context.Background(), raw)
		require.NoError(t, err)
		lambdaResp, ok := resp.(events.APIGatewayV2HTTPResponse)
		require.True(t, ok)
		require.Equal(t, http.StatusOK, lambdaResp.StatusCode)
		require.Equal(t, "https://example.com/articles/probe-article", fakeRepo.gotID)
		require.Equal(t, "application/activity+json", lambdaResp.Headers["content-type"])

		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(lambdaResp.Body), &body))
		require.Equal(t, "https://example.com/articles/probe-article", body["id"])
		require.Equal(t, activitypub.ArticleType, body["type"])
		require.Equal(t, "Probe Article", body["name"])
		require.Equal(t, "Probe summary", body["summary"])
		require.Equal(t, "<p>probe body</p>", body["content"])
		t.Logf("article federation probe url=%s type=%s content_type=%s", body["id"], body["type"], lambdaResp.Headers["content-type"])
	})
}

func TestHandleGetObject_PrivateVisibilityGate_Round29(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	instanceRepo := &fakeInstanceRepo{state: &storageModels.InstanceState{Locked: false}}
	authorID := "https://example.com/users/alice"
	followersCollection := authorID + "/followers"
	privateNote := func() *activitypub.Note {
		return &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice/statuses/private-1",
				Type: activitypub.NoteType,
				To:   []string{followersCollection},
			},
			Content:      "followers only",
			AttributedTo: authorID,
		}
	}
	requestCtx := func() *apptheory.Context {
		return &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/private-1",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{
				"username": "alice",
				"id":       "private-1",
			},
		}
	}

	t.Run("non-public canonical object is hidden from unsigned fetch even when authorized fetch is globally disabled", func(t *testing.T) {
		authFetch := &fakeAuthorizedFetch{enabled: false, verifyErr: errors.New("missing signature")}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: privateNote()},
			authorizedFetchService: authFetch,
		}

		resp, err := h.HandleGetObject(requestCtx())

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
		require.Equal(t, 1, authFetch.calls)
	})

	t.Run("signed unrelated actor remains hidden from followers-only object", func(t *testing.T) {
		authFetch := &fakeAuthorizedFetch{
			enabled: true,
			actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/charlie"},
				PreferredUsername: "charlie",
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: privateNote()},
			authorizedFetchService: authFetch,
			relationshipRepo:       &fakeRelationshipChecker{},
		}

		resp, err := h.HandleGetObject(requestCtx())

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
		require.Equal(t, 1, authFetch.calls)
	})

	t.Run("accepted follower can fetch followers-only object", func(t *testing.T) {
		actorID := "https://remote.example/users/bob"
		authFetch := &fakeAuthorizedFetch{
			enabled: true,
			actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: actorID},
				PreferredUsername: "bob",
			},
		}
		relationships := &fakeRelationshipChecker{
			following: map[string]bool{actorID + "|" + authorID: true},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             &fakeObjectRepo{obj: privateNote()},
			authorizedFetchService: authFetch,
			relationshipRepo:       relationships,
		}

		resp, err := h.HandleGetObject(requestCtx())

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])
		require.Equal(t, 1, relationships.calls)
	})

	t.Run("accepted follower can fetch followers-only article route", func(t *testing.T) {
		actorID := "https://remote.example/users/bob"
		authFetch := &fakeAuthorizedFetch{
			enabled: true,
			actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: actorID},
				PreferredUsername: "bob",
			},
		}
		relationships := &fakeRelationshipChecker{
			following: map[string]bool{actorID + "|" + authorID: true},
		}
		article := &activitypub.Article{
			Note: activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/articles/private-article",
					Type: activitypub.ArticleType,
					To:   []string{followersCollection},
				},
				Content:      "followers only article",
				AttributedTo: authorID,
			},
			Name: "Private Article",
		}
		objRepo := &fakeObjectRepo{obj: article}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: authFetch,
			relationshipRepo:       relationships,
		}

		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/articles/private-article",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"slug": "private-article"},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, "https://example.com/articles/private-article", objRepo.gotID)
		require.Equal(t, []string{"application/activity+json"}, resp.Headers["content-type"])
		require.Equal(t, 1, relationships.calls)
	})

	t.Run("explicitly addressed actor can fetch non-public object without follower lookup", func(t *testing.T) {
		actorID := "https://remote.example/users/bob"
		note := privateNote()
		note.To = []string{actorID}
		relationships := &fakeRelationshipChecker{}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   &fakeObjectRepo{obj: note},
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled: true,
				actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: actorID},
					PreferredUsername: "bob",
				},
			},
			relationshipRepo: relationships,
		}

		resp, err := h.HandleGetObject(requestCtx())

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, 0, relationships.calls)
	})

	t.Run("author can fetch own non-public object", func(t *testing.T) {
		relationships := &fakeRelationshipChecker{}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   &fakeObjectRepo{obj: privateNote()},
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled: true,
				actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: authorID},
					PreferredUsername: "alice",
				},
			},
			relationshipRepo: relationships,
		}

		resp, err := h.HandleGetObject(requestCtx())

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusOK, resp.Status)
		require.Equal(t, 0, relationships.calls)
	})
}

func TestObjectAuthorizationHelpers_Round29(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})
	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	authorID := "https://example.com/users/alice"
	bobID := "https://remote.example/users/bob"
	publicNote := &activitypub.Note{
		BaseObject:   activitypub.BaseObject{ID: "public", Type: activitypub.NoteType, To: []string{activitypub.PublicAddress}},
		AttributedTo: authorID,
	}
	privateNote := &activitypub.Note{
		BaseObject:   activitypub.BaseObject{ID: "private", Type: activitypub.NoteType, To: []string{authorID + "/followers"}},
		AttributedTo: authorID,
	}

	t.Run("authorized actor helper covers deny and allow branches", func(t *testing.T) {
		h := &Handler{}
		allowed, err := h.authorizedActorCanFetchObject(context.Background(), publicNote, nil)
		require.NoError(t, err)
		require.True(t, allowed)

		allowed, err = h.authorizedActorCanFetchObject(context.Background(), privateNote, nil)
		require.NoError(t, err)
		require.False(t, allowed)

		allowed, err = h.authorizedActorCanFetchObject(context.Background(), privateNote, &activitypub.Actor{})
		require.NoError(t, err)
		require.False(t, allowed)

		allowed, err = h.authorizedActorCanFetchObject(context.Background(), privateNote, &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: authorID + "/"},
		})
		require.NoError(t, err)
		require.True(t, allowed)

		allowed, err = h.authorizedActorCanFetchObject(context.Background(), map[string]any{
			"id":           "direct",
			"type":         "Note",
			"attributedTo": authorID,
			"to":           []any{bobID},
		}, &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: bobID}})
		require.NoError(t, err)
		require.True(t, allowed)

		allowed, err = h.authorizedActorCanFetchObject(context.Background(), privateNote, &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: bobID},
		})
		require.NoError(t, err)
		require.False(t, allowed)

		relErr := errors.New("relationship down")
		allowed, err = (&Handler{relationshipRepo: &fakeRelationshipChecker{err: relErr}}).
			authorizedActorCanFetchObject(context.Background(), privateNote, &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: bobID},
			})
		require.ErrorIs(t, err, relErr)
		require.False(t, allowed)
	})

	t.Run("authorized fetch helper covers response branches", func(t *testing.T) {
		reqCtx := &apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/private",
				Headers: map[string][]string{"host": {"example.com"}},
			},
		}

		actor, resp, err := (&Handler{}).verifyObjectAuthorizedFetch(context.Background(), reqCtx, "private", "req", true)
		require.NoError(t, err)
		require.Nil(t, actor)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)

		actor, resp, err = (&Handler{authorizedFetchService: &fakeAuthorizedFetch{actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: bobID},
		}}}).verifyObjectAuthorizedFetch(context.Background(), reqCtx, "private", "req", false)
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Nil(t, resp)

		_, resp, err = (&Handler{authorizedFetchService: &fakeAuthorizedFetch{verifyErr: errors.New("missing signature")}}).
			verifyObjectAuthorizedFetch(context.Background(), reqCtx, "private", "req", false)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.Status)

		_, resp, err = (&Handler{authorizedFetchService: &fakeAuthorizedFetch{verifyErr: errors.New("bad signature")}}).
			verifyObjectAuthorizedFetch(context.Background(), reqCtx, "private", "req", false)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.Status)

		badCtx := &apptheory.Context{Request: apptheory.Request{Method: "bad method", Path: "/objects/private"}}
		_, resp, err = (&Handler{authorizedFetchService: &fakeAuthorizedFetch{}}).
			verifyObjectAuthorizedFetch(context.Background(), badCtx, "private", "req", false)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)

		_, resp, err = (&Handler{authorizedFetchService: &fakeAuthorizedFetch{}}).
			verifyObjectAuthorizedFetch(context.Background(), badCtx, "private", "req", true)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("identity and recipient extraction helpers cover object shapes", func(t *testing.T) {
		require.Empty(t, objectsActorID(nil))
		require.Equal(t, "https://example.com/users/carol", objectsActorID(&activitypub.Actor{
			URL: "https://example.com/users/carol",
		}))
		require.Equal(t, "dana", objectsActorID(&activitypub.Actor{PreferredUsername: "dana"}))

		require.Empty(t, objectsAttributedActorID(nil))
		require.Equal(t, authorID, objectsAttributedActorID(privateNote))
		require.Equal(t, authorID, objectsAttributedActorID(&activitypub.Activity{Actor: authorID}))
		require.Equal(t, authorID, objectsAttributedActorID(map[string]any{"attributedTo": authorID}))
		require.Equal(t, bobID, objectsAttributedActorID(map[string]any{"actor": bobID}))
		require.Equal(t, authorID, objectsAttributedActorID(struct {
			AttributedTo string `json:"attributedTo"`
		}{AttributedTo: authorID}))
		require.Empty(t, objectsAttributedActorID(map[string]any{"bad": make(chan int)}))

		require.Empty(t, objectsAllRecipients(nil))
		require.Equal(t, []string{authorID + "/followers"}, objectsAllRecipients(privateNote))
		require.Equal(t, []string{bobID}, objectsAllRecipients(&activitypub.Activity{
			BaseObject: activitypub.BaseObject{BTo: []string{bobID}},
		}))
		require.Equal(t, []string{bobID, authorID}, objectsAllRecipients(map[string]any{
			"to":  bobID,
			"bcc": []any{authorID, ""},
		}))
		require.Equal(t, []string{bobID}, objectsAllRecipients(struct {
			CC []string `json:"cc"`
		}{CC: []string{bobID}}))
		require.Empty(t, objectsAllRecipients(map[string]any{"bad": make(chan int)}))
	})

	t.Run("recipient matching helpers cover edge cases", func(t *testing.T) {
		require.False(t, objectsRecipientsContainActor([]string{bobID}, nil))
		require.True(t, objectsRecipientsContainActor([]string{"bob@remote.example"}, &activitypub.Actor{
			BaseObject: activitypub.BaseObject{ID: bobID},
		}))
		require.False(t, objectsRecipientsContainFollowersCollection([]string{"/followers"}, ""))
		require.True(t, objectsRecipientsContainFollowersCollection([]string{authorID + "/followers/"}, authorID))
		require.False(t, objectsActorIdentifiersMatch("", authorID))
		require.True(t, objectsActorIdentifiersMatch("https://example.com/users/Alice", "alice"))

		oldCfg := cfg
		cfg = nil
		require.NotEmpty(t, objectsLocalDomain())
		cfg = oldCfg
	})
}

func TestObjectHelpers_UncoveredBranches_Round12(t *testing.T) {
	h := &Handler{}
	require.True(t, h.parseDateTime(time.Now()).After(time.Time{}))
	require.True(t, h.parseDateTime("not-a-time").IsZero())
	require.Equal(t, "", h.generateWarningHTML(false, "cw"))
	require.Contains(t, h.generateWarningHTML(true, "cw"), "cw")
	require.Equal(t, "", h.generateUpdatedHTML(time.Time{}))
	require.Contains(t, h.generateUpdatedHTML(time.Date(2026, time.May, 19, 12, 0, 0, 0, time.UTC)), "Updated")
	require.Equal(t, "@", h.extractUsernameFromURL(""))
	objectID, lookupID := h.resolveObjectLookup(nil)
	require.Empty(t, objectID)
	require.Empty(t, lookupID)
	require.Empty(t, objectsHeaderValue(nil, "accept"))
	require.Empty(t, objectsHeaderValue(&apptheory.Context{
		Request: apptheory.Request{Headers: map[string][]string{"accept": {}}},
	}, "accept"))
	require.Equal(t, []string{"x"}, objectsAppendRecipients(nil, []string{"", "x"}))
	_, err := objectsStripHiddenRecipientsJSON([]byte("{broken"))
	require.Error(t, err)

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodGet,
			Path:   "/objects/123",
			Headers: map[string][]string{
				"host":                     {"internal.execute-api.us-east-1.amazonaws.com"},
				"x-lesser-forwarded-host":  {"example.com"},
				"x-lesser-forwarded-proto": {"https"},
				"some":                     {"Header"},
			},
			Query: map[string][]string{"q": {"1"}},
		},
	}
	req, err := h.convertAppTheoryRequest(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/objects/123?q=1", req.URL.String())
	require.Equal(t, "example.com", req.Host)
	require.Equal(t, "example.com", req.Header.Get("Host"))
}

func TestHandleTombstonedObject_NilRepoBranch_Round12(t *testing.T) {
	h := &Handler{}
	resp, handled, err := h.handleTombstonedObject(context.Background(), "lookup", "object", "req", false)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.False(t, handled)
}

func TestObjectsSecurityHeaders_Round12(t *testing.T) {
	mw := objectsActivityPubSecurityHeaders()
	wrapped := mw(func(_ *apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{
			Status: 200,
			Headers: map[string][]string{
				"content-type": {"text/html; charset=utf-8"},
			},
			Body: []byte("<!DOCTYPE html><html></html>"),
		}, nil
	})

	resp, err := wrapped(&apptheory.Context{
		Request: apptheory.Request{Method: http.MethodGet, Path: "/objects/123"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Headers["content-security-policy"])
	require.Contains(t, resp.Headers["content-security-policy"][0], "script-src 'none'")
}

// TestTombstoneVisibility_Round38 verifies tombstone visibility is governed by
// the IsPublic field, which records whether the original object was publicly
// addressed. Non-public tombstones are hidden unless the requester is verified
// as the original author; public tombstones are returned unconditionally.
// This is consistent with live-object visibility semantics (see
// TestHandleGetObject_PrivateVisibilityGate_Round29). CSR-030 regression coverage.
func TestTombstoneVisibility_Round38(t *testing.T) {
	origCfg := cfg
	origLogger := logger
	t.Cleanup(func() {
		cfg = origCfg
		logger = origLogger
	})

	cfg = &config.Config{Domain: "example.com"}
	logger = zap.NewNop()

	instanceRepo := &fakeInstanceRepo{
		state: &storageModels.InstanceState{Locked: false},
	}

	deletedAt := time.Date(2026, time.May, 22, 10, 0, 0, 0, time.UTC)

	t.Run("non-public attributed tombstone hidden as not found when auth enabled and no signature", func(t *testing.T) {
		objID := "https://example.com/users/alice/statuses/private-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled:   true,
				verifyErr: errors.New("missing signature"),
			},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/private-deleted",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice", "id": "private-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("non-public attributed tombstone hidden when authorized fetch is globally disabled", func(t *testing.T) {
		// CSR-030 fix: non-public (IsPublic=false) attributed tombstone
		// must NOT be returned to unsigned/public fetch even when authorized
		// fetch is globally disabled. Consistent with
		// TestHandleGetObject_PrivateVisibilityGate_Round29.
		objID := "https://example.com/users/alice/statuses/private-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice/statuses/private-deleted",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"username": "alice", "id": "private-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("non-public attributed tombstone hidden for HTML when authorized fetch is disabled", func(t *testing.T) {
		objID := "https://example.com/users/alice/statuses/private-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice/statuses/private-deleted",
				Headers: map[string][]string{"accept": {"text/html"}},
			},
			Params: map[string]string{"username": "alice", "id": "private-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("public tombstone visible when authorized fetch is globally disabled", func(t *testing.T) {
		// Publicly-addressed (IsPublic=true) tombstone is safe to show
		// unconditionally. The original object was public.
		objID := "https://example.com/users/alice/statuses/public-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/users/alice/statuses/public-deleted",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"username": "alice", "id": "public-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Tombstone", body["type"])
	})

	t.Run("public tombstone visible when auth enabled and verified non-author requests", func(t *testing.T) {
		// Public tombstone visible even to verified non-author.
		objID := "https://example.com/users/alice/statuses/public-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled: true,
				actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"},
				},
			},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/public-deleted",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice", "id": "public-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
	})

	t.Run("non-public tombstone shown when verified actor matches author", func(t *testing.T) {
		objID := "https://example.com/users/alice/statuses/private-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled: true,
				actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/alice",
					},
				},
			},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/private-deleted",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice", "id": "private-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
	})

	t.Run("non-public tombstone hidden when verified actor does not match author", func(t *testing.T) {
		objID := "https://example.com/users/alice/statuses/private-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled: true,
				actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/bob",
					},
				},
			},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/private-deleted",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice", "id": "private-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("legacy tombstone without IsPublic or AttributedTo hidden when auth disabled", func(t *testing.T) {
		// Legacy tombstone (IsPublic=false zero-value, AttributedTo empty):
		// hide conservatively because we cannot prove public visibility or author.
		objID := "https://example.com/objects/legacy-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    "/objects/legacy-deleted",
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "legacy-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("legacy tombstone without IsPublic or AttributedTo hidden when auth enabled and no signature", func(t *testing.T) {
		objID := "https://example.com/objects/legacy-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "",
				IsPublic:     false,
			},
		}
		h := &Handler{
			instanceRepo: instanceRepo,
			objectRepo:   objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{
				enabled:   true,
				verifyErr: errors.New("missing signature"),
			},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/objects/legacy-deleted",
				Headers: map[string][]string{
					"accept": {"application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"id": "legacy-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("public tombstone with HTML Accept returns text response", func(t *testing.T) {
		// When the client requests text/html, authorized fetch is not
		// enforced and the tombstone handler returns a plain-text 410
		// rather than an ActivityPub JSON tombstone payload.
		objID := "https://example.com/users/alice/statuses/public-deleted"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone: &storageModels.Tombstone{
				ID:           objID,
				FormerType:   activitypub.NoteType,
				Deleted:      deletedAt,
				DeletedBy:    "https://example.com/users/alice",
				AttributedTo: "https://example.com/users/alice",
				IsPublic:     true,
			},
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: true},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/users/alice/statuses/public-deleted",
				Headers: map[string][]string{
					"accept": {"text/html, application/activity+json"},
					"host":   {"example.com"},
				},
			},
			Params: map[string]string{"username": "alice", "id": "public-deleted"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusGone, resp.Status)
		// wantsHTML path returns a plain-text deletion message, not JSON.
		require.Contains(t, string(resp.Body), "deleted")
	})

	t.Run("tombstoned object with missing metadata is hidden when auth disabled", func(t *testing.T) {
		// IsTombstoned=true but GetTombstone returns an error because
		// the tombstone metadata row is missing. Without metadata the handler
		// cannot prove the deleted object was public, so it must hide the tombstone.
		objID := "https://example.com/objects/broken-tombstone"
		objRepo := &fakeObjectRepo{
			err:        errors.New("not found"),
			tombstoned: true,
			tombstone:  nil, // nil → GetTombstone returns "not found"
		}
		h := &Handler{
			instanceRepo:           instanceRepo,
			objectRepo:             objRepo,
			authorizedFetchService: &fakeAuthorizedFetch{enabled: false},
		}
		resp, err := h.HandleGetObject(&apptheory.Context{
			Request: apptheory.Request{
				Method:  http.MethodGet,
				Path:    objID,
				Headers: map[string][]string{"accept": {"application/activity+json"}},
			},
			Params: map[string]string{"id": "broken-tombstone"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})
}

// TestHandleTombstonedObjectVisible_NilRepoBranch_Round38 verifies the
// nil-object-repo defensive guard in handleTombstonedObjectVisible returns
// unhandled (handled=false) without panicking.
func TestHandleTombstonedObjectVisible_NilRepoBranch_Round38(t *testing.T) {
	h := &Handler{}
	resp, handled, err := h.handleTombstonedObjectVisible(
		context.Background(), "lookup", "object", "req", false, nil,
	)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.False(t, handled)
}
