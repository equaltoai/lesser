package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

const testFaceTheoryDependency = "https://github.com/theory-cloud/FaceTheory/releases/download/" +
	"v3.2.2/theory-cloud-facetheory-3.2.2.tgz"

func testFaceTheoryPackageJSON(name string) string {
	return `{"name":"` + name + `","version":"1.2.3","dependencies":{"@theory-cloud/facetheory":"` +
		testFaceTheoryDependency + `"}}`
}

func TestParseClientInstallArgs_RequiresFlags(t *testing.T) {
	_, err := parseClientInstallArgs(nil)
	require.Error(t, err)

	_, err = parseClientInstallArgs([]string{"--app", "app"})
	require.Error(t, err)
}

func TestParseClientInstallArgs_AllowsAmbientCredentials(t *testing.T) {
	args, err := parseClientInstallArgs([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--stage", "dev",
		"--skip-build",
	})
	require.NoError(t, err)
	require.Empty(t, args.AWSProfile)
	require.Equal(t, "dev", args.Stage)
	require.True(t, args.SkipBuild)
}

func TestPrepareClientInstallPackage_BuildsManifest(t *testing.T) {
	appRoot := t.TempDir()
	serverDir := filepath.Join(appRoot, "build", "server")
	assetsDir := filepath.Join(appRoot, "build", "client")

	require.NoError(t, osMkdirAll(serverDir))
	require.NoError(t, osMkdirAll(filepath.Join(assetsDir, "_assets")))
	require.NoError(t, osWriteFile(filepath.Join(serverDir, "entry.mjs"), "export async function handler() {}", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(serverDir, "chunks", "dep.mjs"), "export const dep = true", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(assetsDir, "index.html"), "<!doctype html>", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(assetsDir, "_assets", "app.js"), "console.log('ok')", 0o644))

	cfg := &faceTheoryLesserConfig{
		SchemaVersion: clientInstallManifestSchemaVersion,
		AppName:       "demo-client",
		DisplayName:   "Demo Client",
		Version:       "1.2.3",
		Server: faceTheoryLesserServerConfig{
			Dir:   "build/server",
			Entry: "/entry.mjs",
		},
		Assets: faceTheoryLesserAssetsConfig{
			Dir: "build/client",
		},
	}
	pkg := &nodePackageJSON{Name: "@equaltoai/demo-client", Version: "9.9.9"}

	plan, err := prepareClientInstallPackage(appRoot, cfg, pkg)
	require.NoError(t, err)
	require.Equal(t, "demo-client", plan.AppName)
	require.Equal(t, "Demo Client", plan.DisplayName)
	require.Equal(t, "1.2.3", plan.Version)
	require.NotEmpty(t, plan.InstallID)
	require.Equal(t, filepath.Join(appRoot, "build", "server"), plan.ServerDir)
	require.Equal(t, filepath.Join(appRoot, "build", "client"), plan.AssetsDir)
	require.Equal(t, clientInstallManifestSchemaVersion, plan.Manifest.SchemaVersion)
	require.Equal(t, clientInstallManifestKind, plan.Manifest.Kind)
	require.Equal(t, clientInstallBasePath, plan.Manifest.BasePath)
	require.Equal(t, "entry.mjs", plan.Manifest.Server.Entry)
	require.Equal(t, "handler", plan.Manifest.Server.ExportName)
	require.Equal(t, []string{"chunks/dep.mjs", "entry.mjs"}, plan.Manifest.Server.Files)
	require.Equal(t, "/"+clientInstallAssetsRoot, plan.Manifest.Assets.PathPrefix)
	require.Equal(t, []string{"_assets/app.js", "index.html"}, plan.Manifest.Assets.Files)
}

