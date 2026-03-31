package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/releaseassets"
	"github.com/stretchr/testify/require"
)

func TestInstallReleaseLambdaAssets(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")

	result, err := installReleaseLambdaAssetsForTest(releaseDir, assetRoot)
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", result.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", result.GitSHA)
	require.Equal(t, []string{
		filepath.Join(assetRoot, "bin", "api.zip"),
		filepath.Join(assetRoot, "bin", "inbox.zip"),
	}, result.Files)

	apiBytes, err := os.ReadFile(filepath.Join(assetRoot, "bin", "api.zip"))
	require.NoError(t, err)
	require.Equal(t, "api zip", string(apiBytes))

	inboxBytes, err := os.ReadFile(filepath.Join(assetRoot, "bin", "inbox.zip"))
	require.NoError(t, err)
	require.Equal(t, "inbox zip", string(inboxBytes))
}

func TestInstallReleaseLambdaAssets_DoesNotStageUnderRepoRootTmp(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	targetRepo := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})
	require.NoError(t, os.WriteFile(filepath.Join(targetRepo, "tmp"), []byte("blocked"), 0o644))

	assetRoot := filepath.Join(t.TempDir(), "deploy", "lambda-assets")
	result, err := installReleaseLambdaAssetsForTest(releaseDir, assetRoot)
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(assetRoot, "bin", "api.zip"),
		filepath.Join(assetRoot, "bin", "inbox.zip"),
	}, result.Files)
	require.NoDirExists(t, filepath.Join(targetRepo, "tmp", "release-lambda-assets"))
}

func TestEnsureReleaseStagingDir_UsesDeployWorkspaceRoot(t *testing.T) {
	assetRoot := filepath.Join(t.TempDir(), "deploy", "lambda-assets")

	stagingDir, err := ensureReleaseStagingDir(assetRoot)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(stagingDir) })

	require.DirExists(t, stagingDir)
	require.Equal(t, filepath.Dir(assetRoot), filepath.Dir(stagingDir))
}

func TestEnsureReleaseStagingDir_ErrorsWhenWorkspaceRootBlocked(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(workspaceRoot, []byte("blocked"), 0o644))

	_, err := ensureReleaseStagingDir(filepath.Join(workspaceRoot, "lambda-assets"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "create release workspace root")
}

func TestInstallReleaseLambdaAssets_ErrorsWhenRequiredFileMissing(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	require.NoError(t, os.Remove(filepath.Join(releaseDir, releaseassets.ChecksumsFileName)))

	_, err := installReleaseLambdaAssetsForTest(releaseDir, filepath.Join(t.TempDir(), "lambda-assets"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "required release file checksums.txt")
}

func TestInstallReleaseLambdaAssets_ErrorsOnChecksumMismatch(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName), []byte("{}\n"), 0o644))

	_, err := installReleaseLambdaAssetsForTest(releaseDir, filepath.Join(t.TempDir(), "lambda-assets"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "lesser-lambda-bundle.json checksum mismatch")
}

func TestInstallReleaseLambdaAssets_DoesNotRequireRepoInventoryCheckout(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	result, err := installReleaseLambdaAssetsForTest(releaseDir, filepath.Join(t.TempDir(), "lambda-assets"))
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", result.Version)
}

