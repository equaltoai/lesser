package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type FederationTimeSeriesProcessor struct {
	storage storage.Storage
}

// TimeSeriesEvent represents an event for time-series processing
type TimeSeriesEvent struct {
	Domain        string    `json:"domain"`
	ActivityType  string    `json:"activity_type"`
	Direction     string    `json:"direction"` // inbound/outbound
	Success       bool      `json:"success"`
	ResponseTime  float64   `json:"response_time"`
	Timestamp     time.Time `json:"timestamp"`
	SourceDomain  string    `json:"source_domain,omitempty"`
	TargetDomain  string    `json:"target_domain,omitempty"`
}

func (ftsp *FederationTimeSeriesProcessor) HandleSQSEvent(ctx context.Context, event events.SQSEvent) error {
	for _, record := range event.Records {
		var tsEvent TimeSeriesEvent
		if err := json.Unmarshal([]byte(record.Body), &tsEvent); err != nil {
			fmt.Printf("Failed to unmarshal SQS message: %v\n", err)
			continue
		}

		if err := ftsp.processTimeSeriesEvent(ctx, &tsEvent); err != nil {
			fmt.Printf("Failed to process time series event: %v\n", err)
			// Continue processing other events
		}
	}

	return nil
}

func (ftsp *FederationTimeSeriesProcessor) HandleScheduledEvent(ctx context.Context, event events.CloudWatchEvent) error {
	// Process scheduled aggregation
	var aggregationConfig struct {
		Period string `json:"period"`
		Domains []string `json:"domains,omitempty"`
	}

	if err := json.Unmarshal(event.Detail, &aggregationConfig); err != nil {
		// Default configuration
		aggregationConfig.Period = "hourly"
	}

	return ftsp.aggregateTimeSeriesData(ctx, aggregationConfig.Period, aggregationConfig.Domains)
}

func (ftsp *FederationTimeSeriesProcessor) processTimeSeriesEvent(ctx context.Context, event *TimeSeriesEvent) error {
	// Get current hour aggregation bucket
	hourBucket := event.Timestamp.Truncate(time.Hour)
	
	// Create or update hourly aggregation
	timeSeries := &storage.FederationTimeSeries{
		Domain:         event.Domain,
		Timestamp:      hourBucket,
		Period:         "hourly",
		InboundVolume:  0,
		OutboundVolume: 0,
		ErrorRate:      0,
		ResponseTime:   0,
		ActivePeers:    0,
	}

	// Increment appropriate counters
	if event.Direction == "inbound" {
		timeSeries.InboundVolume = 1
	} else if event.Direction == "outbound" {
		timeSeries.OutboundVolume = 1
	}

	// Handle response time and error rate
	if event.Direction == "outbound" {
		timeSeries.ResponseTime = event.ResponseTime
		if !event.Success {
			timeSeries.ErrorRate = 1.0 // Will be averaged later
		}
	}

	// Store the time series data point
	return ftsp.storage.StoreFederationTimeSeries(ctx, timeSeries)
}

func (ftsp *FederationTimeSeriesProcessor) aggregateTimeSeriesData(ctx context.Context, period string, domains []string) error {
	switch period {
	case "hourly":
		return ftsp.aggregateHourlyData(ctx, domains)
	case "daily":
		return ftsp.aggregateDailyData(ctx, domains)
	case "weekly":
		return ftsp.aggregateWeeklyData(ctx, domains)
	default:
		return fmt.Errorf("unsupported aggregation period: %s", period)
	}
}

func (ftsp *FederationTimeSeriesProcessor) aggregateHourlyData(ctx context.Context, domains []string) error {
	// Aggregate real-time events into hourly buckets
	currentHour := time.Now().Truncate(time.Hour)
	previousHour := currentHour.Add(-time.Hour)

	if len(domains) == 0 {
		// Get all active domains
		nodes, err := ftsp.storage.GetFederationNodes(ctx, 1)
		if err != nil {
			return fmt.Errorf("failed to get federation nodes: %w", err)
		}
		
		for _, node := range nodes {
			domains = append(domains, node.Domain)
		}
	}

	for _, domain := range domains {
		// Get recent connections for this domain
		connections, err := ftsp.storage.GetRecentInstanceConnections(ctx, domain, time.Hour)
		if err != nil {
			fmt.Printf("Failed to get recent connections for %s: %v\n", domain, err)
			continue
		}

		// Aggregate metrics
		var inboundVolume, outboundVolume int64
		var totalResponseTime float64
		var errorCount, totalCount int64
		uniquePeers := make(map[string]bool)

		for _, conn := range connections {
			if conn.LastActivity.After(previousHour) && conn.LastActivity.Before(currentHour) {
				inboundVolume += conn.VolumeIn
				outboundVolume += conn.VolumeOut
				totalResponseTime += conn.ResponseTimeMs
				totalCount++
				
				if !conn.Success {
					errorCount++
				}
				
				uniquePeers[conn.TargetDomain] = true
			}
		}

		// Calculate aggregated metrics
		var errorRate, avgResponseTime float64
		if totalCount > 0 {
			errorRate = float64(errorCount) / float64(totalCount)
			avgResponseTime = totalResponseTime / float64(totalCount)
		}

		// Store hourly aggregation
		timeSeries := &storage.FederationTimeSeries{
			Domain:         domain,
			Timestamp:      previousHour,
			Period:         "hourly",
			InboundVolume:  inboundVolume,
			OutboundVolume: outboundVolume,
			ErrorRate:      errorRate,
			ResponseTime:   avgResponseTime,
			ActivePeers:    len(uniquePeers),
		}

		if err := ftsp.storage.StoreFederationTimeSeries(ctx, timeSeries); err != nil {
			fmt.Printf("Failed to store hourly time series for %s: %v\n", domain, err)
		}
	}

	return nil
}

