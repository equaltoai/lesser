package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	tablecore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

type costTrackingRepoRecorder struct {
	createCalls int
	createErr   error
}

func (r *costTrackingRepoRecorder) Create(_ context.Context, _ *models.DynamoDBCostRecord) error {
	r.createCalls++
	return r.createErr
}

type importRepoRecorder struct {
	statusCalls       []string
	statusErrByStatus map[string]error

	progressCalls []int
	progressErr   error

	budgetCalls int
	budgetErr   error
}

func (r *importRepoRecorder) UpdateImportStatus(_ context.Context, _ string, status string, _ map[string]any, _ string) error {
	r.statusCalls = append(r.statusCalls, status)
	if err, ok := r.statusErrByStatus[status]; ok {
		return err
	}
	return nil
}

func (r *importRepoRecorder) UpdateBudgetUsage(_ context.Context, _ string, _ string, _ int64, _ int64) error {
	r.budgetCalls++
	return r.budgetErr
}

func (r *importRepoRecorder) UpdateImportProgress(_ context.Context, _ string, progress int) error {
	r.progressCalls = append(r.progressCalls, progress)
	return r.progressErr
}

func setAWSEnvForS3Test(t *testing.T, endpoint string) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ENDPOINT_URL_S3", endpoint)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	t.Setenv("AWS_S3_USE_PATH_STYLE", "true")
}

func TestImportProcessor_DetectFormat_Round12(t *testing.T) {
	require.Equal(t, "json", detectFormat([]byte(`{"ok":true}`)))
	require.Equal(t, "activitypub", detectFormat([]byte(`{"@context":"https://www.w3.org/ns/activitystreams","type":"Collection"}`)))
	require.Equal(t, "csv", detectFormat([]byte("Account address\nalice@example.com\n")))
	require.Equal(t, "unknown", detectFormat([]byte("not json and not csv")))
}

func TestImportProcessor_ProcessCSVImport_Round12(t *testing.T) {
	p := &ImportProcessor{
		baseURL: "https://example.com",
		logger:  zap.NewNop(),
		importRepo: &importRepoRecorder{
			progressErr: errors.New("progress update failed"),
		},
		repos: importStorageStub{
			object: objectCreatorFunc(func(_ context.Context, _ any) error { return nil }),
			actor: actorGetterFunc(func(_ context.Context, username string) (*activitypub.Actor, error) {
				return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/" + username}}, nil
			}),
			activity: activityCreatorFunc(func(_ context.Context, _ *activitypub.Activity) error { return nil }),
			bookmark: bookmarkCreatorFunc(func(_ context.Context, _ string, _ string) (*models.Bookmark, error) { return &models.Bookmark{}, nil }),
		},
	}

	t.Run("header read failure", func(t *testing.T) {
		_, err := p.processCSVImport(context.Background(), ImportProcessorEvent{Type: "followers"}, []byte{}, nil)
		require.Error(t, err)
	})

	t.Run("followers", func(t *testing.T) {
		result, err := p.processCSVImport(context.Background(), ImportProcessorEvent{Type: "followers"}, []byte("Account address\nalice@example.com\n"), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Skipped)
	})

	t.Run("following", func(t *testing.T) {
		result, err := p.processCSVImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "following"}, []byte("Account address\nbob@remote.example\n"), &models.ImportCostTracking{})
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
	})

	t.Run("blocks", func(t *testing.T) {
		result, err := p.processCSVImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "blocks"}, []byte("Account address\nbob@remote.example\n"), &models.ImportCostTracking{})
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
	})

	t.Run("mutes", func(t *testing.T) {
		result, err := p.processCSVImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "mutes"}, []byte("Account address,Hide notifications\nbob@remote.example,true\n"), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
	})

	t.Run("bookmarks", func(t *testing.T) {
		result, err := p.processCSVImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "bookmarks"}, []byte("URI\nhttps://example.com/statuses/1\n"), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := p.processCSVImport(context.Background(), ImportProcessorEvent{Type: "unknown"}, []byte("Account address\nx\n"), nil)
		require.Error(t, err)
	})
}

