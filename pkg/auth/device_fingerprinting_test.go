package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type inMemoryDeviceRepo struct {
	devicesByID map[string]*storage.Device
	errOnList   error
}

func newInMemoryDeviceRepo() *inMemoryDeviceRepo {
	return &inMemoryDeviceRepo{devicesByID: make(map[string]*storage.Device)}
}

func (r *inMemoryDeviceRepo) GetUserDevices(_ context.Context, username string) ([]*storage.Device, error) {
	if r.errOnList != nil {
		return nil, r.errOnList
	}
	var result []*storage.Device
	for _, device := range r.devicesByID {
		if device.Username == username {
			result = append(result, device)
		}
	}
	return result, nil
}

func (r *inMemoryDeviceRepo) CreateDevice(_ context.Context, device *storage.Device) error {
	r.devicesByID[device.DeviceID] = device
	return nil
}

func (r *inMemoryDeviceRepo) GetDevice(_ context.Context, deviceID string) (*storage.Device, error) {
	device, ok := r.devicesByID[deviceID]
	if !ok {
		return nil, errors.New("not found")
	}
	return device, nil
}

func (r *inMemoryDeviceRepo) UpdateDevice(_ context.Context, device *storage.Device) error {
	if _, ok := r.devicesByID[device.DeviceID]; !ok {
		return errors.New("not found")
	}
	r.devicesByID[device.DeviceID] = device
	return nil
}

func TestDeviceFingerprintManager_GenerateAndValidateDevice(t *testing.T) {
	t.Parallel()

	repo := newInMemoryDeviceRepo()
	dfm := NewDeviceFingerprintManager(repo, zap.NewNop(), &DeviceFingerprintConfig{
		EnableFingerprinting:  true,
		StrictFingerprinting:  true,
		FingerprintTTL:        time.Hour,
		TrustNewDevices:       false,
		RequireDeviceApproval: false,
		MaxDevicesPerUser:     20,
		DeviceTrustThreshold:  time.Hour,
	})

	ctx := context.Background()

	repo.devicesByID["dev-1"] = &storage.Device{
		DeviceID:      "dev-1",
		Username:      "alice",
		DeviceName:    "Phone",
		DeviceType:    "mobile",
		LastIPAddress: "192.0.2.10",
		LastUserAgent: "Mozilla/5.0 (iPhone)",
		CreatedAt:     time.Now().Add(-2 * time.Hour),
		LastSeenAt:    time.Now().Add(-time.Hour),
		TrustLevel:    TrustLevelTrusted,
	}

	fp := dfm.GenerateEnhancedFingerprint(
		"Mozilla/5.0 (iPhone)",
		"192.0.2.10",
		"en-US",
		"gzip",
		map[string]string{"timezone": "UTC"},
	)
	require.Equal(t, "IPv4", fp.IPVersion)
	require.NotEmpty(t, fp.BasicFingerprint)
	require.NotEmpty(t, fp.ExtendedFingerprint)
	require.Greater(t, fp.FingerprintEntropy, 0.0)

	result, err := dfm.ValidateDevice(ctx, "alice", fp)
	require.NoError(t, err)
	require.True(t, result.IsKnownDevice)
	require.Equal(t, "dev-1", result.DeviceID)
	require.Equal(t, TrustLevelTrusted, result.TrustLevel)
	require.GreaterOrEqual(t, result.MatchConfidence, 0.95)
	require.False(t, result.RequiresChallenge)
	require.False(t, result.RequiresApproval)
	require.Empty(t, result.ChangedAttributes)
	require.Equal(t, 0.0, result.RiskScore)
}

func TestDeviceFingerprintManager_NewDeviceFlow_RequiresChallengeAndApproval(t *testing.T) {
	t.Parallel()

	repo := newInMemoryDeviceRepo()
	dfm := NewDeviceFingerprintManager(repo, zap.NewNop(), &DeviceFingerprintConfig{
		EnableFingerprinting:  true,
		StrictFingerprinting:  false,
		FingerprintTTL:        time.Hour,
		TrustNewDevices:       false,
		RequireDeviceApproval: true,
		MaxDevicesPerUser:     20,
		DeviceTrustThreshold:  time.Hour,
	})

	fp := dfm.GenerateEnhancedFingerprint("curl/8.0", "bad-ip", "", "", nil)
	require.Equal(t, "unknown", fp.IPVersion)

	result, err := dfm.ValidateDevice(context.Background(), "alice", fp)
	require.NoError(t, err)
	require.False(t, result.IsKnownDevice)
	require.True(t, result.RequiresChallenge)
	require.True(t, result.RequiresApproval)
	require.Greater(t, result.RiskScore, 0.0)
}

func TestDeviceFingerprintManager_RegisterAndUpdateDeviceFingerprint(t *testing.T) {
	t.Parallel()

	repo := newInMemoryDeviceRepo()
	dfm := NewDeviceFingerprintManager(repo, zap.NewNop(), &DeviceFingerprintConfig{
		EnableFingerprinting:  true,
		StrictFingerprinting:  false,
		FingerprintTTL:        time.Hour,
		TrustNewDevices:       true,
		RequireDeviceApproval: false,
		MaxDevicesPerUser:     1,
		DeviceTrustThreshold:  time.Hour,
	})

	fp := dfm.GenerateEnhancedFingerprint("Mozilla/5.0 (Windows NT 10.0)", "192.0.2.10", "", "", nil)

	deviceInfo, err := dfm.RegisterNewDevice(context.Background(), "alice", fp, "Laptop")
	require.NoError(t, err)
	require.NotEmpty(t, deviceInfo.DeviceID)
	require.Equal(t, "trusted", deviceInfo.TrustLevel)
	require.False(t, deviceInfo.RequiresApproval)

	// Exceed device limit.
	_, err = dfm.RegisterNewDevice(context.Background(), "alice", fp, "Laptop2")
	require.ErrorIs(t, err, ErrMaxDevicesExceeded)

	// Promote trust level based on age threshold.
	device, err := repo.GetDevice(context.Background(), deviceInfo.DeviceID)
	require.NoError(t, err)
	device.TrustLevel = "untrusted"
	device.CreatedAt = time.Now().Add(-2 * time.Hour)
	repo.devicesByID[device.DeviceID] = device

	fp2 := dfm.GenerateEnhancedFingerprint("Mozilla/5.0 (Windows NT 10.0)", "192.0.2.11", "", "", nil)
	require.NoError(t, dfm.UpdateDeviceFingerprint(context.Background(), device.DeviceID, fp2))

	updatedDevice, err := repo.GetDevice(context.Background(), device.DeviceID)
	require.NoError(t, err)
	require.Equal(t, "trusted", updatedDevice.TrustLevel)
	require.Equal(t, "192.0.2.11", updatedDevice.LastIPAddress)
}
