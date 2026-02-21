package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyStageEnvDefaults_SetsDefaultWhenUnset(t *testing.T) {
	t.Setenv("STAGE", "")
	applyStageEnvDefaults()
	require.Equal(t, "dev", os.Getenv("STAGE"))
}

func TestApplyStageEnvDefaults_DoesNotOverride(t *testing.T) {
	t.Setenv("STAGE", "prod")
	applyStageEnvDefaults()
	require.Equal(t, "prod", os.Getenv("STAGE"))
}

func TestResolveToolJobs_PrefersLesserJobsThenGOMAXPROCS(t *testing.T) {
	t.Setenv(lesserToolJobsEnvVar, "2")
	t.Setenv(goMaxProcsEnvVar, "5")
	require.Equal(t, 2, resolveToolJobs())

	t.Setenv(lesserToolJobsEnvVar, "nope")
	t.Setenv(goMaxProcsEnvVar, "5")
	require.Equal(t, defaultCLIMaxToolJobs, resolveToolJobs())
}

func TestApplyToolParallelismDefaults_SetsGOMAXPROCSAndGOFLAGSWhenUnset(t *testing.T) {
	t.Setenv(lesserToolJobsEnvVar, "4")
	t.Setenv(goMaxProcsEnvVar, "")
	t.Setenv(goFlagsEnvVar, "")

	applyToolParallelismDefaults()
	require.Equal(t, "4", os.Getenv(goMaxProcsEnvVar))
	require.Equal(t, "-p=4", os.Getenv(goFlagsEnvVar))
}

func TestApplyToolParallelismDefaults_AppendsToExistingGOFLAGS(t *testing.T) {
	t.Setenv(lesserToolJobsEnvVar, "3")
	t.Setenv(goMaxProcsEnvVar, "")
	t.Setenv(goFlagsEnvVar, "-trimpath")

	applyToolParallelismDefaults()
	require.Equal(t, "3", os.Getenv(goMaxProcsEnvVar))
	require.Equal(t, "-trimpath -p=3", os.Getenv(goFlagsEnvVar))
}

func TestApplyToolParallelismDefaults_DoesNotOverrideExistingSettings(t *testing.T) {
	t.Setenv(lesserToolJobsEnvVar, "6")
	t.Setenv(goMaxProcsEnvVar, "12")
	t.Setenv(goFlagsEnvVar, "-p=20")

	applyToolParallelismDefaults()
	require.Equal(t, "12", os.Getenv(goMaxProcsEnvVar))
	require.Equal(t, "-p=20", os.Getenv(goFlagsEnvVar))
}

func TestApplyToolParallelismDefaults_DoesNotModifyGOFLAGSWhenDashPTokenPresent(t *testing.T) {
	t.Setenv(lesserToolJobsEnvVar, "5")
	t.Setenv(goMaxProcsEnvVar, "")
	t.Setenv(goFlagsEnvVar, "-trimpath -p")

	applyToolParallelismDefaults()
	require.Equal(t, "5", os.Getenv(goMaxProcsEnvVar))
	require.Equal(t, "-trimpath -p", os.Getenv(goFlagsEnvVar))
}

func TestResolveToolJobs_FallsBackToRuntime(t *testing.T) {
	t.Setenv(lesserToolJobsEnvVar, "")
	t.Setenv(goMaxProcsEnvVar, "")

	jobs := resolveToolJobs()
	require.GreaterOrEqual(t, jobs, 1)
	require.LessOrEqual(t, jobs, defaultCLIMaxToolJobs)
}