func TestInstallReleaseLambdaAssets_AcceptsPathSortedBundleManifest(t *testing.T) {
	sourceRepo := testRepoWithCanonicalInventory(t, []string{"graphql", "graphql-ws"})
	require.NoError(t, os.MkdirAll(filepath.Join(sourceRepo, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRepo, "bin", "graphql.zip"), []byte("graphql zip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRepo, "bin", "graphql-ws.zip"), []byte("graphql-ws zip"), 0o644))

	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	result, err := installReleaseLambdaAssetsForTest(releaseDir, filepath.Join(t.TempDir(), "lambda-assets"))
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", result.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", result.GitSHA)
}

func TestInstallReleaseLambdaAssets_AcceptsCanonicalInventoryDeclaredOutOfOrder(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	result, err := installReleaseLambdaAssetsForTest(releaseDir, filepath.Join(t.TempDir(), "lambda-assets"))
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", result.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", result.GitSHA)
}

func TestVerifyReleaseChecksums_MissingEntry(t *testing.T) {
	releaseDir := t.TempDir()
	files := releaseFileSet{
		checksumsPath:        filepath.Join(releaseDir, releaseassets.ChecksumsFileName),
		releaseManifestPath:  filepath.Join(releaseDir, releaseassets.ReleaseManifestName),
		bundleArchivePath:    filepath.Join(releaseDir, releaseassets.LambdaBundleArchiveName),
		bundleManifestPath:   filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName),
		authUIArchivePath:    filepath.Join(releaseDir, releaseassets.AuthUIBundleArchiveName),
		assemblyArchivePath:  filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName),
		assemblyManifestPath: filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName),
	}

	for _, path := range []string{
		files.releaseManifestPath,
		files.bundleArchivePath,
		files.bundleManifestPath,
		files.authUIArchivePath,
		files.assemblyArchivePath,
		files.assemblyManifestPath,
	} {
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
	}

	err := verifyReleaseChecksums(files, map[string]string{
		releaseassets.ReleaseManifestName:     fileSHA256Hex(t, files.releaseManifestPath),
		releaseassets.LambdaBundleArchiveName: fileSHA256Hex(t, files.bundleArchivePath),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksums.txt missing entry for lesser-lambda-bundle.json")
}

func installReleaseLambdaAssetsForTest(releaseDir string, assetRoot string) (releaseLambdaInstallResult, error) {
	release, err := loadVerifiedReleaseAssets(releaseDir)
	if err != nil {
		return releaseLambdaInstallResult{}, err
	}
	return installReleaseLambdaAssetsFromVerified(release, assetRoot)
}

func TestVerifyReleaseChecksums_ChecksumMismatch(t *testing.T) {
	releaseDir := t.TempDir()
	files := releaseFileSet{
		checksumsPath:        filepath.Join(releaseDir, releaseassets.ChecksumsFileName),
		releaseManifestPath:  filepath.Join(releaseDir, releaseassets.ReleaseManifestName),
		bundleArchivePath:    filepath.Join(releaseDir, releaseassets.LambdaBundleArchiveName),
		bundleManifestPath:   filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName),
		authUIArchivePath:    filepath.Join(releaseDir, releaseassets.AuthUIBundleArchiveName),
		assemblyArchivePath:  filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName),
		assemblyManifestPath: filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName),
	}

	for _, path := range []string{
		files.releaseManifestPath,
		files.bundleArchivePath,
		files.bundleManifestPath,
		files.authUIArchivePath,
		files.assemblyArchivePath,
		files.assemblyManifestPath,
	} {
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
	}

	err := verifyReleaseChecksums(files, map[string]string{
		releaseassets.ReleaseManifestName:        fileSHA256Hex(t, files.releaseManifestPath),
		releaseassets.LambdaBundleArchiveName:    fileSHA256Hex(t, files.bundleArchivePath),
		releaseassets.LambdaBundleManifestName:   strings.Repeat("0", 64),
		releaseassets.AuthUIBundleArchiveName:    fileSHA256Hex(t, files.authUIArchivePath),
		releaseassets.DeployAssemblyArchiveName:  fileSHA256Hex(t, files.assemblyArchivePath),
		releaseassets.DeployAssemblyManifestName: fileSHA256Hex(t, files.assemblyManifestPath),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "lesser-lambda-bundle.json checksum mismatch")
}

func TestRequiredReleaseFiles_RejectsDirectory(t *testing.T) {
	releaseDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.ChecksumsFileName), []byte("sum"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(releaseDir, releaseassets.ReleaseManifestName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.LambdaBundleArchiveName), []byte("bundle"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.LambdaBundleManifestName), []byte("manifest"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.AuthUIBundleArchiveName), []byte("auth-ui"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName), []byte("assembly"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName), []byte("descriptor"), 0o644))

	_, err := requiredReleaseFiles(releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "required release file lesser-release.json is a directory")
}

func TestReadReleaseManifestFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), releaseassets.ReleaseManifestName)
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o644))

	_, err := readReleaseManifestFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read release manifest")
}

func TestReadReleaseManifestFile_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), releaseassets.ReleaseManifestName)
	data := `{
  "schema": 1,
  "name": "lesser",
  "version": "v1.2.3",
  "git_sha": "0123456789abcdef0123456789abcdef01234567",
  "go_version": "go1.26.1",
  "cdk": {"major": 2},
  "artifacts": {
    "receipt_schema_version": 7,
    "deploy_artifacts": {
      "schema_version": 1,
      "lambda_bundle": {
        "path": "lesser-lambda-bundle.tar.gz",
        "manifest_path": "lesser-lambda-bundle.json",
        "manifest_kind": "lesser.lambda_bundle_manifest",
        "manifest_schema_version": 1
      }
    }
  }
}`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	manifest, err := readReleaseManifestFile(path)
	require.NoError(t, err)
	require.Equal(t, "lesser", manifest.Name)
}

func TestReadLambdaBundleManifestFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), releaseassets.LambdaBundleManifestName)
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o644))

	_, err := readLambdaBundleManifestFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read lambda bundle manifest")
}

func TestReadLambdaBundleManifestFile_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), releaseassets.LambdaBundleManifestName)
	data := `{
  "kind": "lesser.lambda_bundle_manifest",
  "schema_version": 1,
  "release": {
    "name": "lesser",
    "version": "v1.2.3",
    "git_sha": "0123456789abcdef0123456789abcdef01234567"
  },
  "bundle": {
    "path": "lesser-lambda-bundle.tar.gz",
    "format": "tar.gz",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "inventory_source": {
    "path": "infra/cdk/inventory/lambdas.go",
    "kind": "lesser.lambda_inventory"
  },
  "files": [{
    "path": "bin/api.zip",
    "lambda": "api",
    "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "size_bytes": 7
  }]
}`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	manifest, err := readLambdaBundleManifestFile(path)
	require.NoError(t, err)
	require.Equal(t, "lesser.lambda_bundle_manifest", manifest.Kind)
}