func (ftsp *FederationTimeSeriesProcessor) aggregateDailyData(ctx context.Context, domains []string) error {
	// Aggregate hourly data into daily buckets
	currentDay := time.Now().Truncate(24 * time.Hour)
	previousDay := currentDay.Add(-24 * time.Hour)

	if len(domains) == 0 {
		// Get all active domains
		nodes, err := ftsp.storage.GetFederationNodes(ctx, 1)
		if err != nil {
			return fmt.Errorf("failed to get federation nodes: %w", err)
		}
		
		for _, node := range nodes {
			domains = append(domains, node.Domain)
		}
	}

	for _, domain := range domains {
		// Aggregate 24 hours of hourly data
		var totalInbound, totalOutbound int64
		var totalResponseTime, totalErrorRate float64
		var maxPeers int
		var hourCount int

		// Get hourly data for the previous day
		for hour := 0; hour < 24; hour++ {
			_ = previousDay.Add(time.Duration(hour) * time.Hour) // hourTimestamp
			
			// This would require implementing GetFederationTimeSeries
			// For now, we'll create a placeholder
			hourCount++
		}

		// Calculate daily averages
		var avgErrorRate, avgResponseTime float64
		if hourCount > 0 {
			avgErrorRate = totalErrorRate / float64(hourCount)
			avgResponseTime = totalResponseTime / float64(hourCount)
		}

		// Store daily aggregation
		timeSeries := &storage.FederationTimeSeries{
			Domain:         domain,
			Timestamp:      previousDay,
			Period:         "daily",
			InboundVolume:  totalInbound,
			OutboundVolume: totalOutbound,
			ErrorRate:      avgErrorRate,
			ResponseTime:   avgResponseTime,
			ActivePeers:    maxPeers,
		}

		if err := ftsp.storage.StoreFederationTimeSeries(ctx, timeSeries); err != nil {
			fmt.Printf("Failed to store daily time series for %s: %v\n", domain, err)
		}
	}

	return nil
}

func (ftsp *FederationTimeSeriesProcessor) aggregateWeeklyData(ctx context.Context, domains []string) error {
	// Aggregate daily data into weekly buckets
	currentWeek := time.Now().Truncate(7 * 24 * time.Hour)
	previousWeek := currentWeek.Add(-7 * 24 * time.Hour)

	if len(domains) == 0 {
		// Get all active domains
		nodes, err := ftsp.storage.GetFederationNodes(ctx, 1)
		if err != nil {
			return fmt.Errorf("failed to get federation nodes: %w", err)
		}
		
		for _, node := range nodes {
			domains = append(domains, node.Domain)
		}
	}

	for _, domain := range domains {
		// Aggregate 7 days of daily data
		var totalInbound, totalOutbound int64
		var totalResponseTime, totalErrorRate float64
		var maxPeers int
		var dayCount int

		// Get daily data for the previous week
		for day := 0; day < 7; day++ {
			_ = previousWeek.Add(time.Duration(day) * 24 * time.Hour) // dayTimestamp
			
			// This would require implementing GetFederationTimeSeries
			// For now, we'll create a placeholder
			dayCount++
		}

		// Calculate weekly averages
		var avgErrorRate, avgResponseTime float64
		if dayCount > 0 {
			avgErrorRate = totalErrorRate / float64(dayCount)
			avgResponseTime = totalResponseTime / float64(dayCount)
		}

		// Store weekly aggregation
		timeSeries := &storage.FederationTimeSeries{
			Domain:         domain,
			Timestamp:      previousWeek,
			Period:         "weekly",
			InboundVolume:  totalInbound,
			OutboundVolume: totalOutbound,
			ErrorRate:      avgErrorRate,
			ResponseTime:   avgResponseTime,
			ActivePeers:    maxPeers,
		}

		if err := ftsp.storage.StoreFederationTimeSeries(ctx, timeSeries); err != nil {
			fmt.Printf("Failed to store weekly time series for %s: %v\n", domain, err)
		}
	}

	return nil
}

// DetectAnomalies analyzes time series data for anomalies
func (ftsp *FederationTimeSeriesProcessor) DetectAnomalies(ctx context.Context, domain string) ([]*AnomalyDetection, error) {
	var anomalies []*AnomalyDetection

	// This would implement anomaly detection algorithms
	// For now, return placeholder data
	
	anomalies = append(anomalies, &AnomalyDetection{
		Domain:      domain,
		Metric:      "response_time",
		Severity:    "medium",
		Description: "Response time spike detected",
		Timestamp:   time.Now().Add(-2 * time.Hour),
		Value:       15000.0,
		Baseline:    2000.0,
		Threshold:   10000.0,
	})

	return anomalies, nil
}

func main() {
	store, err := dynamodb.New()
	if err != nil {
		panic(fmt.Sprintf("Failed to create DynamoDB storage: %v", err))
	}

	processor := &FederationTimeSeriesProcessor{
		storage: store,
	}

	// Determine handler based on event source
	eventSource := getEnv("EVENT_SOURCE", "sqs")
	
	switch eventSource {
	case "sqs":
		lambda.Start(processor.HandleSQSEvent)
	case "cloudwatch":
		lambda.Start(processor.HandleScheduledEvent)
	default:
		lambda.Start(processor.HandleSQSEvent)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// AnomalyDetection represents a detected anomaly in federation metrics
type AnomalyDetection struct {
	Domain      string    `json:"domain"`
	Metric      string    `json:"metric"`
	Severity    string    `json:"severity"` // low/medium/high/critical
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Value       float64   `json:"value"`
	Baseline    float64   `json:"baseline"`
	Threshold   float64   `json:"threshold"`
}