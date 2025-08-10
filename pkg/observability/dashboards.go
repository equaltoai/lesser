// Package observability provides CloudWatch dashboard configuration and management
package observability

import (
	"encoding/json"
	"fmt"
	"os"
)

// DashboardConfig represents a complete CloudWatch dashboard configuration
type DashboardConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Widgets     []DashboardWidget      `json:"widgets"`
	Period      int                    `json:"period"`
	Region      string                 `json:"region"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// DashboardWidget represents a single widget in a dashboard
type DashboardWidget struct {
	Type       string                 `json:"type"`
	X          int                    `json:"x"`
	Y          int                    `json:"y"`
	Width      int                    `json:"width"`
	Height     int                    `json:"height"`
	Properties DashboardWidgetProps   `json:"properties"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DashboardWidgetProps contains widget-specific properties
type DashboardWidgetProps struct {
	Title      string            `json:"title"`
	View       string            `json:"view,omitempty"`
	Stacked    bool              `json:"stacked,omitempty"`
	Region     string            `json:"region"`
	Period     int               `json:"period,omitempty"`
	Stat       string            `json:"stat,omitempty"`
	YAxis      *YAxisConfig      `json:"yAxis,omitempty"`
	Metrics    [][]interface{}   `json:"metrics"`
	Annotations *AnnotationConfig `json:"annotations,omitempty"`
}

// YAxisConfig configures the Y-axis of a widget
type YAxisConfig struct {
	Left  *AxisConfig `json:"left,omitempty"`
	Right *AxisConfig `json:"right,omitempty"`
}

// AxisConfig configures an individual axis
type AxisConfig struct {
	Min   float64 `json:"min,omitempty"`
	Max   float64 `json:"max,omitempty"`
	Label string  `json:"label,omitempty"`
}

// AnnotationConfig configures annotations and alarms
type AnnotationConfig struct {
	Horizontal []HorizontalAnnotation `json:"horizontal,omitempty"`
	Vertical   []VerticalAnnotation   `json:"vertical,omitempty"`
}

// HorizontalAnnotation represents a horizontal line annotation
type HorizontalAnnotation struct {
	Value     float64 `json:"value"`
	Label     string  `json:"label,omitempty"`
	Color     string  `json:"color,omitempty"`
	Fill      string  `json:"fill,omitempty"`
	Visible   bool    `json:"visible,omitempty"`
}

// VerticalAnnotation represents a vertical line annotation
type VerticalAnnotation struct {
	Value   string `json:"value"`
	Label   string `json:"label,omitempty"`
	Color   string `json:"color,omitempty"`
	Visible bool   `json:"visible,omitempty"`
}