func TestReadReleaseChecksums_ParseError(t *testing.T) {
	checksumsPath := filepath.Join(t.TempDir(), "checksums.txt")
	require.NoError(t, os.WriteFile(checksumsPath, []byte("not-a-valid-line\n"), 0o644))

	_, err := readReleaseChecksums(checksumsPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected '<sha256>  <file>'")
}

func TestValidateReleaseManifest(t *testing.T) {
	base := validReleaseManifest()

	tests := []struct {
		name        string
		mutate      func(*releaseassets.ReleaseManifest)
		wantErrText string
	}{
		{
			name: "valid",
		},
		{
			name: "schema",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Schema = 2
			},
			wantErrText: "unsupported release manifest schema",
		},
		{
			name: "name",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Name = "other"
			},
			wantErrText: "unexpected release manifest name",
		},
		{
			name: "deploy schema",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Artifacts.DeployArtifacts.SchemaVersion = 2
			},
			wantErrText: "unsupported deploy artifact schema",
		},
		{
			name: "bundle path",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Artifacts.DeployArtifacts.LambdaBundle.Path = "bundle.tgz"
			},
			wantErrText: "unexpected lambda bundle path",
		},
		{
			name: "manifest path",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestPath = "bundle.json"
			},
			wantErrText: "unexpected lambda bundle manifest path",
		},
		{
			name: "manifest kind",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestKind = "other.kind"
			},
			wantErrText: "unexpected lambda bundle manifest kind",
		},
		{
			name: "manifest schema version",
			mutate: func(manifest *releaseassets.ReleaseManifest) {
				manifest.Artifacts.DeployArtifacts.LambdaBundle.ManifestSchemaVersion = 2
			},
			wantErrText: "unsupported lambda bundle manifest schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := base
			if tt.mutate != nil {
				tt.mutate(&manifest)
			}

			err := validateReleaseManifest(manifest)
			if tt.wantErrText == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrText)
		})
	}
}

