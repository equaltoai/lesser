package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	lessertheorydb "github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap"
)

type remoteNoteMaterializationEvidence struct {
	TableName             string
	ResolvedStage         string
	ResolvedProfile       string
	NoteURL               string
	CanonicalStatusID     string
	AuthorID              string
	ObjectRowFound        bool
	CanonicalStatusFound  bool
	URLIndexFound         bool
	AuthorTimelineChecked bool
	AuthorTimelineFound   bool
	Classification        string
}

type remoteNoteEvidence struct {
	Found       bool
	AuthorID    string
	PublishedAt int64
}

var (
	resolveCommonMigrationCLIOptionsFn       = resolveCommonMigrationCLIOptions
	registerDefaultTypeConvertersFn          = lessertheorydb.RegisterDefaultTypeConverters
	executeVerifyRemoteNoteMaterializationFn = executeVerifyRemoteNoteMaterialization
	printRemoteNoteMaterializationEvidenceFn = printRemoteNoteMaterializationEvidence
	fetchRemoteNoteObjectEvidenceFn          = fetchRemoteNoteObjectEvidence
	fetchRemoteNoteCanonicalStatusEvidenceFn = fetchRemoteNoteCanonicalStatusEvidence
	fetchRemoteNoteURLIndexEvidenceFn        = fetchRemoteNoteURLIndexEvidence
	fetchRemoteNoteAuthorTimelineEvidenceFn  = fetchRemoteNoteAuthorTimelineEvidence
	objectRepoGetObjectFn                    = func(ctx context.Context, objectRepo *repositories.ObjectRepository, noteURL string) (any, error) {
		return objectRepo.GetObject(ctx, noteURL)
	}
	statusRepoGetStatusFn = func(ctx context.Context, statusRepo *repositories.StatusRepository, canonicalStatusID string) (*models.Status, error) {
		return statusRepo.GetStatus(ctx, canonicalStatusID)
	}
	statusRepoGetStatusByURLFn = func(ctx context.Context, statusRepo *repositories.StatusRepository, noteURL string) (*models.Status, error) {
		return statusRepo.GetStatusByURL(ctx, noteURL)
	}
	loadRemoteNoteAuthorTimelineStatusFn = loadRemoteNoteAuthorTimelineStatus
)

