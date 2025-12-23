// Package naming provides deterministic names and domains for Lesser deployments.
package naming

import (
	"fmt"
	"regexp"
	"strings"

	liftnaming "github.com/pay-theory/lift/pkg/naming"
)

// Stage identifies a deployment stage.
type Stage string

// Supported deployment stages.
const (
	StageDev     Stage = "dev"
	StageStaging Stage = "staging"
	StageLive    Stage = "live"
	StageShared  Stage = "shared"
)

// DefaultAppName is used when an explicit app name is not provided.
const DefaultAppName = "lesser"

var appSlugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// NormalizeAppName validates and normalizes an app slug.
func NormalizeAppName(app string) (string, error) {
	app = strings.ToLower(strings.TrimSpace(app))
	if app == "" {
		return "", fmt.Errorf("app name is required")
	}
	if !appSlugRE.MatchString(app) {
		return "", fmt.Errorf("invalid app name %q (expected lowercase slug: [a-z0-9] and hyphens)", app)
	}
	return app, nil
}

func appOrDefault(app string) string {
	app = strings.TrimSpace(app)
	if app == "" {
		return DefaultAppName
	}
	normalized, err := NormalizeAppName(app)
	if err != nil {
		// This is a programming/configuration error for infrastructure code paths.
		panic(err)
	}
	return normalized
}

// StageForEnvironment maps common environment aliases to canonical deployment stages.
func StageForEnvironment(environment string) Stage {
	value := strings.TrimSpace(strings.ToLower(environment))
	switch value {
	case "", "<nil>":
		return StageDev
	case "dev", "development":
		return StageDev
	case "test", "stage", "staging":
		return StageStaging
	case "prod", "production", "live":
		return StageLive
	default:
		return Stage(value)
	}
}

// IsLiveEnvironment returns true when the provided environment maps to the live stage.
func IsLiveEnvironment(environment string) bool {
	return StageForEnvironment(environment) == StageLive
}

// IsLiveStage returns true when the stage is live.
func IsLiveStage(stage Stage) bool {
	return stage == StageLive
}

// SharedStackName returns "<app>-shared".
func SharedStackName(app string) string {
	return fmt.Sprintf("%s-%s", appOrDefault(app), StageShared)
}

// StageStackName returns "<app>-<stage>".
func StageStackName(app string, stage Stage) string {
	return fmt.Sprintf("%s-%s", appOrDefault(app), stage)
}

// SharedResourceName returns "<app>-shared-<resource>".
func SharedResourceName(app string, resource string) string {
	return fmt.Sprintf("%s-%s-%s", appOrDefault(app), StageShared, strings.TrimSpace(resource))
}

// StageResourceName returns "<app>-<stage>-<resource>".
func StageResourceName(app string, stage Stage, resource string) string {
	return fmt.Sprintf("%s-%s-%s", appOrDefault(app), stage, strings.TrimSpace(resource))
}

// ResourceName returns "<default-app>-<stage>-<resource>" using a loosely-typed environment string.
func ResourceName(resource string, environment string) string {
	return ResourceNameWithApp(DefaultAppName, resource, environment)
}

// ResourceNameWithApp returns "<app>-<stage>-<resource>" using a loosely-typed environment string.
func ResourceNameWithApp(app string, resource string, environment string) string {
	return StageResourceName(appOrDefault(app), StageForEnvironment(environment), resource)
}

// GlobalResourceName returns "<app>-<stage>-<resource>-<account>-<region>".
func GlobalResourceName(app string, stage Stage, resource string, accountID string, region string) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s", appOrDefault(app), stage, strings.TrimSpace(resource), strings.TrimSpace(accountID), strings.TrimSpace(region))
}

// S3BucketName returns a deterministic S3 bucket name, including account and region for global uniqueness.
func S3BucketName(app string, stage Stage, bucketPurpose string, accountID string, region string) string {
	return liftnaming.SanitizeS3BucketName(GlobalResourceName(app, stage, bucketPurpose, accountID, region))
}

// StageDomain returns the stage domain. Live uses the base domain, other stages use "<stage>.<base-domain>".
func StageDomain(stage Stage, baseDomain string) string {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(baseDomain)), ".")
	if base == "" {
		return ""
	}
	if stage == StageLive {
		return base
	}
	return fmt.Sprintf("%s.%s", stage, base)
}

// ServiceDomain returns "<service>.<stage-domain>", where stage-domain is derived by StageDomain.
func ServiceDomain(service string, stage Stage, baseDomain string) string {
	svc := strings.ToLower(strings.TrimSpace(service))
	stageDomain := StageDomain(stage, baseDomain)
	if svc == "" || stageDomain == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", svc, stageDomain)
}
