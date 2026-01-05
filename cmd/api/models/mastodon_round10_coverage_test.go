package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPollMarshalJSON_Round10Coverage(t *testing.T) {
	t.Run("response options use OptionsData", func(t *testing.T) {
		p := Poll{
			ID: "poll-1",
			OptionsData: []PollOption{
				{Title: "a", VotesCount: 1},
				{Title: "b", VotesCount: 2},
			},
		}

		b, err := json.Marshal(p)
		require.NoError(t, err)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(b, &decoded))

		options, ok := decoded["options"].([]any)
		require.True(t, ok)
		require.Len(t, options, 2)

		first, ok := options[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "a", first["title"])
	})

	t.Run("request options use string Options", func(t *testing.T) {
		p := Poll{
			Options: []string{"a", "b"},
		}

		b, err := json.Marshal(p)
		require.NoError(t, err)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(b, &decoded))

		options, ok := decoded["options"].([]any)
		require.True(t, ok)
		require.Len(t, options, 2)
		require.Equal(t, "a", options[0])
	})
}
