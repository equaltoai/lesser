package bulk

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recordingPublisher struct {
	mu         sync.Mutex
	userEvents map[string][]streaming.Event
}

func (p *recordingPublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.userEvents == nil {
		p.userEvents = make(map[string][]streaming.Event)
	}
	if event != nil {
		p.userEvents[userID] = append(p.userEvents[userID], *event)
	}
	return nil
}

func (p *recordingPublisher) PublishToStream(_ context.Context, _ string, _ *streaming.Event) error {
	return nil
}

func (p *recordingPublisher) PublishToConversation(_ context.Context, _ string, _ *streaming.Event) error {
	return nil
}

func (p *recordingPublisher) Close() error { return nil }

type recordingFederation struct {
	mu         sync.Mutex
	activities []*activitypub.Activity
	err        error
}

func (f *recordingFederation) QueueActivity(_ context.Context, activity *activitypub.Activity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activities = append(f.activities, activity)
	return f.err
}

type fakeStatusRepo struct {
	getStatusFn    func(ctx context.Context, statusID string) (*models.Status, error)
	deleteStatusFn func(ctx context.Context, statusID string) error
	updateStatusFn func(ctx context.Context, status *models.Status) error
}

func (r *fakeStatusRepo) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	return r.getStatusFn(ctx, statusID)
}
func (r *fakeStatusRepo) DeleteStatus(ctx context.Context, statusID string) error {
	return r.deleteStatusFn(ctx, statusID)
}
func (r *fakeStatusRepo) UpdateStatus(ctx context.Context, status *models.Status) error {
	return r.updateStatusFn(ctx, status)
}

type fakeRelationshipRepo struct {
	createRelationshipFn func(ctx context.Context, followerUsername, followingUsername, activityID string) error
	deleteBlockFn        func(ctx context.Context, blockerActor, blockedActor string) error
	createMuteFn         func(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, duration *time.Duration) error
	createBlockFn        func(ctx context.Context, blockerActor, blockedActor, activityID string) error
}

func (r *fakeRelationshipRepo) CreateRelationship(ctx context.Context, followerUsername, followingUsername, activityID string) error {
	return r.createRelationshipFn(ctx, followerUsername, followingUsername, activityID)
}
func (r *fakeRelationshipRepo) DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error {
	return r.deleteBlockFn(ctx, blockerActor, blockedActor)
}
func (r *fakeRelationshipRepo) CreateMute(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, duration *time.Duration) error {
	return r.createMuteFn(ctx, muterActor, mutedActor, activityID, hideNotifications, duration)
}
func (r *fakeRelationshipRepo) CreateBlock(ctx context.Context, blockerActor, blockedActor, activityID string) error {
	return r.createBlockFn(ctx, blockerActor, blockedActor, activityID)
}

type fakeListRepo struct {
	addFn    func(ctx context.Context, listID, memberUsername string) error
	removeFn func(ctx context.Context, listID, memberUsername string) error
}

func (r *fakeListRepo) AddListMember(ctx context.Context, listID, memberUsername string) error {
	return r.addFn(ctx, listID, memberUsername)
}
func (r *fakeListRepo) RemoveListMember(ctx context.Context, listID, memberUsername string) error {
	return r.removeFn(ctx, listID, memberUsername)
}

func newServiceHarness(t *testing.T) (*Service, *recordingPublisher, *recordingFederation, *fakeStatusRepo, *fakeRelationshipRepo, *fakeListRepo) {
	t.Helper()

	publisher := &recordingPublisher{}
	federation := &recordingFederation{}

	statusRepo := &fakeStatusRepo{
		getStatusFn: func(context.Context, string) (*models.Status, error) {
			return nil, errors.New("unconfigured")
		},
		deleteStatusFn: func(context.Context, string) error { return errors.New("unconfigured") },
		updateStatusFn: func(context.Context, *models.Status) error { return errors.New("unconfigured") },
	}

	relationshipRepo := &fakeRelationshipRepo{
		createRelationshipFn: func(context.Context, string, string, string) error { return nil },
		deleteBlockFn:        func(context.Context, string, string) error { return nil },
		createMuteFn:         func(context.Context, string, string, string, bool, *time.Duration) error { return nil },
		createBlockFn:        func(context.Context, string, string, string) error { return nil },
	}

	listRepo := &fakeListRepo{
		addFn:    func(context.Context, string, string) error { return nil },
		removeFn: func(context.Context, string, string) error { return nil },
	}

	svc := NewService(nil, nil, nil, nil, nil, publisher, federation, zap.NewNop(), "example.com")
	svc.statusRepo = statusRepo
	svc.relationshipRepo = relationshipRepo
	svc.listRepo = listRepo

	return svc, publisher, federation, statusRepo, relationshipRepo, listRepo
}

