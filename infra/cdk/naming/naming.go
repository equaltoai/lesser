package naming

import (
	"fmt"
	"strings"
)

const RepoName = "lesser"

type Stage string

const (
	StageLab   Stage = "lab"
	StageStudy Stage = "study"
	StageLive  Stage = "live"
)

// StageForEnvironment maps CDK context environments (development/staging/production)
// and Make targets (dev/test/live) to Lift-aligned stages (lab/study/live).
func StageForEnvironment(environment string) Stage {
	value := strings.TrimSpace(strings.ToLower(environment))
	switch value {
	case "lab", "dev", "development":
		return StageLab
	case "study", "test", "testing", "staging":
		return StageStudy
	case "live", "prod", "production":
		return StageLive
	default:
		if value == "" {
			return StageLab
		}
		// Fall back to the provided value for non-standard environments.
		return Stage(value)
	}
}

func IsLiveEnvironment(environment string) bool {
	return StageForEnvironment(environment) == StageLive
}

func ResourceName(resource string, environment string) string {
	return fmt.Sprintf("%s-%s-%s", RepoName, resource, StageForEnvironment(environment))
}

func ResourceNameWithApp(appName string, resource string, environment string) string {
	app := strings.TrimSpace(appName)
	if app == "" {
		app = RepoName
	}
	return fmt.Sprintf("%s-%s-%s", app, resource, StageForEnvironment(environment))
}