func TestRunClientInstall_RecordsInstallInReceiptAndPublishesManifest(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	previousUploadDir := uploadDirWithPrefixFn
	previousPutObjectString := putObjectStringFn
	previousInvalidate := invalidateClientPathsFn
	previousWriteReceipt := writeReceiptFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWS
		uploadDirWithPrefixFn = previousUploadDir
		putObjectStringFn = previousPutObjectString
		invalidateClientPathsFn = previousInvalidate
		writeReceiptFn = previousWriteReceipt
	})

	appRoot := t.TempDir()
	serverDir := filepath.Join(appRoot, "dist", "server")
	assetsDir := filepath.Join(appRoot, "dist", "client")
	require.NoError(t, osMkdirAll(serverDir))
	require.NoError(t, osMkdirAll(filepath.Join(assetsDir, "_assets")))
	require.NoError(t, osWriteFile(filepath.Join(serverDir, "handler.mjs"), "export async function handler() {}", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(assetsDir, "index.html"), "<!doctype html>", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(assetsDir, "_assets", "app.js"), "console.log('ok')", 0o644))

	configPath := filepath.Join(appRoot, clientInstallConfigFileName)
	require.NoError(t, osWriteFile(configPath, strings.Join([]string{
		"{",
		`  "schema_version": 1,`,
		`  "app_name": "demo-client",`,
		`  "display_name": "Demo Client",`,
		`  "version": "1.2.3",`,
		`  "server": {"dir": "dist/server", "entry": "handler.mjs"},`,
		`  "assets": {"dir": "dist/client"}`,
		"}",
		"",
	}, "\n"), 0o644))
	require.NoError(t, osWriteFile(filepath.Join(appRoot, "package.json"), strings.Join([]string{
		"{",
		`  "name": "@equaltoai/demo-client",`,
		`  "version": "1.2.3",`,
		`  "dependencies": {"@theory-cloud/facetheory": "` + testFaceTheoryDependency + `"}`,
		"}",
		"",
	}, "\n"), 0o644))

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		nil,
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages = map[string]*stageReceipt{
		"dev": {
			StackName: "app-dev",
			StackOutputs: map[string]string{
				"ClientBucketName":         "dev-client",
				"ClientArtifactBucketName": "dev-client-artifacts",
				"ClientInstallManifestKey": "install/dev/current.json",
				"FrontendDistributionId":   "DIST123",
			},
		},
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	type uploadCall struct {
		Bucket string
		Prefix string
		Dir    string
	}
	type putCall struct {
		Bucket       string
		Key          string
		Content      string
		ContentType  string
		CacheControl string
	}

	var uploads []uploadCall
	var puts []putCall
	var invalidations []string
	var wroteReceipt *upReceipt

	loadAWSConfigForCLIFn = func(_ context.Context, profile string) (aws.Config, string, error) {
		require.Equal(t, "profile", profile)
		return aws.Config{Region: "us-east-1"}, profile, nil
	}
	uploadDirWithPrefixFn = func(_ context.Context, _ s3PutObjectAPI, bucket, prefix, dir string) error {
		uploads = append(uploads, uploadCall{Bucket: bucket, Prefix: prefix, Dir: dir})
		return nil
	}
	putObjectStringFn = func(_ context.Context, _ s3PutObjectAPI, bucket, key, content, contentType, cacheControl string) error {
		puts = append(puts, putCall{
			Bucket:       bucket,
			Key:          key,
			Content:      content,
			ContentType:  contentType,
			CacheControl: cacheControl,
		})
		return nil
	}
	invalidateClientPathsFn = func(_ context.Context, _ *cloudfront.Client, distID string) error {
		invalidations = append(invalidations, distID)
		return nil
	}
	writeReceiptFn = func(path string, next *upReceipt) error {
		require.Equal(t, statePath, path)
		wroteReceipt = next
		return nil
	}

	err := runClientInstall([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--stage", "dev",
		"--state", statePath,
		"--config", configPath,
		"--skip-build",
	})
	require.NoError(t, err)
	require.Len(t, uploads, 2)
	require.Equal(t, "dev-client-artifacts", uploads[0].Bucket)
	require.Equal(t, filepath.Join(appRoot, "dist", "server"), uploads[0].Dir)
	require.Equal(t, "dev-client", uploads[1].Bucket)
	require.Equal(t, clientInstallAssetsRoot, uploads[1].Prefix)
	require.Equal(t, filepath.Join(appRoot, "dist", "client"), uploads[1].Dir)
	require.Equal(t, []string{"DIST123"}, invalidations)
	require.Len(t, puts, 2)

	var manifest clientInstallManifest
	require.NoError(t, json.Unmarshal([]byte(puts[0].Content), &manifest))
	require.Equal(t, clientInstallManifestKind, manifest.Kind)
	require.Equal(t, clientInstallBasePath, manifest.BasePath)
	require.Equal(t, "demo-client", manifest.AppName)
	require.Equal(t, "Demo Client", manifest.DisplayName)
	require.Equal(t, "1.2.3", manifest.Version)
	require.Equal(t, "handler.mjs", manifest.Server.Entry)
	require.Equal(t, "handler", manifest.Server.ExportName)
	require.Equal(t, clientInstallAssetsRoot, manifest.Assets.Root)
	require.Equal(t, "/"+clientInstallAssetsRoot, manifest.Assets.PathPrefix)
	require.Equal(t, []string{"_assets/app.js", "index.html"}, manifest.Assets.Files)
	require.Equal(t, "application/json; charset=utf-8", puts[0].ContentType)
	require.Equal(t, "no-store", puts[0].CacheControl)
	require.Equal(t, "dev-client-artifacts", puts[0].Bucket)
	require.Equal(t, filepath.ToSlash(filepath.Join(clientInstallHistoryRoot, manifest.InstallID, "manifest.json")), puts[0].Key)
	require.Equal(t, "install/dev/current.json", puts[1].Key)

	require.NotNil(t, wroteReceipt)
	install := wroteReceipt.Stages["dev"].ClientInstall
	require.NotNil(t, install)
	require.Equal(t, "demo-client", install.AppName)
	require.Equal(t, "Demo Client", install.DisplayName)
	require.Equal(t, "1.2.3", install.Version)
	require.Equal(t, manifest.InstallID, install.InstallID)
	require.Equal(t, "install/dev/current.json", install.ManifestKey)
	require.Equal(t, filepath.ToSlash(filepath.Join(clientInstallHistoryRoot, manifest.InstallID, "server")), install.ServerRoot)
	require.Equal(t, clientInstallAssetsRoot, install.AssetsRoot)
	require.False(t, install.InstalledAt.IsZero())
}

