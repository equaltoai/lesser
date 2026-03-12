package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestZipSingleFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "bootstrap")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o600))

	zipPath := filepath.Join(tmp, "out.zip")
	require.NoError(t, zipSingleFile(zipPath, "bootstrap", src))

	data, err := os.ReadFile(zipPath)
	require.NoError(t, err)

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, r.File, 1)
	require.Equal(t, "bootstrap", r.File[0].Name)

	t.Run("missing input file errors", func(t *testing.T) {
		err := zipSingleFile(filepath.Join(tmp, "missing.zip"), "bootstrap", filepath.Join(tmp, "nope"))
		require.Error(t, err)
	})
}

func TestZipSingleFile_ErrorBranches(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "bootstrap")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o600))

	t.Run("output dir exists as file", func(t *testing.T) {
		notDir := filepath.Join(tmp, "notdir")
		require.NoError(t, os.WriteFile(notDir, []byte("x"), 0o600))
		err := zipSingleFile(filepath.Join(notDir, "out.zip"), "bootstrap", src)
		require.Error(t, err)
	})

	t.Run("tmp path exists as directory", func(t *testing.T) {
		zipPath := filepath.Join(tmp, "out2.zip")
		require.NoError(t, os.MkdirAll(zipPath+".tmp", 0o755))
		err := zipSingleFile(zipPath, "bootstrap", src)
		require.Error(t, err)
	})

	t.Run("rename fails when destination is directory", func(t *testing.T) {
		zipPath := filepath.Join(tmp, "out3.zip")
		require.NoError(t, os.MkdirAll(zipPath, 0o755))
		err := zipSingleFile(zipPath, "bootstrap", src)
		require.Error(t, err)
	})
}