func TestNewService_DefaultLogger(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil, nil, "example.com")
	require.NotNil(t, svc.logger)
}

func TestNewService_AssignsConcreteReposWhenProvided(t *testing.T) {
	svc := NewService(
		&repositories.StatusRepository{},
		nil,
		nil,
		&repositories.ListRepository{},
		&repositories.RelationshipRepository{},
		nil,
		nil,
		zap.NewNop(),
		"example.com",
	)

	require.NotNil(t, svc.statusRepo)
	require.NotNil(t, svc.listRepo)
	require.NotNil(t, svc.relationshipRepo)
}

func TestService_processBulkFollow_SuccessAndError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success queues federation and publishes progress/completion", func(t *testing.T) {
		svc, pub, fed, _, relRepo, _ := newServiceHarness(t)
		relRepo.createRelationshipFn = func(context.Context, string, string, string) error { return nil }

		op := &Operation{ID: "op", Type: "bulk_follow", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &FollowCommand{Username: "alice", AccountIDs: []string{"bob"}}
		svc.processBulkFollow(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Processed)
		require.Equal(t, 1, op.Succeeded)
		require.Equal(t, 0, op.Failed)

		fed.mu.Lock()
		require.Len(t, fed.activities, 1)
		fed.mu.Unlock()

		pub.mu.Lock()
		require.NotEmpty(t, pub.userEvents["alice"])
		pub.mu.Unlock()
	})

	t.Run("error records failures but continues", func(t *testing.T) {
		svc, pub, fed, _, relRepo, _ := newServiceHarness(t)
		relRepo.createRelationshipFn = func(context.Context, string, string, string) error { return errors.New("create failed") }

		op := &Operation{ID: "op", Type: "bulk_follow", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &FollowCommand{Username: "alice", AccountIDs: []string{"bob"}}
		svc.processBulkFollow(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Processed)
		require.Equal(t, 0, op.Succeeded)
		require.Equal(t, 1, op.Failed)
		require.NotEmpty(t, op.Errors)

		fed.mu.Lock()
		require.Empty(t, fed.activities)
		fed.mu.Unlock()

		pub.mu.Lock()
		require.NotEmpty(t, pub.userEvents["alice"])
		pub.mu.Unlock()
	})
}

func TestService_processBulkDeleteStatuses_Scenarios(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		svc, _, _, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) { return nil, errors.New("not found") }

		op := &Operation{ID: "op", Type: "bulk_delete_statuses", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &DeleteStatusesCommand{Username: "alice", StatusIDs: []string{"s1"}}
		svc.processBulkDeleteStatuses(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Failed)
	})

	t.Run("unauthorized", func(t *testing.T) {
		svc, _, _, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
			return &models.Status{StatusID: "s1", AuthorUsername: "bob"}, nil
		}

		op := &Operation{ID: "op", Type: "bulk_delete_statuses", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &DeleteStatusesCommand{Username: "alice", StatusIDs: []string{"s1"}}
		svc.processBulkDeleteStatuses(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Failed)
	})

	t.Run("delete error", func(t *testing.T) {
		svc, _, _, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
			return &models.Status{StatusID: "s1", AuthorUsername: "alice"}, nil
		}
		statusRepo.deleteStatusFn = func(context.Context, string) error { return errors.New("delete failed") }

		op := &Operation{ID: "op", Type: "bulk_delete_statuses", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &DeleteStatusesCommand{Username: "alice", StatusIDs: []string{"s1"}}
		svc.processBulkDeleteStatuses(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Failed)
	})

	t.Run("success publishes events and queues federation", func(t *testing.T) {
		svc, pub, fed, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
			return &models.Status{StatusID: "s1", AuthorUsername: "alice"}, nil
		}
		statusRepo.deleteStatusFn = func(context.Context, string) error { return nil }

		op := &Operation{ID: "op", Type: "bulk_delete_statuses", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &DeleteStatusesCommand{Username: "alice", StatusIDs: []string{"s1"}}
		svc.processBulkDeleteStatuses(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Succeeded)

		fed.mu.Lock()
		require.NotEmpty(t, fed.activities)
		fed.mu.Unlock()

		pub.mu.Lock()
		require.NotEmpty(t, pub.userEvents["alice"])
		pub.mu.Unlock()
	})
}

