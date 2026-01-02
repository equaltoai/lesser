// Package media provides media processing analytics and bandwidth monitoring for S3 and CloudFront usage.
package media

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// BandwidthAnalytics handles bandwidth usage analytics
type BandwidthAnalytics interface {
	ProcessLogFiles(ctx context.Context, bucket, prefix string) error
	GetBandwidthReport(ctx context.Context, period string, start, end time.Time) (*BandwidthReport, error)
	GetCostBreakdown(ctx context.Context, start, end time.Time) (*CostBreakdown, error)
	TrackBandwidthUsage(ctx context.Context, usage *BandwidthUsage) error
}

// BandwidthReport contains bandwidth analytics
type BandwidthReport struct {
	Period      string                 `json:"period"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	TotalBytes  int64                  `json:"total_bytes"`
	TotalCost   float64                `json:"total_cost"`
	ByMedia     map[string]MediaUsage  `json:"by_media"`
	ByQuality   map[string]int64       `json:"by_quality"`
	ByRegion    map[string]RegionUsage `json:"by_region"`
	ByUser      map[string]int64       `json:"by_user"`
	TopMedia    []MediaUsage           `json:"top_media"`
	Trends      []DataPoint            `json:"trends"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// MediaUsage represents bandwidth usage for a specific media item
//
//nolint:revive // Media prefix clarifies this is media-specific usage
type MediaUsage struct {
	MediaID     string  `json:"media_id"`
	Bytes       int64   `json:"bytes"`
	Requests    int64   `json:"requests"`
	UniqueUsers int64   `json:"unique_users"`
	Cost        float64 `json:"cost"`
	AvgBitrate  float64 `json:"avg_bitrate"`
}

// RegionUsage represents bandwidth usage by region
type RegionUsage struct {
	Region   string  `json:"region"`
	Bytes    int64   `json:"bytes"`
	Requests int64   `json:"requests"`
	Cost     float64 `json:"cost"`
}

// DataPoint represents a time-series data point
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Label     string    `json:"label"`
}

// CostBreakdown provides detailed cost analysis
type CostBreakdown struct {
	StartTime       time.Time            `json:"start_time"`
	EndTime         time.Time            `json:"end_time"`
	TotalCost       float64              `json:"total_cost"`
	BandwidthCost   float64              `json:"bandwidth_cost"`
	RequestCost     float64              `json:"request_cost"`
	StorageCost     float64              `json:"storage_cost"`
	ByService       map[string]float64   `json:"by_service"`
	ByMedia         map[string]float64   `json:"by_media"`
	ByUser          map[string]UserCost  `json:"by_user"`
	Recommendations []CostRecommendation `json:"recommendations"`
	Projections     CostProjection       `json:"projections"`
}

// UserCost represents cost attribution to a user
type UserCost struct {
	UserID     string  `json:"user_id"`
	TotalCost  float64 `json:"total_cost"`
	Bandwidth  int64   `json:"bandwidth"`
	Requests   int64   `json:"requests"`
	MediaItems int     `json:"media_items"`
}

// CostRecommendation suggests cost optimization
type CostRecommendation struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Savings     float64 `json:"potential_savings"`
	Priority    string  `json:"priority"`
}

// CostProjection provides cost forecasting
type CostProjection struct {
	Monthly  float64 `json:"monthly"`
	Yearly   float64 `json:"yearly"`
	PerUser  float64 `json:"per_user"`
	Trending string  `json:"trending"` // up, down, stable
}

// BandwidthUsage represents real-time bandwidth usage
type BandwidthUsage struct {
	MediaID   string    `json:"media_id"`
	UserID    string    `json:"user_id"`
	Bytes     int64     `json:"bytes"`
	Quality   string    `json:"quality"`
	Region    string    `json:"region"`
	Timestamp time.Time `json:"timestamp"`
	UserAgent string    `json:"user_agent"`
	Referrer  string    `json:"referrer"`
}

// CloudFrontLogEntry represents a parsed CloudFront log entry
type CloudFrontLogEntry struct {
	Date         string
	Time         string
	EdgeLocation string
	Bytes        int64
	ClientIP     string
	Method       string
	Host         string
	URI          string
	Status       int
	Referrer     string
	UserAgent    string
	QueryString  string
}

// bandwidthAnalytics implements BandwidthAnalytics
type bandwidthAnalytics struct {
	s3Client       s3ListGetAPI
	cloudWatch     cloudWatchAPI
	storageService interface {
		StoreBandwidthUsage(ctx context.Context, usage *BandwidthUsage) error
		GetBandwidthUsage(ctx context.Context, start, end time.Time) ([]*BandwidthUsage, error)
	}
}