func TestExtractBundleToStaging_ErrorsOnUnexpectedEntry(t *testing.T) {
	manifest := validBundleManifest()
	stagingDir := t.TempDir()
	bundlePath := writeBundleArchive(t, map[string]string{
		"bin/api.zip":   "api zip",
		"bin/inbox.zip": "inbox zip",
		"bin/extra.zip": "extra zip",
	})

	manifest.Files[0].SHA256 = sha256HexString("api zip")
	manifest.Files[1].SHA256 = sha256HexString("inbox zip")

	err := extractBundleToStaging(bundlePath, manifest, stagingDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected bundle archive entry bin/extra.zip")
}

func TestExtractBundleToStaging_ErrorsOnMissingFiles(t *testing.T) {
	manifest := validBundleManifest()
	stagingDir := t.TempDir()
	bundlePath := writeBundleArchive(t, map[string]string{
		"bin/api.zip": "api zip",
	})

	manifest.Files[0].SHA256 = sha256HexString("api zip")
	manifest.Files[1].SHA256 = sha256HexString("inbox zip")

	err := extractBundleToStaging(bundlePath, manifest, stagingDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "lambda bundle archive missing files")
	require.Contains(t, err.Error(), "bin/inbox.zip")
}

func TestExtractBundleEntry_ErrorsOnGuardClauses(t *testing.T) {
	manifestFile := releaseassets.LambdaBundleManifestFile{
		Path:      "bin/api.zip",
		Lambda:    "api",
		SHA256:    sha256HexString("api zip"),
		SizeBytes: int64(len("api zip")),
	}
	expectedFiles := map[string]releaseassets.LambdaBundleManifestFile{
		manifestFile.Path: manifestFile,
	}

	t.Run("nil header", func(t *testing.T) {
		err := extractBundleEntry(nil, nil, t.TempDir(), expectedFiles, map[string]struct{}{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "bundle archive entry is nil")
	})

	t.Run("unexpected type", func(t *testing.T) {
		err := extractBundleEntry(nil, &tar.Header{
			Name:     manifestFile.Path,
			Typeflag: tar.TypeDir,
		}, t.TempDir(), expectedFiles, map[string]struct{}{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected bundle entry type")
	})

	t.Run("duplicate entry", func(t *testing.T) {
		err := extractBundleEntry(nil, &tar.Header{
			Name:     manifestFile.Path,
			Typeflag: tar.TypeReg,
			Size:     manifestFile.SizeBytes,
		}, t.TempDir(), expectedFiles, map[string]struct{}{
			manifestFile.Path: {},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate bundle archive entry")
	})

	t.Run("path traversal", func(t *testing.T) {
		err := extractBundleEntry(nil, &tar.Header{
			Name:     "bin/../api.zip",
			Typeflag: tar.TypeReg,
			Size:     manifestFile.SizeBytes,
		}, t.TempDir(), expectedFiles, map[string]struct{}{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not contain '..'")
	})

	t.Run("windows separator", func(t *testing.T) {
		err := extractBundleEntry(nil, &tar.Header{
			Name:     `bin\\api.zip`,
			Typeflag: tar.TypeReg,
			Size:     manifestFile.SizeBytes,
		}, t.TempDir(), expectedFiles, map[string]struct{}{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must use forward slashes")
	})

	t.Run("size mismatch", func(t *testing.T) {
		err := extractBundleEntry(nil, &tar.Header{
			Name:     manifestFile.Path,
			Typeflag: tar.TypeReg,
			Size:     manifestFile.SizeBytes + 1,
		}, t.TempDir(), expectedFiles, map[string]struct{}{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "size mismatch")
	})
}

func TestWriteVerifiedExtractedFile_ErrorsOnChecksumMismatch(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "bin", "api.zip")
	err := writeVerifiedExtractedFile(targetPath, strings.NewReader("api zip"), releaseassets.LambdaBundleManifestFile{
		Path:      "bin/api.zip",
		Lambda:    "api",
		SHA256:    sha256HexString("other"),
		SizeBytes: int64(len("api zip")),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")
}

func TestWriteVerifiedExtractedFile_ErrorsOnSizeMismatch(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "bin", "api.zip")
	err := writeVerifiedExtractedFile(targetPath, strings.NewReader("api zip"), releaseassets.LambdaBundleManifestFile{
		Path:      "bin/api.zip",
		Lambda:    "api",
		SHA256:    sha256HexString("api zip"),
		SizeBytes: int64(len("api zip") + 1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "size mismatch")
}

func TestCopyFile_ErrorsWhenSourceMissing(t *testing.T) {
	err := copyFile(filepath.Join(t.TempDir(), "bin", "api.zip"), filepath.Join(t.TempDir(), "missing.zip"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "open extracted file")
}

func TestInstallExtractedBundleFiles_Success(t *testing.T) {
	stagingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stagingDir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stagingDir, "bin", "api.zip"), []byte("api zip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stagingDir, "bin", "inbox.zip"), []byte("inbox zip"), 0o644))

	assetRoot := filepath.Join(t.TempDir(), "lambda-assets")
	files, err := installExtractedBundleFiles(assetRoot, stagingDir, []releaseassets.LambdaBundleManifestFile{
		{Path: "bin/api.zip", Lambda: "api"},
		{Path: "bin/inbox.zip", Lambda: "inbox"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(assetRoot, "bin", "api.zip"),
		filepath.Join(assetRoot, "bin", "inbox.zip"),
	}, files)

	apiBytes, err := os.ReadFile(filepath.Join(assetRoot, "bin", "api.zip"))
	require.NoError(t, err)
	require.Equal(t, "api zip", string(apiBytes))
}

func TestCopyFileAndChecksumHelpers_Success(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "staging", "api.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
	require.NoError(t, os.WriteFile(sourcePath, []byte("api zip"), 0o644))

	targetPath := filepath.Join(root, "deploy", "api.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(targetPath), 0o755))
	require.NoError(t, copyFile(targetPath, sourcePath))

	targetBytes, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, "api zip", string(targetBytes))

	actualSHA, err := sha256File(targetPath)
	require.NoError(t, err)
	require.Equal(t, sha256HexString("api zip"), actualSHA)
	require.NoError(t, verifyFileChecksum(targetPath, actualSHA, "api.zip"))

	type checksumDoc struct {
		Name string `json:"name"`
	}
	jsonPath := filepath.Join(root, "doc.json")
	require.NoError(t, os.WriteFile(jsonPath, []byte(`{"name":"lesser"}`), 0o644))

	var doc checksumDoc
	require.NoError(t, readJSONFile(jsonPath, &doc))
	require.Equal(t, "lesser", doc.Name)
}

func TestCopyFile_ErrorsWhenTargetCreateFails(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "staging", "api.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
	require.NoError(t, os.WriteFile(sourcePath, []byte("api zip"), 0o644))

	err := copyFile(filepath.Join(root, "missing", "nested", "api.zip"), sourcePath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create target file")
}

func TestInstallReleaseLambdaAssetsFromVerified_ErrorsWhenWorkspaceBlocked(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)
	release, err := loadVerifiedReleaseAssets(releaseDir)
	require.NoError(t, err)

	blockedParent := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blockedParent, []byte("blocked"), 0o644))

	_, err = installReleaseLambdaAssetsFromVerified(release, filepath.Join(blockedParent, "lambda-assets"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "reset release workspace")
}

func TestValidateBundleManifest(t *testing.T) {
	releaseManifest := validReleaseManifest()
	tests := []struct {
		name        string
		mutate      func(*releaseassets.LambdaBundleManifest)
		wantErrText string
	}{
		{
			name: "valid",
		},
		{
			name: "kind",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Kind = "other.kind"
			},
			wantErrText: "unexpected lambda bundle manifest kind",
		},
		{
			name: "schema",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.SchemaVersion = 2
			},
			wantErrText: "unsupported lambda bundle manifest schema",
		},
		{
			name: "release name",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Release.Name = "other"
			},
			wantErrText: "unexpected lambda bundle release name",
		},
		{
			name: "release version",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Release.Version = "v9.9.9"
			},
			wantErrText: "does not match release manifest",
		},
		{
			name: "git sha",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Release.GitSHA = "ffffffffffffffffffffffffffffffffffffffff"
			},
			wantErrText: "does not match release manifest",
		},
		{
			name: "bundle path",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Bundle.Path = "bundle.tgz"
			},
			wantErrText: "does not match release manifest",
		},
		{
			name: "bundle format",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Bundle.Format = "zip"
			},
			wantErrText: "unexpected lambda bundle format",
		},
		{
			name: "inventory path",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.InventorySource.Path = "inventory.json"
			},
			wantErrText: "unexpected lambda inventory path",
		},
		{
			name: "inventory kind",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.InventorySource.Kind = "other.inventory"
			},
			wantErrText: "unexpected lambda inventory kind",
		},
		{
			name: "empty files",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Files = nil
			},
			wantErrText: "has no files",
		},
		{
			name: "non normalized path",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Files[0].Path = "bin/../api.zip"
			},
			wantErrText: "is not normalized",
		},
		{
			name: "non bin path",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Files[0].Path = "dist/api.zip"
			},
			wantErrText: "must be bin/<lambda>.zip",
		},
		{
			name: "lambda mismatch",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Files[0].Lambda = "graphql"
			},
			wantErrText: "does not match lambda",
		},
		{
			name: "unsorted",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Files[0], manifest.Files[1] = manifest.Files[1], manifest.Files[0]
			},
			wantErrText: "must be sorted",
		},
		{
			name: "duplicate",
			mutate: func(manifest *releaseassets.LambdaBundleManifest) {
				manifest.Files[1].Path = manifest.Files[0].Path
				manifest.Files[1].Lambda = manifest.Files[0].Lambda
			},
			wantErrText: "duplicate lambda bundle manifest file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validBundleManifest()
			if tt.mutate != nil {
				tt.mutate(&manifest)
			}

			err := validateBundleManifest(manifest, releaseManifest)
			if tt.wantErrText == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrText)
		})
	}
}