// CreateLesserOverviewDashboard creates the main overview dashboard for Lesser
func CreateLesserOverviewDashboard(region, environment string) *DashboardConfig {
	return &DashboardConfig{
		Name:        fmt.Sprintf("Lesser-Overview-%s", environment),
		Description: "Lesser ActivityPub Platform - Overview Dashboard",
		Period:      300, // 5 minutes
		Region:      region,
		Widgets: []DashboardWidget{
			// Row 1: Key Performance Indicators
			createMetricWidget("API Request Rate", 0, 0, 8, 6, [][]interface{}{
				{"Lesser/API", MetricThroughput, DimensionService, "api", map[string]string{"stat": "Sum"}},
				{".", MetricRequestsPerSecond, ".", ".", map[string]string{"stat": "Average"}},
			}, "timeSeries"),
			
			createMetricWidget("Error Rates", 8, 0, 8, 6, [][]interface{}{
				{"Lesser/API", MetricErrorRate, DimensionService, "api", map[string]string{"stat": "Average"}},
				{"Lesser/Federation", MetricFederationError, DimensionService, "inbox", map[string]string{"stat": "Sum"}},
				{"Lesser/Media", "MediaProcessingErrors", DimensionService, "media-processor", map[string]string{"stat": "Sum"}},
			}, "timeSeries"),
			
			createMetricWidget("Response Times (P99)", 16, 0, 8, 6, [][]interface{}{
				{"Lesser/API", MetricLatencyP99, DimensionService, "api", map[string]string{"stat": "Average"}},
				{"Lesser/Federation", MetricFederationLatency, DimensionService, "inbox", map[string]string{"stat": "Average"}},
				{"Lesser/Media", MetricMediaProcessingTime, DimensionService, "media-processor", map[string]string{"stat": "Average"}},
			}, "timeSeries"),

			// Row 2: System Health
			createMetricWidget("Lambda Cold Starts", 0, 6, 6, 6, [][]interface{}{
				{"Lesser/API", MetricColdStarts, DimensionService, "api", map[string]string{"stat": "Sum"}},
				{"Lesser/Federation", MetricColdStarts, DimensionService, "inbox", map[string]string{"stat": "Sum"}},
				{"Lesser/Media", MetricColdStarts, DimensionService, "media-processor", map[string]string{"stat": "Sum"}},
			}, "timeSeries"),
			
			createMetricWidget("System Health", 6, 6, 6, 6, [][]interface{}{
				{"Lesser/API", MetricSystemHealth, DimensionService, "api", map[string]string{"stat": "Average"}},
				{"Lesser/Federation", MetricSystemHealth, DimensionService, "inbox", map[string]string{"stat": "Average"}},
				{"Lesser/Media", MetricSystemHealth, DimensionService, "media-processor", map[string]string{"stat": "Average"}},
			}, "timeSeries"),
			
			createMetricWidget("Queue Depths", 12, 6, 6, 6, [][]interface{}{
				{"Lesser/Media", MetricQueueDepth, DimensionQueue, "media-processing", map[string]string{"stat": "Maximum"}},
				{"Lesser/Federation", MetricQueueDepth, DimensionQueue, "federation-delivery", map[string]string{"stat": "Maximum"}},
			}, "timeSeries"),
			
			createNumberWidget("Active Users (24h)", 18, 6, 6, 6, [][]interface{}{
				{"Lesser/API", MetricDailyActiveUsers, DimensionService, "api", map[string]string{"stat": "Maximum"}},
			}),

			// Row 3: Business Metrics
			createMetricWidget("Posts Per Minute", 0, 12, 8, 6, [][]interface{}{
				{"Lesser/API", MetricPostsPerMinute, DimensionService, "api", map[string]string{"stat": "Sum"}},
			}, "timeSeries"),
			
			createMetricWidget("Federation Activity", 8, 12, 8, 6, [][]interface{}{
				{"Lesser/Federation", MetricInboxMessages, DimensionService, "inbox", map[string]string{"stat": "Sum"}},
				{"Lesser/Federation", MetricOutboxMessages, DimensionService, "outbox", map[string]string{"stat": "Sum"}},
			}, "timeSeries"),
			
			createMetricWidget("Media Processing", 16, 12, 8, 6, [][]interface{}{
				{"Lesser/Media", MetricMediaProcessing, DimensionService, "media-processor", map[string]string{"stat": "Sum"}},
				{"Lesser/Media", "MediaProcessingCompleted", DimensionService, "media-processor", map[string]string{"stat": "Sum"}},
				{"Lesser/Media", "MediaProcessingFailed", DimensionService, "media-processor", map[string]string{"stat": "Sum"}},
			}, "timeSeries"),

			// Row 4: Cost Tracking
			createMetricWidget("Estimated Costs (USD)", 0, 18, 12, 6, [][]interface{}{
				{"Lesser/API", MetricCost, DimensionService, "api", map[string]string{"stat": "Sum"}},
				{"Lesser/Federation", MetricCost, DimensionService, "inbox", map[string]string{"stat": "Sum"}},
				{"Lesser/Media", MetricCost, DimensionService, "media-processor", map[string]string{"stat": "Sum"}},
			}, "timeSeries"),
			
			createNumberWidget("Cost Per User (USD)", 12, 18, 6, 6, [][]interface{}{
				{"Lesser/API", MetricCostPerUser, DimensionService, "api", map[string]string{"stat": "Average"}},
			}),
			
			createNumberWidget("Cost Per Request (μ¢)", 18, 18, 6, 6, [][]interface{}{
				{"Lesser/API", MetricCostPerRequest, DimensionService, "api", map[string]string{"stat": "Average"}},
			}),
		},
	}
}

