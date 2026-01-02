package auth

import (
	"testing"

	storageinterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recoveryProviderStub struct {
	recovery *repositories.RecoveryRepository
}

func (r recoveryProviderStub) Account() *repositories.AccountRepository               { return nil }
func (r recoveryProviderStub) Actor() storageinterfaces.ActorRepository               { return nil }
func (r recoveryProviderStub) Activity() storageinterfaces.ActivityRepository         { return nil }
func (r recoveryProviderStub) Notification() storageinterfaces.NotificationRepository { return nil }
func (r recoveryProviderStub) Recovery() *repositories.RecoveryRepository             { return r.recovery }
func (r recoveryProviderStub) Audit() *repositories.AuditRepository                   { return nil }

func TestNewRecoveryCodeService_WiresDependencies(t *testing.T) {
	repo := &repositories.RecoveryRepository{}
	svc := NewRecoveryCodeService(recoveryProviderStub{recovery: repo}, zap.NewNop())
	require.NotNil(t, svc)

	got, ok := svc.repo.(*repositories.RecoveryRepository)
	require.True(t, ok)
	require.Same(t, repo, got)
}