func TestNormalizeReleaseAssetPath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		wantErrText string
	}{
		{name: "valid", input: "bin/api.zip", want: "bin/api.zip"},
		{name: "empty", input: "   ", wantErrText: "path is empty"},
		{name: "absolute", input: "/tmp/api.zip", wantErrText: "path must be relative"},
		{name: "traversal", input: "../api.zip", wantErrText: "path must stay within release asset root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeReleaseAssetPath(tt.input)
			if tt.wantErrText == "" {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrText)
		})
	}
}

func TestInstallReleaseDeployAssemblyFromVerified_StagesTemplatesAndAssets(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	payload := validReleaseAssemblyPayloadManifest()
	assetBody := "console.log('release asset');\n"
	payload.Assets = []releaseDeployAssemblyAssetManifest{
		{
			ObjectKey:   "release/v1/assets/frontend/main.js",
			ArchivePath: "assets/frontend/main.js",
			SHA256:      sha256HexString(assetBody),
			SizeBytes:   int64(len(assetBody)),
		},
	}
	writeDeployAssemblyFixture(t, releaseDir, payload, map[string]string{
		"assets/frontend/main.js": assetBody,
	})

	release, err := loadVerifiedReleaseAssets(releaseDir)
	require.NoError(t, err)

	result, err := installReleaseDeployAssemblyFromVerified(release, filepath.Join(t.TempDir(), "deploy-assembly"))
	require.NoError(t, err)
	require.Equal(t, "manifest.json", result.PayloadEntrypoint)
	require.FileExists(t, result.SharedTemplate)
	require.FileExists(t, result.StageTemplates[naming.StageDev])
	require.FileExists(t, result.StageTemplates[naming.StageStaging])
	require.FileExists(t, result.StageTemplates[naming.StageLive])
	require.Len(t, result.Assets, 1)
	require.Equal(t, "release/v1/assets/frontend/main.js", result.Assets[0].ObjectKey)
	require.FileExists(t, result.Assets[0].LocalPath)

	assetBytes, err := os.ReadFile(result.Assets[0].LocalPath)
	require.NoError(t, err)
	require.Equal(t, assetBody, string(assetBytes))
}

func TestInstallReleaseDeployAssets_StagesWorkspaceRoots(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	workspaceRoot := filepath.Join(t.TempDir(), "deploy")
	result, err := installReleaseDeployAssets(releaseDir, workspaceRoot)
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", result.Version)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", result.GitSHA)
	require.Equal(t, filepath.Join(workspaceRoot, "lambda-assets"), result.LambdaAssetRoot)
	require.Equal(t, filepath.Join(workspaceRoot, releaseAuthUIWorkspaceName), result.AuthUIDir)
	require.Equal(t, filepath.Join(workspaceRoot, releaseDeployAssemblyWorkspaceName), result.Assembly.RootDir)
	require.FileExists(t, filepath.Join(result.AuthUIDir, "index.html"))
	require.FileExists(t, result.Assembly.SharedTemplate)
	require.Len(t, result.LambdaFiles, 2)
}