func TestRunClientInstall_PropagatesErrors(t *testing.T) {
	t.Run("parse args", func(t *testing.T) {
		err := runClientInstall(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "required flags")
	})

	t.Run("build command", func(t *testing.T) {
		err := runClientInstall([]string{
			"--app", "bad_app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
		})
		require.Error(t, err)
	})

	t.Run("publish stage", func(t *testing.T) {
		previousLoadAWS := loadAWSConfigForCLIFn
		previousUploadDir := uploadDirWithPrefixFn
		previousPutObjectString := putObjectStringFn
		previousInvalidate := invalidateClientPathsFn
		previousWriteReceipt := writeReceiptFn
		t.Cleanup(func() {
			loadAWSConfigForCLIFn = previousLoadAWS
			uploadDirWithPrefixFn = previousUploadDir
			putObjectStringFn = previousPutObjectString
			invalidateClientPathsFn = previousInvalidate
			writeReceiptFn = previousWriteReceipt
		})

		appRoot, configPath, statePath := writeClientInstallFixture(t)
		_ = appRoot
		loadAWSConfigForCLIFn = func(_ context.Context, profile string) (aws.Config, string, error) {
			return aws.Config{Region: "us-east-1"}, profile, nil
		}
		uploadDirWithPrefixFn = func(context.Context, s3PutObjectAPI, string, string, string) error {
			return errSentinel
		}
		putObjectStringFn = func(context.Context, s3PutObjectAPI, string, string, string, string, string) error { return nil }
		invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return nil }
		writeReceiptFn = func(string, *upReceipt) error { return nil }

		err := runClientInstall([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--stage", "dev",
			"--state", statePath,
			"--config", configPath,
			"--skip-build",
		})
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "upload SSR server bundle")
	})
}

func TestResolveClientInstallRoot_SearchesUpwardAndValidatesExplicitPath(t *testing.T) {
	previousWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(previousWD) })

	root := t.TempDir()
	configPath := filepath.Join(root, clientInstallConfigFileName)
	require.NoError(t, osWriteFile(configPath, `{"schema_version":1,"server":{"dir":"server","entry":"handler.mjs"},"assets":{"dir":"client"}}`, 0o644))

	nested := filepath.Join(root, "apps", "demo")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.Chdir(nested))

	appRoot, resolvedConfig, err := resolveClientInstallRoot("")
	require.NoError(t, err)
	require.Equal(t, root, appRoot)
	require.Equal(t, configPath, resolvedConfig)

	_, _, err = resolveClientInstallRoot(filepath.Join(root, "missing.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "client install config not found")

	empty := t.TempDir()
	require.NoError(t, os.Chdir(empty))
	_, _, err = resolveClientInstallRoot("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to locate")
}

func TestNewClientInstallCommand_PropagatesErrors(t *testing.T) {
	t.Run("input normalization", func(t *testing.T) {
		_, err := newClientInstallCommand(clientInstallArgs{
			App:        "bad_app",
			BaseDomain: "example.com",
			AWSProfile: "profile",
		})
		require.Error(t, err)
	})

	t.Run("artifact preparation", func(t *testing.T) {
		_, err := newClientInstallCommand(clientInstallArgs{
			App:        "app",
			BaseDomain: "example.com",
			AWSProfile: "profile",
			ConfigPath: filepath.Join(t.TempDir(), "missing.json"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "client install config not found")
	})

	t.Run("receipt resolution", func(t *testing.T) {
		appRoot, configPath, _ := writeClientInstallFixture(t)
		_ = appRoot
		_, err := newClientInstallCommand(clientInstallArgs{
			App:        "app",
			BaseDomain: "example.com",
			AWSProfile: "profile",
			ConfigPath: configPath,
			StatePath:  filepath.Join(t.TempDir(), "missing-state.json"),
			SkipBuild:  true,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "deployment receipt not found")
	})

	t.Run("aws client creation", func(t *testing.T) {
		previousLoadAWS := loadAWSConfigForCLIFn
		t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWS })

		appRoot, configPath, statePath := writeClientInstallFixture(t)
		_ = appRoot
		loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
			return aws.Config{}, "", errSentinel
		}

		_, err := newClientInstallCommand(clientInstallArgs{
			App:        "app",
			BaseDomain: "example.com",
			AWSProfile: "profile",
			ConfigPath: configPath,
			StatePath:  statePath,
			SkipBuild:  true,
		})
		require.ErrorIs(t, err, errSentinel)
	})
}

func TestPrepareClientInstallArtifacts_RunsInstallAndBuild(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	appRoot := t.TempDir()
	configPath := filepath.Join(appRoot, clientInstallConfigFileName)
	require.NoError(t, osWriteFile(configPath, strings.Join([]string{
		"{",
		`  "schema_version": 1,`,
		`  "build": {"command": ["pnpm", "build:ssr"]},`,
		`  "server": {"dir": "dist/server", "entry": "handler.mjs"},`,
		`  "assets": {"dir": "dist/client"}`,
		"}",
		"",
	}, "\n"), 0o644))
	require.NoError(t, osWriteFile(filepath.Join(appRoot, "package.json"), strings.Join([]string{
		"{",
		`  "name": "@equaltoai/demo-client",`,
		`  "version": "1.2.3",`,
		`  "dependencies": {"@theory-cloud/facetheory": "` + testFaceTheoryDependency + `"}`,
		"}",
		"",
	}, "\n"), 0o644))
	require.NoError(t, osWriteFile(filepath.Join(appRoot, "dist", "server", "handler.mjs"), "export async function handler() {}", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(appRoot, "dist", "client", "index.html"), "<!doctype html>", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(appRoot, "dist", "client", "_assets", "app.js"), "console.log('ok')", 0o644))

	type commandCall struct {
		name string
		args []string
		dir  string
	}
	var calls []commandCall
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		calls = append(calls, commandCall{name: name, args: append([]string(nil), args...), dir: opts.Dir})
		return nil
	}

	appRootResolved, resolvedConfig, plan, err := prepareClientInstallArtifacts(clientInstallArgs{
		ConfigPath: configPath,
	})
	require.NoError(t, err)
	require.Equal(t, appRoot, appRootResolved)
	require.Equal(t, configPath, resolvedConfig)
	require.NotNil(t, plan)
	require.Equal(t, []commandCall{
		{name: "pnpm", args: []string{"install", "--frozen-lockfile"}, dir: appRoot},
		{name: "pnpm", args: []string{"build:ssr"}, dir: appRoot},
	}, calls)
}