func TestService_processBulkDelete_Scenarios(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success deletes and publishes deletion/progress/completion", func(t *testing.T) {
		svc, pub, fed, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
			return &models.Status{StatusID: "c1", AuthorUsername: "alice"}, nil
		}
		statusRepo.deleteStatusFn = func(context.Context, string) error { return nil }

		op := &Operation{ID: "op", Type: "bulk_delete", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &DeleteCommand{Username: "alice", ContentIDs: []string{"c1"}, ContentType: StatusStatus}
		svc.processBulkDelete(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Succeeded)

		fed.mu.Lock()
		require.NotEmpty(t, fed.activities)
		fed.mu.Unlock()

		pub.mu.Lock()
		require.NotEmpty(t, pub.userEvents["alice"])
		pub.mu.Unlock()
	})

	t.Run("unsupported content type", func(t *testing.T) {
		svc, _, _, _, _, _ := newServiceHarness(t)

		op := &Operation{ID: "op", Type: "bulk_delete", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		cmd := &DeleteCommand{Username: "alice", ContentIDs: []string{"c1"}, ContentType: "media"}
		svc.processBulkDelete(ctx, op, cmd)

		require.Equal(t, StatusCompleted, op.Status)
		require.Equal(t, 1, op.Failed)
	})

	t.Run("not found and unauthorized and delete error", func(t *testing.T) {
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) { return nil, errors.New("not found") }

			op := &Operation{ID: "op", Type: "bulk_delete", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			cmd := &DeleteCommand{Username: "alice", ContentIDs: []string{"c1"}, ContentType: StatusStatus}
			svc.processBulkDelete(ctx, op, cmd)
			require.Equal(t, 1, op.Failed)
		}
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
				return &models.Status{StatusID: "c1", AuthorUsername: "bob"}, nil
			}

			op := &Operation{ID: "op", Type: "bulk_delete", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			cmd := &DeleteCommand{Username: "alice", ContentIDs: []string{"c1"}, ContentType: StatusStatus}
			svc.processBulkDelete(ctx, op, cmd)
			require.Equal(t, 1, op.Failed)
		}
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
				return &models.Status{StatusID: "c1", AuthorUsername: "alice"}, nil
			}
			statusRepo.deleteStatusFn = func(context.Context, string) error { return errors.New("delete failed") }

			op := &Operation{ID: "op", Type: "bulk_delete", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			cmd := &DeleteCommand{Username: "alice", ContentIDs: []string{"c1"}, ContentType: StatusStatus}
			svc.processBulkDelete(ctx, op, cmd)
			require.Equal(t, 1, op.Failed)
		}
	})
}