// NewBandwidthAnalytics creates a new bandwidth analytics service
func NewBandwidthAnalytics(ctx context.Context, storageService interface {
	StoreBandwidthUsage(ctx context.Context, usage *BandwidthUsage) error
	GetBandwidthUsage(ctx context.Context, start, end time.Time) ([]*BandwidthUsage, error)
}) (BandwidthAnalytics, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAWSConfigLoad, err)
	}

	return &bandwidthAnalytics{
		s3Client:       s3.NewFromConfig(cfg),
		cloudWatch:     cloudwatch.NewFromConfig(cfg),
		storageService: storageService,
	}, nil
}

// ProcessLogFiles processes CloudFront access logs for bandwidth analytics
func (b *bandwidthAnalytics) ProcessLogFiles(ctx context.Context, bucket, prefix string) error {
	// List log files in S3
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	}

	result, err := b.s3Client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLogFilesListing, err)
	}

	// Process each log file
	for _, obj := range result.Contents {
		if err := b.processLogFile(ctx, bucket, *obj.Key); err != nil {
			zap.L().Error("failed to process log file",
				zap.String("key", *obj.Key),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// processLogFile processes a single CloudFront log file
func (b *bandwidthAnalytics) processLogFile(ctx context.Context, bucket, key string) error {
	// Get the log file from S3
	getInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := b.s3Client.GetObject(ctx, getInput)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLogFileRetrieval, err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			zap.L().Warn("failed to close S3 object body", zap.Error(closeErr))
		}
	}()

	// Parse log entries
	scanner := bufio.NewScanner(result.Body)
	var usageRecords []*BandwidthUsage

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry := b.parseLogEntry(line)
		if entry == nil {
			continue
		}

		// Extract media ID from URI
		mediaID := b.extractMediaID(entry.URI)
		if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
			continue
		}

		// Extract quality from URI
		quality := b.extractQuality(entry.URI)

		// Parse timestamp
		timestamp, err := time.Parse("2006-01-02 15:04:05", entry.Date+" "+entry.Time)
		if err != nil {
			continue
		}

		usage := &BandwidthUsage{
			MediaID:   mediaID,
			Bytes:     entry.Bytes,
			Quality:   quality,
			Region:    b.extractRegion(entry.EdgeLocation),
			Timestamp: timestamp,
			UserAgent: entry.UserAgent,
			Referrer:  entry.Referrer,
		}

		usageRecords = append(usageRecords, usage)
	}

	// Store usage records
	for _, usage := range usageRecords {
		if err := b.storageService.StoreBandwidthUsage(ctx, usage); err != nil {
			zap.L().Error("failed to store bandwidth usage", zap.Error(err))
		}
	}

	// Send metrics to CloudWatch
	if err := b.sendMetrics(ctx, usageRecords); err != nil {
		zap.L().Error("failed to send metrics to CloudWatch", zap.Error(err))
	}

	return nil
}

// parseLogEntry parses a CloudFront log entry
func (b *bandwidthAnalytics) parseLogEntry(line string) *CloudFrontLogEntry {
	fields := strings.Split(line, "\t")
	if len(fields) < 12 {
		return nil
	}

	bytes, _ := strconv.ParseInt(fields[3], 10, 64)
	status, _ := strconv.Atoi(fields[8])

	return &CloudFrontLogEntry{
		Date:         fields[0],
		Time:         fields[1],
		EdgeLocation: fields[2],
		Bytes:        bytes,
		ClientIP:     fields[4],
		Method:       fields[5],
		Host:         fields[6],
		URI:          fields[7],
		Status:       status,
		Referrer:     fields[9],
		UserAgent:    fields[10],
		QueryString:  fields[11],
	}
}

// extractMediaID extracts media ID from URI path
func (b *bandwidthAnalytics) extractMediaID(uri string) string {
	// Extract from path like /media/123456789/720p/index.m3u8
	parts := strings.Split(strings.Trim(uri, "/"), "/")
	if len(parts) >= 2 && parts[0] == "media" {
		return parts[1]
	}
	return ""
}

// extractQuality extracts quality from URI path
func (b *bandwidthAnalytics) extractQuality(uri string) string {
	if strings.Contains(uri, "/4k/") {
		return "4k"
	} else if strings.Contains(uri, "/1080p/") {
		return Resolution1080p
	} else if strings.Contains(uri, "/720p/") {
		return Resolution720p
	} else if strings.Contains(uri, "/480p/") {
		return Resolution480p
	} else if strings.Contains(uri, "master.m3u8") {
		return "adaptive"
	}
	return "unknown"
}