func TestNormalizeClientInstallInputs_ValidatesAndTrims(t *testing.T) {
	_, _, _, err := normalizeClientInstallInputs(clientInstallArgs{
		App:        "bad_app",
		BaseDomain: "example.com",
		AWSProfile: "profile",
	})
	require.Error(t, err)

	_, _, _, err = normalizeClientInstallInputs(clientInstallArgs{
		App:        "demo-app",
		BaseDomain: "https://example.com",
		AWSProfile: "profile",
	})
	require.Error(t, err)

	app, baseDomain, awsProfile, err := normalizeClientInstallInputs(clientInstallArgs{
		App:        "demo-app",
		BaseDomain: "Example.COM.",
		AWSProfile: "   ",
	})
	require.NoError(t, err)
	require.Equal(t, "demo-app", app)
	require.Equal(t, "example.com", baseDomain)
	require.Empty(t, awsProfile)

	app, baseDomain, awsProfile, err = normalizeClientInstallInputs(clientInstallArgs{
		App:        " Demo-App ",
		BaseDomain: " Example.COM. ",
		AWSProfile: " profile ",
	})
	require.NoError(t, err)
	require.Equal(t, "demo-app", app)
	require.Equal(t, "example.com", baseDomain)
	require.Equal(t, "profile", awsProfile)
}

func TestClientInstallValidationHelpers(t *testing.T) {
	t.Run("read config validates schema and required fields", func(t *testing.T) {
		root := t.TempDir()
		invalidSchema := filepath.Join(root, "invalid-schema.json")
		require.NoError(t, osWriteFile(invalidSchema, `{"schema_version":2,"server":{"dir":"server","entry":"handler.mjs"},"assets":{"dir":"client"}}`, 0o644))
		_, err := readFaceTheoryLesserConfig(invalidSchema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported client install config schema_version")

		missingFields := filepath.Join(root, "missing-fields.json")
		require.NoError(t, osWriteFile(missingFields, `{"schema_version":1,"server":{"dir":"","entry":""},"assets":{"dir":""}}`, 0o644))
		_, err = readFaceTheoryLesserConfig(missingFields)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires server.dir and server.entry")

		missingAssets := filepath.Join(root, "missing-assets.json")
		require.NoError(t, osWriteFile(missingAssets, `{"schema_version":1,"server":{"dir":"server","entry":"handler.mjs"},"assets":{"dir":""}}`, 0o644))
		_, err = readFaceTheoryLesserConfig(missingAssets)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires assets.dir")
	})

	t.Run("package validation accepts dev dependency and rejects missing package", func(t *testing.T) {
		require.Error(t, validateFaceTheoryPackage(nil))
		require.NoError(t, validateFaceTheoryPackage(&nodePackageJSON{
			DevDependencies: map[string]string{"@theory-cloud/facetheory": testFaceTheoryDependency},
		}))
		require.Error(t, validateFaceTheoryPackage(&nodePackageJSON{}))
		require.True(t, hasNodeDependency(map[string]string{"x": "1"}, "x"))
		require.False(t, hasNodeDependency(nil, "x"))
	})

	t.Run("read package json surfaces parse errors", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "package.json")
		require.NoError(t, osWriteFile(path, "{not-json}", 0o644))
		_, err := readNodePackageJSON(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse package.json")
	})
}