func TestImportProcessor_ProcessJSONImport_Round12(t *testing.T) {
	listCreateErr := errors.New("list create failed")
	listMemberCreateErr := errors.New("list member create failed")

	makeProcessor := func(createErrForList, createErrForMember error) *ImportProcessor {
		return &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			importRepo: &importRepoRecorder{
				progressErr: errors.New("progress update failed"),
			},
			repos: importStorageStub{
				object: objectCreatorFunc(func(_ context.Context, obj any) error {
					switch obj.(type) {
					case *models.List:
						return createErrForList
					case *models.ListMember:
						return createErrForMember
					}
					return nil
				}),
			},
		}
	}

	t.Run("lists happy path", func(t *testing.T) {
		p := makeProcessor(nil, nil)
		result, err := p.processJSONImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "lists"}, []byte(`{"Friends":["bob@remote.example"]}`), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Success)
	})

	t.Run("lists create fails", func(t *testing.T) {
		p := makeProcessor(listCreateErr, nil)
		result, err := p.processJSONImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "lists"}, []byte(`{"Friends":["bob@remote.example"]}`), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Failed)
		require.NotEmpty(t, result.Errors)
	})

	t.Run("list member create fails", func(t *testing.T) {
		p := makeProcessor(nil, listMemberCreateErr)
		result, err := p.processJSONImport(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "lists"}, []byte(`{"Friends":["bob@remote.example"]}`), nil)
		require.NoError(t, err)
		require.Equal(t, 1, result.Failed)
		require.NotEmpty(t, result.Errors)
	})

	t.Run("parse failure returns wrapped error", func(t *testing.T) {
		p := makeProcessor(nil, nil)
		_, err := p.processJSONImport(context.Background(), ImportProcessorEvent{Type: "lists"}, []byte("{bad json"), nil)
		require.Error(t, err)
	})

	t.Run("unsupported type returns validation error", func(t *testing.T) {
		p := makeProcessor(nil, nil)
		_, err := p.processJSONImport(context.Background(), ImportProcessorEvent{Type: "following"}, []byte(`{}`), nil)
		require.Error(t, err)
	})
}

func TestImportProcessor_ActivityPubImport_Round12(t *testing.T) {
	p := &ImportProcessor{
		baseURL: "https://example.com",
		logger:  zap.NewNop(),
		importRepo: &importRepoRecorder{
			progressErr: errors.New("progress update failed"),
		},
		repos: importStorageStub{
			object: objectCreatorFunc(func(_ context.Context, _ any) error { return nil }),
			actor: actorGetterFunc(func(_ context.Context, username string) (*activitypub.Actor, error) {
				return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/" + username}}, nil
			}),
			activity: activityCreatorFunc(func(_ context.Context, _ *activitypub.Activity) error { return nil }),
		},
	}

	t.Run("only archive is supported", func(t *testing.T) {
		_, err := p.processActivityPubImport(context.Background(), ImportProcessorEvent{Type: "followers"}, []byte(`{}`), nil)
		require.Error(t, err)
	})

	t.Run("parse failure", func(t *testing.T) {
		_, err := p.processActivityPubImport(context.Background(), ImportProcessorEvent{Type: "archive"}, []byte("{bad json"), nil)
		require.Error(t, err)
	})

	t.Run("no items found", func(t *testing.T) {
		_, err := p.processActivityPubImport(context.Background(), ImportProcessorEvent{Type: "archive"}, []byte(`{"@context":"https://www.w3.org/ns/activitystreams"}`), nil)
		require.Error(t, err)
	})

	t.Run("items with mixed success and failure", func(t *testing.T) {
		items := []any{
			"not-a-map",
			map[string]any{},
			map[string]any{"type": "Create"},
			map[string]any{"type": "Create", "object": "not-a-map"},
			map[string]any{"type": "Create", "object": map[string]any{"type": "Note", "id": "https://example.com/objects/1", "content": "hi"}},
			map[string]any{"type": "Follow", "object": "bob@remote.example"},
			map[string]any{"type": "Like", "object": "https://example.com/objects/2"},
			map[string]any{"type": "Announce", "object": ""},
			map[string]any{"type": "Note"},
			map[string]any{"type": "Unknown"},
		}

		result, err := p.processActivityPubItems(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "archive"}, items, &models.ImportCostTracking{})
		require.NoError(t, err)
		require.Greater(t, result.Success, 0)
		require.Greater(t, result.Failed, 0)
	})
}

