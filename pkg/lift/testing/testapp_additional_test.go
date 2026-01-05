package testing

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	lifttesting "github.com/pay-theory/lift/pkg/testing"
	"github.com/stretchr/testify/require"
)

func TestParseRawPath_ParsesQueryParams(t *testing.T) {
	path, query := parseRawPath("/foo?bar=baz&empty=&multi=a&multi=b")
	require.Equal(t, "/foo", path)
	require.Equal(t, map[string]string{
		"bar":   "baz",
		"empty": "",
		"multi": "a",
	}, query)
}

func TestParseRawPath_InvalidURLFallsBackToRawPath(t *testing.T) {
	raw := "/foo%zz"
	path, query := parseRawPath(raw)
	require.Equal(t, raw, path)
	require.Nil(t, query)
}

func TestEncodeBody_CoversAllInputForms(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		data, err := encodeBody(nil)
		require.NoError(t, err)
		require.Nil(t, data)
	})

	t.Run("bytes", func(t *testing.T) {
		data, err := encodeBody([]byte("abc"))
		require.NoError(t, err)
		require.Equal(t, []byte("abc"), data)
	})

	t.Run("string", func(t *testing.T) {
		data, err := encodeBody("abc")
		require.NoError(t, err)
		require.Equal(t, []byte("abc"), data)
	})

	t.Run("struct", func(t *testing.T) {
		data, err := encodeBody(map[string]string{"k": "v"})
		require.NoError(t, err)
		require.Contains(t, string(data), `"k":"v"`)
	})

	t.Run("marshal error", func(t *testing.T) {
		_, err := encodeBody(make(chan int))
		require.Error(t, err)
	})
}

func TestLiftResponseToTestResponse_HandlesBodyVariants(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		resp := liftResponseToTestResponse(nil)
		require.Equal(t, 500, resp.StatusCode)
		require.Contains(t, resp.Body, "missing response")
		require.Equal(t, "application/json", resp.Headers["Content-Type"])
	})

	t.Run("string body", func(t *testing.T) {
		resp := liftResponseToTestResponse(&lift.Response{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       "ok",
		})
		require.Equal(t, 200, resp.StatusCode)
		require.Equal(t, "ok", resp.Body)
	})

	t.Run("bytes body base64", func(t *testing.T) {
		raw := []byte("bin")
		resp := liftResponseToTestResponse(&lift.Response{
			StatusCode:       200,
			Headers:          map[string]string{},
			Body:             raw,
			IsBase64Encoded:  true,
			// written is intentionally left false; this adapter only reads fields.
		})
		require.Equal(t, base64.StdEncoding.EncodeToString(raw), resp.Body)
	})

	t.Run("bytes body plain", func(t *testing.T) {
		resp := liftResponseToTestResponse(&lift.Response{
			StatusCode: 200,
			Headers:    map[string]string{},
			Body:       []byte("plain"),
		})
		require.Equal(t, "plain", resp.Body)
	})

	t.Run("json marshal ok", func(t *testing.T) {
		resp := liftResponseToTestResponse(&lift.Response{
			StatusCode: 200,
			Headers:    nil,
			Body:       map[string]string{"k": "v"},
		})
		require.Equal(t, 200, resp.StatusCode)
		require.Contains(t, resp.Body, `"k":"v"`)
		require.NotNil(t, resp.Headers)
	})

	t.Run("json marshal error", func(t *testing.T) {
		resp := liftResponseToTestResponse(&lift.Response{
			StatusCode: 200,
			Headers:    map[string]string{},
			Body:       make(chan int),
		})
		require.Contains(t, resp.Body, "failed to encode response body")
	})
}