func TestClientInstallFileReaders_PropagateReadErrors(t *testing.T) {
	_, err := readFaceTheoryLesserConfig(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "read client install config")

	_, err = readNodePackageJSON(filepath.Join(t.TempDir(), "missing-package.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "read package.json")
}

func TestPrepareClientInstallArtifacts_PropagatesErrors(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	t.Run("missing config path", func(t *testing.T) {
		_, _, _, err := prepareClientInstallArtifacts(clientInstallArgs{
			ConfigPath: filepath.Join(t.TempDir(), "missing.json"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "client install config not found")
	})

	t.Run("missing package.json", func(t *testing.T) {
		appRoot := t.TempDir()
		configPath := filepath.Join(appRoot, clientInstallConfigFileName)
		require.NoError(t, osWriteFile(configPath, `{"schema_version":1,"server":{"dir":"dist/server","entry":"handler.mjs"},"assets":{"dir":"dist/client"}}`, 0o644))

		_, _, _, err := prepareClientInstallArtifacts(clientInstallArgs{
			ConfigPath: configPath,
			SkipBuild:  true,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "read package.json")
	})

	t.Run("missing facetheory dependency", func(t *testing.T) {
		appRoot := t.TempDir()
		configPath := filepath.Join(appRoot, clientInstallConfigFileName)
		require.NoError(t, osWriteFile(configPath, `{"schema_version":1,"server":{"dir":"dist/server","entry":"handler.mjs"},"assets":{"dir":"dist/client"}}`, 0o644))
		require.NoError(t, osWriteFile(filepath.Join(appRoot, "package.json"), `{"name":"demo-client","version":"1.2.3"}`, 0o644))

		_, _, _, err := prepareClientInstallArtifacts(clientInstallArgs{
			ConfigPath: configPath,
			SkipBuild:  true,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "@theory-cloud/facetheory")
	})

	t.Run("install failure", func(t *testing.T) {
		appRoot := t.TempDir()
		configPath := filepath.Join(appRoot, clientInstallConfigFileName)
		require.NoError(t, osWriteFile(configPath, `{"schema_version":1,"server":{"dir":"dist/server","entry":"handler.mjs"},"assets":{"dir":"dist/client"}}`, 0o644))
		require.NoError(t, osWriteFile(filepath.Join(appRoot, "package.json"), testFaceTheoryPackageJSON("demo-client"), 0o644))
		runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }

		_, _, _, err := prepareClientInstallArtifacts(clientInstallArgs{
			ConfigPath: configPath,
		})
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "pnpm install (client app)")
	})

	t.Run("build failure", func(t *testing.T) {
		appRoot := t.TempDir()
		configPath := filepath.Join(appRoot, clientInstallConfigFileName)
		require.NoError(t, osWriteFile(configPath, `{"schema_version":1,"server":{"dir":"dist/server","entry":"handler.mjs"},"assets":{"dir":"dist/client"}}`, 0o644))
		require.NoError(t, osWriteFile(filepath.Join(appRoot, "package.json"), testFaceTheoryPackageJSON("demo-client"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(appRoot, "node_modules"), 0o755))
		runCommandFn = func(context.Context, string, []string, execOptions) error { return errSentinel }

		_, _, _, err := prepareClientInstallArtifacts(clientInstallArgs{
			ConfigPath: configPath,
		})
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "build FaceTheory app")
	})
}

func TestResolveClientInstallStageTargets_DefaultsAndErrors(t *testing.T) {
	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		nil,
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages = map[string]*stageReceipt{
		"dev": {
			StackName: "app-dev",
			StackOutputs: map[string]string{
				"FrontendDistributionId": "DIST123",
			},
		},
	}

	targets, err := resolveClientInstallStageTargets(receipt, naming.StageDev)
	require.NoError(t, err)
	require.Equal(t, "dev", targets.stageKey)
	require.Equal(t, "app-dev-client-123456789012-us-east-1", targets.assetBucket)
	require.Equal(t, "app-dev-client-artifacts-123456789012-us-east-1", targets.artifactBucket)
	require.Equal(t, clientInstallDefaultManifestKey, targets.manifestKey)
	require.Equal(t, "DIST123", targets.distributionID)

	receipt.Stages["live"] = &stageReceipt{StackName: "app-live", StackOutputs: map[string]string{}}
	_, err = resolveClientInstallStageTargets(receipt, naming.StageLive)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing FrontendDistributionId")

	delete(receipt.Stages, "dev")
	_, err = resolveClientInstallStageTargets(receipt, naming.StageDev)
	require.Error(t, err)
	require.Contains(t, err.Error(), `receipt missing stage "dev"`)
}

func TestPublishClientInstallStages_StopsOnError(t *testing.T) {
	previousUploadDir := uploadDirWithPrefixFn
	t.Cleanup(func() { uploadDirWithPrefixFn = previousUploadDir })

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		nil,
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages = map[string]*stageReceipt{
		"dev": {
			StackName: "app-dev",
			StackOutputs: map[string]string{
				"ClientBucketName":         "dev-client",
				"ClientArtifactBucketName": "dev-client-artifacts",
				"FrontendDistributionId":   "DIST123",
			},
		},
		"live": {
			StackName: "app-live",
			StackOutputs: map[string]string{
				"ClientBucketName":         "live-client",
				"ClientArtifactBucketName": "live-client-artifacts",
				"FrontendDistributionId":   "DIST456",
			},
		},
	}

	var calls []string
	uploadDirWithPrefixFn = func(_ context.Context, _ s3PutObjectAPI, bucket, prefix, dir string) error {
		calls = append(calls, bucket+":"+prefix+":"+dir)
		return errSentinel
	}

	err := publishClientInstallStages(context.Background(), &clientInstallCommand{
		receipt:            receipt,
		stages:             []naming.Stage{naming.StageDev, naming.StageLive},
		serverRoot:         "installs/demo/server",
		historyManifestKey: "installs/demo/manifest.json",
		manifestContent:    "{}\n",
		pkgPlan: &clientInstallPackage{
			ServerDir:   "/tmp/server",
			AssetsDir:   "/tmp/assets",
			AppName:     "demo-client",
			DisplayName: "Demo Client",
			Version:     "1.2.3",
			InstallID:   "demo",
		},
		s3Client: &s3.Client{},
		cfClient: &cloudfront.Client{},
	})
	require.ErrorIs(t, err, errSentinel)
	require.Len(t, calls, 1)
	require.Contains(t, err.Error(), "upload SSR server bundle (dev)")
}

func TestPrepareClientInstallPackage_DefaultsAndErrors(t *testing.T) {
	t.Run("falls back metadata defaults", func(t *testing.T) {
		appRoot := t.TempDir()
		serverDir := filepath.Join(appRoot, "build", "server")
		assetsDir := filepath.Join(appRoot, "build", "client")

		require.NoError(t, osMkdirAll(serverDir))
		require.NoError(t, osMkdirAll(assetsDir))
		require.NoError(t, osWriteFile(filepath.Join(serverDir, "handler.mjs"), "export async function handler() {}", 0o644))
		require.NoError(t, osWriteFile(filepath.Join(assetsDir, "index.html"), "<!doctype html>", 0o644))

		plan, err := prepareClientInstallPackage(appRoot, &faceTheoryLesserConfig{
			SchemaVersion: clientInstallManifestSchemaVersion,
			Server: faceTheoryLesserServerConfig{
				Dir:   "build/server",
				Entry: "handler.mjs",
			},
			Assets: faceTheoryLesserAssetsConfig{
				Dir: "build/client",
			},
		}, &nodePackageJSON{})
		require.NoError(t, err)
		require.Equal(t, "facetheory-client", plan.AppName)
		require.Equal(t, "facetheory-client", plan.DisplayName)
		require.Equal(t, "0.0.0", plan.Version)
	})

	t.Run("surfaces missing server entry", func(t *testing.T) {
		appRoot := t.TempDir()
		require.NoError(t, osMkdirAll(filepath.Join(appRoot, "build", "server")))
		require.NoError(t, osWriteFile(filepath.Join(appRoot, "build", "client", "index.html"), "<!doctype html>", 0o644))

		_, err := prepareClientInstallPackage(appRoot, &faceTheoryLesserConfig{
			SchemaVersion: clientInstallManifestSchemaVersion,
			Server: faceTheoryLesserServerConfig{
				Dir:   "build/server",
				Entry: "handler.mjs",
			},
			Assets: faceTheoryLesserAssetsConfig{
				Dir: "build/client",
			},
		}, &nodePackageJSON{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "server entry is missing")
	})

	t.Run("surfaces empty asset directory", func(t *testing.T) {
		appRoot := t.TempDir()
		require.NoError(t, osWriteFile(filepath.Join(appRoot, "build", "server", "handler.mjs"), "export async function handler() {}", 0o644))
		require.NoError(t, osMkdirAll(filepath.Join(appRoot, "build", "client")))

		_, err := prepareClientInstallPackage(appRoot, &faceTheoryLesserConfig{
			SchemaVersion: clientInstallManifestSchemaVersion,
			Server: faceTheoryLesserServerConfig{
				Dir:   "build/server",
				Entry: "handler.mjs",
			},
			Assets: faceTheoryLesserAssetsConfig{
				Dir: "build/client",
			},
		}, &nodePackageJSON{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "asset directory")
	})
}

func TestPublishClientInstallStage_PropagatesUploadFailure(t *testing.T) {
	previousUploadDir := uploadDirWithPrefixFn
	t.Cleanup(func() { uploadDirWithPrefixFn = previousUploadDir })

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		nil,
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages = map[string]*stageReceipt{
		"dev": {
			StackName: "app-dev",
			StackOutputs: map[string]string{
				"ClientBucketName":         "dev-client",
				"ClientArtifactBucketName": "dev-client-artifacts",
				"FrontendDistributionId":   "DIST123",
			},
		},
	}

	uploadDirWithPrefixFn = func(context.Context, s3PutObjectAPI, string, string, string) error {
		return errSentinel
	}

	command := &clientInstallCommand{
		serverRoot:         "installs/demo/server",
		historyManifestKey: "installs/demo/manifest.json",
		manifestContent:    "{}\n",
		receipt:            receipt,
		pkgPlan: &clientInstallPackage{
			ServerDir:   "/tmp/server",
			AssetsDir:   "/tmp/assets",
			AppName:     "demo-client",
			DisplayName: "Demo Client",
			Version:     "1.2.3",
			InstallID:   "demo",
		},
		s3Client: &s3.Client{},
		cfClient: &cloudfront.Client{},
	}

	err := publishClientInstallStage(context.Background(), command, naming.StageDev)
	require.ErrorIs(t, err, errSentinel)
	require.Contains(t, err.Error(), "upload SSR server bundle (dev)")
}

func TestInstallNodeModulesIfNeeded_SkipsExistingDirectory(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	appRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(appRoot, "node_modules"), 0o755))
	runCommandFn = func(context.Context, string, []string, execOptions) error {
		t.Fatal("runCommandFn should not be called when node_modules already exists")
		return nil
	}

	require.NoError(t, installNodeModulesIfNeeded(appRoot))
}

func TestInstallNodeModulesIfNeeded_WrapsInstallError(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	appRoot := t.TempDir()
	runCommandFn = func(context.Context, string, []string, execOptions) error {
		return errSentinel
	}

	err := installNodeModulesIfNeeded(appRoot)
	require.ErrorIs(t, err, errSentinel)
	require.Contains(t, err.Error(), "pnpm install (client app)")
}

func TestRunFaceTheoryBuild_UsesDefaultCommand(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	var calledName string
	var calledArgs []string
	var calledDir string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		calledName = name
		calledArgs = append([]string(nil), args...)
		calledDir = opts.Dir
		return nil
	}

	require.NoError(t, runFaceTheoryBuild(context.Background(), "/tmp/demo", nil))
	require.Equal(t, "pnpm", calledName)
	require.Equal(t, []string{"build"}, calledArgs)
	require.Equal(t, "/tmp/demo", calledDir)
}

func TestRunFaceTheoryBuild_WrapsBuildError(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		return errSentinel
	}

	err := runFaceTheoryBuild(context.Background(), "/tmp/demo", &faceTheoryLesserConfig{
		Build: &faceTheoryLesserBuildConfig{Command: []string{"pnpm", "build:ssr"}},
	})
	require.ErrorIs(t, err, errSentinel)
	require.Contains(t, err.Error(), "build FaceTheory app")
}

func TestResolveClientInstallReceipt_ErrorsOnInvalidStage(t *testing.T) {
	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		nil,
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	_, _, _, err := resolveClientInstallReceipt("app", "example.com", clientInstallArgs{
		StatePath: statePath,
		Stage:     "nope",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --stage")
}

func TestNewClientInstallAWSClients_PropagatesLoadError(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWS })

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{}, "", errSentinel
	}

	_, _, err := newClientInstallAWSClients("profile")
	require.ErrorIs(t, err, errSentinel)
}

func TestNewClientInstallAWSClients_UsesAmbientCredentialChain(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWS })

	loadAWSConfigForCLIFn = func(_ context.Context, profile string) (aws.Config, string, error) {
		require.Empty(t, profile)
		return aws.Config{Region: "us-east-1"}, "", nil
	}

	s3Client, cfClient, err := newClientInstallAWSClients("")
	require.NoError(t, err)
	require.NotNil(t, s3Client)
	require.NotNil(t, cfClient)
}