// CreateAPIPerformanceDashboard creates a detailed API performance dashboard
func CreateAPIPerformanceDashboard(region, environment string) *DashboardConfig {
	return &DashboardConfig{
		Name:        fmt.Sprintf("Lesser-API-Performance-%s", environment),
		Description: "Lesser API - Detailed Performance Metrics",
		Period:      300,
		Region:      region,
		Widgets: []DashboardWidget{
			// Row 1: Request Metrics by Endpoint
			createMetricWidget("Requests by Endpoint", 0, 0, 12, 6, [][]interface{}{
				{"Lesser/API", MetricThroughput, DimensionEndpoint, "/api/v1/statuses", DimensionMethod, "POST"},
				{".", ".", ".", "/api/v1/timelines/home", ".", "GET"},
				{".", ".", ".", "/api/v1/accounts/verify_credentials", ".", "GET"},
				{".", ".", ".", "/api/v1/statuses", ".", "GET"},
				{".", ".", ".", "/api/v1/accounts/relationships", ".", "GET"},
			}, "timeSeries"),
			
			createMetricWidget("Response Times by Endpoint", 12, 0, 12, 6, [][]interface{}{
				{"Lesser/API", MetricLatencyP90, DimensionEndpoint, "/api/v1/statuses", DimensionMethod, "POST"},
				{".", ".", ".", "/api/v1/timelines/home", ".", "GET"},
				{".", ".", ".", "/api/v1/accounts/verify_credentials", ".", "GET"},
			}, "timeSeries"),

			// Row 2: Error Analysis
			createMetricWidget("Errors by Status Code", 0, 6, 8, 6, [][]interface{}{
				{"Lesser/API", "ErrorRequests", DimensionStatusCode, "400"},
				{".", ".", ".", "401"},
				{".", ".", ".", "403"},
				{".", ".", ".", "404"},
				{".", ".", ".", "429"},
				{".", ".", ".", "500"},
			}, "timeSeries"),
			
			createMetricWidget("Error Types", 8, 6, 8, 6, [][]interface{}{
				{"Lesser/API", MetricErrors, DimensionErrorType, ErrorTypeAuthentication},
				{".", ".", ".", ErrorTypeAuthorization},
				{".", ".", ".", ErrorTypeValidation},
				{".", ".", ".", ErrorTypeRateLimit},
				{".", ".", ".", ErrorTypeInternal},
			}, "timeSeries"),
			
			createNumberWidget("Success Rate %", 16, 6, 8, 6, [][]interface{}{
				{"Lesser/API", MetricSuccessRate, DimensionService, "api", map[string]string{"stat": "Average"}},
			}),

			// Row 3: Database Performance
			createMetricWidget("Database Latency", 0, 12, 12, 6, [][]interface{}{
				{"Lesser/API", MetricDynamoReadLatency, DimensionService, "api"},
				{"Lesser/API", MetricDynamoWriteLatency, DimensionService, "api"},
			}, "timeSeries"),
			
			createMetricWidget("Database Capacity", 12, 12, 12, 6, [][]interface{}{
				{"Lesser/API", MetricDynamoReadCapacity, DimensionService, "api"},
				{"Lesser/API", MetricDynamoWriteCapacity, DimensionService, "api"},
			}, "timeSeries"),

			// Row 4: Lambda Performance
			createMetricWidget("Lambda Duration & Memory", 0, 18, 12, 6, [][]interface{}{
				{"AWS/Lambda", "Duration", "FunctionName", fmt.Sprintf("lesser-api-%s", environment)},
				{".", "MemoryUtilization", ".", "."},
			}, "timeSeries"),
			
			createMetricWidget("Lambda Concurrency", 12, 18, 12, 6, [][]interface{}{
				{"AWS/Lambda", "ConcurrentExecutions", "FunctionName", fmt.Sprintf("lesser-api-%s", environment)},
				{"AWS/Lambda", "Throttles", ".", "."},
			}, "timeSeries"),
		},
	}
}