func TestImportProcessor_ProcessImportJob_AndHandleSQS_Round12(t *testing.T) {
	t.Run("processImportJob covers csv/json/activitypub formats", func(t *testing.T) {
		makeProcessor := func(repo *importRepoRecorder, costRepo *costTrackingRepoRecorder) *ImportProcessor {
			return &ImportProcessor{
				importRepo:       repo,
				costTrackingRepo: costRepo,
				cfg:              &config.Config{DynamoTableName: "table"},
				logger:           zap.NewNop(),
				bucketName:       "bucket",
				baseURL:          "https://example.com",
				repos: importStorageStub{
					object: objectCreatorFunc(func(_ context.Context, _ any) error { return nil }),
				},
			}
		}

		t.Run("csv followers success", func(t *testing.T) {
			setAWSEnvForS3Test(t, "https://example.com")

			repo := &importRepoRecorder{statusErrByStatus: map[string]error{"processing": errors.New("status update failed")}, budgetErr: errors.New("budget update failed")}
			costRepo := &costTrackingRepoRecorder{createErr: errors.New("tracking create failed")}
			p := makeProcessor(repo, costRepo)

			require.NoError(t, p.initializeAWSClients(context.Background()))
			p.s3Client = &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("Account address\nalice@example.com\n")))}, nil
			}}
			require.NoError(t, p.processImportJob(context.Background(), ImportProcessorEvent{ImportID: "imp-1", Username: "alice", Type: "followers", Mode: "merge", S3Key: "followers.csv"}))
			require.Contains(t, repo.statusCalls, "processing")
			require.Contains(t, repo.statusCalls, "completed")
			require.Equal(t, 1, costRepo.createCalls)
		})

		t.Run("json lists success", func(t *testing.T) {
			setAWSEnvForS3Test(t, "https://example.com")

			repo := &importRepoRecorder{}
			costRepo := &costTrackingRepoRecorder{}
			p := makeProcessor(repo, costRepo)

			require.NoError(t, p.initializeAWSClients(context.Background()))
			p.s3Client = &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(`{"Friends":["bob@remote.example"]}`)))}, nil
			}}
			require.NoError(t, p.processImportJob(context.Background(), ImportProcessorEvent{ImportID: "imp-2", Username: "alice", Type: "lists", Mode: "merge", S3Key: "lists.json"}))
		})

		t.Run("activitypub archive success", func(t *testing.T) {
			setAWSEnvForS3Test(t, "https://example.com")

			repo := &importRepoRecorder{}
			costRepo := &costTrackingRepoRecorder{}
			p := makeProcessor(repo, costRepo)

			require.NoError(t, p.initializeAWSClients(context.Background()))
			p.s3Client = &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte(`{"@context":"https://www.w3.org/ns/activitystreams","orderedItems":[{"type":"Unknown"}]}`)))}, nil
			}}
			require.NoError(t, p.processImportJob(context.Background(), ImportProcessorEvent{ImportID: "imp-3", Username: "alice", Type: "archive", Mode: "merge", S3Key: "archive.json"}))
		})

		t.Run("download failure returns wrapped error", func(t *testing.T) {
			setAWSEnvForS3Test(t, "https://example.com")

			repo := &importRepoRecorder{}
			costRepo := &costTrackingRepoRecorder{}
			p := makeProcessor(repo, costRepo)

			require.NoError(t, p.initializeAWSClients(context.Background()))
			p.s3Client = &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return nil, errors.New("not found")
			}}
			require.Error(t, p.processImportJob(context.Background(), ImportProcessorEvent{ImportID: "imp-4", Username: "alice", Type: "followers", Mode: "merge", S3Key: "missing.csv"}))
		})

		t.Run("unsupported format returns error", func(t *testing.T) {
			setAWSEnvForS3Test(t, "https://example.com")

			repo := &importRepoRecorder{}
			costRepo := &costTrackingRepoRecorder{}
			p := makeProcessor(repo, costRepo)

			require.NoError(t, p.initializeAWSClients(context.Background()))
			p.s3Client = &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("hello world")))}, nil
			}}
			require.Error(t, p.processImportJob(context.Background(), ImportProcessorEvent{ImportID: "imp-5", Username: "alice", Type: "followers", Mode: "merge", S3Key: "unknown.bin"}))
		})

		t.Run("import status update failure returns wrapped error", func(t *testing.T) {
			setAWSEnvForS3Test(t, "https://example.com")

			repo := &importRepoRecorder{statusErrByStatus: map[string]error{"completed": errors.New("status update failed")}}
			costRepo := &costTrackingRepoRecorder{}
			p := makeProcessor(repo, costRepo)

			require.NoError(t, p.initializeAWSClients(context.Background()))
			p.s3Client = &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("Account address\nalice@example.com\n")))}, nil
			}}
			require.Error(t, p.processImportJob(context.Background(), ImportProcessorEvent{ImportID: "imp-6", Username: "alice", Type: "followers", Mode: "merge", S3Key: "followers.csv"}))
		})
	})

	t.Run("HandleSQSMessage parses formats and updates failures", func(t *testing.T) {
		setAWSEnvForS3Test(t, "https://example.com")
		origNew := newS3Client
		t.Cleanup(func() { newS3Client = origNew })
		newS3Client = func(_ aws.Config) s3API {
			return &s3ClientStub{getObjectFn: func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
				if strings.Contains(aws.ToString(input.Key), "bad.csv") {
					return nil, errors.New("not found")
				}
				return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("Account address\nalice@example.com\n")))}, nil
			}}
		}

		repo := &importRepoRecorder{statusErrByStatus: map[string]error{"failed": errors.New("failed status update failed")}}
		costRepo := &costTrackingRepoRecorder{}
		p := &ImportProcessor{
			importRepo:       repo,
			costTrackingRepo: costRepo,
			cfg:              &config.Config{DynamoTableName: "table"},
			logger:           zap.NewNop(),
			bucketName:       "bucket",
			baseURL:          "https://example.com",
		}

		event := events.SQSEvent{
			Records: []events.SQSMessage{
				{MessageId: "1", Body: `{"import_id":"imp-1","username":"alice","type":"followers","mode":"merge","s3_key":"good.csv","timestamp":123}`},
				{MessageId: "2", Body: `{"import_id":"imp-2","username":"alice","type":"followers","mode":"merge","s3_key":"good.csv","timestamp":"bad"}`},
				{MessageId: "3", Body: `{bad json`},
				{MessageId: "4", Body: `{"import_id":"imp-4","username":"alice","type":"followers","mode":"merge","s3_key":"bad.csv","timestamp":123}`},
			},
		}

		for _, msg := range event.Records {
			require.NoError(t, p.HandleSQSMessage(nil, msg))
		}
		require.NotEmpty(t, repo.statusCalls)
	})
}

