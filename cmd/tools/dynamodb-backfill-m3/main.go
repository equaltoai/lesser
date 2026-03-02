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

type relayRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK" json:"-"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK" json:"-"`
}

func (relayRecord) TableName() string {
	return models.MainTableName
}

type circuitStateRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK" json:"-"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK" json:"-"`
}

func (circuitStateRecord) TableName() string {
	return models.MainTableName
}

type moderationEventRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI4PK string `theorydb:"index:gsi4,pk,attr:gsi4PK" json:"-"`
	GSI4SK string `theorydb:"index:gsi4,sk,attr:gsi4SK" json:"-"`
}

func (moderationEventRecord) TableName() string {
	return models.MainTableName
}

type statusRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	GSI8PK string `theorydb:"index:gsi8,pk,attr:gsi8PK" json:"-"`
	GSI8SK string `theorydb:"index:gsi8,sk,attr:gsi8SK" json:"-"`

	StatusID    string    `theorydb:"attr:statusID" json:"-"`
	PublishedAt time.Time `theorydb:"attr:publishedAt" json:"-"`
	AuthorID    string    `theorydb:"attr:authorID" json:"-"`
	InReplyToID string    `theorydb:"attr:inReplyToID" json:"-"`
	Deleted     bool      `theorydb:"attr:deleted" json:"-"`
}

func (statusRecord) TableName() string {
	return models.MainTableName
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	fs := flag.NewFlagSet("dynamodb-backfill-m3", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		tableName   string
		region      string
		endpoint    string
		localDomain string
		dryRun      bool
	)

	fs.StringVar(&tableName, "table", os.Getenv("LESSER_DYNAMODB_TABLE"), "DynamoDB table name (or env LESSER_DYNAMODB_TABLE)")
	fs.StringVar(&region, "region", envOrDefault("AWS_REGION", "us-east-1"), "AWS region")
	fs.StringVar(&endpoint, "endpoint", os.Getenv("DYNAMODB_ENDPOINT"), "DynamoDB endpoint override (optional; for local DynamoDB)")
	fs.StringVar(&localDomain, "local-domain", strings.TrimSpace(os.Getenv("LESSER_DOMAIN")), "local instance domain (optional; used for LOCAL_COMMENTS recount)")
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

	relayUpdated, err := backfillRelays(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("relay backfill failed", zap.Error(err))
		return 1
	}
	updated += relayUpdated

	circuitUpdated, err := backfillCircuitStates(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("circuit state backfill failed", zap.Error(err))
		return 1
	}
	updated += circuitUpdated

	moderationUpdated, err := backfillModerationEvents(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("moderation event backfill failed", zap.Error(err))
		return 1
	}
	updated += moderationUpdated

	statusUpdated, totalStatuses, localComments, err := backfillStatusesAndRecount(ctx, db, logger, dryRun, localDomain)
	if err != nil {
		logger.Error("status backfill failed", zap.Error(err))
		return 1
	}
	updated += statusUpdated

	metricUpdated, err := writeInstanceMetrics(ctx, db, logger, dryRun, totalStatuses, localComments)
	if err != nil {
		logger.Error("instance metrics update failed", zap.Error(err))
		return 1
	}
	updated += metricUpdated

	logger.Info("backfill complete",
		zap.Bool("dry_run", dryRun),
		zap.Int("items_updated", updated),
		zap.Int64("total_statuses", totalStatuses),
		zap.Int64("local_comments", localComments),
		zap.Duration("duration", time.Since(start)))

	return 0
}

