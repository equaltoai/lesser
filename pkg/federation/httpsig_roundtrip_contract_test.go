package federation

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildHTTPSignatureString_HostAndQueryMatrix(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		host    string
		headers map[string]string
		signed  []string
		want    string
	}{
		{
			name:   "prefers request host over url host",
			url:    "https://remote.example/inbox?shared=true",
			host:   "public.example",
			signed: []string{RequestTargetHeader, "host"},
			want:   "(request-target): post /inbox?shared=true\nhost: public.example",
		},
		{
			name:   "falls back to url host when host empty",
			url:    "https://remote.example/inbox",
			signed: []string{RequestTargetHeader, "host"},
			want:   "(request-target): post /inbox\nhost: remote.example",
		},
		{
			name: "preserves mixed-case host value and content type",
			url:  "https://remote.example/users/alice/inbox?cursor=next",
			host: "Theory.EXAMPLE",
			headers: map[string]string{
				"Date":         "Wed, 08 Apr 2026 14:15:00 GMT",
				"Content-Type": "application/activity+json",
			},
			signed: []string{RequestTargetHeader, "host", "date", "content-type"},
			want: "(request-target): post /users/alice/inbox?cursor=next\n" +
				"host: Theory.EXAMPLE\n" +
				"date: Wed, 08 Apr 2026 14:15:00 GMT\n" +
				"content-type: application/activity+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
			req.Host = tt.host
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			got, err := BuildHTTPSignatureString(req, tt.signed)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