func TestLoadLambdaNamesFromInventory(t *testing.T) {
	repoRoot := t.TempDir()
	inventoryPath := filepath.Join(repoRoot, "infra", "cdk", "inventory")
	require.NoError(t, os.MkdirAll(inventoryPath, 0o755))

	content := []byte(`package inventory
var Lambdas = []struct{ Name string }{
  {Name: "api"},
  {Name: "inbox"},
  {Name: "api"}, // duplicate
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(inventoryPath, "lambdas.go"), content, 0o644))

	names, err := loadLambdaNamesFromInventory(repoRoot)
	require.NoError(t, err)
	require.Equal(t, []string{"api", "inbox"}, names)

	t.Run("missing inventory file errors", func(t *testing.T) {
		_, err := loadLambdaNamesFromInventory(t.TempDir())
		require.Error(t, err)
	})

	t.Run("no names found errors", func(t *testing.T) {
		repoRoot := t.TempDir()
		inventoryPath := filepath.Join(repoRoot, "infra", "cdk", "inventory")
		require.NoError(t, os.MkdirAll(inventoryPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(inventoryPath, "lambdas.go"), []byte("package inventory\n"), 0o644))
		_, err := loadLambdaNamesFromInventory(repoRoot)
		require.Error(t, err)
	})
}

func TestBuildLambdaZips_UsesInjectedBuilders(t *testing.T) {
	previousNames := loadLambdaNamesFromInventoryFn
	previousLatest := latestGoSourceUpdateFn
	previousBuild := buildLambdaBinaryFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		loadLambdaNamesFromInventoryFn = previousNames
		latestGoSourceUpdateFn = previousLatest
		buildLambdaBinaryFn = previousBuild
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()
	loadLambdaNamesFromInventoryFn = func(string) ([]string, error) {
		return []string{"lambda-one"}, nil
	}
	latestGoSourceUpdateFn = func(string) (time.Time, error) {
		return time.Now(), nil
	}
	buildLambdaBinaryFn = func(_ string, _ string, _ string, outPath string) error {
		return os.WriteFile(outPath, []byte("bin"), 0o600)
	}
	zipSingleFileFn = zipSingleFile

	require.NoError(t, buildLambdaZips(repoRoot, true))
	require.FileExists(t, filepath.Join(repoRoot, "bin", "lambda-one.zip"))
}

func TestBuildLambdaZips_PropagatesErrors(t *testing.T) {
	previousNames := loadLambdaNamesFromInventoryFn
	previousLatest := latestGoSourceUpdateFn
	t.Cleanup(func() {
		loadLambdaNamesFromInventoryFn = previousNames
		latestGoSourceUpdateFn = previousLatest
	})

	loadLambdaNamesFromInventoryFn = func(string) ([]string, error) { return nil, errSentinel }
	require.ErrorIs(t, buildLambdaZips(t.TempDir(), false), errSentinel)

	loadLambdaNamesFromInventoryFn = func(string) ([]string, error) { return []string{"lambda"}, nil }
	latestGoSourceUpdateFn = func(string) (time.Time, error) { return time.Time{}, errSentinel }
	require.ErrorIs(t, buildLambdaZips(t.TempDir(), false), errSentinel)
}

func TestBuildLambdaZips_PropagatesBuildAndZipErrors(t *testing.T) {
	previousNames := loadLambdaNamesFromInventoryFn
	previousLatest := latestGoSourceUpdateFn
	previousBuild := buildLambdaBinaryFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		loadLambdaNamesFromInventoryFn = previousNames
		latestGoSourceUpdateFn = previousLatest
		buildLambdaBinaryFn = previousBuild
		zipSingleFileFn = previousZip
	})

	loadLambdaNamesFromInventoryFn = func(string) ([]string, error) { return []string{"lambda-one"}, nil }
	latestGoSourceUpdateFn = func(string) (time.Time, error) { return time.Now(), nil }

	t.Run("build error", func(t *testing.T) {
		buildLambdaBinaryFn = func(string, string, string, string) error { return errSentinel }
		zipSingleFileFn = func(string, string, string) error { return nil }
		require.ErrorIs(t, buildLambdaZips(t.TempDir(), true), errSentinel)
	})

	t.Run("zip error", func(t *testing.T) {
		buildLambdaBinaryFn = func(_ string, _ string, _ string, outPath string) error {
			return os.WriteFile(outPath, []byte("bin"), 0o600)
		}
		zipSingleFileFn = func(string, string, string) error { return errSentinel }
		require.ErrorIs(t, buildLambdaZips(t.TempDir(), true), errSentinel)
	})
}

func TestBuildLambdaZips_RebuildsWhenArtifactsAreStale(t *testing.T) {
	previousNames := loadLambdaNamesFromInventoryFn
	previousLatest := latestGoSourceUpdateFn
	previousBuild := buildLambdaBinaryFn
	previousZip := zipSingleFileFn
	t.Cleanup(func() {
		loadLambdaNamesFromInventoryFn = previousNames
		latestGoSourceUpdateFn = previousLatest
		buildLambdaBinaryFn = previousBuild
		zipSingleFileFn = previousZip
	})

	repoRoot := t.TempDir()

	loadLambdaNamesFromInventoryFn = func(string) ([]string, error) { return []string{"lambda-one"}, nil }
	latestGoSourceUpdateFn = func(string) (time.Time, error) { return time.Now().Add(time.Hour), nil }

	zipPath := filepath.Join(repoRoot, "bin", "lambda-one.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o600))
	require.NoError(t, os.Chtimes(zipPath, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)))

	var built, zipped bool
	buildLambdaBinaryFn = func(_ string, _ string, _ string, outPath string) error {
		built = true
		return os.WriteFile(outPath, []byte("bin"), 0o600)
	}
	zipSingleFileFn = func(path string, _ string, _ string) error {
		zipped = true
		return os.WriteFile(path, []byte("zip"), 0o600)
	}

	require.NoError(t, buildLambdaZips(repoRoot, false))
	require.True(t, built)
	require.True(t, zipped)
}

func TestBuildLambdaZips_SkipsWhenArtifactsAreFresh(t *testing.T) {
	previousNames := loadLambdaNamesFromInventoryFn
	previousLatest := latestGoSourceUpdateFn
	previousBuild := buildLambdaBinaryFn
	t.Cleanup(func() {
		loadLambdaNamesFromInventoryFn = previousNames
		latestGoSourceUpdateFn = previousLatest
		buildLambdaBinaryFn = previousBuild
	})

	repoRoot := t.TempDir()
	loadLambdaNamesFromInventoryFn = func(string) ([]string, error) {
		return []string{"lambda-one"}, nil
	}
	sourceTime := time.Now().Add(-1 * time.Hour)
	latestGoSourceUpdateFn = func(string) (time.Time, error) {
		return sourceTime, nil
	}

	zipPath := filepath.Join(repoRoot, "bin", "lambda-one.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o600))
	require.NoError(t, os.Chtimes(zipPath, time.Now(), time.Now()))

	var called int
	buildLambdaBinaryFn = func(string, string, string, string) error {
		called++
		return nil
	}

	require.NoError(t, buildLambdaZips(repoRoot, false))
	require.Equal(t, 0, called)
}

func TestLatestGoSourceUpdate_TracksFiles(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.sum"), []byte(""), 0o644))

	for _, dir := range []string{"cmd", "pkg", "graph", "graphql"} {
		require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, dir), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, dir, "x.go"), []byte("package x\n"), 0o644))
	}

	_, err := latestGoSourceUpdate(repoRoot)
	require.NoError(t, err)
}

func TestLatestGoSourceUpdate_SkipsMissingDirsAndErrorsOnMissingFiles(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.sum"), []byte(""), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "cmd"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "cmd", "x.go"), []byte("package x\n"), 0o644))

	_, err := latestGoSourceUpdate(repoRoot)
	require.NoError(t, err)

	repoRoot2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot2, "go.mod"), []byte("module example.com/x\n"), 0o644))
	_, err = latestGoSourceUpdate(repoRoot2)
	require.Error(t, err)
}

func TestLatestGoSourceUpdate_WalkDirErrorAndMissingGoMod(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.sum"), []byte(""), 0o644))
	_, err := latestGoSourceUpdate(repoRoot)
	require.Error(t, err)

	repoRoot2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot2, "go.mod"), []byte("module example.com/x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot2, "go.sum"), []byte(""), 0o644))

	cmdDir := filepath.Join(repoRoot2, "cmd")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "x.go"), []byte("package x\n"), 0o644))

	pkgDir := filepath.Join(repoRoot2, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.Chmod(pkgDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(pkgDir, 0o755) })

	_, err = latestGoSourceUpdate(repoRoot2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scan")
}

func TestBuildLambdaBinary_WiresGoBuildCommand(t *testing.T) {
	previous := runExecCmdFn
	t.Cleanup(func() { runExecCmdFn = previous })

	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "tmp", "go-cache")
	outPath := filepath.Join(repoRoot, "bin", "bootstrap")

	var sawCmd *exec.Cmd
	runExecCmdFn = func(cmd *exec.Cmd) error {
		sawCmd = cmd
		return nil
	}

	require.NoError(t, buildLambdaBinary(repoRoot, cacheDir, "sse", outPath))
	require.NotNil(t, sawCmd)
	require.Contains(t, sawCmd.Args, "-tags")
	require.Contains(t, sawCmd.Args, "lambda.norpc")
	require.Equal(t, repoRoot, sawCmd.Dir)

	env := strings.Join(sawCmd.Env, "\n")
	require.Contains(t, env, "GOOS=linux")
	require.Contains(t, env, "GOARCH=arm64")
	require.Contains(t, env, "CGO_ENABLED=0")
	require.Contains(t, env, "GOCACHE="+cacheDir)
	require.Contains(t, env, "GOTOOLCHAIN="+defaultGoToolchainForDir(repoRoot))

	runExecCmdFn = func(cmd *exec.Cmd) error {
		require.NotContains(t, cmd.Args, "-tags")
		return nil
	}
	require.NoError(t, buildLambdaBinary(repoRoot, cacheDir, "inbox", outPath))
}

func TestBuildLambdaBinary_WrapsErrors(t *testing.T) {
	previous := runExecCmdFn
	t.Cleanup(func() { runExecCmdFn = previous })

	runExecCmdFn = func(*exec.Cmd) error { return errors.New("boom") }
	err := buildLambdaBinary(t.TempDir(), t.TempDir(), "lambda", "/tmp/out")
	require.Error(t, err)
	require.Contains(t, err.Error(), "go build lambda")
}
