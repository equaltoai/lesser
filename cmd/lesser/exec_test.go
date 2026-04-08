package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeEnvForDirAndSetEnv(t *testing.T) {
	base := []string{"A=1", "B=2"}
	merged := mergeEnvForDir(base, map[string]string{
		"B": "3",
		"C": "4",
	}, "")
	require.ElementsMatch(t, []string{"A=1", "B=3", "C=4", "GOTOOLCHAIN=" + defaultGoToolchainForDir("")}, merged)

	require.Equal(t, []string{"A=1", "B=2", "C=4"}, setEnv(base, "C", "4"))
	require.Equal(t, []string{"A=1", "B=9"}, setEnv(base, "B", "9"))
}

func TestMergeEnv_PrependsGoBinToPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOPATH", root)

	merged := mergeEnvForDir([]string{"PATH=/usr/bin"}, map[string]string{}, "")
	require.Contains(t, merged, "GOTOOLCHAIN="+defaultGoToolchainForDir(""))
	require.Contains(t, merged, "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+"/usr/bin")
}

func TestMergeEnv_EmptyPathUsesGoBin(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOPATH", root)

	merged := mergeEnvForDir([]string{"PATH="}, map[string]string{}, "")
	require.Contains(t, merged, "GOTOOLCHAIN="+defaultGoToolchainForDir(""))
	require.Contains(t, merged, "PATH="+filepath.Join(root, "bin"))
}

func TestRunCommand_SuccessAndFailure(t *testing.T) {
	require.NoError(t, runCommand(context.Background(), "bash", []string{"-lc", "exit 0"}, execOptions{}))

	err := runCommand(context.Background(), "bash", []string{"-lc", "exit 3"}, execOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bash -lc exit 3:")
}

func TestRunCommand_MissingToolReturnsError(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOPATH", root)
	t.Setenv("PATH", t.TempDir())

	err := runCommand(context.Background(), "definitely-not-a-tool", nil, execOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "definitely-not-a-tool")
}

func TestLookPathInEnvAndIsExecutable(t *testing.T) {
	t.Run("returns_name_when_contains_separator", func(t *testing.T) {
		path, err := lookPathInEnv("./tool", []string{"PATH=/usr/bin"})
		require.NoError(t, err)
		require.Equal(t, "./tool", path)
	})

	t.Run("missing_path_returns_error", func(t *testing.T) {
		_, err := lookPathInEnv("tool", []string{"PATH="})
		require.Error(t, err)
	})

	t.Run("finds_executable_on_path", func(t *testing.T) {
		dir := t.TempDir()
		candidate := filepath.Join(dir, "tool")
		require.NoError(t, os.WriteFile(candidate, []byte("#!/bin/sh\nexit 0\n"), 0o755))

		path, err := lookPathInEnv("tool", []string{"PATH=" + dir})
		require.NoError(t, err)
		require.Equal(t, candidate, path)
		require.True(t, isExecutable(path))
	})

	t.Run("non_executable_is_not_selected", func(t *testing.T) {
		dir := t.TempDir()
		candidate := filepath.Join(dir, "tool")
		require.NoError(t, os.WriteFile(candidate, []byte("nope"), 0o644))

		require.False(t, isExecutable(candidate))
		_, err := lookPathInEnv("tool", []string{"PATH=" + dir})
		require.Error(t, err)
	})
}

func TestGoPathBin_UsesGoEnvWhenGOPATHUnset(t *testing.T) {
	goExe, err := exec.LookPath("go")
	require.NoError(t, err)

	t.Setenv("GOPATH", "")
	t.Setenv("PATH", filepath.Dir(goExe))

	bin := goPathBin()
	require.NotEmpty(t, bin)
	require.True(t, strings.HasSuffix(bin, string(os.PathSeparator)+"bin"))
}

func TestGoPathBin_ReturnsEmptyWhenGOPATHStartsWithSeparator(t *testing.T) {
	t.Setenv("GOPATH", string(os.PathListSeparator)+t.TempDir())
	require.Empty(t, goPathBin())
}

func TestGoPathBin_ReturnsEmptyWhenGoMissingAndGOPATHUnset(t *testing.T) {
	t.Setenv("GOPATH", "")
	t.Setenv("PATH", t.TempDir())
	require.Empty(t, goPathBin())
}

func TestCacheDirVersionKey_FallsBackWhenGoMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	key := cacheDirVersionKey()
	require.Equal(t, sanitizeCacheKey(runtime.Version()), key)
}

func TestCacheDirVersionKey_UsesEffectiveGoToolchain(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "go")
	expectedToolchain := defaultGoToolchainForDir("")
	script := `#!/bin/sh
if [ "$1" = "env" ] && [ "$2" = "GOVERSION" ]; then
	if [ "${GOTOOLCHAIN:-}" = "` + expectedToolchain + `" ]; then
		printf 'go1.26.2'
	else
		printf 'go1.26.1'
	fi
	exit 0
fi
exit 1
`
	require.NoError(t, os.WriteFile(goPath, []byte(script), 0o755))

	t.Setenv("PATH", dir)
	previous, hadPrevious := os.LookupEnv("GOTOOLCHAIN")
	require.NoError(t, os.Unsetenv("GOTOOLCHAIN"))
	t.Cleanup(func() {
		if !hadPrevious {
			return
		}
		require.NoError(t, os.Setenv("GOTOOLCHAIN", previous))
	})

	require.Equal(t, "go1.26.2", cacheDirVersionKey())
}