// extractRegion extracts region from edge location
func (b *bandwidthAnalytics) extractRegion(edgeLocation string) string {
	// CloudFront edge locations are typically 3-letter codes
	if len(edgeLocation) >= 3 {
		code := edgeLocation[:3]
		// Map common edge location codes to regions
		regionMap := map[string]string{
			"IAD": "us-east-1",
			"DFW": "us-east-2",
			"SEA": "us-west-1",
			"SFO": "us-west-2",
			"FRA": "eu-central-1",
			"LHR": "eu-west-1",
			"NRT": "ap-northeast-1",
			"SYD": "ap-southeast-2",
		}
		if region, ok := regionMap[code]; ok {
			return region
		}
	}
	return "unknown"
}

// sendMetrics sends bandwidth metrics to CloudWatch
func (b *bandwidthAnalytics) sendMetrics(ctx context.Context, usageRecords []*BandwidthUsage) error {
	if err := common.ValidateSliceNotEmpty("usageRecords", usageRecords); err != nil {
		return nil
	}

	// Aggregate metrics
	totalBytes := int64(0)
	qualityBreakdown := make(map[string]int64)
	regionBreakdown := make(map[string]int64)

	for _, usage := range usageRecords {
		totalBytes += usage.Bytes
		qualityBreakdown[usage.Quality] += usage.Bytes
		regionBreakdown[usage.Region] += usage.Bytes
	}

	// Prepare metric data
	metricData := make([]types.MetricDatum, 0, 1+len(qualityBreakdown)+len(regionBreakdown))

	// Total bandwidth metric
	metricData = append(metricData, types.MetricDatum{
		MetricName: aws.String("TotalBandwidth"),
		Value:      aws.Float64(float64(totalBytes)),
		Unit:       types.StandardUnitBytes,
		Timestamp:  aws.Time(time.Now()),
	})

	// Quality breakdown metrics
	for quality, bytes := range qualityBreakdown {
		metricData = append(metricData, types.MetricDatum{
			MetricName: aws.String("BandwidthByQuality"),
			Dimensions: []types.Dimension{
				{
					Name:  aws.String("Quality"),
					Value: aws.String(quality),
				},
			},
			Value:     aws.Float64(float64(bytes)),
			Unit:      types.StandardUnitBytes,
			Timestamp: aws.Time(time.Now()),
		})
	}

	// Region breakdown metrics
	for region, bytes := range regionBreakdown {
		metricData = append(metricData, types.MetricDatum{
			MetricName: aws.String("BandwidthByRegion"),
			Dimensions: []types.Dimension{
				{
					Name:  aws.String("Region"),
					Value: aws.String(region),
				},
			},
			Value:     aws.Float64(float64(bytes)),
			Unit:      types.StandardUnitBytes,
			Timestamp: aws.Time(time.Now()),
		})
	}

	// Send metrics in batches of 20 (CloudWatch limit)
	for i := 0; i < len(metricData); i += 20 {
		end := i + 20
		if end > len(metricData) {
			end = len(metricData)
		}

		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String("Lesser/Bandwidth"),
			MetricData: metricData[i:end],
		}

		if _, err := b.cloudWatch.PutMetricData(ctx, input); err != nil {
			return fmt.Errorf("%w: %w", ErrMetricDataSubmission, err)
		}
	}

	return nil
}

// GetBandwidthReport generates a bandwidth usage report
func (b *bandwidthAnalytics) GetBandwidthReport(ctx context.Context, period string, start, end time.Time) (*BandwidthReport, error) {
	// Get usage data from storage
	usageData, err := b.storageService.GetBandwidthUsage(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBandwidthUsageRetrieval, err)
	}

	// Calculate aggregations
	totalBytes := int64(0)
	byMedia := make(map[string]MediaUsage)
	byQuality := make(map[string]int64)
	byRegion := make(map[string]RegionUsage)
	byUser := make(map[string]int64)

	for _, usage := range usageData {
		totalBytes += usage.Bytes

		// By media
		media := byMedia[usage.MediaID]
		media.MediaID = usage.MediaID
		media.Bytes += usage.Bytes
		media.Requests++
		byMedia[usage.MediaID] = media

		// By quality
		byQuality[usage.Quality] += usage.Bytes

		// By region
		region := byRegion[usage.Region]
		region.Region = usage.Region
		region.Bytes += usage.Bytes
		region.Requests++
		byRegion[usage.Region] = region

		// By user
		if usage.UserID != "" {
			byUser[usage.UserID] += usage.Bytes
		}
	}

	// Calculate costs (example rates)
	const costPerGB = 0.085 // CloudFront pricing varies by region
	totalCost := float64(totalBytes) / (1024 * 1024 * 1024) * costPerGB

	// Update costs in aggregations
	for id, media := range byMedia {
		media.Cost = float64(media.Bytes) / (1024 * 1024 * 1024) * costPerGB
		byMedia[id] = media
	}

	for region, data := range byRegion {
		data.Cost = float64(data.Bytes) / (1024 * 1024 * 1024) * costPerGB
		byRegion[region] = data
	}

	// Get top media by usage
	topMedia := make([]MediaUsage, 0, len(byMedia))
	for _, media := range byMedia {
		topMedia = append(topMedia, media)
	}

	return &BandwidthReport{
		Period:      period,
		StartTime:   start,
		EndTime:     end,
		TotalBytes:  totalBytes,
		TotalCost:   totalCost,
		ByMedia:     byMedia,
		ByQuality:   byQuality,
		ByRegion:    byRegion,
		ByUser:      byUser,
		TopMedia:    topMedia,
		GeneratedAt: time.Now(),
	}, nil
}