func TestImportProcessor_WithInvocationTableContext_Round12(t *testing.T) {
	baseDB := &lambdaTimeoutRecorderDB{}
	p := &ImportProcessor{
		db:               baseDB,
		importRepo:       &importRepoRecorder{},
		costTrackingRepo: &costTrackingRepoRecorder{},
		cfg:              &config.Config{DynamoTableName: "table"},
		logger:           zap.NewNop(),
	}

	require.Same(t, p, p.withInvocationTableContext(context.Background()))

	deadline := time.Now().Add(time.Minute).Round(0)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	runner := p.withInvocationTableContext(ctx)
	require.NotSame(t, p, runner)
	require.Equal(t, 1, baseDB.lambdaTimeoutCalls)
	require.NotNil(t, runner.importRepo)
	require.NotNil(t, runner.costTrackingRepo)
	require.NotNil(t, runner.repos)

	timedDB, ok := runner.db.(*lambdaTimeoutRecorderDB)
	require.True(t, ok)
	require.Equal(t, deadline, timedDB.lambdaDeadline)
}

func TestImportProcessor_HandleSQSMessageAppliesEventContextDeadline_Round12(t *testing.T) {
	setAWSEnvForS3Test(t, "https://example.com")

	baseDB := &lambdaTimeoutRecorderDB{}
	repo := &importRepoRecorder{}
	costRepo := &costTrackingRepoRecorder{}
	p := &ImportProcessor{
		db:               baseDB,
		importRepo:       repo,
		costTrackingRepo: costRepo,
		s3Client: &s3ClientStub{getObjectFn: func(_ *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader([]byte("Account address\nalice@example.com\n")))}, nil
		}},
		cfg:        &config.Config{},
		logger:     zap.NewNop(),
		bucketName: "bucket",
		baseURL:    "https://example.com",
		repos: importStorageStub{
			object: objectCreatorFunc(func(_ context.Context, _ any) error { return nil }),
		},
	}

	deadline := time.Now().Add(time.Minute).Round(0)
	runCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	msg := events.SQSMessage{
		MessageId:      "1",
		EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:import-queue",
		Body:           `{"import_id":"imp-1","username":"alice","type":"followers","mode":"merge","s3_key":"followers.csv","timestamp":123}`,
	}

	app := apptheory.New()
	app.SQS("import-queue", p.HandleSQSMessage)
	resp := app.ServeSQS(runCtx, events.SQSEvent{Records: []events.SQSMessage{msg}})

	require.Empty(t, resp.BatchItemFailures)
	require.Equal(t, 1, baseDB.lambdaTimeoutCalls)
	require.Contains(t, repo.statusCalls, "processing")
	require.Contains(t, repo.statusCalls, "completed")
}

