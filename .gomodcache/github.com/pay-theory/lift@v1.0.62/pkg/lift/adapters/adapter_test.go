package adapters

import (
	"testing"
)

func TestAdapterRegistry_DetectAndAdapt(t *testing.T) {
	registry := NewAdapterRegistry()

	tests := []struct {
		name         string
		event        any
		expectedType TriggerType
		shouldError  bool
	}{
		{
			name: "API Gateway V2 Event",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /hello",
				"requestContext": map[string]any{
					"requestId": "test-request-id",
					"http": map[string]any{
						"method": "GET",
						"path":   "/hello",
					},
				},
				"headers": map[string]any{
					"content-type": "application/json",
				},
				"body": `{"message": "hello"}`,
			},
			expectedType: TriggerAPIGatewayV2,
			shouldError:  false,
		},
		{
			name: "API Gateway V1 Event",
			event: map[string]any{
				"resource":   "/hello",
				"httpMethod": "GET",
				"path":       "/hello",
				"requestContext": map[string]any{
					"requestId": "test-request-id",
				},
				"headers": map[string]any{
					"Content-Type": "application/json",
				},
				"body": `{"message": "hello"}`,
			},
			expectedType: TriggerAPIGateway,
			shouldError:  false,
		},
		{
			name: "SQS Event",
			event: map[string]any{
				"Records": []any{
					map[string]any{
						"eventSource":   "aws:sqs",
						"body":          `{"message": "hello"}`,
						"receiptHandle": "test-receipt-handle",
						"messageId":     "test-message-id",
					},
				},
			},
			expectedType: TriggerSQS,
			shouldError:  false,
		},
		{
			name: "S3 Event",
			event: map[string]any{
				"Records": []any{
					map[string]any{
						"eventSource": "aws:s3",
						"eventName":   "ObjectCreated:Put",
						"s3": map[string]any{
							"bucket": map[string]any{
								"name": "test-bucket",
							},
							"object": map[string]any{
								"key": "test-key",
							},
						},
					},
				},
			},
			expectedType: TriggerS3,
			shouldError:  false,
		},
		{
			name: "EventBridge Event",
			event: map[string]any{
				"source":      "myapp.orders",
				"detail-type": "Order Placed",
				"detail": map[string]any{
					"orderId": "12345",
				},
				"time": "2023-01-01T00:00:00Z",
				"id":   "test-event-id",
			},
			expectedType: TriggerEventBridge,
			shouldError:  false,
		},
		{
			name: "Scheduled Event (via EventBridge)",
			event: map[string]any{
				"source":      "aws.events",
				"detail-type": "Scheduled Event",
				"time":        "2023-01-01T00:00:00Z",
				"id":          "test-event-id",
				"resources":   []any{"arn:aws:events:us-east-1:123456789012:rule/my-rule"},
				"detail":      map[string]any{},
			},
			expectedType: TriggerEventBridge,
			shouldError:  false,
		},
		{
			name: "Unknown Event",
			event: map[string]any{
				"unknown": "event",
			},
			expectedType: TriggerUnknown,
			shouldError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := registry.DetectAndAdapt(tt.event)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if request.TriggerType != tt.expectedType {
				t.Errorf("expected trigger type %s, got %s", tt.expectedType, request.TriggerType)
			}

			if request.RawEvent == nil {
				t.Errorf("expected raw event to be preserved")
			}
		})
	}
}

func TestAPIGatewayV2Adapter_Adapt(t *testing.T) {
	adapter := NewAPIGatewayV2Adapter()

	event := map[string]any{
		"version":  "2.0",
		"routeKey": "POST /users",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
			"timeEpoch": "1640995200",
			"http": map[string]any{
				"method": "POST",
				"path":   "/users",
			},
		},
		"headers": map[string]any{
			"Content-Type":  "application/json",
			"Authorization": "Bearer token123",
		},
		"queryStringParameters": map[string]any{
			"page":  "1",
			"limit": "10",
		},
		"pathParameters": map[string]any{
			"id": "123",
		},
		"body":            `{"name": "John Doe"}`,
		"isBase64Encoded": false,
	}

	request, err := adapter.Adapt(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify basic fields
	if request.TriggerType != TriggerAPIGatewayV2 {
		t.Errorf("expected trigger type %s, got %s", TriggerAPIGatewayV2, request.TriggerType)
	}

	if request.Method != "POST" {
		t.Errorf("expected method POST, got %s", request.Method)
	}

	if request.Path != "/users" {
		t.Errorf("expected path /users, got %s", request.Path)
	}

	// Verify headers (should be lowercase)
	if request.Headers["content-type"] != "application/json" {
		t.Errorf("expected content-type header, got %v", request.Headers)
	}

	if request.Headers["authorization"] != "Bearer token123" {
		t.Errorf("expected authorization header, got %v", request.Headers)
	}

	// Verify query parameters
	if request.QueryParams["page"] != "1" {
		t.Errorf("expected page query param, got %v", request.QueryParams)
	}

	// Verify path parameters
	if request.PathParams["id"] != "123" {
		t.Errorf("expected id path param, got %v", request.PathParams)
	}

	// Verify body
	expectedBody := `{"name": "John Doe"}`
	if string(request.Body) != expectedBody {
		t.Errorf("expected body %s, got %s", expectedBody, string(request.Body))
	}

	// Verify metadata
	if request.EventID != "test-request-id" {
		t.Errorf("expected event ID test-request-id, got %s", request.EventID)
	}
}