func TestReadRequestedGoToolchain(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/lesser\n\ngo 1.26.2\n"), 0o644))

	toolchain, err := readRequestedGoToolchain(repoRoot)
	require.NoError(t, err)
	require.Equal(t, "go1.26.2", toolchain)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/lesser\n\ngo 1.26.2\ntoolchain go1.26.3\n"), 0o644))
	toolchain, err = readRequestedGoToolchain(repoRoot)
	require.NoError(t, err)
	require.Equal(t, "go1.26.3", toolchain)
}

func TestSanitizeCacheKey_EmptyReturnsUnknown(t *testing.T) {
	require.Equal(t, "unknown", sanitizeCacheKey("   "))
}

func TestGetEnv_MissingKeyReturnsEmpty(t *testing.T) {
	require.Empty(t, getEnv([]string{"A=1"}, "B"))
}

func TestIsExecutable_DirectoryIsFalse(t *testing.T) {
	dir := t.TempDir()
	require.False(t, isExecutable(dir))
}

func TestLookPathInEnv_EmptyDirUsesDot(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	workDir := t.TempDir()
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() { _ = os.Chdir(wd) })

	candidate := filepath.Join(workDir, "tool")
	require.NoError(t, os.WriteFile(candidate, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	got, err := lookPathInEnv("tool", []string{"PATH=:" + t.TempDir()})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(".", "tool"), got)
}

func TestCacheDirHelpers(t *testing.T) {
	repoRoot := t.TempDir()

	t.Run("defaults", func(t *testing.T) {
		t.Setenv("GOCACHE", "")
		t.Setenv("XDG_CACHE_HOME", "")

		goCache, err := ensureGoCacheDir(repoRoot)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(repoRoot, "tmp", "go-cache", cacheDirVersionKey()), goCache)
		require.DirExists(t, goCache)

		xdgCache, err := ensureXDGCacheDir(repoRoot)
		require.NoError(t, err)
		require.Equal(t, filepath.Join(repoRoot, "tmp", "xdg-cache", cacheDirVersionKey()), xdgCache)
		require.DirExists(t, xdgCache)
	})

	t.Run("overrides", func(t *testing.T) {
		goCacheOverride := filepath.Join(repoRoot, "go-cache-override")
		xdgCacheOverride := filepath.Join(repoRoot, "xdg-cache-override")
		t.Setenv("GOCACHE", goCacheOverride)
		t.Setenv("XDG_CACHE_HOME", xdgCacheOverride)

		otherRoot := t.TempDir()

		goCache, err := ensureGoCacheDir(otherRoot)
		require.NoError(t, err)
		require.Equal(t, goCacheOverride, goCache)
		require.DirExists(t, goCache)

		xdgCache, err := ensureXDGCacheDir(otherRoot)
		require.NoError(t, err)
		require.Equal(t, xdgCacheOverride, xdgCache)
		require.DirExists(t, xdgCache)
	})

	t.Run("override files error", func(t *testing.T) {
		goCacheFile := filepath.Join(repoRoot, "go-cache-file")
		xdgCacheFile := filepath.Join(repoRoot, "xdg-cache-file")
		require.NoError(t, os.WriteFile(goCacheFile, []byte("x"), 0o600))
		require.NoError(t, os.WriteFile(xdgCacheFile, []byte("x"), 0o600))

		t.Setenv("GOCACHE", goCacheFile)
		t.Setenv("XDG_CACHE_HOME", xdgCacheFile)

		_, err := ensureGoCacheDir(repoRoot)
		require.Error(t, err)

		_, err = ensureXDGCacheDir(repoRoot)
		require.Error(t, err)
	})
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("LESSER_TEST_ENV_OR_DEFAULT", "")
	require.Equal(t, "fallback", envOrDefault("LESSER_TEST_ENV_OR_DEFAULT", "fallback"))

	t.Setenv("LESSER_TEST_ENV_OR_DEFAULT", "  value ")
	require.Equal(t, "value", envOrDefault("LESSER_TEST_ENV_OR_DEFAULT", "fallback"))
}

func TestCaptureCommandOutput_SuccessAndFailure(t *testing.T) {
	dir := t.TempDir()
	out, err := captureCommandOutput(context.Background(), dir, map[string]string{}, "bash", "-lc", "printf 'hello'")
	require.NoError(t, err)
	require.Equal(t, "hello", out)

	_, err = captureCommandOutput(context.Background(), dir, map[string]string{}, "bash", "-lc", "echo boom; exit 2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}