type s3ClientStub struct {
	getObjectFn func(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

func (s *s3ClientStub) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if s.getObjectFn != nil {
		return s.getObjectFn(input)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(nil))}, nil
}

type lambdaTimeoutRecorderDB struct {
	lambdaTimeoutCalls int
	lambdaDeadline     time.Time
}

func (db *lambdaTimeoutRecorderDB) Model(any) tablecore.Query { return nil }
func (db *lambdaTimeoutRecorderDB) Migrate() error            { return nil }
func (db *lambdaTimeoutRecorderDB) AutoMigrate(...any) error  { return nil }
func (db *lambdaTimeoutRecorderDB) Close() error              { return nil }
func (db *lambdaTimeoutRecorderDB) WithContext(context.Context) tablecore.DB {
	return db
}
func (db *lambdaTimeoutRecorderDB) WithLambdaTimeout(ctx context.Context) tablecore.DB {
	db.lambdaTimeoutCalls++
	deadline, _ := ctx.Deadline()
	return &lambdaTimeoutRecorderDB{lambdaDeadline: deadline}
}

func TestImportTransaction_RollbackFailure_Round12(t *testing.T) {
	logger := zap.NewNop()

	tx := NewImportTransaction("import-1", logger)
	tx.AddOperation(func() error { return nil }, func() error { return errors.New("rollback failed") })
	require.Error(t, tx.rollback(1))

	tx2 := NewImportTransaction("import-2", logger)
	tx2.AddOperation(func() error { return nil }, func() error { return errors.New("rollback failed") })
	tx2.AddOperation(func() error { return errors.New("operation failed") }, nil)
	require.Error(t, tx2.Execute(context.Background()))
}

func TestImportProcessor_CSVHelpers_ErrorBranches_Round12(t *testing.T) {
	ctx := context.Background()

	t.Run("processFollowingCSV parse errors and validation skips", func(t *testing.T) {
		p := &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			importRepo: &importRepoRecorder{
				progressErr: errors.New("progress update failed"),
			},
			repos: importStorageStub{
				object: objectCreatorFunc(func(_ context.Context, _ any) error { return errors.New("create failed") }),
				actor: actorGetterFunc(func(_ context.Context, username string) (*activitypub.Actor, error) {
					return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/" + username}}, nil
				}),
				activity: activityCreatorFunc(func(_ context.Context, _ *activitypub.Activity) error { return nil }),
			},
		}

		result, err := p.processFollowingCSV(ctx, ImportProcessorEvent{ImportID: "imp-1", Username: "alice"}, csv.NewReader(strings.NewReader("\"bad\n")), &models.ImportCostTracking{})
		require.NoError(t, err)
		require.Equal(t, 1, result.Failed)
		require.NotEmpty(t, result.Errors)

		result, err = p.processFollowingCSV(ctx, ImportProcessorEvent{ImportID: "imp-1", Username: "alice"}, csv.NewReader(strings.NewReader("\n")), &models.ImportCostTracking{})
		require.NoError(t, err)
		require.Equal(t, 0, result.Success+result.Skipped+result.Failed)

		result, err = p.processFollowingCSV(ctx, ImportProcessorEvent{ImportID: "imp-1", Username: "alice"}, csv.NewReader(strings.NewReader("bob@remote.example\n")), &models.ImportCostTracking{})
		require.NoError(t, err)
		require.Equal(t, 1, result.Failed)
	})

	t.Run("processBookmarksCSV parse and bookmark failures", func(t *testing.T) {
		p := &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			importRepo: &importRepoRecorder{
				progressErr: errors.New("progress update failed"),
			},
		}

		result, err := p.processBookmarksCSV(ctx, ImportProcessorEvent{ImportID: "imp-1", Username: "alice"}, csv.NewReader(strings.NewReader("\"bad\n")))
		require.NoError(t, err)
		require.Equal(t, 1, result.Failed)

		p.repos = importStorageStub{
			bookmark: bookmarkCreatorFunc(func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
				return nil, errors.New("bookmark create failed")
			}),
		}
		result, err = p.processBookmarksCSV(ctx, ImportProcessorEvent{ImportID: "imp-1", Username: "alice"}, csv.NewReader(strings.NewReader("https://example.com/statuses/1\n")))
		require.NoError(t, err)
		require.Equal(t, 1, result.Failed)
		require.NotEmpty(t, result.Errors)
	})
}