// CreateFederationDashboard creates a dashboard focused on ActivityPub federation
func CreateFederationDashboard(region, environment string) *DashboardConfig {
	return &DashboardConfig{
		Name:        fmt.Sprintf("Lesser-Federation-%s", environment),
		Description: "Lesser Federation - ActivityPub Health & Performance",
		Period:      300,
		Region:      region,
		Widgets: []DashboardWidget{
			// Row 1: Federation Overview
			createMetricWidget("Inbox Messages", 0, 0, 8, 6, [][]interface{}{
				{"Lesser/Federation", MetricInboxMessages, DimensionService, "inbox"},
			}, "timeSeries"),
			
			createMetricWidget("Outbox Messages", 8, 0, 8, 6, [][]interface{}{
				{"Lesser/Federation", MetricOutboxMessages, DimensionService, "outbox"},
			}, "timeSeries"),
			
			createMetricWidget("Signature Verification", 16, 0, 8, 6, [][]interface{}{
				{"Lesser/Federation", MetricSignatureVerification, "status", "success"},
				{".", ".", ".", "failure"},
			}, "timeSeries"),

			// Row 2: Federation Health by Instance
			createMetricWidget("Federation Success by Instance", 0, 6, 12, 6, [][]interface{}{
				{"Lesser/Federation", MetricFederationSuccess, DimensionInstance, "mastodon.social"},
				{".", ".", ".", "mastodon.online"},
				{".", ".", ".", "fosstodon.org"},
				{".", ".", ".", "pixelfed.social"},
			}, "timeSeries"),
			
			createMetricWidget("Federation Errors by Instance", 12, 6, 12, 6, [][]interface{}{
				{"Lesser/Federation", MetricFederationError, DimensionInstance, "mastodon.social"},
				{".", ".", ".", "mastodon.online"},
				{".", ".", ".", "fosstodon.org"},
				{".", ".", ".", "pixelfed.social"},
			}, "timeSeries"),

			// Row 3: Performance Metrics
			createMetricWidget("Federation Latency", 0, 12, 8, 6, [][]interface{}{
				{"Lesser/Federation", MetricFederationLatency, DimensionService, "inbox"},
				{"Lesser/Federation", MetricFederationLatency, DimensionService, "outbox"},
			}, "timeSeries"),
			
			createMetricWidget("Queue Processing", 8, 12, 8, 6, [][]interface{}{
				{"Lesser/Federation", MetricQueueDepth, DimensionQueue, "federation-delivery"},
				{"Lesser/Federation", MetricQueueProcessingTime, DimensionQueue, "federation-delivery"},
			}, "timeSeries"),
			
			createNumberWidget("Active Instances", 16, 12, 8, 6, [][]interface{}{
				{"Lesser/Federation", "ActiveInstances", DimensionService, "federation"},
			}),

			// Row 4: Error Analysis
			createMetricWidget("Error Types", 0, 18, 12, 6, [][]interface{}{
				{"Lesser/Federation", MetricErrors, DimensionErrorType, ErrorTypeFederation},
				{".", ".", ".", ErrorTypeTimeout},
				{".", ".", ".", ErrorTypeAuthentication},
				{".", ".", ".", ErrorTypeRateLimit},
			}, "timeSeries"),
			
			createNumberWidget("Success Rate %", 12, 18, 12, 6, [][]interface{}{
				{"Lesser/Federation", MetricSuccessRate, DimensionService, "federation"},
			}),
		},
	}
}

