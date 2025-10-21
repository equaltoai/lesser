package disaster

import (
	"context"
	"testing"
	"time"
)

func TestNewDisasterRecoveryManager_DefaultDurations(t *testing.T) {
	config := DRConfig{
		PrimaryRegion: "us-east-1",
		BackupRegions: []string{"us-west-2"},
		HealthCheck: HealthCheckConfig{
			Enabled:  true,
			Interval: 0,
		},
		TestingSchedule: TestingScheduleConfig{
			Enabled:      true,
			Frequency:    0,
			NotifyBefore: time.Second,
		},
	}

	drm := NewDisasterRecoveryManager(config)

	if drm.config.HealthCheck.Interval != defaultHealthCheckInterval {
		t.Fatalf("expected health check interval default %v, got %v", defaultHealthCheckInterval, drm.config.HealthCheck.Interval)
	}

	if drm.config.TestingSchedule.Frequency != defaultDRTestFrequency {
		t.Fatalf("expected testing schedule frequency default %v, got %v", defaultDRTestFrequency, drm.config.TestingSchedule.Frequency)
	}
}

func TestDisasterRecoveryManager_StartMonitoring_InvalidSchedule(t *testing.T) {
	config := DRConfig{
		PrimaryRegion: "us-east-1",
		BackupRegions: []string{"us-west-2"},
		TestingSchedule: TestingScheduleConfig{
			Enabled:      true,
			Frequency:    time.Minute,
			NotifyBefore: time.Minute,
		},
	}

	drm := NewDisasterRecoveryManager(config)

	if err := drm.StartMonitoring(context.Background()); err == nil {
		t.Fatal("expected error for invalid testing schedule, got nil")
	}
}

func TestDisasterRecoveryManager_PerformDRTestReleasesLock(t *testing.T) {
	config := DRConfig{
		PrimaryRegion: "us-east-1",
		BackupRegions: []string{"us-west-2"},
		TestingSchedule: TestingScheduleConfig{
			Enabled:      true,
			Frequency:    time.Minute,
			NotifyBefore: 50 * time.Millisecond,
		},
		Notifications: NotificationConfig{Enabled: false},
	}

	drm := NewDisasterRecoveryManager(config)
	drm.currentState.RegionHealth["us-west-2"] = RegionHealth{
		Region:       "us-west-2",
		Status:       HealthStatusHealthy,
		Availability: 99.9,
		ResponseTime: 10 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		drm.performDRTest(ctx)
		close(done)
	}()

	// Allow performDRTest to enter the notification wait path
	time.Sleep(5 * time.Millisecond)

	healthHandled := make(chan struct{})
	go func() {
		drm.handleHealthEvent(ctx, HealthEvent{
			Region:       "us-east-1",
			Status:       HealthStatusHealthy,
			Timestamp:    time.Now(),
			ResponseTime: 5 * time.Millisecond,
		})
		close(healthHandled)
	}()

	select {
	case <-healthHandled:
		// Expected: health event handled while DR test is waiting
	case <-time.After(drm.config.TestingSchedule.NotifyBefore / 2):
		cancel()
		t.Fatal("health event handler blocked while performDRTest held the lock")
	}

	cancel()
	<-done
}