func TestImportProcessor_AdditionalErrorBranches_Round12(t *testing.T) {
	t.Run("initializeAWSClients propagates config load error", func(t *testing.T) {
		origLoad := loadAWSConfig
		origNew := newS3Client
		t.Cleanup(func() {
			loadAWSConfig = origLoad
			newS3Client = origNew
		})

		loadAWSConfig = func(_ context.Context) (aws.Config, error) {
			return aws.Config{}, errors.New("config load failed")
		}
		newS3Client = func(_ aws.Config) s3API {
			t.Fatal("newS3Client should not be called when loadAWSConfig fails")
			return nil
		}

		p := &ImportProcessor{}
		require.Error(t, p.initializeAWSClients(context.Background()))
	})

	t.Run("bookmarkStatus handles missing repo and create errors", func(t *testing.T) {
		p := &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			repos: importStorageStub{
				bookmark: nil,
			},
		}
		require.Error(t, p.bookmarkStatus(context.Background(), "alice", "https://example.com/statuses/1"))

		p.repos = importStorageStub{
			bookmark: bookmarkCreatorFunc(func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
				return nil, errors.New("bookmark create failed")
			}),
		}
		require.Error(t, p.bookmarkStatus(context.Background(), "alice", "https://example.com/statuses/1"))
	})

	t.Run("createOrUpdateList and addToList cover BeforeCreate failures", func(t *testing.T) {
		p := &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			repos: importStorageStub{
				object: objectCreatorFunc(func(_ context.Context, _ any) error { return nil }),
			},
		}

		_, err := p.createOrUpdateList(context.Background(), "", "Friends")
		require.Error(t, err)

		require.Error(t, p.addToList(context.Background(), "alice", "", "bob@remote.example"))
	})

	t.Run("import helpers exercise missing-field branches", func(t *testing.T) {
		p := &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			repos: importStorageStub{
				object: objectCreatorFunc(func(_ context.Context, _ any) error { return nil }),
			},
		}

		require.Error(t, p.importFollowActivity(context.Background(), ImportProcessorEvent{Username: "alice"}, map[string]any{"object": 123}))
		require.Error(t, p.importLikeActivity(context.Background(), ImportProcessorEvent{Username: "alice"}, map[string]any{"object": 123}))
		require.Error(t, p.importAnnounceActivity(context.Background(), ImportProcessorEvent{Username: "alice"}, map[string]any{"object": 123}))
	})

	t.Run("importObject covers published parsing branches", func(t *testing.T) {
		calls := 0
		p := &ImportProcessor{
			baseURL: "https://example.com",
			logger:  zap.NewNop(),
			repos: importStorageStub{
				object: objectCreatorFunc(func(_ context.Context, _ any) error { calls++; return nil }),
			},
		}

		require.Error(t, p.importObject(context.Background(), ImportProcessorEvent{}, map[string]any{}))
		require.NoError(t, p.importObject(context.Background(), ImportProcessorEvent{}, map[string]any{
			"id":           "https://example.com/objects/1",
			"type":         "Note",
			"content":      "hello",
			"published":    "2024-01-02T03:04:05Z",
			"attributedTo": 123,
		}))
		require.NoError(t, p.importObject(context.Background(), ImportProcessorEvent{}, map[string]any{
			"id":        "https://example.com/objects/2",
			"type":      "Note",
			"content":   "hello",
			"published": "not-a-time",
		}))
		require.NoError(t, p.importObject(context.Background(), ImportProcessorEvent{}, map[string]any{
			"id":      "https://example.com/objects/3",
			"type":    "Note",
			"content": "hello",
		}))
		require.GreaterOrEqual(t, calls, 3)
	})
}

