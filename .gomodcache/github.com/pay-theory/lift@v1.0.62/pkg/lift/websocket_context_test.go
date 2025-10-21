package lift

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketContext_AsWebSocket(t *testing.T) {
	tests := []struct {
		setupContext   func() *Context
		name           string
		expectedConnID string
		expectedStage  string
		expectError    bool
	}{
		{
			name: "successful conversion from websocket event",
			setupContext: func() *Context {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerWebSocket,
						Metadata: map[string]any{
							"connectionId": "test-conn-123",
							"stage":        "prod",
							"domainName":   "example.execute-api.us-east-1.amazonaws.com",
						},
						RawEvent: map[string]any{
							"requestContext": map[string]any{
								"connectionId": "test-conn-123",
								"stage":        "prod",
								"domainName":   "example.execute-api.us-east-1.amazonaws.com",
							},
						},
					},
				}
				return NewContext(context.Background(), req)
			},
			expectError:    false,
			expectedConnID: "test-conn-123",
			expectedStage:  "prod",
		},
		{
			name: "conversion from non-websocket event fails",
			setupContext: func() *Context {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerAPIGateway,
						RawEvent: map[string]any{
							"httpMethod": "GET",
						},
					},
				}
				return NewContext(context.Background(), req)
			},
			expectError: true,
		},
		{
			name: "conversion with missing connection id still works",
			setupContext: func() *Context {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerWebSocket,
						Metadata: map[string]any{
							"stage": "prod",
						},
					},
				}
				return NewContext(context.Background(), req)
			},
			expectError:    false,
			expectedConnID: "",
			expectedStage:  "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupContext()
			wsCtx, err := ctx.AsWebSocket()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, wsCtx)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, wsCtx)
				assert.Equal(t, tt.expectedConnID, wsCtx.ConnectionID())
				assert.Equal(t, tt.expectedStage, wsCtx.Stage())
				assert.Equal(t, ctx, wsCtx.Context)
			}
		})
	}
}

func TestWebSocketContext_WithRegion(t *testing.T) {
	req := &Request{
		Request: &adapters.Request{
			TriggerType: TriggerWebSocket,
			Metadata: map[string]any{
				"connectionId": "test-conn",
			},
		},
	}
	ctx := NewContext(context.Background(), req)
	wsCtx, err := ctx.AsWebSocket()
	assert.NoError(t, err)

	// Test setting region
	result := wsCtx.WithRegion("us-west-2")
	assert.Equal(t, wsCtx, result) // Should return self for chaining
	assert.Equal(t, "us-west-2", wsCtx.GetRegion())
}

func TestWebSocketContext_GetRegion(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *WebSocketContext
		expectedRegion string
	}{
		{
			name: "explicitly set region",
			setupContext: func() *WebSocketContext {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerWebSocket,
						Metadata: map[string]any{
							"connectionId": "test-conn",
						},
					},
				}
				ctx := NewContext(context.Background(), req)
				wsCtx, err := ctx.AsWebSocket()
				assert.NoError(t, err)
				wsCtx.WithRegion("eu-west-1")
				return wsCtx
			},
			expectedRegion: "eu-west-1",
		},
		{
			name: "default region when none set",
			setupContext: func() *WebSocketContext {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerWebSocket,
						Metadata: map[string]any{
							"connectionId": "test-conn",
						},
					},
				}
				ctx := NewContext(context.Background(), req)
				wsCtx, err := ctx.AsWebSocket()
				assert.NoError(t, err)
				return wsCtx
			},
			expectedRegion: "us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsCtx := tt.setupContext()
			region := wsCtx.GetRegion()
			assert.Equal(t, tt.expectedRegion, region)
		})
	}
}

func TestWebSocketContext_HelperMethods(t *testing.T) {
	tests := []struct {
		name                 string
		routeKey             string
		expectedIsConnect    bool
		expectedIsDisconnect bool
		expectedIsMessage    bool
	}{
		{
			name:                 "connect event",
			routeKey:             "$connect",
			expectedIsConnect:    true,
			expectedIsDisconnect: false,
			expectedIsMessage:    false,
		},
		{
			name:                 "disconnect event",
			routeKey:             "$disconnect",
			expectedIsConnect:    false,
			expectedIsDisconnect: true,
			expectedIsMessage:    false,
		},
		{
			name:                 "default message event",
			routeKey:             "$default",
			expectedIsConnect:    false,
			expectedIsDisconnect: false,
			expectedIsMessage:    true,
		},
		{
			name:                 "custom route event",
			routeKey:             "customAction",
			expectedIsConnect:    false,
			expectedIsDisconnect: false,
			expectedIsMessage:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Request: &adapters.Request{
					TriggerType: TriggerWebSocket,
					Metadata: map[string]any{
						"connectionId": "test-conn",
						"routeKey":     tt.routeKey,
					},
				},
			}
			ctx := NewContext(context.Background(), req)
			wsCtx, err := ctx.AsWebSocket()
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedIsConnect, wsCtx.IsConnectEvent())
			assert.Equal(t, tt.expectedIsDisconnect, wsCtx.IsDisconnectEvent())
			assert.Equal(t, tt.expectedIsMessage, wsCtx.IsMessageEvent())
		})
	}
}

