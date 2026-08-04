package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func TestAppTheoryHelpersRound20_QueryValues(t *testing.T) {
	t.Run("nil_ctx", func(t *testing.T) {
		require.Nil(t, queryValues(nil, "tags"))
	})

	t.Run("empty_key", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{Query: map[string][]string{"tags": {"a"}}}}
		require.Nil(t, queryValues(ctx, ""))
		require.Nil(t, queryValues(ctx, "   "))
	})

	t.Run("direct_key_trims_and_filters", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Query: map[string][]string{
					"tags": {" a ", "", "  ", "b"},
				},
			},
		}
		require.Equal(t, []string{" a ", "", "  ", "b"}, queryValues(ctx, "tags"))
	})

	t.Run("exact_key_match_only", func(t *testing.T) {
		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Query: map[string][]string{
					" Tags ": {"one", " two "},
				},
			},
		}
		require.Nil(t, queryValues(ctx, "tags"))
	})

	t.Run("missing_key", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{Query: map[string][]string{"other": {"x"}}}}
		require.Nil(t, queryValues(ctx, "tags"))
	})
}
