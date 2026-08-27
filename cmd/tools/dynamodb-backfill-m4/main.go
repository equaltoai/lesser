package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

	trustUpdated, err := backfillTrustRelationshipGSI3(ctx, db, logger, dryRun)
	if err != nil {
		logger.Error("trust relationship gsi3 backfill failed", zap.Error(err))
		return 1
	}
	updated += trustUpdated

	logger.Info("backfill complete",
		zap.Bool("dry_run", dryRun),
		zap.Int("items_updated", updated),
		zap.Duration("duration", time.Since(start)))

	return 0
}

// backfillTrustRelationshipGSI3 sets gsi3PK/gsi3SK on legacy TrustRelationship
// rows (written before the global-listing GSI3 key existed) so the admin
// GetAllTrustRelationships query can serve from a keyed query instead of a
// full-table scan. Offline one-time migration only — never on a request path.
func backfillTrustRelationshipGSI3(ctx context.Context, db core.DB, logger *zap.Logger, dryRun bool) (int, error) {
	var rows []models.TrustRelationship
	err := db.WithContext(ctx).Model(&models.TrustRelationship{}).
		Filter("PK", "BEGINS_WITH", "TRUST#").
		Filter("SK", "BEGINS_WITH", "TRUSTEE#").
		Scan(&rows)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		trusterID, category, trusteeID, ok := parseTrustRelationshipKeys(row.PK, row.SK)
		if !ok {
			continue
		}

		expectedPK := "TRUST_RELATIONSHIPS"
		expectedSK := fmt.Sprintf("TRUST#%s#%s#TRUSTEE#%s", trusterID, category, trusteeID)

		if row.GSI3PK == expectedPK && row.GSI3SK == expectedSK {
			continue
		}

		logger.Debug("backfilling trust relationship gsi3 keys",
			zap.String("pk", row.PK),
			zap.String("sk", row.SK),
			zap.String("gsi3PK", expectedPK),
			zap.String("gsi3SK", expectedSK))

		if dryRun {
			updated++
			continue
		}

		builder := db.WithContext(ctx).Model(&models.TrustRelationship{}).
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

	logger.Info("trust relationship gsi3 backfill complete",
		zap.Int("items_updated", updated),
		zap.Int("rows_scanned", len(rows)))

	return updated, nil
}

// parseTrustRelationshipKeys derives the truster, category, and trustee from a
// relationship's PK/SK pair. PK is TRUST#{trusterID}#{category} and SK is
// TRUSTEE#{trusteeID}; the truster ID is not allowed to contain '#', so a
// three-part split is the canonical parse (mirrors models.TrustRelationship
// UpdateKeys construction).
func parseTrustRelationshipKeys(pk, sk string) (trusterID, category, trusteeID string, ok bool) {
	parts := strings.Split(pk, "#")
	if len(parts) != 3 || parts[0] != "TRUST" {
		return "", "", "", false
	}
	if strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return "", "", "", false
	}
	trusteeID = strings.TrimPrefix(sk, "TRUSTEE#")
	if trusteeID == sk || strings.TrimSpace(trusteeID) == "" {
		return "", "", "", false
	}
	return parts[1], parts[2], trusteeID, true
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
