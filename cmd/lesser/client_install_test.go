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
	"github.com/stretchr/testify/require"
)

func TestParseClientInstallArgs_RequiresFlags(t *testing.T) {
	_, err := parseClientInstallArgs(nil)
	require.Error(t, err)

	_, err = parseClientInstallArgs([]string{"--app", "app"})
	require.Error(t, err)
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
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousUploadDir := uploadDirWithPrefixFn
	previousPutObjectString := putObjectStringFn
	previousInvalidate := invalidateClientPathsFn
	previousWriteReceipt := writeReceiptFn
	t.Cleanup(func() {
		loadAWSConfigFromProfileFn = previousLoadAWS
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
		`  "dependencies": {"@theory-cloud/facetheory": "^0.1.0"}`,
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

	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
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