func TestInstallReleaseDeployAssemblyFromVerified_ErrorsWhenArchiveContentsDrift(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	writeDeployAssemblyFixture(t, releaseDir, validReleaseAssemblyPayloadManifest(), map[string]string{
		"assets/frontend/extra.js": "console.log('extra');\n",
	})

	release, err := loadVerifiedReleaseAssets(releaseDir)
	require.NoError(t, err)

	_, err = installReleaseDeployAssemblyFromVerified(release, filepath.Join(t.TempDir(), "deploy-assembly"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "deploy assembly archive contents do not match payload manifest")
}

func TestInstallReleaseAuthUIBundleFromVerified_ErrorsWhenIndexMissing(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	writeAuthUIBundleFixture(t, releaseDir, map[string]string{
		"assets/app.js": "console.log('auth');\n",
	})

	release, err := loadVerifiedReleaseAssets(releaseDir)
	require.NoError(t, err)

	err = installReleaseAuthUIBundleFromVerified(release, filepath.Join(t.TempDir(), "auth-ui"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth-ui archive missing index.html")
}

func TestInstallReleaseDeployAssets_ErrorsWhenAuthUIBundleInvalid(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	writeAuthUIBundleFixture(t, releaseDir, map[string]string{
		"assets/app.js": "console.log('auth');\n",
	})

	_, err := installReleaseDeployAssets(releaseDir, filepath.Join(t.TempDir(), "deploy"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth-ui archive missing index.html")
}

func TestLoadVerifiedReleaseAssets_RejectsDeployAssemblyDescriptorMismatch(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	descriptorPath := filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName)
	descriptor, err := readDeployAssemblyDescriptorFile(descriptorPath)
	require.NoError(t, err)

	descriptor.Release.Version = "v9.9.9"
	descriptorBytes, err := json.MarshalIndent(descriptor, "", "  ")
	require.NoError(t, err)
	descriptorBytes = append(descriptorBytes, '\n')
	require.NoError(t, os.WriteFile(descriptorPath, descriptorBytes, 0o644))
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))

	_, err = loadVerifiedReleaseAssets(releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match release manifest")
}

func TestLoadVerifiedReleaseAssets_RejectsInvalidDeployAssemblyDescriptorJSON(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	descriptorPath := filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName)
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{"), 0o644))
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))

	_, err := loadVerifiedReleaseAssets(releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read deploy assembly descriptor")
}

func TestLoadVerifiedReleaseAssets_RejectsDeployAssemblyArchiveChecksumMismatch(t *testing.T) {
	sourceRepo := testRepoWithCanonicalLambdaArtifacts(t, map[string]string{
		"api":   "api zip",
		"inbox": "inbox zip",
	})
	releaseDir := testReleaseDirFromRepo(t, sourceRepo)

	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName), []byte("tampered"), 0o644))
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))

	_, err := loadVerifiedReleaseAssets(releaseDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "deploy assembly archive checksum mismatch")
}

func TestExtractTarGzArchive_ErrorsOnInvalidArchiveAndVisitorFailure(t *testing.T) {
	invalidArchive := filepath.Join(t.TempDir(), "invalid.tar.gz")
	require.NoError(t, os.WriteFile(invalidArchive, []byte("not a gzip stream"), 0o644))

	err := extractTarGzArchive(invalidArchive, func(*tar.Header, *tar.Reader) error { return nil })
	require.Error(t, err)
	require.Contains(t, err.Error(), "open gzip stream")

	validArchive := filepath.Join(t.TempDir(), "valid.tar.gz")
	writeTarGzFixture(t, validArchive, map[string]string{
		"assets/plain.txt": "plain",
	})

	err = extractTarGzArchive(validArchive, func(*tar.Header, *tar.Reader) error {
		return errSentinel
	})
	require.ErrorIs(t, err, errSentinel)
}

func TestWriteExtractedFile_ErrorsOnCreateAndFinalizeFailures(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blockedParent, []byte("blocked"), 0o644))

	err := writeExtractedFile(filepath.Join(blockedParent, "plain.txt"), strings.NewReader("plain"), 5, "plain.txt")
	require.ErrorContains(t, err, "create extraction dir")

	root := t.TempDir()
	targetPath := filepath.Join(root, "existing-dir")
	require.NoError(t, os.MkdirAll(targetPath, 0o755))

	err = writeExtractedFile(targetPath, strings.NewReader("plain"), 5, "plain.txt")
	require.ErrorContains(t, err, "finalize extracted file")
}

func TestResetAndFinalizeReleaseWorkspaceDir_ErrorsWhenParentBlocked(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blockedParent, []byte("blocked"), 0o644))

	err := resetReleaseWorkspaceDir(filepath.Join(blockedParent, "workspace"))
	require.ErrorContains(t, err, "reset release workspace")

	staging := filepath.Join(t.TempDir(), "staging")
	require.NoError(t, os.MkdirAll(staging, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "new.txt"), []byte("new"), 0o644))

	err = finalizeReleaseWorkspaceDir(filepath.Join(t.TempDir(), "missing-parent", "workspace"), staging)
	require.ErrorContains(t, err, "finalize release workspace")
}

func testReleaseDirFromRepo(t *testing.T, repoRoot string) string {
	t.Helper()

	const (
		version = "v1.2.3"
		gitSHA  = "0123456789abcdef0123456789abcdef01234567"
	)

	releaseDir := t.TempDir()
	for _, assetName := range []string{
		"lesser-linux-amd64",
		"lesser-linux-arm64",
		"lesser-darwin-amd64",
		"lesser-darwin-arm64",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(releaseDir, assetName), []byte(assetName), 0o755))
	}
	require.NoError(t, releaseassets.WriteAuthUIBundle(repoRoot, releaseDir))
	writeMinimalDeployAssembly(t, releaseDir, version, gitSHA)
	files, err := releaseassets.WriteLambdaBundle(repoRoot, releaseDir)
	require.NoError(t, err)
	_, err = releaseassets.WriteLambdaBundleManifest(releaseDir, version, gitSHA, files)
	require.NoError(t, err)
	_, err = releaseassets.WriteReleaseManifest(releaseDir, releaseassets.ReleaseManifestInput{
		Version:              version,
		GitSHA:               gitSHA,
		GoVersion:            "go1.26.1",
		CDKMajor:             2,
		ReceiptSchemaVersion: 7,
	})
	require.NoError(t, err)
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))

	return releaseDir
}