func backfillRelays(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []relayRecord
	err := db.WithContext(ctx).Model(&relayRecord{}).
		Filter("PK", "BEGINS_WITH", "RELAY#").
		Filter("SK", "=", models.SKInfo).
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		url := strings.TrimPrefix(row.PK, "RELAY#")
		if strings.TrimSpace(url) == "" {
			continue
		}

		expectedPK := "RELAYS"
		expectedSK := fmt.Sprintf("URL#%s", url)
		if row.GSI8PK == expectedPK && row.GSI8SK == expectedSK {
			continue
		}

		logger.Debug("backfilling relay listing keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi8PK", expectedPK),
			zap.String("gsi8SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&relayRecord{}).
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

	return updated, nil
}

func backfillCircuitStates(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []circuitStateRecord
	err := db.WithContext(ctx).Model(&circuitStateRecord{}).
		Filter("PK", "BEGINS_WITH", "CIRCUIT#").
		Filter("SK", "=", models.SKState).
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		instanceID := strings.TrimPrefix(row.PK, "CIRCUIT#")
		if strings.TrimSpace(instanceID) == "" {
			continue
		}

		expectedPK := "CIRCUIT_STATES"
		expectedSK := fmt.Sprintf("INSTANCE#%s", instanceID)
		if row.GSI8PK == expectedPK && row.GSI8SK == expectedSK {
			continue
		}

		logger.Debug("backfilling circuit state listing keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi8PK", expectedPK),
			zap.String("gsi8SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&circuitStateRecord{}).
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

	return updated, nil
}

func backfillModerationEvents(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []moderationEventRecord
	err := db.WithContext(ctx).Model(&moderationEventRecord{}).
		Filter("PK", "BEGINS_WITH", "EVENT#").
		Filter("SK", "BEGINS_WITH", "TIME#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		expectedPK := "MODERATION_EVENTS"
		expectedSK := row.SK
		if row.GSI4PK == expectedPK && row.GSI4SK == expectedSK {
			continue
		}

		logger.Debug("backfilling moderation event listing keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi4PK", expectedPK),
			zap.String("gsi4SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&moderationEventRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI4PK", expectedPK).
			Set("GSI4SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, err
		}
		updated++
	}

	return updated, nil
}

func backfillStatusesAndRecount(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool, localDomain string) (updated int, totalStatuses int64, localComments int64, err error) {
	var rows []statusRecord
	err = db.WithContext(ctx).Model(&statusRecord{}).
		Filter("PK", "BEGINS_WITH", "status#").
		Filter("SK", "BEGINS_WITH", "status#").
		Scan(&rows)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, row := range rows {
		statusID := strings.TrimSpace(row.StatusID)
		if statusID == "" {
			statusID = strings.TrimPrefix(row.PK, "status#")
		}
		if statusID == "" {
			continue
		}

		if !row.Deleted {
			totalStatuses++
			if row.InReplyToID != "" && (localDomain == "" || strings.Contains(row.AuthorID, localDomain)) {
				localComments++
			}
		}

		if row.PublishedAt.IsZero() {
			logger.Warn("status missing publishedAt; skipping admin timeline backfill",
				zap.String("pk", row.PK),
				zap.String("sk", row.SK),
				zap.String("status_id", statusID))
			continue
		}

		expectedPK := "ADMIN_TIMELINE"
		expectedSK := fmt.Sprintf("%d#%s", row.PublishedAt.Unix(), statusID)
		if row.GSI8PK == expectedPK && row.GSI8SK == expectedSK {
			continue
		}

		logger.Debug("backfilling admin timeline keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi8PK", expectedPK),
			zap.String("gsi8SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&statusRecord{}).
			Where("PK", "=", row.PK).
			Where("SK", "=", row.SK).
			UpdateBuilder().
			Set("GSI8PK", expectedPK).
			Set("GSI8SK", expectedSK)

		if err := builder.Execute(); err != nil {
			return updated, totalStatuses, localComments, err
		}
		updated++
	}

	return updated, totalStatuses, localComments, nil
}

func writeInstanceMetrics(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool, totalStatuses, localComments int64) (int, error) {
	now := time.Now()
	updated := 0

	logger.Info("seeding instance metrics counters",
		zap.Int64("total_statuses", totalStatuses),
		zap.Int64("local_comments", localComments),
		zap.String("pk", "INSTANCE#METRICS"))

	if dryRun {
		return 2, nil
	}

	totalBuilder := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "TOTAL_STATUSES").
		UpdateBuilder().
		Set("TotalStatuses", totalStatuses).
		Set("Value", totalStatuses).
		Set("UpdatedAt", now)

	if err := totalBuilder.Execute(); err != nil {
		return updated, err
	}
	updated++

	commentsBuilder := db.WithContext(ctx).Model(&models.InstanceMetrics{}).
		Where("PK", "=", "INSTANCE#METRICS").
		Where("SK", "=", "LOCAL_COMMENTS").
		UpdateBuilder().
		Set("Value", localComments).
		Set("UpdatedAt", now)

	if err := commentsBuilder.Execute(); err != nil {
		return updated, err
	}
	updated++

	return updated, nil
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
