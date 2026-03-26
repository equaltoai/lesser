package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultCLIStage                         = "dev"
	defaultCLIMaxToolJobs                   = 4
	lesserToolJobsEnvVar                    = "LESSER_JOBS"
	goMaxProcsEnvVar                        = "GOMAXPROCS"
	goFlagsEnvVar                           = "GOFLAGS"
	goBuildParallelismFlag                  = "-p"
	lesserDefaultedGoMaxProcsEnvVar         = "LESSER_DEFAULTED_GOMAXPROCS"
	lesserDefaultedGoFlagsParallelismEnvVar = "LESSER_DEFAULTED_GOFLAGS_PARALLELISM"
)

func applyCLIDefaultEnv() {
	applyStageEnvDefaults()
	applyToolParallelismDefaults()
}

func applyStageEnvDefaults() {
	if strings.TrimSpace(os.Getenv("STAGE")) == "" {
		_ = os.Setenv("STAGE", defaultCLIStage)
	}
}

func applyToolParallelismDefaults() {
	jobs := resolveToolJobs()
	if jobs <= 0 {
		return
	}

	if strings.TrimSpace(os.Getenv(goMaxProcsEnvVar)) == "" {
		_ = os.Setenv(goMaxProcsEnvVar, fmt.Sprintf("%d", jobs))
		_ = os.Setenv(lesserDefaultedGoMaxProcsEnvVar, "1")
	}

	currentGoFlags := strings.TrimSpace(os.Getenv(goFlagsEnvVar))
	if goFlagsHasBuildParallelism(currentGoFlags) {
		return
	}

	if currentGoFlags == "" {
		_ = os.Setenv(goFlagsEnvVar, fmt.Sprintf("%s=%d", goBuildParallelismFlag, jobs))
		_ = os.Setenv(lesserDefaultedGoFlagsParallelismEnvVar, "1")
		return
	}
	_ = os.Setenv(goFlagsEnvVar, currentGoFlags+" "+fmt.Sprintf("%s=%d", goBuildParallelismFlag, jobs))
	_ = os.Setenv(lesserDefaultedGoFlagsParallelismEnvVar, "1")
}

func resolveToolJobs() int {
	if v := strings.TrimSpace(os.Getenv(lesserToolJobsEnvVar)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}

	if v := strings.TrimSpace(os.Getenv(goMaxProcsEnvVar)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > defaultCLIMaxToolJobs {
				return defaultCLIMaxToolJobs
			}
			return n
		}
	}

	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	if n > defaultCLIMaxToolJobs {
		return defaultCLIMaxToolJobs
	}
	return n
}

func goFlagsHasBuildParallelism(goFlags string) bool {
	for _, field := range strings.Fields(goFlags) {
		if field == goBuildParallelismFlag {
			return true
		}
		if strings.HasPrefix(field, goBuildParallelismFlag+"=") {
			return true
		}
	}
	return false
}