func runVerifyRemoteNoteMaterialization(argv []string) error {
	fs := flag.NewFlagSet("lesser verify remote-note-materialization", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var app string
	var env string
	var awsProfile string
	var tableName string
	var noteURL string
	var authorID string

	fs.StringVar(&app, "app", envOrDefault("LESSER_APP", ""), "app slug (default: lesser)")
	fs.StringVar(&env, "env", valueDev, "deployment stage (dev|staging|live)")
	fs.StringVar(&awsProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (env: AWS_PROFILE)")
	fs.StringVar(&tableName, "table", "", "explicit DynamoDB table name override")
	fs.StringVar(&noteURL, "note-url", "", "remote note URL to trace")
	fs.StringVar(&authorID, "author-id", "", "expected remote author actor ID (optional)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	noteURL = strings.TrimSpace(noteURL)
	if noteURL == "" {
		return fmt.Errorf("--note-url is required")
	}
	if err := activitypub.ValidateURL(noteURL, "note_url"); err != nil {
		return err
	}

	ctx := context.Background()
	awsCfg, resolvedTableName, resolvedProfile, err := resolveCommonMigrationCLIOptionsFn(ctx, commonMigrationCLIOptions{
		App:        app,
		Env:        env,
		AWSProfile: awsProfile,
		TableName:  tableName,
	})
	if err != nil {
		return err
	}

	db, err := tabletheoryNewFn(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := registerDefaultTypeConvertersFn(db); err != nil {
		return err
	}

	prevTableName := models.MainTableName
	models.MainTableName = resolvedTableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	evidence, err := executeVerifyRemoteNoteMaterializationFn(ctx, db, resolvedTableName, noteURL, authorID)
	if err != nil {
		return err
	}
	evidence.TableName = resolvedTableName
	evidence.ResolvedStage = env
	evidence.ResolvedProfile = resolvedProfile

	printRemoteNoteMaterializationEvidenceFn(evidence)
	return nil
}

func executeVerifyRemoteNoteMaterialization(ctx context.Context, db core.DB, tableName, noteURL, suppliedAuthorID string) (remoteNoteMaterializationEvidence, error) {
	logger := zap.NewNop()
	objectRepo := repositories.NewObjectRepository(db, tableName, "", logger)
	statusRepo := repositories.NewStatusRepository(db, tableName, logger, nil)

	evidence := remoteNoteMaterializationEvidence{
		NoteURL:           strings.TrimSpace(noteURL),
		CanonicalStatusID: models.CanonicalStatusIDForDomain(noteURL, ""),
		AuthorID:          strings.TrimSpace(suppliedAuthorID),
	}

	objectEvidence, err := fetchRemoteNoteObjectEvidenceFn(ctx, objectRepo, noteURL)
	if err != nil {
		return remoteNoteMaterializationEvidence{}, err
	}
	evidence.ObjectRowFound = objectEvidence.Found

	statusEvidence, err := fetchRemoteNoteCanonicalStatusEvidenceFn(ctx, statusRepo, evidence.CanonicalStatusID)
	if err != nil {
		return remoteNoteMaterializationEvidence{}, err
	}
	evidence.CanonicalStatusFound = statusEvidence.Found

	urlEvidence, err := fetchRemoteNoteURLIndexEvidenceFn(ctx, statusRepo, noteURL, evidence.CanonicalStatusID)
	if err != nil {
		return remoteNoteMaterializationEvidence{}, err
	}
	evidence.URLIndexFound = urlEvidence.Found

	evidence.AuthorID = firstNonEmptyString(evidence.AuthorID, statusEvidence.AuthorID, urlEvidence.AuthorID, objectEvidence.AuthorID)
	publishedAtUnix := firstNonZeroInt64(statusEvidence.PublishedAt, urlEvidence.PublishedAt, objectEvidence.PublishedAt)
	if evidence.AuthorID != "" && publishedAtUnix > 0 {
		evidence.AuthorTimelineChecked = true
		found, err := fetchRemoteNoteAuthorTimelineEvidenceFn(ctx, db, evidence.AuthorID, publishedAtUnix, evidence.CanonicalStatusID)
		if err != nil {
			return remoteNoteMaterializationEvidence{}, err
		}
		evidence.AuthorTimelineFound = found
	}

	evidence.Classification = classifyRemoteNoteMaterializationEvidence(evidence)
	return evidence, nil
}

func fetchRemoteNoteObjectEvidence(ctx context.Context, objectRepo *repositories.ObjectRepository, noteURL string) (remoteNoteEvidence, error) {
	object, err := objectRepoGetObjectFn(ctx, objectRepo, noteURL)
	if err != nil {
		if isRemoteNoteVerificationNotFound(err) {
			return remoteNoteEvidence{}, nil
		}
		return remoteNoteEvidence{}, err
	}

	note, ok := object.(*activitypub.Note)
	if !ok || note == nil {
		return remoteNoteEvidence{Found: true}, nil
	}

	publishedAtUnix := int64(0)
	if note.Published != nil {
		publishedAtUnix = note.Published.Unix()
	}

	return remoteNoteEvidence{
		Found:       true,
		AuthorID:    strings.TrimSpace(note.AttributedTo),
		PublishedAt: publishedAtUnix,
	}, nil
}

func fetchRemoteNoteCanonicalStatusEvidence(ctx context.Context, statusRepo *repositories.StatusRepository, canonicalStatusID string) (remoteNoteEvidence, error) {
	status, err := statusRepoGetStatusFn(ctx, statusRepo, canonicalStatusID)
	if err != nil {
		if isRemoteNoteVerificationNotFound(err) {
			return remoteNoteEvidence{}, nil
		}
		return remoteNoteEvidence{}, err
	}
	if status == nil {
		return remoteNoteEvidence{}, nil
	}

	return remoteNoteEvidence{
		Found:       true,
		AuthorID:    strings.TrimSpace(status.AuthorID),
		PublishedAt: status.PublishedAt.Unix(),
	}, nil
}

func fetchRemoteNoteURLIndexEvidence(ctx context.Context, statusRepo *repositories.StatusRepository, noteURL, canonicalStatusID string) (remoteNoteEvidence, error) {
	status, err := statusRepoGetStatusByURLFn(ctx, statusRepo, noteURL)
	if err != nil {
		if isRemoteNoteVerificationNotFound(err) {
			return remoteNoteEvidence{}, nil
		}
		return remoteNoteEvidence{}, err
	}
	if status == nil || strings.TrimSpace(status.StatusID) != strings.TrimSpace(canonicalStatusID) {
		return remoteNoteEvidence{}, nil
	}

	return remoteNoteEvidence{
		Found:       true,
		AuthorID:    strings.TrimSpace(status.AuthorID),
		PublishedAt: status.PublishedAt.Unix(),
	}, nil
}

func fetchRemoteNoteAuthorTimelineEvidence(ctx context.Context, db core.DB, authorID string, publishedAtUnix int64, canonicalStatusID string) (bool, error) {
	status, err := loadRemoteNoteAuthorTimelineStatusFn(ctx, db, authorID, publishedAtUnix, canonicalStatusID)
	if err != nil {
		if dynamormerrors.IsNotFound(err) || common.IsNotFound(err) {
			return false, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return false, nil
		}
		return false, err
	}

	return status != nil && strings.TrimSpace(status.StatusID) == strings.TrimSpace(canonicalStatusID), nil
}

func loadRemoteNoteAuthorTimelineStatus(ctx context.Context, db core.DB, authorID string, publishedAtUnix int64, canonicalStatusID string) (*models.Status, error) {
	var status models.Status
	err := db.WithContext(ctx).Model(&models.Status{}).
		Index("gsi1").
		Where("gsi1PK", "=", "AUTHOR#"+strings.TrimSpace(authorID)).
		Where("gsi1SK", "=", fmt.Sprintf("%d#%s", publishedAtUnix, strings.TrimSpace(canonicalStatusID))).
		First(&status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

func classifyRemoteNoteMaterializationEvidence(evidence remoteNoteMaterializationEvidence) string {
	switch {
	case evidence.ObjectRowFound && evidence.CanonicalStatusFound && evidence.URLIndexFound && (!evidence.AuthorTimelineChecked || evidence.AuthorTimelineFound):
		return "materialized"
	case !evidence.ObjectRowFound && !evidence.CanonicalStatusFound:
		return "missing_inbox_object"
	case evidence.ObjectRowFound && !evidence.CanonicalStatusFound:
		return "missing_canonical_status"
	case !evidence.ObjectRowFound && evidence.CanonicalStatusFound:
		return "status_materialized_without_object_row"
	case evidence.CanonicalStatusFound && !evidence.URLIndexFound && evidence.AuthorTimelineChecked && !evidence.AuthorTimelineFound:
		return "missing_url_and_author_timeline_index"
	case evidence.CanonicalStatusFound && !evidence.URLIndexFound:
		return "missing_url_index"
	case evidence.CanonicalStatusFound && evidence.AuthorTimelineChecked && !evidence.AuthorTimelineFound:
		return "missing_author_timeline_index"
	case evidence.CanonicalStatusFound && !evidence.AuthorTimelineChecked:
		return "status_found_author_timeline_unchecked"
	default:
		return "partial_evidence"
	}
}

func printRemoteNoteMaterializationEvidence(evidence remoteNoteMaterializationEvidence) {
	fmt.Println("verify remote-note-materialization complete")
	fmt.Printf("table_name: %s\n", evidence.TableName)
	fmt.Printf("resolved_stage: %s\n", evidence.ResolvedStage)
	fmt.Printf("resolved_profile: %s\n", evidence.ResolvedProfile)
	fmt.Printf("note_url: %s\n", evidence.NoteURL)
	fmt.Printf("canonical_status_id: %s\n", evidence.CanonicalStatusID)
	fmt.Printf("author_id: %s\n", evidence.AuthorID)
	fmt.Printf("object_row: %t\n", evidence.ObjectRowFound)
	fmt.Printf("canonical_status_row: %t\n", evidence.CanonicalStatusFound)
	fmt.Printf("url_index: %t\n", evidence.URLIndexFound)
	fmt.Printf("author_timeline_checked: %t\n", evidence.AuthorTimelineChecked)
	fmt.Printf("author_timeline: %t\n", evidence.AuthorTimelineFound)
	fmt.Printf("classification: %s\n", evidence.Classification)
}

func isRemoteNoteVerificationNotFound(err error) bool {
	if err == nil {
		return false
	}
	if common.IsNotFound(err) || dynamormerrors.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