func TestWebSocketContext_GetAuthorizationFromQuery(t *testing.T) {
	tests := []struct {
		name         string
		setupContext func() *WebSocketContext
		expectedAuth string
	}{
		{
			name: "authorization in query params",
			setupContext: func() *WebSocketContext {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerWebSocket,
						QueryParams: map[string]string{
							"Authorization": "Bearer token123",
						},
						Metadata: map[string]any{
							"connectionId": "test-conn",
						},
					},
					QueryParams: map[string]string{
						"Authorization": "Bearer token123",
					},
				}
				ctx := NewContext(context.Background(), req)
				wsCtx, err := ctx.AsWebSocket()
				assert.NoError(t, err)
				return wsCtx
			},
			expectedAuth: "Bearer token123",
		},
		{
			name: "no authorization in query params",
			setupContext: func() *WebSocketContext {
				req := &Request{
					Request: &adapters.Request{
						TriggerType: TriggerWebSocket,
						QueryParams: map[string]string{
							"other": "value",
						},
						Metadata: map[string]any{
							"connectionId": "test-conn",
						},
					},
					QueryParams: map[string]string{
						"other": "value",
					},
				}
				ctx := NewContext(context.Background(), req)
				wsCtx, err := ctx.AsWebSocket()
				assert.NoError(t, err)
				return wsCtx
			},
			expectedAuth: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsCtx := tt.setupContext()
			auth := wsCtx.GetAuthorizationFromQuery()
			assert.Equal(t, tt.expectedAuth, auth)
		})
	}
}

func TestWebSocketContext_EndpointMethods(t *testing.T) {
	tests := []struct {
		name               string
		metadata           map[string]any
		expectedConnID     string
		expectedRouteKey   string
		expectedEventType  string
		expectedStage      string
		expectedDomainName string
		expectedEndpoint   string
	}{
		{
			name: "all metadata present",
			metadata: map[string]any{
				"connectionId":       "conn-123",
				"routeKey":           "$connect",
				"eventType":          "CONNECT",
				"stage":              "prod",
				"domainName":         "api.example.com",
				"managementEndpoint": "https://api.example.com/prod",
			},
			expectedConnID:     "conn-123",
			expectedRouteKey:   "$connect",
			expectedEventType:  "CONNECT",
			expectedStage:      "prod",
			expectedDomainName: "api.example.com",
			expectedEndpoint:   "https://api.example.com/prod",
		},
		{
			name:               "empty metadata",
			metadata:           map[string]any{},
			expectedConnID:     "",
			expectedRouteKey:   "",
			expectedEventType:  "",
			expectedStage:      "",
			expectedDomainName: "",
			expectedEndpoint:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Request: &adapters.Request{
					TriggerType: TriggerWebSocket,
					Metadata:    tt.metadata,
				},
			}
			ctx := NewContext(context.Background(), req)
			wsCtx, err := ctx.AsWebSocket()
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedConnID, wsCtx.ConnectionID())
			assert.Equal(t, tt.expectedRouteKey, wsCtx.RouteKey())
			assert.Equal(t, tt.expectedEventType, wsCtx.EventType())
			assert.Equal(t, tt.expectedStage, wsCtx.Stage())
			assert.Equal(t, tt.expectedDomainName, wsCtx.DomainName())
			assert.Equal(t, tt.expectedEndpoint, wsCtx.ManagementEndpoint())
		})
	}
}

