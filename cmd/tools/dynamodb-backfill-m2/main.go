package main

import (
	"context"
	"flag"
	"fmt"
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

type keyRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"`

	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"-"`

	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK" json:"-"`
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK" json:"-"`
}

func (keyRecord) TableName() string {
	return models.MainTableName
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	fs := flag.NewFlagSet("dynamodb-backfill-m2", flag.ContinueOnError)
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

	filterUpdated, err := backfillFilters(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("filter backfill failed", zap.Error(err))
		return 1
	}
	updated += filterUpdated

	deviceUpdated, err := backfillDevices(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("device backfill failed", zap.Error(err))
		return 1
	}
	updated += deviceUpdated

	activityUpdated, err := backfillActivities(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("activity backfill failed", zap.Error(err))
		return 1
	}
	updated += activityUpdated

	logger.Info("backfill complete",
		zap.Bool("dry_run", dryRun),
		zap.Int("items_updated", updated),
		zap.Duration("duration", time.Since(start)))

	return 0
}

func backfillFilters(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []keyRecord
	err := db.WithContext(ctx).Model(&keyRecord{}).
		Filter("PK", "BEGINS_WITH", "USER#").
		Filter("SK", "BEGINS_WITH", "FILTER#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		expectedPK := row.SK
		expectedSK := row.PK
		if row.GSI1PK == expectedPK && row.GSI1SK == expectedSK {
			continue
		}

		logger.Debug("backfilling filter index keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi1PK", expectedPK),
			zap.String("gsi1SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&keyRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI1PK", expectedPK).
			Set("GSI1SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, err
		}
		updated++
	}

	return updated, nil
}

func backfillDevices(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []keyRecord
	err := db.WithContext(ctx).Model(&keyRecord{}).
		Filter("PK", "BEGINS_WITH", "USER#").
		Filter("SK", "BEGINS_WITH", "DEVICE#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		deviceID := strings.TrimPrefix(row.SK, "DEVICE#")
		if strings.TrimSpace(deviceID) == "" {
			continue
		}

		expectedPK := "DEVICEID#" + deviceID
		expectedSK := row.PK
		if row.GSI3PK == expectedPK && row.GSI3SK == expectedSK {
			continue
		}

		logger.Debug("backfilling device index keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi3PK", expectedPK),
			zap.String("gsi3SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&keyRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI3PK", expectedPK).
			Set("GSI3SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, err
		}
		updated++
	}

	return updated, nil
}

func backfillActivities(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []keyRecord
	err := db.WithContext(ctx).Model(&keyRecord{}).
		Filter("PK", "BEGINS_WITH", "ACTOR#").
		Filter("SK", "BEGINS_WITH", "ACTIVITY#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		activityID, ok := parseActivityIDFromSK(row.SK)
		if !ok {
			continue
		}

		expectedPK := "ACTIVITYID#" + activityID
		expectedSK := row.SK
		if row.GSI2PK == expectedPK && row.GSI2SK == expectedSK {
			continue
		}

		logger.Debug("backfilling activity index keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi2PK", expectedPK),
			zap.String("gsi2SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&keyRecord{}).
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

	return updated, nil
}

func parseActivityIDFromSK(sk string) (string, bool) {
	if !strings.HasPrefix(sk, "ACTIVITY#") {
		return "", false
	}
	// rest is "<timestamp>#<activity_id...>"
	rest := strings.TrimPrefix(sk, "ACTIVITY#")
	_, id, ok := strings.Cut(rest, "#")
	if !ok {
		return "", false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	return id, true
}

func envOrDefault(key, defaultValue string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultValue
}

func newLogger() *zap.Logger {
	enc := zap.NewProductionEncoderConfig()
	enc.TimeKey = "ts"
	enc.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(zapcore.NewJSONEncoder(enc), zapcore.AddSync(os.Stderr), zap.NewAtomicLevelAt(zapcore.InfoLevel))
	return zap.New(core)
}
