package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type aggregatedMetricsRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"-"`

	Period      string    `theorydb:"attr:period" json:"-"`
	Type        string    `theorydb:"attr:type" json:"-"`
	Service     string    `theorydb:"attr:service" json:"-"`
	WindowStart time.Time `theorydb:"attr:windowStart" json:"-"`
}

func (aggregatedMetricsRecord) TableName() string {
	return models.MainTableName
}

type bookmarkRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK" json:"-"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK" json:"-"`

	Username     string    `theorydb:"attr:username" json:"-"`
	ObjectID     string    `theorydb:"attr:objectID" json:"-"`
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"-"`
	RecordType   string    `theorydb:"attr:recordType" json:"-"`
	TimeRecordSK string    `theorydb:"attr:timeRecordSK" json:"-"`
}

func (bookmarkRecord) TableName() string {
	return models.MainTableName
}

type federationEdgeRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK" json:"-"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK" json:"-"`

	SourceDomain   string    `theorydb:"attr:sourceDomain" json:"-"`
	TargetDomain   string    `theorydb:"attr:targetDomain" json:"-"`
	ConnectionType string    `theorydb:"attr:connectionType" json:"-"`
	Strength       float64   `theorydb:"attr:strength" json:"-"`
	LastActivity   time.Time `theorydb:"attr:lastActivity" json:"-"`
}

func (federationEdgeRecord) TableName() string {
	return models.MainTableName
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	fs := flag.NewFlagSet("dynamodb-backfill-m4", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		tableName string
		region    string
		endpoint  string
		dryRun    bool
	)

	fs.StringVar(&tableName, "table", os.Getenv("LESSER_DYNAMODB_TABLE"), "DynamoDB table name (or env LESSER_DYNAMODB_TABLE)")
	fs.StringVar(&region, "region", envOrDefault("AWS_REGION", "us-east-1"), "AWS region")
	fs.StringVar(&endpoint, "endpoint", os.Getenv("DYNAMODB_ENDPOINT"), "DynamoDB endpoint override (optional; for local DynamoDB)")
	fs.BoolVar(&dryRun, "dry-run", true, "when true, log planned updates without writing")

	if err := fs.Parse(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 2
	}

	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		_, _ = fmt.Fprintln(os.Stderr, "Error: --table (or LESSER_DYNAMODB_TABLE) is required")
		return 2
	}

	logger := newLogger()
	defer func() { _ = logger.Sync() }()

	// Override model table name resolution so this tool can target any environment/stage.
	models.MainTableName = tableName

	cfg := session.Config{Region: region}
	if strings.TrimSpace(endpoint) != "" {
		cfg.Endpoint = strings.TrimSpace(endpoint)
	}

	db, err := tabletheory.New(cfg)
	if err != nil {
		logger.Error("failed to initialize TableTheory client", zap.Error(err))
		return 1
	}

	start := time.Now()
	updated := 0

	metricsUpdated, err := backfillAggregatedMetricsGSI2(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("aggregated metrics backfill failed", zap.Error(err))
		return 1
	}
	updated += metricsUpdated

	bookmarkUpdated, err := backfillBookmarkObjectIndex(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("bookmark backfill failed", zap.Error(err))
		return 1
	}
	updated += bookmarkUpdated

	edgeUpdated, err := backfillFederationEdgeStrongestIndex(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("federation edge backfill failed", zap.Error(err))
		return 1
	}
	updated += edgeUpdated

	logger.Info("backfill complete",
		zap.Bool("dry_run", dryRun),
		zap.Int("items_updated", updated),
		zap.Duration("duration", time.Since(start)))

	return 0
}