func TestPublishClientInstallStage_PropagatesManifestAndInvalidationErrors(t *testing.T) {
	previousUploadDir := uploadDirWithPrefixFn
	previousPutObjectString := putObjectStringFn
	previousInvalidate := invalidateClientPathsFn
	t.Cleanup(func() {
		uploadDirWithPrefixFn = previousUploadDir
		putObjectStringFn = previousPutObjectString
		invalidateClientPathsFn = previousInvalidate
	})

	newCommand := func() *clientInstallCommand {
		receipt := newUpReceipt(
			"app",
			"example.com",
			"profile",
			"123456789012",
			"us-east-1",
			nil,
			hostedZone{ID: "Z1", Name: "example.com"},
		)
		receipt.Stages = map[string]*stageReceipt{
			"dev": {
				StackName: "app-dev",
				StackOutputs: map[string]string{
					"ClientBucketName":         "dev-client",
					"ClientArtifactBucketName": "dev-client-artifacts",
					"FrontendDistributionId":   "DIST123",
				},
			},
		}
		return &clientInstallCommand{
			serverRoot:         "installs/demo/server",
			historyManifestKey: "installs/demo/manifest.json",
			manifestContent:    "{}\n",
			receipt:            receipt,
			pkgPlan: &clientInstallPackage{
				ServerDir:   "/tmp/server",
				AssetsDir:   "/tmp/assets",
				AppName:     "demo-client",
				DisplayName: "Demo Client",
				Version:     "1.2.3",
				InstallID:   "demo",
			},
			s3Client: &s3.Client{},
			cfClient: &cloudfront.Client{},
		}
	}

	t.Run("manifest upload failure", func(t *testing.T) {
		uploadDirWithPrefixFn = func(context.Context, s3PutObjectAPI, string, string, string) error { return nil }
		putObjectStringFn = func(context.Context, s3PutObjectAPI, string, string, string, string, string) error {
			return errSentinel
		}
		invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return nil }

		err := publishClientInstallStage(context.Background(), newCommand(), naming.StageDev)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "upload install manifest history (dev)")
	})

	t.Run("cloudfront invalidation failure", func(t *testing.T) {
		uploadDirWithPrefixFn = func(context.Context, s3PutObjectAPI, string, string, string) error { return nil }
		putObjectStringFn = func(context.Context, s3PutObjectAPI, string, string, string, string, string) error { return nil }
		invalidateClientPathsFn = func(context.Context, *cloudfront.Client, string) error { return errSentinel }

		err := publishClientInstallStage(context.Background(), newCommand(), naming.StageDev)
		require.ErrorIs(t, err, errSentinel)
		require.Contains(t, err.Error(), "cloudfront invalidation (dev)")
	})
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func osWriteFile(path string, data string, perm uint32) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), os.FileMode(perm))
}