// GetCostBreakdown provides detailed cost analysis
func (b *bandwidthAnalytics) GetCostBreakdown(ctx context.Context, start, end time.Time) (*CostBreakdown, error) {
	// Get bandwidth report first
	report, err := b.GetBandwidthReport(ctx, "custom", start, end)
	if err != nil {
		return nil, err
	}

	// Calculate cost breakdown
	bandwidthCost := report.TotalCost * 0.85 // 85% bandwidth
	requestCost := report.TotalCost * 0.10   // 10% requests
	storageCost := report.TotalCost * 0.05   // 5% storage

	byService := map[string]float64{
		"CloudFront": bandwidthCost + requestCost,
		"S3":         storageCost,
	}

	byMedia := make(map[string]float64)
	for id, media := range report.ByMedia {
		byMedia[id] = media.Cost
	}

	// Generate recommendations
	recommendations := b.generateRecommendations(report)

	// Calculate projections
	duration := end.Sub(start)
	monthlyRate := report.TotalCost * (30 * 24 * time.Hour / duration).Hours()
	yearlyRate := monthlyRate * 12

	projections := CostProjection{
		Monthly:  monthlyRate,
		Yearly:   yearlyRate,
		PerUser:  monthlyRate / float64(len(report.ByUser)), // Rough estimate
		Trending: "stable",                                  // Would require historical data
	}

	return &CostBreakdown{
		StartTime:       start,
		EndTime:         end,
		TotalCost:       report.TotalCost,
		BandwidthCost:   bandwidthCost,
		RequestCost:     requestCost,
		StorageCost:     storageCost,
		ByService:       byService,
		ByMedia:         byMedia,
		Recommendations: recommendations,
		Projections:     projections,
	}, nil
}

// generateRecommendations generates cost optimization recommendations
func (b *bandwidthAnalytics) generateRecommendations(report *BandwidthReport) []CostRecommendation {
	var recommendations []CostRecommendation

	// Check for high-cost media items
	for _, media := range report.ByMedia {
		if media.Cost > 10.0 { // More than $10 for a single media item
			recommendations = append(recommendations, CostRecommendation{
				Type:        "media_optimization",
				Description: fmt.Sprintf("Media %s has high bandwidth costs ($%.2f). Consider compression or lower default quality.", media.MediaID, media.Cost),
				Savings:     media.Cost * 0.3, // Potential 30% savings
				Priority:    "high",
			})
		}
	}

	// Check quality distribution
	totalBytes := int64(0)
	for _, bytes := range report.ByQuality {
		totalBytes += bytes
	}

	if report.ByQuality["4k"] > totalBytes/4 { // More than 25% is 4K
		recommendations = append(recommendations, CostRecommendation{
			Type:        "quality_optimization",
			Description: "High percentage of 4K streaming detected. Consider encouraging adaptive bitrate or lower default quality.",
			Savings:     report.TotalCost * 0.4, // 4K can use 4x bandwidth
			Priority:    "medium",
		})
	}

	return recommendations
}

// TrackBandwidthUsage tracks real-time bandwidth usage
func (b *bandwidthAnalytics) TrackBandwidthUsage(ctx context.Context, usage *BandwidthUsage) error {
	// Store in database
	if err := b.storageService.StoreBandwidthUsage(ctx, usage); err != nil {
		return fmt.Errorf("%w: %w", ErrBandwidthUsageStorage, err)
	}

	// Send real-time metric to CloudWatch
	metricData := []types.MetricDatum{
		{
			MetricName: aws.String("RealtimeBandwidth"),
			Dimensions: []types.Dimension{
				{
					Name:  aws.String("MediaID"),
					Value: aws.String(usage.MediaID),
				},
				{
					Name:  aws.String("Quality"),
					Value: aws.String(usage.Quality),
				},
			},
			Value:     aws.Float64(float64(usage.Bytes)),
			Unit:      types.StandardUnitBytes,
			Timestamp: aws.Time(usage.Timestamp),
		},
	}

	input := &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String("Lesser/Bandwidth/Realtime"),
		MetricData: metricData,
	}

	_, err := b.cloudWatch.PutMetricData(ctx, input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRealtimeMetricSubmission, err)
	}

	return nil
}