func backfillAggregatedMetricsGSI2(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []aggregatedMetricsRecord
	err := db.WithContext(ctx).Model(&aggregatedMetricsRecord{}).
		Filter("PK", "BEGINS_WITH", "metrics_agg#").
		Filter("SK", "BEGINS_WITH", "window#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		period := strings.TrimSpace(row.Period)
		metricType := strings.TrimSpace(row.Type)
		service := strings.TrimSpace(row.Service)

		if period == "" || metricType == "" {
			// Best-effort parse: metrics_agg#{period}#{type}
			parts := strings.Split(row.PK, "#")
			if len(parts) >= 3 {
				period = strings.TrimSpace(parts[1])
				metricType = strings.TrimSpace(parts[2])
			}
		}

		windowStart := row.WindowStart
		if windowStart.IsZero() {
			// Best-effort parse: window#{rfc3339}
			raw := strings.TrimPrefix(row.SK, "window#")
			if parsed, parseErr := time.Parse(time.RFC3339, raw); parseErr == nil {
				windowStart = parsed
			}
		}

		if period == "" || metricType == "" || windowStart.IsZero() {
			continue
		}

		expectedPK := fmt.Sprintf("METRICS_AGG#%s", period)
		expectedSK := fmt.Sprintf("WINDOW#%s#TYPE#%s#SERVICE#%s",
			windowStart.UTC().Format(time.RFC3339),
			strings.ToLower(metricType),
			strings.ToLower(service),
		)

		if row.GSI2PK == expectedPK && row.GSI2SK == expectedSK {
			continue
		}

		logger.Debug("backfilling aggregated metrics gsi2 keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi2PK", expectedPK),
			zap.String("gsi2SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&aggregatedMetricsRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI2PK", expectedPK).
			Set("GSI2SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, err
		}
		updated++
	}

	logger.Info("aggregated metrics gsi2 backfill complete",
		zap.Int("items_updated", updated),
		zap.Int("rows_scanned", len(rows)))

	return updated, nil
}

func backfillBookmarkObjectIndex(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []bookmarkRecord
	err := db.WithContext(ctx).Model(&bookmarkRecord{}).
		Filter("PK", "BEGINS_WITH", "BOOKMARK#").
		Filter("SK", "BEGINS_WITH", "OBJECT#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		username := strings.TrimPrefix(row.PK, "BOOKMARK#")
		objectID := row.ObjectID
		if objectID == "" {
			objectID = strings.TrimPrefix(row.SK, "OBJECT#")
		}

		if strings.TrimSpace(username) == "" || strings.TrimSpace(objectID) == "" || row.CreatedAt.IsZero() {
			continue
		}

		expectedPK := fmt.Sprintf("BOOKMARK_OBJECT#%s", objectID)
		expectedSK := fmt.Sprintf("USER#%s#TIME#%s", username, row.CreatedAt.Format(time.RFC3339Nano))

		if row.GSI8PK == expectedPK && row.GSI8SK == expectedSK {
			continue
		}

		logger.Debug("backfilling bookmark object index keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi8PK", expectedPK),
			zap.String("gsi8SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&bookmarkRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI8PK", expectedPK).
			Set("GSI8SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, err
		}
		updated++
	}

	logger.Info("bookmark object index backfill complete",
		zap.Int("items_updated", updated),
		zap.Int("rows_scanned", len(rows)))

	return updated, nil
}

func backfillFederationEdgeStrongestIndex(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []federationEdgeRecord
	err := db.WithContext(ctx).Model(&federationEdgeRecord{}).
		Filter("PK", "BEGINS_WITH", "FEDERATION_EDGE#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		source := row.SourceDomain
		if source == "" {
			source = strings.TrimPrefix(row.PK, "FEDERATION_EDGE#")
		}
		target := row.TargetDomain
		if target == "" {
			target = row.SK
		}

		connectionType := strings.TrimSpace(row.ConnectionType)
		if connectionType == "" {
			continue
		}

		strengthScaled := int64(math.Round(row.Strength * 1000000))
		if strengthScaled < 0 {
			strengthScaled = 0
		}
		if strengthScaled > 1000000 {
			strengthScaled = 1000000
		}

		expectedPK := fmt.Sprintf("FED_EDGES#TYPE#%s", connectionType)
		expectedSK := fmt.Sprintf(
			"STRENGTH#%07d#LAST#%013d#SRC#%s#TGT#%s",
			strengthScaled,
			row.LastActivity.Unix(),
			source,
			target,
		)

		if row.GSI8PK == expectedPK && row.GSI8SK == expectedSK {
			continue
		}

		logger.Debug("backfilling federation edge strongest index keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi8PK", expectedPK),
			zap.String("gsi8SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&federationEdgeRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI8PK", expectedPK).
			Set("GSI8SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, err
		}
		updated++
	}

	logger.Info("federation edge strongest index backfill complete",
		zap.Int("items_updated", updated),
		zap.Int("rows_scanned", len(rows)))

	return updated, nil
}

func newLogger() *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding:         "json",
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
	logger, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}
	return logger
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