func TestTestApp_doRequest_ErrorPaths(t *testing.T) {
	t.Run("body encode error", func(t *testing.T) {
		app := NewTestApp()
		resp := app.POST("/any", make(chan int))
		require.Equal(t, 500, resp.StatusCode)
		require.Contains(t, resp.Body, "failed to encode request body")
	})

	t.Run("handler returns error", func(t *testing.T) {
		app := NewTestApp()
		_ = app.App().GET("/boom", func(_ *lift.Context) error {
			return errors.New("boom")
		})

		resp := app.GET("/boom")
		require.Equal(t, 500, resp.StatusCode)
		require.Contains(t, resp.Body, "Internal Server Error")
	})

	t.Run("error response write failure", func(t *testing.T) {
		app := NewTestApp()
		_ = app.App().GET("/boom", func(ctx *lift.Context) error {
			_ = ctx.JSON(map[string]string{"written": "true"})
			return errors.New("boom")
		})

		resp := app.GET("/boom")
		require.Equal(t, 500, resp.StatusCode)
		require.Contains(t, resp.Body, "failed to handle request")
	})
}

func TestTestApp_HandleRequest_ExercisesEventRouting(t *testing.T) {
	app := NewTestApp()
	_ = app.App().GET("/ping", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })
	_ = app.App().POST("/ping", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })
	_ = app.App().PUT("/ping", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })
	_ = app.App().DELETE("/ping", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })
	_ = app.App().PATCH("/ping", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })

	t.Run("non-map event", func(t *testing.T) {
		resp := app.HandleRequest("not a map")
		require.Equal(t, 200, resp.StatusCode)
		require.Contains(t, resp.Body, "event_handled")
	})

	t.Run("api gateway event routes method", func(t *testing.T) {
		resp := app.HandleRequest(map[string]interface{}{
			"httpMethod": "GET",
			"path":       "/ping",
			"headers": map[string]interface{}{
				"X-Test": "1",
			},
		})
		require.Equal(t, 200, resp.StatusCode)
		require.Contains(t, resp.Body, `"ok"`)
	})

	t.Run("api gateway routes write methods", func(t *testing.T) {
		for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
			resp := app.HandleRequest(map[string]interface{}{
				"httpMethod": method,
				"path":       "/ping",
				"body":       `{"x":"y"}`,
			})
			require.Equal(t, 200, resp.StatusCode)
		}
	})

	t.Run("api gateway method not allowed", func(t *testing.T) {
		resp := app.HandleRequest(map[string]interface{}{
			"httpMethod": "TRACE",
			"path":       "/ping",
		})
		require.Equal(t, 405, resp.StatusCode)
	})

	t.Run("sqs event handled", func(t *testing.T) {
		resp := app.HandleRequest(map[string]interface{}{
			"Records": []interface{}{
				map[string]interface{}{"eventSource": "aws:sqs"},
			},
		})
		require.Equal(t, 200, resp.StatusCode)
		require.Contains(t, resp.Body, "processed")
	})

	t.Run("unknown map event uses default", func(t *testing.T) {
		resp := app.HandleRequest(map[string]interface{}{"foo": "bar"})
		require.Equal(t, 200, resp.StatusCode)
		require.Contains(t, resp.Body, "event_handled")
	})
}

func TestTestResponse_JSON_WrapsLiftTestingResponse(t *testing.T) {
	resp := &TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 200, Body: `{"k":"v"}`}}
	var out map[string]string
	require.NoError(t, resp.JSON(&out))
	require.Equal(t, "v", out["k"])
}

func TestTestApp_MethodHelpers_PUT_DELETE_PATCH(t *testing.T) {
	app := NewTestApp()
	_ = app.App().PUT("/resource", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })
	_ = app.App().DELETE("/resource", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })
	_ = app.App().PATCH("/resource", func(ctx *lift.Context) error { return ctx.JSON(map[string]string{"ok": "true"}) })

	require.Equal(t, 200, app.PUT("/resource", map[string]string{"k": "v"}).StatusCode)
	require.Equal(t, 200, app.DELETE("/resource").StatusCode)
	require.Equal(t, 200, app.PATCH("/resource", map[string]string{"k": "v"}).StatusCode)
}

func TestTestApp_HandleSQSEvent_EdgeCases(t *testing.T) {
	app := NewTestApp()

	require.Nil(t, app.handleSQSEvent(map[string]interface{}{"Records": "not a slice"}))
	require.Nil(t, app.handleSQSEvent(map[string]interface{}{"Records": []interface{}{"not a map"}}))
	require.Nil(t, app.handleSQSEvent(map[string]interface{}{
		"Records": []interface{}{map[string]interface{}{"eventSource": "aws:s3"}},
	}))
}