func TestSQSAdapter_Adapt(t *testing.T) {
	adapter := NewSQSAdapter()

	event := map[string]any{
		"Records": []any{
			map[string]any{
				"eventSource":   "aws:sqs",
				"body":          `{"orderId": "12345"}`,
				"receiptHandle": "test-receipt-handle",
				"messageId":     "test-message-id",
				"attributes": map[string]any{
					"SentTimestamp": "1640995200000",
				},
			},
			map[string]any{
				"eventSource":   "aws:sqs",
				"body":          `{"orderId": "67890"}`,
				"receiptHandle": "test-receipt-handle-2",
				"messageId":     "test-message-id-2",
			},
		},
	}

	request, err := adapter.Adapt(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify basic fields
	if request.TriggerType != TriggerSQS {
		t.Errorf("expected trigger type %s, got %s", TriggerSQS, request.TriggerType)
	}

	if request.Source != "aws:sqs" {
		t.Errorf("expected source aws:sqs, got %s", request.Source)
	}

	// Verify records
	if len(request.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(request.Records))
	}

	// Verify metadata from first record
	if request.EventID != "test-message-id" {
		t.Errorf("expected event ID test-message-id, got %s", request.EventID)
	}
}

func TestEventBridgeAdapter_Adapt(t *testing.T) {
	adapter := NewEventBridgeAdapter()

	event := map[string]any{
		"source":      "myapp.orders",
		"detail-type": "Order Placed",
		"detail": map[string]any{
			"orderId":    "12345",
			"customerId": "67890",
			"amount":     99.99,
		},
		"time": "2023-01-01T00:00:00Z",
		"id":   "test-event-id",
	}

	request, err := adapter.Adapt(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify basic fields
	if request.TriggerType != TriggerEventBridge {
		t.Errorf("expected trigger type %s, got %s", TriggerEventBridge, request.TriggerType)
	}

	if request.Source != "myapp.orders" {
		t.Errorf("expected source myapp.orders, got %s", request.Source)
	}

	if request.DetailType != "Order Placed" {
		t.Errorf("expected detail type 'Order Placed', got %s", request.DetailType)
	}

	// Verify detail
	if request.Detail["orderId"] != "12345" {
		t.Errorf("expected orderId in detail, got %v", request.Detail)
	}

	// Verify metadata
	if request.EventID != "test-event-id" {
		t.Errorf("expected event ID test-event-id, got %s", request.EventID)
	}

	if request.Timestamp != "2023-01-01T00:00:00Z" {
		t.Errorf("expected timestamp 2023-01-01T00:00:00Z, got %s", request.Timestamp)
	}
}

func TestAdapterRegistry_ListSupportedTriggers(t *testing.T) {
	registry := NewAdapterRegistry()

	triggers := registry.ListSupportedTriggers()

	expectedTriggers := []TriggerType{
		TriggerAPIGateway,
		TriggerAPIGatewayV2,
		TriggerSQS,
		TriggerS3,
		TriggerEventBridge,
		TriggerWebSocket,
	}

	if len(triggers) != len(expectedTriggers) {
		t.Errorf("expected %d triggers, got %d", len(expectedTriggers), len(triggers))
	}

	// Check that all expected triggers are present
	triggerMap := make(map[TriggerType]bool)
	for _, trigger := range triggers {
		triggerMap[trigger] = true
	}

	for _, expected := range expectedTriggers {
		if !triggerMap[expected] {
			t.Errorf("expected trigger %s not found in list", expected)
		}
	}
}