func testRepoWithCanonicalLambdaArtifacts(t *testing.T, files map[string]string) string {
	t.Helper()

	repoRoot := testRepoWithCanonicalInventory(t, []string{"api", "inbox"})
	binDir := filepath.Join(repoRoot, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name+".zip"), []byte(content), 0o644))
	}

	return repoRoot
}

func testRepoWithCanonicalInventory(t *testing.T, lambdas []string) string {
	t.Helper()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "auth-ui", "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "auth-ui", "package.json"), []byte("{\n  \"name\": \"auth-ui\"\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "auth-ui", "dist", "index.html"), []byte("<html/>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "cdk.json"), []byte("{\n  \"app\": \"go run main.go\"\n}\n"), 0o644))
	inventoryPath := filepath.Join(repoRoot, "infra", "cdk", "inventory")
	require.NoError(t, os.MkdirAll(inventoryPath, 0o755))

	content := "package inventory\n\nvar LambdaInventory = []struct{ Name string }{\n"
	for _, lambdaName := range lambdas {
		content += "\t{Name: \"" + lambdaName + "\"},\n"
	}
	content += "}\n"

	require.NoError(t, os.WriteFile(filepath.Join(inventoryPath, "lambdas.go"), []byte(content), 0o644))
	return repoRoot
}

func validReleaseManifest() releaseassets.ReleaseManifest {
	return releaseassets.ReleaseManifest{
		Schema:    1,
		Name:      "lesser",
		Version:   "v1.2.3",
		GitSHA:    "0123456789abcdef0123456789abcdef01234567",
		GoVersion: "go1.26.1",
		CDK: releaseassets.ReleaseCDK{
			Major: 2,
		},
		Artifacts: releaseassets.ReleaseArtifacts{
			ReceiptSchemaVersion: 7,
			DeployArtifacts: releaseassets.ReleaseDeployArtifacts{
				SchemaVersion: releaseassets.ReleaseDeployArtifactsSchemaVersion,
				LambdaBundle: releaseassets.ReleaseLambdaBundleRef{
					Path:                  releaseassets.LambdaBundleArchiveName,
					ManifestPath:          releaseassets.LambdaBundleManifestName,
					ManifestKind:          releaseassets.LambdaBundleManifestKind,
					ManifestSchemaVersion: releaseassets.LambdaBundleManifestSchemaVersion,
				},
				AuthUIBundle: releaseassets.ReleaseAuthUIBundleRef{
					Path:   releaseassets.AuthUIBundleArchiveName,
					Format: "tar.gz",
				},
				DeployAssembly: releaseassets.ReleaseDeployAssemblyRef{
					Path:                  releaseassets.DeployAssemblyArchiveName,
					ManifestPath:          releaseassets.DeployAssemblyManifestName,
					ManifestKind:          releaseassets.DeployAssemblyManifestKind,
					ManifestSchemaVersion: releaseassets.DeployAssemblyManifestSchemaVersion,
				},
			},
		},
	}
}

func writeMinimalDeployAssembly(t *testing.T, releaseDir, version, gitSHA string) {
	t.Helper()

	payload := releaseDeployAssemblyPayloadManifest{
		Kind:          releaseAssemblyPayloadManifestKind,
		SchemaVersion: releaseAssemblyPayloadManifestSchema,
		Release: releaseassets.LambdaBundleRelease{
			Name:    "lesser",
			Version: version,
			GitSHA:  gitSHA,
		},
		Stacks: []releaseDeployAssemblyStackManifest{
			{Name: string(naming.StageShared), TemplatePath: releaseAssemblySharedTemplatePath, SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
			{Name: string(naming.StageDev), Stage: string(naming.StageDev), TemplatePath: "templates/lesser-managed-dev.template.json", SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
			{Name: string(naming.StageStaging), Stage: string(naming.StageStaging), TemplatePath: "templates/lesser-managed-staging.template.json", SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
			{Name: string(naming.StageLive), Stage: string(naming.StageLive), TemplatePath: "templates/lesser-managed-live.template.json", SHA256: sha256HexString(`{"Parameters":{"AppSlug":{"Type":"String"}}}`)},
		},
		Assets: []releaseDeployAssemblyAssetManifest{},
	}

	templateBodies := map[string]string{
		releaseAssemblySharedTemplatePath:                `{"Parameters":{"AppSlug":{"Type":"String"}}}`,
		"templates/lesser-managed-dev.template.json":     `{"Parameters":{"AppSlug":{"Type":"String"}}}`,
		"templates/lesser-managed-staging.template.json": `{"Parameters":{"AppSlug":{"Type":"String"}}}`,
		"templates/lesser-managed-live.template.json":    `{"Parameters":{"AppSlug":{"Type":"String"}}}`,
	}

	archivePath := filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName)
	f, err := os.Create(archivePath)
	require.NoError(t, err)

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	writeEntry := func(name string, data []byte) {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(data)),
		}))
		_, copyErr := tw.Write(data)
		require.NoError(t, copyErr)
	}

	payloadData, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)
	payloadData = append(payloadData, '\n')
	writeEntry("manifest.json", payloadData)
	for _, name := range []string{
		releaseAssemblySharedTemplatePath,
		"templates/lesser-managed-dev.template.json",
		"templates/lesser-managed-staging.template.json",
		"templates/lesser-managed-live.template.json",
	} {
		writeEntry(name, []byte(templateBodies[name]))
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	descriptor := releaseassets.DeployAssemblyDescriptor{
		Kind:          releaseassets.DeployAssemblyManifestKind,
		SchemaVersion: releaseassets.DeployAssemblyManifestSchemaVersion,
		Release: releaseassets.LambdaBundleRelease{
			Name:    "lesser",
			Version: version,
			GitSHA:  gitSHA,
		},
		Assembly: releaseassets.DeployAssemblyAsset{
			Path:   releaseassets.DeployAssemblyArchiveName,
			Format: "tar.gz",
			SHA256: fileSHA256Hex(t, archivePath),
		},
		Payload: releaseassets.DeployAssemblyPayload{
			Kind:            "lesser.cloudformation_release_assembly",
			ContractVersion: 1,
			Entrypoint:      "manifest.json",
		},
		Compatibility: releaseassets.DeployAssemblyCompatibility{
			ReleaseManifestPath:     releaseassets.ReleaseManifestName,
			DeployArtifactsKey:      "deploy_assembly",
			ExecutorContractVersion: 1,
		},
		InstanceInputs: releaseassets.DeployAssemblyInstanceInputs{
			Required: []string{"app_identity", "aws_target", "base_domain", "hosted_zone", "stage_plan"},
			Optional: []string{"feature_config", "managed_service_urls", "provisioning_input", "bootstrap_io"},
		},
		Verification: releaseassets.DeployAssemblyVerification{
			IntegrityRequired: []string{"assembly.sha256", "checksums.txt"},
			PreflightRequired: []string{"instance_input_validation", "release_manifest_compatibility"},
		},
	}

	descriptorData, err := json.MarshalIndent(descriptor, "", "  ")
	require.NoError(t, err)
	descriptorData = append(descriptorData, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName), descriptorData, 0o644))
}

