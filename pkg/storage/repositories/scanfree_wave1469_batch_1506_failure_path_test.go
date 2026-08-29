package repositories

// M1/M2 row-set failure-path tests (rework of #1505 findings). The seam tests
// in scanfree_wave1469_batch_1505_seam_test.go pin the compiled KCE on every
// cursor failure path; these tests prove the BEHAVIOR against a real row set
// in fakedb: a decode-failure cursor must yield a bounded, prefix-filtered
// result (never a PK-only unfiltered walk that returns foreign rows), and a
// prefix-mismatch cursor must cleanly restart under the new prefix.
//
// Mutation kills (both demonstrated): remove the fallback begins_with bound in
// SearchAccounts / GetLoginHistory → the foreign row leaks into the result →
// these tests go RED.

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func newSeamRowDB(t *testing.T) (*captureDB, *fakedb.Fake, core.ExtendedDB) {
	t.Helper()
	client := fakedb.New()
	inner, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	return newCaptureDB(inner), client, inner
}

// TestBatch1506_SearchAccounts_DecodeFailure_BoundedFiltered seeds the
// USER_HANDLE_PREFIX#al partition with alice AND alex (both in the same 2-char
// prefix partition; alex must NOT match query "alice"), then drives
// SearchAccounts with a garbage cursor. The fallback must re-key BEGINS_WITH
// so the result is bounded to alice — the defect returned every partition row.
func TestBatch1506_SearchAccounts_DecodeFailure_BoundedFiltered(t *testing.T) {
	ctx := context.Background()
	_, _, inner := newSeamRowDB(t)
	require.NoError(t, inner.CreateTable(&models.User{}))

	seed := func(username string) {
		u := &models.User{Username: username, Role: "user", Approved: true, CreatedAt: time.Now().UTC()}
		require.NoError(t, u.UpdateKeys())
		require.NoError(t, inner.Model(u).Create())
	}
	seed("alice")
	seed("alex")

	repo := NewAccountRepository(inner, "test-table", "example.com", zap.NewNop())
	result, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: "!!!not-a-cursor!!!"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1, "decode-failure cursor must return a bounded, filtered result (issue #1505 M2)")
	require.Equal(t, "alice", result.Items[0].User.Username)
}

// TestBatch1506_SearchAccounts_PrefixMismatch_CleanRestart drives the same
// partition with a cursor whose PK prefix belongs to a DIFFERENT 2-char prefix
// (normal client behavior when the query text changes between pages). The walk
// must restart keyed on the NEW prefix — bounded and filtered, never an
// unfiltered read of the old partition. alex shares the USER_HANDLE_PREFIX#al
// partition but must never surface: an inclusive equal-bound BETWEEN
// ([al, al]) cannot return two rows with distinct sort keys, so only a
// partition-wide misread — a PK-only unfiltered read, or a prefix-keyed window
// (BETWEEN [al, al~]) over the shared partition — returns both rows and fails
// the exactly-one assertion. The equal-bound-on-query shape is pinned at the
// seam level by TestBatch1505Seam_SearchAccounts_RealChain's prefix-mismatch
// subtest (compiled operator: begins_with).
func TestBatch1506_SearchAccounts_PrefixMismatch_CleanRestart(t *testing.T) {
	ctx := context.Background()
	_, _, inner := newSeamRowDB(t)
	require.NoError(t, inner.CreateTable(&models.User{}))

	seed := func(username string) {
		u := &models.User{Username: username, Role: "user", Approved: true, CreatedAt: time.Now().UTC()}
		require.NoError(t, u.UpdateKeys())
		require.NoError(t, inner.Model(u).Create())
	}
	seed("alice")
	seed("alex")

	repo := NewAccountRepository(inner, "test-table", "example.com", zap.NewNop())
	// Cursor under USER_HANDLE_PREFIX#zz (a different partition than "al").
	cursor := Utils.Pagination.EncodeCursor("USER_HANDLE_PREFIX#zz", "alice")
	result, err := repo.SearchAccounts(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: cursor})
	require.NoError(t, err)
	require.Len(t, result.Items, 1, "prefix-mismatch cursor must cleanly restart under the new prefix (issue #1505 M2)")
	require.Equal(t, "alice", result.Items[0].User.Username)
}

// TestBatch1506_GetLoginHistory_DecodeFailure_BoundedFiltered seeds the
// USER#alice partition with a LOGIN# row AND a foreign (non-LOGIN#) row, then
// drives GetLoginHistory with a garbage cursor. The fallback must re-key
// begins_with LOGIN# so the foreign row never surfaces — the defect walked the
// whole partition unfiltered.
func TestBatch1506_GetLoginHistory_DecodeFailure_BoundedFiltered(t *testing.T) {
	ctx := context.Background()
	_, client, inner := newSeamRowDB(t)
	require.NoError(t, inner.CreateTable(&models.UserLogin{}))

	sAV := func(v string) *types.AttributeValueMemberS { return &types.AttributeValueMemberS{Value: v} }
	require.NoError(t, client.Seed(models.MainTableName,
		map[string]types.AttributeValue{
			"PK": sAV("USER#alice"), "SK": sAV("LOGIN#100"), "username": sAV("alice"),
		},
		map[string]types.AttributeValue{
			"PK": sAV("USER#alice"), "SK": sAV("OTHER#1"), "username": sAV("alice"),
		},
	))

	repo := NewAccountRepository(inner, "test-table", "example.com", zap.NewNop())
	result, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 20, Cursor: "!!!not-a-cursor!!!"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1, "decode-failure cursor must return only LOGIN# rows, never foreign rows (issue #1505 M1)")
}