func writeClientInstallFixture(t *testing.T) (string, string, string) {
	t.Helper()

	appRoot := t.TempDir()
	serverDir := filepath.Join(appRoot, "dist", "server")
	assetsDir := filepath.Join(appRoot, "dist", "client")
	require.NoError(t, osMkdirAll(serverDir))
	require.NoError(t, osMkdirAll(filepath.Join(assetsDir, "_assets")))
	require.NoError(t, osWriteFile(filepath.Join(serverDir, "handler.mjs"), "export async function handler() {}", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(assetsDir, "index.html"), "<!doctype html>", 0o644))
	require.NoError(t, osWriteFile(filepath.Join(assetsDir, "_assets", "app.js"), "console.log('ok')", 0o644))

	configPath := filepath.Join(appRoot, clientInstallConfigFileName)
	require.NoError(t, osWriteFile(configPath, strings.Join([]string{
		"{",
		`  "schema_version": 1,`,
		`  "app_name": "demo-client",`,
		`  "display_name": "Demo Client",`,
		`  "version": "1.2.3",`,
		`  "server": {"dir": "dist/server", "entry": "handler.mjs"},`,
		`  "assets": {"dir": "dist/client"}`,
		"}",
		"",
	}, "\n"), 0o644))
	require.NoError(t, osWriteFile(filepath.Join(appRoot, "package.json"), strings.Join([]string{
		"{",
		`  "name": "@equaltoai/demo-client",`,
		`  "version": "1.2.3",`,
		`  "dependencies": {"@theory-cloud/facetheory": "` + testFaceTheoryDependency + `"}`,
		"}",
		"",
	}, "\n"), 0o644))

	receipt := newUpReceipt(
		"app",
		"example.com",
		"profile",
		"123456789012",
		"us-east-1",
		nil,
		hostedZone{ID: "Z1", Name: "example.com"},
	)
	receipt.Stages = map[string]*stageReceipt{
		"dev": {
			StackName: "app-dev",
			StackOutputs: map[string]string{
				"ClientBucketName":         "dev-client",
				"ClientArtifactBucketName": "dev-client-artifacts",
				"ClientInstallManifestKey": "install/dev/current.json",
				"FrontendDistributionId":   "DIST123",
			},
		},
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, writeReceipt(statePath, receipt))

	return appRoot, configPath, statePath
}