// Edge case tests
func TestWebSocketContext_EdgeCases(t *testing.T) {
	t.Run("nil metadata handling", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata:    nil,
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		assert.Equal(t, "", wsCtx.ConnectionID())
		assert.Equal(t, "", wsCtx.RouteKey())
		assert.Equal(t, "", wsCtx.EventType())
		assert.Equal(t, "", wsCtx.Stage())
		assert.Equal(t, "", wsCtx.DomainName())
		assert.Equal(t, "", wsCtx.ManagementEndpoint())
	})

	t.Run("wrong type in metadata", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"connectionId": 123,  // wrong type
					"routeKey":     true, // wrong type
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		assert.Equal(t, "", wsCtx.ConnectionID())
		assert.Equal(t, "", wsCtx.RouteKey())
	})

	t.Run("GetManagementAPI with missing endpoint", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"connectionId": "test-conn",
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		api, err := wsCtx.GetManagementAPI()
		assert.Error(t, err)
		assert.Nil(t, api)
		assert.Contains(t, err.Error(), "management endpoint not found")
	})

	t.Run("SendMessage with missing connection ID", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"managementEndpoint": "https://api.example.com/prod",
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		err = wsCtx.SendMessage([]byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection ID not found")
	})

	t.Run("large message handling", func(t *testing.T) {
		// Create a 128KB message (max WebSocket frame size)
		largeMessage := make([]byte, 128*1024)
		for i := range largeMessage {
			largeMessage[i] = byte(i % 256)
		}

		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Body:        largeMessage,
				Metadata: map[string]any{
					"connectionId": "test-conn",
				},
			},
			Body: largeMessage,
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		// Verify we can access the large body
		assert.Equal(t, len(largeMessage), len(ctx.Request.Body))
		assert.NotNil(t, wsCtx)
	})

	t.Run("malformed JSON in SendJSONMessage", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"connectionId":       "test-conn",
					"managementEndpoint": "https://api.example.com/prod",
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		// Try to send a channel (cannot be marshaled to JSON)
		err = wsCtx.SendJSONMessage(make(chan int))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal JSON")
	})

	t.Run("BroadcastMessage with empty connection list", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"connectionId":       "test-conn",
					"managementEndpoint": "https://api.example.com/prod",
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		err = wsCtx.BroadcastMessage([]string{}, []byte("test"))
		assert.NoError(t, err) // Should not error on empty list
	})
}

// Test for error handling in API operations
func TestWebSocketContext_APIErrorHandling(t *testing.T) {
	t.Run("SendMessage with GoneException", func(t *testing.T) {
		// This would require mocking the AWS SDK, which is complex
		// For now, we'll just test the structure
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"connectionId":       "test-conn",
					"managementEndpoint": "https://api.example.com/prod",
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		// We can't easily test the actual AWS SDK errors without mocking
		// but we can verify the method exists and accepts the right parameters
		assert.NotNil(t, wsCtx)
	})

	t.Run("Disconnect with various connection IDs", func(t *testing.T) {
		req := &Request{
			Request: &adapters.Request{
				TriggerType: TriggerWebSocket,
				Metadata: map[string]any{
					"connectionId":       "test-conn",
					"managementEndpoint": "https://api.example.com/prod",
				},
			},
		}
		ctx := NewContext(context.Background(), req)
		wsCtx, err := ctx.AsWebSocket()
		assert.NoError(t, err)

		// Test disconnect with empty connection ID
		err = wsCtx.Disconnect("")
		// Without mocking, this will fail due to missing AWS credentials
		// but we're testing that the method exists and accepts parameters
		assert.Error(t, err)
	})
}

// Test concurrent operations safety
func TestWebSocketContext_ConcurrentSafety(t *testing.T) {
	req := &Request{
		Request: &adapters.Request{
			TriggerType: TriggerWebSocket,
			Metadata: map[string]any{
				"connectionId":       "test-conn",
				"routeKey":           "$default",
				"managementEndpoint": "https://api.example.com/prod",
			},
		},
	}
	ctx := NewContext(context.Background(), req)
	wsCtx, err := ctx.AsWebSocket()
	assert.NoError(t, err)

	// Test concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = wsCtx.ConnectionID()
			_ = wsCtx.RouteKey()
			_ = wsCtx.Stage()
			_ = wsCtx.GetRegion()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test passes if no race conditions occur
}

// Test malformed events
func TestWebSocketContext_MalformedEvents(t *testing.T) {
	tests := []struct {
		event     any
		name      string
		expectErr bool
	}{
		{
			name:      "completely empty event",
			event:     map[string]any{},
			expectErr: true,
		},
		{
			name: "missing request context",
			event: map[string]any{
				"body": "test",
			},
			expectErr: true,
		},
		{
			name: "wrong trigger type",
			event: map[string]any{
				"httpMethod": "GET",
				"path":       "/test",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Request: &adapters.Request{
					TriggerType: TriggerAPIGateway, // Wrong type
					RawEvent:    tt.event,
				},
			}
			ctx := NewContext(context.Background(), req)
			wsCtx, err := ctx.AsWebSocket()

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, wsCtx)
			}
		})
	}
}