func TestService_processBulkArchiveRestoreExport_ListAndModeration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("archive/restore success and unsupported type", func(t *testing.T) {
		svc, _, _, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
			return &models.Status{StatusID: "s1", AuthorUsername: "alice"}, nil
		}
		statusRepo.updateStatusFn = func(_ context.Context, status *models.Status) error {
			status.UpdatedAt = time.Now()
			return nil
		}

		archiveOp := &Operation{ID: "op", Type: "bulk_archive", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkArchive(ctx, archiveOp, &ArchiveCommand{Username: "alice", ContentIDs: []string{"s1"}})
		require.Equal(t, StatusCompleted, archiveOp.Status)

		restoreOp := &Operation{ID: "op", Type: "bulk_restore", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkRestore(ctx, restoreOp, &RestoreCommand{Username: "alice", ContentIDs: []string{"s1"}})
		require.Equal(t, StatusCompleted, restoreOp.Status)

		unsupportedArchive := &Operation{ID: "op", Type: "bulk_archive", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkArchive(ctx, unsupportedArchive, &ArchiveCommand{Username: "alice", ContentIDs: []string{"s1"}, ContentType: "media"})
		require.Equal(t, 1, unsupportedArchive.Failed)
	})

	t.Run("archive/restore error paths", func(t *testing.T) {
		// Not found / unauthorized.
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) { return nil, errors.New("not found") }

			op := &Operation{ID: "op", Type: "bulk_archive", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkArchive(ctx, op, &ArchiveCommand{Username: "alice", ContentIDs: []string{"s1"}})
			require.Equal(t, 1, op.Failed)
		}
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
				return &models.Status{StatusID: "s1", AuthorUsername: "bob"}, nil
			}

			op := &Operation{ID: "op", Type: "bulk_restore", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkRestore(ctx, op, &RestoreCommand{Username: "alice", ContentIDs: []string{"s1"}})
			require.Equal(t, 1, op.Failed)
		}
		// Update error.
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
				return &models.Status{StatusID: "s1", AuthorUsername: "alice"}, nil
			}
			statusRepo.updateStatusFn = func(context.Context, *models.Status) error { return errors.New("update failed") }

			op := &Operation{ID: "op", Type: "bulk_archive", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkArchive(ctx, op, &ArchiveCommand{Username: "alice", ContentIDs: []string{"s1"}})
			require.Equal(t, 1, op.Failed)
		}
	})

	t.Run("export formats publish completion", func(t *testing.T) {
		svc, pub, _, statusRepo, _, _ := newServiceHarness(t)
		statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
			return &models.Status{
				StatusID:       "s1",
				AuthorUsername: "alice",
				Content:        "hi",
				Visibility:     "public",
				PublishedAt:    time.Now(),
				Note:           &activitypub.Note{},
			}, nil
		}

		for _, format := range []string{"json", "csv", "activitypub"} {
			op := &Operation{ID: "op-" + format, Type: "bulk_export", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkExport(ctx, op, &ExportCommand{Username: "alice", ContentIDs: []string{"s1"}, Format: format})
			require.Equal(t, StatusCompleted, op.Status)
		}

		pub.mu.Lock()
		require.NotEmpty(t, pub.userEvents["alice"])
		pub.mu.Unlock()
	})

	t.Run("export error paths", func(t *testing.T) {
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) { return nil, errors.New("not found") }
			op := &Operation{ID: "op", Type: "bulk_export", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkExport(ctx, op, &ExportCommand{Username: "alice", ContentIDs: []string{"s1"}, Format: "json"})
			require.Equal(t, 1, op.Failed)
		}
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
				return &models.Status{StatusID: "s1", AuthorUsername: "bob"}, nil
			}
			op := &Operation{ID: "op", Type: "bulk_export", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkExport(ctx, op, &ExportCommand{Username: "alice", ContentIDs: []string{"s1"}, Format: "json"})
			require.Equal(t, 1, op.Failed)
		}
		{
			svc, _, _, statusRepo, _, _ := newServiceHarness(t)
			statusRepo.getStatusFn = func(context.Context, string) (*models.Status, error) {
				return &models.Status{StatusID: "s1", AuthorUsername: "alice"}, nil
			}
			op := &Operation{ID: "op", Type: "bulk_export", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
			svc.processBulkExport(ctx, op, &ExportCommand{Username: "alice", ContentIDs: []string{"s1"}, Format: "json", ContentType: "media"})
			require.Equal(t, 1, op.Failed)
		}
	})

	t.Run("list members add/remove/default and moderation operations", func(t *testing.T) {
		svc, pub, fed, _, relRepo, listRepo := newServiceHarness(t)

		listRepo.addFn = func(context.Context, string, string) error { return nil }
		listRepo.removeFn = func(context.Context, string, string) error { return nil }

		listOp := &Operation{ID: "op", Type: "bulk_list_members_add", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkListMembers(ctx, &ListMembersCommand{Username: "alice", ListID: "l1", AccountIDs: []string{"bob"}, Operation: "add"}, listOp)
		require.Equal(t, "completed", listOp.Status)

		listOp2 := &Operation{ID: "op", Type: "bulk_list_members_remove", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkListMembers(ctx, &ListMembersCommand{Username: "alice", ListID: "l1", AccountIDs: []string{"bob"}, Operation: "remove"}, listOp2)
		require.Equal(t, "completed", listOp2.Status)

		listOp3 := &Operation{ID: "op", Type: "bulk_list_members_invalid", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkListMembers(ctx, &ListMembersCommand{Username: "alice", ListID: "l1", AccountIDs: []string{"bob"}, Operation: "nope"}, listOp3)
		require.Equal(t, 1, listOp3.Failed)

		// Repository error branch.
		listRepo.addFn = func(context.Context, string, string) error { return errors.New("add failed") }
		listOpErr := &Operation{ID: "op", Type: "bulk_list_members_add", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkListMembers(ctx, &ListMembersCommand{Username: "alice", ListID: "l1", AccountIDs: []string{"bob"}, Operation: "add"}, listOpErr)
		require.Equal(t, 1, listOpErr.Failed)

		relRepo.deleteBlockFn = func(context.Context, string, string) error { return nil }
		unblockOp := &Operation{ID: "op", Type: "bulk_unblock", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkUnblock(ctx, unblockOp, &UnblockCommand{Username: "alice", AccountIDs: []string{"bob"}})
		require.Equal(t, StatusCompleted, unblockOp.Status)

		// Error branches.
		relRepo.deleteBlockFn = func(context.Context, string, string) error { return errors.New("unblock failed") }
		unblockOpErr := &Operation{ID: "op", Type: "bulk_unblock", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkUnblock(ctx, unblockOpErr, &UnblockCommand{Username: "alice", AccountIDs: []string{"bob"}})
		require.Equal(t, 1, unblockOpErr.Failed)

		relRepo.createMuteFn = func(context.Context, string, string, string, bool, *time.Duration) error { return nil }
		muteOp := &Operation{ID: "op", Type: "bulk_mute", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkMute(ctx, muteOp, &MuteCommand{Username: "alice", AccountIDs: []string{"bob"}})
		require.Equal(t, StatusCompleted, muteOp.Status)

		relRepo.createMuteFn = func(context.Context, string, string, string, bool, *time.Duration) error {
			return errors.New("mute failed")
		}
		muteOpErr := &Operation{ID: "op", Type: "bulk_mute", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkMute(ctx, muteOpErr, &MuteCommand{Username: "alice", AccountIDs: []string{"bob"}})
		require.Equal(t, 1, muteOpErr.Failed)

		relRepo.createBlockFn = func(context.Context, string, string, string) error { return nil }
		blockOp := &Operation{ID: "op", Type: "bulk_block", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkBlock(ctx, blockOp, &BlockCommand{Username: "alice", AccountIDs: []string{"bob"}})
		require.Equal(t, StatusCompleted, blockOp.Status)

		relRepo.createBlockFn = func(context.Context, string, string, string) error { return errors.New("block failed") }
		blockOpErr := &Operation{ID: "op", Type: "bulk_block", Username: "alice", Status: "processing", Total: 1, StartedAt: time.Now()}
		svc.processBulkBlock(ctx, blockOpErr, &BlockCommand{Username: "alice", AccountIDs: []string{"bob"}})
		require.Equal(t, 1, blockOpErr.Failed)

		fed.mu.Lock()
		require.NotEmpty(t, fed.activities)
		fed.mu.Unlock()

		pub.mu.Lock()
		require.NotEmpty(t, pub.userEvents["alice"])
		pub.mu.Unlock()
	})
}

func TestService_WrapperMethods_Coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	publisher := &recordingPublisher{}

	svc := NewService(nil, nil, nil, nil, nil, publisher, nil, zap.NewNop(), "example.com")

	_, err := svc.BulkFollow(ctx, &FollowCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkDeleteStatuses(ctx, &DeleteStatusesCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkDelete(ctx, &DeleteCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkArchive(ctx, &ArchiveCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkRestore(ctx, &RestoreCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkExport(ctx, &ExportCommand{Username: "alice", Format: "json"})
	require.NoError(t, err)

	_, err = svc.BulkListMembers(ctx, &ListMembersCommand{Username: "alice", ListID: "l1", Operation: "add"})
	require.NoError(t, err)

	_, err = svc.BulkUnblock(ctx, &UnblockCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkMute(ctx, &MuteCommand{Username: "alice"})
	require.NoError(t, err)

	_, err = svc.BulkBlock(ctx, &BlockCommand{Username: "alice"})
	require.NoError(t, err)
}