func TestImportProcessor_Main_AndAdapters_Round12(t *testing.T) {
	t.Run("main registers SQS handler and starts lambda", func(t *testing.T) {
		origStart := lambdaStartFn
		t.Cleanup(func() { lambdaStartFn = origStart })

		startCalls := 0
		lambdaStartFn = func(handler interface{}) {
			startCalls++
			h, ok := handler.(func(context.Context, json.RawMessage) (any, error))
			require.True(t, ok)

			event := events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId:      "1",
						Body:           "{bad json",
						EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-import-processor-queue",
						EventSource:    "aws:sqs",
					},
				},
			}
			raw, err := json.Marshal(event)
			require.NoError(t, err)

			respAny, err := h(context.Background(), raw)
			require.NoError(t, err)
			resp, ok := respAny.(events.SQSEventResponse)
			require.True(t, ok)
			require.Empty(t, resp.BatchItemFailures)
		}

		t.Setenv("APP_NAME", "lesser")
		t.Setenv("STAGE", "dev")
		t.Setenv("ENVIRONMENT", "dev")

		processor = &ImportProcessor{logger: zap.NewNop(), s3Client: &s3ClientStub{}}
		main()
		require.Equal(t, 1, startCalls)
	})

	t.Run("importStorageAdapter nil storage returns nil repos", func(t *testing.T) {
		adapter := importStorageAdapter{storage: nil}
		require.Nil(t, adapter.Object())
		require.Nil(t, adapter.Actor())
		require.Nil(t, adapter.Activity())
		require.Nil(t, adapter.Bookmark())
	})
}

func TestImportProcessor_ErrorConstructors_Round12(t *testing.T) {
	require.Error(t, ErrAWSConfigLoad(errors.New("x")))
	require.Error(t, ErrS3ObjectGet(errors.New("x")))
	require.Error(t, ErrRollbackFailed(errors.New("x")))
	require.Error(t, ErrOperationFailed(errors.New("x")))
	require.Error(t, ErrUnsupportedImportFormat())
	require.Error(t, ErrCSVImportNotSupportedForType("x"))
	require.Error(t, ErrJSONImportNotSupportedForType("x"))
	require.Error(t, ErrActivityPubImportOnlySupportsArchive())
	require.Error(t, ErrNoItemsFoundInActivityPubCollection())
	require.Error(t, ErrItemNotValidActivityPubObject())
	require.Error(t, ErrItemMissingTypeField())
	require.Error(t, ErrCreateActivityMissingObject())
	require.Error(t, ErrCreateActivityObjectNotValid())
	require.Error(t, ErrFollowActivityMissingTargetObject())
	require.Error(t, ErrLikeActivityMissingObjectID())
	require.Error(t, ErrAnnounceActivityMissingObjectID())
	require.Error(t, ErrObjectMissingID())
	require.Error(t, ErrImportDownloadFailed(errors.New("x")))
	require.Error(t, ErrImportProcessFailed(errors.New("x")))
	require.Error(t, ErrImportStatusUpdateFailed(errors.New("x")))
	require.Error(t, ErrCSVHeaderRead(errors.New("x")))
	require.Error(t, ErrJSONParseFailed(errors.New("x")))
	require.Error(t, ErrActivityPubCollectionParseFailed(errors.New("x")))
	require.Error(t, ErrAnnouncePrepFailed(errors.New("x")))
	require.Error(t, ErrBlockPrepFailed(errors.New("x")))
	require.Error(t, ErrMutePrepFailed(errors.New("x")))
	require.Error(t, ErrListPrepFailed(errors.New("x")))
	require.Error(t, ErrListMemberPrepFailed(errors.New("x")))
	require.Error(t, ErrFollowRelationshipStore(errors.New("x")))
	require.Error(t, ErrFollowerActorGet(errors.New("x")))
	require.Error(t, ErrBookmarkCreate(errors.New("x")))
	require.Error(t, ErrListCreate(errors.New("x")))
}