// CreateMediaProcessingDashboard creates a dashboard for media processing metrics
func CreateMediaProcessingDashboard(region, environment string) *DashboardConfig {
	return &DashboardConfig{
		Name:        fmt.Sprintf("Lesser-Media-Processing-%s", environment),
		Description: "Lesser Media Processing - Performance & Health",
		Period:      300,
		Region:      region,
		Widgets: []DashboardWidget{
			// Row 1: Processing Overview
			createMetricWidget("Media Processing Volume", 0, 0, 8, 6, [][]interface{}{
				{"Lesser/Media", MetricMediaProcessing, DimensionService, "media-processor"},
				{"Lesser/Media", "MediaProcessingCompleted", DimensionService, "media-processor"},
				{"Lesser/Media", "MediaProcessingFailed", DimensionService, "media-processor"},
			}, "timeSeries"),
			
			createMetricWidget("Processing Time by Type", 8, 0, 8, 6, [][]interface{}{
				{"Lesser/Media", MetricMediaProcessingTime, DimensionMediaType, mediaTypeImage},
				{".", ".", ".", mediaTypeVideo},
				{".", ".", ".", mediaTypeAudio},
			}, "timeSeries"),
			
			createMetricWidget("Queue Depth", 16, 0, 8, 6, [][]interface{}{
				{"Lesser/Media", MetricQueueDepth, DimensionQueue, "media-processing"},
			}, "timeSeries"),

			// Row 2: Media Types
			createMetricWidget("Processing by Media Type", 0, 6, 12, 6, [][]interface{}{
				{"Lesser/Media", MetricMediaProcessing, DimensionMediaType, mediaTypeImage},
				{".", ".", ".", mediaTypeVideo},
				{".", ".", ".", mediaTypeAudio},
				{".", ".", ".", mediaTypeGifv},
			}, "timeSeries"),
			
			createMetricWidget("File Sizes Processed", 12, 6, 12, 6, [][]interface{}{
				{"Lesser/Media", "MediaFileSizeProcessed", DimensionMediaType, mediaTypeImage, map[string]string{"stat": "Average"}},
				{".", ".", ".", mediaTypeVideo, map[string]string{"stat": "Average"}},
				{".", ".", ".", mediaTypeAudio, map[string]string{"stat": "Average"}},
			}, "timeSeries"),

			// Row 3: Error Analysis
			createMetricWidget("Processing Errors", 0, 12, 8, 6, [][]interface{}{
				{"Lesser/Media", "MediaProcessingErrors", DimensionErrorType, ErrorTypeValidation},
				{".", ".", ".", ErrorTypeTimeout},
				{".", ".", ".", ErrorTypeInternal},
			}, "timeSeries"),
			
			createMetricWidget("Retry Analysis", 8, 12, 8, 6, [][]interface{}{
				{"Lesser/Media", "MediaProcessingRetry", DimensionService, "media-processor"},
				{"Lesser/Media", "MediaProcessingFailed", DimensionService, "media-processor"},
			}, "timeSeries"),
			
			createNumberWidget("Success Rate %", 16, 12, 8, 6, [][]interface{}{
				{"Lesser/Media", MetricSuccessRate, DimensionService, "media-processor"},
			}),

			// Row 4: Cost & Performance
			createMetricWidget("Processing Costs", 0, 18, 12, 6, [][]interface{}{
				{"Lesser/Media", MetricCost, DimensionService, "media-processor"},
			}, "timeSeries"),
			
			createMetricWidget("Lambda Performance", 12, 18, 12, 6, [][]interface{}{
				{"AWS/Lambda", "Duration", "FunctionName", fmt.Sprintf("lesser-media-processor-%s", environment)},
				{"AWS/Lambda", "MemoryUtilization", ".", "."},
			}, "timeSeries"),
		},
	}
}

// Helper functions to create different widget types

func createMetricWidget(title string, x, y, width, height int, metrics [][]interface{}, viewType string) DashboardWidget {
	return DashboardWidget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
		Properties: DashboardWidgetProps{
			Title:   title,
			View:    viewType,
			Region:  os.Getenv("AWS_REGION"),
			Period:  300,
			Metrics: metrics,
			YAxis: &YAxisConfig{
				Left: &AxisConfig{
					Min: 0,
				},
			},
			Annotations: &AnnotationConfig{
				Horizontal: []HorizontalAnnotation{
					{
						Value:   AlertP2ErrorRatePercent,
						Label:   "P2 Threshold",
						Color:   "#ff7f0e",
						Visible: true,
					},
					{
						Value:   AlertP1ErrorRatePercent,
						Label:   "P1 Threshold", 
						Color:   "#d62728",
						Visible: true,
					},
				},
			},
		},
	}
}

func createNumberWidget(title string, x, y, width, height int, metrics [][]interface{}) DashboardWidget {
	return DashboardWidget{
		Type:   "metric",
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
		Properties: DashboardWidgetProps{
			Title:   title,
			View:    "singleValue",
			Region:  os.Getenv("AWS_REGION"),
			Period:  300,
			Metrics: metrics,
			Stat:    "Average",
		},
	}
}

// ToJSON converts dashboard config to JSON string
func (dc *DashboardConfig) ToJSON() (string, error) {
	jsonBytes, err := json.MarshalIndent(dc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal dashboard config: %w", err)
	}
	return string(jsonBytes), nil
}

// GetAllDashboards returns all available dashboard configurations
func GetAllDashboards(region, environment string) []*DashboardConfig {
	return []*DashboardConfig{
		CreateLesserOverviewDashboard(region, environment),
		CreateAPIPerformanceDashboard(region, environment),
		CreateFederationDashboard(region, environment),
		CreateMediaProcessingDashboard(region, environment),
	}
}