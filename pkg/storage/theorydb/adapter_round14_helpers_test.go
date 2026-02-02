package theorydb

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/require"
)

type repoStdSig struct {
	items  []string
	cursor string
	err    error
}

func (r repoStdSig) SearchUsers(ctx context.Context, query string, limit int, cursor string) ([]interface{}, string, error) {
	_ = ctx
	_ = query
	_ = limit
	_ = cursor
	out := make([]interface{}, len(r.items))
	for i, v := range r.items {
		out[i] = v
	}
	return out, r.cursor, r.err
}

func (r repoStdSig) SearchStatuses(ctx context.Context, query string, limit int, cursor string) ([]interface{}, string, error) {
	return r.SearchUsers(ctx, query, limit, cursor)
}

func (r repoStdSig) SearchHashtags(ctx context.Context, query string, limit int, cursor string) ([]interface{}, string, error) {
	return r.SearchUsers(ctx, query, limit, cursor)
}

type repoPageSig struct {
	items      []string
	nextCursor string
	err        error
}

func (r repoPageSig) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	_ = ctx
	_ = query
	_ = opts
	if r.err != nil {
		return nil, r.err
	}
	return &interfaces.PaginatedResult[string]{Items: r.items, NextCursor: r.nextCursor}, nil
}

func (r repoPageSig) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	return r.SearchAccounts(ctx, query, opts)
}

func (r repoPageSig) SearchHashtags(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	return r.SearchAccounts(ctx, query, opts)
}

func (r repoPageSig) GetUserTimeline(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	return r.SearchAccounts(ctx, username, opts)
}

func (r repoPageSig) GetUserNotifications(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	return r.SearchAccounts(ctx, username, opts)
}

func (r repoPageSig) GetUserMedia(ctx context.Context, username string, opts *interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	if opts == nil {
		return nil, errors.New("opts required")
	}
	return r.SearchAccounts(ctx, username, *opts)
}

type repoOddSig struct{}

func (repoOddSig) SearchAccounts(ctx context.Context, query string) error {
	_ = ctx
	_ = query
	return nil
}

type repoAllZeroes struct{}

func (repoAllZeroes) OnlyContext(ctx context.Context) error {
	_ = ctx
	return nil
}

func (repoAllZeroes) InvalidArgs(ctx context.Context, query string, limit int) ([]string, error) {
	_ = ctx
	_ = query
	_ = limit
	return []string{"x"}, nil
}

func (repoAllZeroes) StdThree(ctx context.Context, query string, limit int, cursor string) ([]string, string, error) {
	_ = ctx
	_ = query
	_ = limit
	_ = cursor
	return []string{"x"}, "next", nil
}

func (repoAllZeroes) StdThreeErr(ctx context.Context, query string, limit int, cursor string) ([]string, string, error) {
	_ = ctx
	_ = query
	_ = limit
	_ = cursor
	return []string{"x"}, "next", errors.New("boom")
}

func (repoAllZeroes) PageTwo(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[string], error) {
	_ = ctx
	_ = query
	_ = opts
	return &interfaces.PaginatedResult[string]{Items: []string{"x"}, NextCursor: "next"}, nil
}

func (repoAllZeroes) TwoItems(ctx context.Context, query string, opts interfaces.PaginationOptions) ([]string, error) {
	_ = ctx
	_ = query
	_ = opts
	return []string{"x"}, nil
}

type repoGetStdSig struct {
	items  []interface{}
	cursor string
	err    error
}

func (r repoGetStdSig) GetTimeline(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error) {
	_ = ctx
	_ = username
	_ = limit
	_ = cursor
	return r.items, r.cursor, r.err
}

func (r repoGetStdSig) GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error) {
	return r.GetTimeline(ctx, username, limit, cursor)
}

func (r repoGetStdSig) GetMediaAttachmentsByUser(ctx context.Context, username string, limit int, cursor string) ([]interface{}, string, error) {
	return r.GetTimeline(ctx, username, limit, cursor)
}