func writeAuthUIBundleFixture(t *testing.T, releaseDir string, entries map[string]string) {
	t.Helper()

	writeTarGzFixture(t, filepath.Join(releaseDir, releaseassets.AuthUIBundleArchiveName), entries)
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))
}

func writeDeployAssemblyFixture(
	t *testing.T,
	releaseDir string,
	payload releaseDeployAssemblyPayloadManifest,
	extraEntries map[string]string,
) {
	t.Helper()

	releaseManifest, err := readReleaseManifestFile(filepath.Join(releaseDir, releaseassets.ReleaseManifestName))
	require.NoError(t, err)

	entries := map[string]string{}
	payloadBytes, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)
	entries["manifest.json"] = string(append(payloadBytes, '\n'))

	for _, stack := range payload.Stacks {
		entries[stack.TemplatePath] = `{"Parameters":{"AppSlug":{"Type":"String"}}}`
	}
	for path, body := range extraEntries {
		entries[path] = body
	}

	archivePath := filepath.Join(releaseDir, releaseassets.DeployAssemblyArchiveName)
	writeTarGzFixture(t, archivePath, entries)

	descriptor := validDeployAssemblyDescriptor(releaseManifest)
	descriptor.Release.Version = releaseManifest.Version
	descriptor.Release.GitSHA = releaseManifest.GitSHA
	descriptor.Assembly.SHA256 = fileSHA256Hex(t, archivePath)

	descriptorBytes, err := json.MarshalIndent(descriptor, "", "  ")
	require.NoError(t, err)
	descriptorBytes = append(descriptorBytes, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, releaseassets.DeployAssemblyManifestName), descriptorBytes, 0o644))
	require.NoError(t, releaseassets.WriteChecksums(releaseDir))
}

func validBundleManifest() releaseassets.LambdaBundleManifest {
	return releaseassets.LambdaBundleManifest{
		Kind:          releaseassets.LambdaBundleManifestKind,
		SchemaVersion: releaseassets.LambdaBundleManifestSchemaVersion,
		Release: releaseassets.LambdaBundleRelease{
			Name:    "lesser",
			Version: "v1.2.3",
			GitSHA:  "0123456789abcdef0123456789abcdef01234567",
		},
		Bundle: releaseassets.LambdaBundleAsset{
			Path:   releaseassets.LambdaBundleArchiveName,
			Format: "tar.gz",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		InventorySource: releaseassets.LambdaBundleInventorySource{
			Path: releaseassets.LambdaInventoryPath,
			Kind: releaseassets.LambdaInventoryKind,
		},
		Files: []releaseassets.LambdaBundleManifestFile{
			{
				Path:      "bin/api.zip",
				Lambda:    "api",
				SHA256:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				SizeBytes: 7,
			},
			{
				Path:      "bin/inbox.zip",
				Lambda:    "inbox",
				SHA256:    "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				SizeBytes: 9,
			},
		},
	}
}

func writeBundleArchive(t *testing.T, entries map[string]string) string {
	t.Helper()

	bundlePath := filepath.Join(t.TempDir(), releaseassets.LambdaBundleArchiveName)
	writeTarGzFixture(t, bundlePath, entries)
	return bundlePath
}

func writeTarGzFixture(t *testing.T, path string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	for name, content := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
}

func fileSHA256Hex(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256HexString(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
