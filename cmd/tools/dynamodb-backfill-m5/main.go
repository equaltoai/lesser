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

type aiCostRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"`

	OperationID   string    `theorydb:"attr:operationID" json:"-"`
	OperationType string    `theorydb:"attr:operationType" json:"-"`
	BillingPeriod string    `theorydb:"attr:billingPeriod" json:"-"`
	Timestamp     time.Time `theorydb:"attr:timestamp" json:"-"`
}

func (aiCostRecord) TableName() string {
	return models.MainTableName
}

type federationCostRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"`

	Domain       string    `theorydb:"attr:domain" json:"-"`
	ActivityType string    `theorydb:"attr:activityType" json:"-"`
	ActivityID   string    `theorydb:"attr:activityID" json:"-"`
	Timestamp    time.Time `theorydb:"attr:timestamp" json:"-"`
}

func (federationCostRecord) TableName() string {
	return models.MainTableName
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	fs := flag.NewFlagSet("dynamodb-backfill-m5", flag.ContinueOnError)
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

	aiUpdated, err := backfillAICostTimeIndex(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("ai cost backfill failed", zap.Error(err))
		return 1
	}
	updated += aiUpdated

	fedUpdated, err := backfillFederationCostTimeIndex(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("federation cost backfill failed", zap.Error(err))
		return 1
	}
	updated += fedUpdated

	logger.Info("backfill complete",
		zap.Bool("dry_run", dryRun),
		zap.Int("items_updated", updated),
		zap.Duration("duration", time.Since(start)))

	return 0
}

func backfillAICostTimeIndex(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []aiCostRecord
	err := db.WithContext(ctx).Model(&aiCostRecord{}).
		Filter("PK", "BEGINS_WITH", "AI_COST#").
		Filter("SK", "=", models.SKMetadata).
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		if row.Timestamp.IsZero() {
			continue
		}

		ts := row.Timestamp.UTC()
		operationID := strings.TrimSpace(row.OperationID)
		if operationID == "" {
			operationID = strings.TrimPrefix(row.PK, "AI_COST#")
		}

		if strings.TrimSpace(operationID) == "" {
			continue
		}

		billingPeriod := strings.TrimSpace(row.BillingPeriod)
		if billingPeriod == "" {
			billingPeriod = ts.Format("2006-01")
		}

		expectedPK := fmt.Sprintf("AI_COSTS#%s", billingPeriod)
		expectedSK := fmt.Sprintf(
			"TS#%013d#TYPE#%s#OP#%s",
			ts.UnixMilli(),
			strings.ToLower(strings.TrimSpace(row.OperationType)),
			operationID,
		)

		if row.GSI1PK == expectedPK && row.GSI1SK == expectedSK {
			continue
		}

		logger.Debug("backfilling ai cost gsi1 keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi1PK", expectedPK),
			zap.String("gsi1SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&aiCostRecord{}).
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

	logger.Info("ai cost gsi1 backfill complete",
		zap.Int("items_updated", updated),
		zap.Int("rows_scanned", len(rows)))

	return updated, nil
}

func backfillFederationCostTimeIndex(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []federationCostRecord
	err := db.WithContext(ctx).Model(&federationCostRecord{}).
		Filter("PK", "BEGINS_WITH", "FED_COST#").
		Filter("SK", "BEGINS_WITH", "ACTIVITY#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		if row.Timestamp.IsZero() {
			continue
		}

		ts := row.Timestamp.UTC()
		domain := strings.ToLower(strings.TrimSpace(row.Domain))
		activityType := strings.ToLower(strings.TrimSpace(row.ActivityType))
		activityID := strings.TrimSpace(row.ActivityID)

		if domain == "" || activityID == "" {
			continue
		}

		expectedPK := fmt.Sprintf("FED_COSTS#DOMAIN#%s#%s", domain, ts.Format("2006-01"))
		expectedSK := fmt.Sprintf("TS#%013d#TYPE#%s#ID#%s", ts.UnixMilli(), activityType, activityID)

		if row.GSI1PK == expectedPK && row.GSI1SK == expectedSK {
			continue
		}

		logger.Debug("backfilling federation cost gsi1 keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi1PK", expectedPK),
			zap.String("gsi1SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&federationCostRecord{}).
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

	logger.Info("federation cost gsi1 backfill complete",
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