func TestAdapterHelpers_extractItemsAndCursor(t *testing.T) {
	type page struct {
		Items      []string
		NextCursor string
	}

	t.Run("false for non-pointer", func(t *testing.T) {
		items, cursor, ok := extractItemsAndCursor(page{Items: []string{"x"}, NextCursor: "c"})
		require.False(t, ok)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("false for nil pointer", func(t *testing.T) {
		var p *page
		items, cursor, ok := extractItemsAndCursor(p)
		require.False(t, ok)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("false for pointer to non-struct", func(t *testing.T) {
		s := []string{"x"}
		items, cursor, ok := extractItemsAndCursor(&s)
		require.False(t, ok)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("false when required fields missing", func(t *testing.T) {
		type badPage struct {
			Items  []string
			Cursor string
		}
		items, cursor, ok := extractItemsAndCursor(&badPage{Items: []string{"x"}, Cursor: "c"})
		require.False(t, ok)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("false when NextCursor not string", func(t *testing.T) {
		type badPage struct {
			Items      []string
			NextCursor int
		}
		items, cursor, ok := extractItemsAndCursor(&badPage{Items: []string{"x"}, NextCursor: 1})
		require.False(t, ok)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("extracts items and cursor", func(t *testing.T) {
		items, cursor, ok := extractItemsAndCursor(&page{Items: []string{"x"}, NextCursor: "c"})
		require.True(t, ok)
		require.Equal(t, []string{"x"}, items)
		require.Equal(t, "c", cursor)
	})
}

func TestAdapterHelpers_callRepositoryMethod(t *testing.T) {
	ctx := context.Background()

	t.Run("method not found", func(t *testing.T) {
		items, cursor, handled, err := callRepositoryMethod(ctx, repoAllZeroes{}, "Nope", "q", 1, "c")
		require.False(t, handled)
		require.NoError(t, err)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("unsupported input patterns are ignored", func(t *testing.T) {
		items, cursor, handled, err := callRepositoryMethod(ctx, repoAllZeroes{}, "OnlyContext", "q", 1, "c")
		require.False(t, handled)
		require.NoError(t, err)
		require.Nil(t, items)
		require.Empty(t, cursor)

		items, cursor, handled, err = callRepositoryMethod(ctx, repoAllZeroes{}, "InvalidArgs", "q", 1, "c")
		require.False(t, handled)
		require.NoError(t, err)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})

	t.Run("standard signature success", func(t *testing.T) {
		items, cursor, handled, err := callRepositoryMethod(ctx, repoAllZeroes{}, "StdThree", "q", 1, "c")
		require.True(t, handled)
		require.NoError(t, err)
		require.Equal(t, []string{"x"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("standard signature error", func(t *testing.T) {
		items, cursor, handled, err := callRepositoryMethod(ctx, repoAllZeroes{}, "StdThreeErr", "q", 1, "c")
		require.True(t, handled)
		require.Error(t, err)
		require.Equal(t, []string{"x"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("pagination signature extracts items and cursor", func(t *testing.T) {
		items, cursor, handled, err := callRepositoryMethod(ctx, repoAllZeroes{}, "PageTwo", "q", 1, "c")
		require.True(t, handled)
		require.NoError(t, err)
		require.Equal(t, []string{"x"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("two-value signature treats non-page as items", func(t *testing.T) {
		items, cursor, handled, err := callRepositoryMethod(ctx, repoAllZeroes{}, "TwoItems", "q", 1, "c")
		require.True(t, handled)
		require.NoError(t, err)
		require.Equal(t, []string{"x"}, items)
		require.Empty(t, cursor)
	})
}

func TestAdapterHelpers_buildRepositoryMethodArgs(t *testing.T) {
	ctx := context.Background()

	t.Run("false when too few args", func(t *testing.T) {
		method := reflect.ValueOf(repoAllZeroes{}).MethodByName("OnlyContext")
		require.True(t, method.IsValid())
		args, ok := buildRepositoryMethodArgs(ctx, method.Type(), "q", 1, "c")
		require.False(t, ok)
		require.Nil(t, args)
	})

	t.Run("true for (ctx, string, limit, cursor)", func(t *testing.T) {
		method := reflect.ValueOf(repoAllZeroes{}).MethodByName("StdThree")
		require.True(t, method.IsValid())
		args, ok := buildRepositoryMethodArgs(ctx, method.Type(), "q", 1, "c")
		require.True(t, ok)
		require.Len(t, args, 4)
	})

	t.Run("true for (ctx, string, PaginationOptions)", func(t *testing.T) {
		method := reflect.ValueOf(repoAllZeroes{}).MethodByName("PageTwo")
		require.True(t, method.IsValid())
		args, ok := buildRepositoryMethodArgs(ctx, method.Type(), "q", 1, "c")
		require.True(t, ok)
		require.Len(t, args, 3)
	})

	t.Run("false for unsupported (ctx, string, int)", func(t *testing.T) {
		method := reflect.ValueOf(repoAllZeroes{}).MethodByName("InvalidArgs")
		require.True(t, method.IsValid())
		args, ok := buildRepositoryMethodArgs(ctx, method.Type(), "q", 1, "c")
		require.False(t, ok)
		require.Nil(t, args)
	})
}

func TestAdapterHelpers_createTypedFallbackHandler(t *testing.T) {
	calls := map[string]func(r interface{}) ([]interface{}, string, error, bool){
		"X": func(r interface{}) ([]interface{}, string, error, bool) {
			_ = r
			return []interface{}{"ok"}, "c", nil, true
		},
	}

	t.Run("executes method when exists", func(t *testing.T) {
		h := createTypedFallbackHandler("X", calls)
		items, cursor, err, ok := h(repoAllZeroes{})
		require.True(t, ok)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"ok"}, items)
		require.Equal(t, "c", cursor)
	})

	t.Run("returns false when missing", func(t *testing.T) {
		h := createTypedFallbackHandler("Missing", calls)
		items, cursor, err, ok := h(repoAllZeroes{})
		require.False(t, ok)
		require.NoError(t, err)
		require.Nil(t, items)
		require.Empty(t, cursor)
	})
}

func TestAdapterHelpers_createReflectionBasedMethodCallsMap_Errors(t *testing.T) {
	ctx := context.Background()

	calls := createReflectionBasedMethodCallsMap(ctx, "q", 1, "c", []RepositoryMethodCall{
		{MethodName: "X", RepositoryMethod: "StdThreeErr"},
	})

	fn, ok := calls["X"]
	require.True(t, ok)

	items, cursor, err, handled := fn(repoAllZeroes{})
	require.True(t, handled)
	require.Error(t, err)
	require.Nil(t, items)
	require.Empty(t, cursor)
}

func TestAdapterHelpers_executeSearchMethodWithTypedFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("primary SearchUsers hits direct interface", func(t *testing.T) {
		items, cursor, err := executeSearchMethodWithTypedFallback[string](
			ctx,
			repoStdSig{items: []string{"a"}, cursor: "next"},
			repoPageSig{},
			"SearchUsers",
			"q",
			2,
			"c",
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"a"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("primary SearchUsers returns error", func(t *testing.T) {
		_, _, err := executeSearchMethodWithTypedFallback[string](
			ctx,
			repoStdSig{err: errors.New("boom")},
			repoPageSig{},
			"SearchUsers",
			"q",
			2,
			"c",
		)
		require.Error(t, err)
	})

	t.Run("fallback SearchUsers uses SearchAccounts via PaginationOptions", func(t *testing.T) {
		items, cursor, err := executeSearchMethodWithTypedFallback[string](
			ctx,
			repoOddSig{},
			repoPageSig{items: []string{"a"}, nextCursor: "next"},
			"SearchUsers",
			"q",
			2,
			"c",
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"a"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("fallback propagates errors", func(t *testing.T) {
		_, _, err := executeSearchMethodWithTypedFallback[string](
			ctx,
			repoOddSig{},
			repoPageSig{err: errors.New("boom")},
			"SearchUsers",
			"q",
			2,
			"c",
		)
		require.Error(t, err)
	})

	t.Run("unknown method returns empty result", func(t *testing.T) {
		items, cursor, err := executeSearchMethodWithTypedFallback[string](
			ctx,
			repoStdSig{},
			repoPageSig{},
			"Unknown",
			"q",
			2,
			"c",
		)
		require.NoError(t, err)
		require.Empty(t, cursor)
		require.Empty(t, items)
	})
}

func TestAdapterHelpers_executeGetMethodWithTypedFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("primary GetTimeline hits direct interface", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoGetStdSig{items: []interface{}{"ok"}, cursor: "next"},
			"GetTimeline",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"ok"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("primary GetNotifications hits direct interface", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoGetStdSig{items: []interface{}{"ok"}, cursor: "next"},
			"GetNotifications",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"ok"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("primary GetMediaAttachmentsByUser hits direct interface", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoGetStdSig{items: []interface{}{"ok"}, cursor: "next"},
			"GetMediaAttachmentsByUser",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"ok"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("primary GetTimeline returns error", func(t *testing.T) {
		_, _, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoGetStdSig{err: errors.New("boom")},
			"GetTimeline",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.Error(t, err)
	})

	t.Run("fallback GetTimeline uses GetUserTimeline via PaginationOptions", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoPageSig{items: []string{"a"}, nextCursor: "next"},
			"GetTimeline",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"a"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("fallback GetNotifications uses GetUserNotifications via PaginationOptions", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoPageSig{items: []string{"a"}, nextCursor: "next"},
			"GetNotifications",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"a"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("fallback GetMediaAttachmentsByUser uses GetUserMedia via *PaginationOptions", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoPageSig{items: []string{"a"}, nextCursor: "next"},
			"GetMediaAttachmentsByUser",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []interface{}{"a"}, items)
		require.Equal(t, "next", cursor)
	})

	t.Run("unknown method returns empty result", func(t *testing.T) {
		items, cursor, err := executeGetMethodWithTypedFallback[string](
			ctx,
			repoPageSig{items: []string{"a"}, nextCursor: "next"},
			"Unknown",
			"user",
			2,
			"c",
			nil,
			nil,
		)
		require.NoError(t, err)
		require.Empty(t, cursor)
		require.Empty(t, items)
	})
}
