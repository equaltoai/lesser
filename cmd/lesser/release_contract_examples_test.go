package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type lambdaBundleManifestExample struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	Release       struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		GitSHA  string `json:"git_sha"`
	} `json:"release"`
	Bundle struct {
		Path   string `json:"path"`
		Format string `json:"format"`
		SHA256 string `json:"sha256"`
	} `json:"bundle"`
	InventorySource struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"inventory_source"`
	Files []struct {
		Path      string `json:"path"`
		Lambda    string `json:"lambda"`
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"size_bytes"`
	} `json:"files"`
}

type deployAssemblyDescriptorExample struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	Release       struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		GitSHA  string `json:"git_sha"`
	} `json:"release"`
	Assembly struct {
		Path   string `json:"path"`
		Format string `json:"format"`
		SHA256 string `json:"sha256"`
	} `json:"assembly"`
	Payload struct {
		Kind            string `json:"kind"`
		ContractVersion int    `json:"contract_version"`
		Entrypoint      string `json:"entrypoint"`
	} `json:"payload"`
	Compatibility struct {
		ReleaseManifestPath   string `json:"release_manifest_path"`
		DeployArtifactsKey    string `json:"deploy_artifacts_key"`
		ExecutorContractValue int    `json:"executor_contract_version"`
	} `json:"compatibility"`
	InstanceInputs struct {
		Required []string `json:"required"`
		Optional []string `json:"optional"`
	} `json:"instance_inputs"`
	Verification struct {
		IntegrityRequired []string `json:"integrity_required"`
		PreflightRequired []string `json:"preflight_required"`
	} `json:"verification"`
}

func TestLambdaBundleManifestExampleContract(t *testing.T) {
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "contracts", "examples", "lesser-lambda-bundle.example.json"))
	require.NoError(t, err)

	var manifest lambdaBundleManifestExample
	require.NoError(t, json.Unmarshal(data, &manifest))

	require.Equal(t, "lesser.lambda_bundle_manifest", manifest.Kind)
	require.Equal(t, 1, manifest.SchemaVersion)
	require.Equal(t, "lesser", manifest.Release.Name)
	require.Regexp(t, regexp.MustCompile(`^v[0-9].*$`), manifest.Release.Version)
	require.Regexp(t, regexp.MustCompile(`^[a-f0-9]{40}$`), manifest.Release.GitSHA)
	require.Equal(t, "lesser-lambda-bundle.tar.gz", manifest.Bundle.Path)
	require.Equal(t, "tar.gz", manifest.Bundle.Format)
	require.Regexp(t, regexp.MustCompile(`^[a-f0-9]{64}$`), manifest.Bundle.SHA256)
	require.Equal(t, "infra/cdk/inventory/lambdas.go", manifest.InventorySource.Path)
	require.Equal(t, "lesser.lambda_inventory", manifest.InventorySource.Kind)
	require.NotEmpty(t, manifest.Files)

	require.True(t, sort.SliceIsSorted(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	}))

	seen := make(map[string]struct{}, len(manifest.Files))
	sha256Pattern := regexp.MustCompile(`^[a-f0-9]{64}$`)
	pathPattern := regexp.MustCompile(`^bin/[a-z0-9-]+\.zip$`)
	for _, file := range manifest.Files {
		require.Regexp(t, pathPattern, file.Path)
		require.Equal(t, strings.TrimSuffix(filepath.Base(file.Path), ".zip"), file.Lambda)
		require.Regexp(t, sha256Pattern, file.SHA256)
		require.Greater(t, file.SizeBytes, int64(0))

		_, exists := seen[file.Path]
		require.False(t, exists, "duplicate manifest entry for %s", file.Path)
		seen[file.Path] = struct{}{}
	}
}

func TestLambdaBundleManifestSchemaContract(t *testing.T) {
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "contracts", "lambda-bundle-manifest.schema.json"))
	require.NoError(t, err)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema))

	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema["$schema"])
	require.Equal(t, "Lesser Lambda Bundle Manifest", schema["title"])
	require.ElementsMatch(t, []string{
		"kind",
		"schema_version",
		"release",
		"bundle",
		"inventory_source",
		"files",
	}, anySliceToStrings(t, schema["required"]))

	properties := anyMap(t, schema["properties"])
	require.Equal(t, "lesser.lambda_bundle_manifest", anyMap(t, properties["kind"])["const"])
	require.Equal(t, float64(1), anyMap(t, properties["schema_version"])["const"])

	bundleProps := anyMap(t, anyMap(t, properties["bundle"])["properties"])
	require.Equal(t, "lesser-lambda-bundle.tar.gz", anyMap(t, bundleProps["path"])["const"])
	require.Equal(t, "tar.gz", anyMap(t, bundleProps["format"])["const"])

	inventoryProps := anyMap(t, anyMap(t, properties["inventory_source"])["properties"])
	require.Equal(t, "infra/cdk/inventory/lambdas.go", anyMap(t, inventoryProps["path"])["const"])
	require.Equal(t, "lesser.lambda_inventory", anyMap(t, inventoryProps["kind"])["const"])
}

func TestDeployAssemblyDescriptorExampleContract(t *testing.T) {
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "contracts", "examples", "lesser-deploy-assembly.example.json"))
	require.NoError(t, err)

	var descriptor deployAssemblyDescriptorExample
	require.NoError(t, json.Unmarshal(data, &descriptor))

	require.Equal(t, "lesser.deploy_assembly_descriptor", descriptor.Kind)
	require.Equal(t, 1, descriptor.SchemaVersion)
	require.Equal(t, "lesser", descriptor.Release.Name)
	require.Regexp(t, regexp.MustCompile(`^v[0-9].*$`), descriptor.Release.Version)
	require.Regexp(t, regexp.MustCompile(`^[a-f0-9]{40}$`), descriptor.Release.GitSHA)
	require.Equal(t, "lesser-deploy-assembly.tar.gz", descriptor.Assembly.Path)
	require.Equal(t, "tar.gz", descriptor.Assembly.Format)
	require.Regexp(t, regexp.MustCompile(`^[a-f0-9]{64}$`), descriptor.Assembly.SHA256)
	require.Regexp(t, regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`), descriptor.Payload.Kind)
	require.GreaterOrEqual(t, descriptor.Payload.ContractVersion, 1)
	require.NotEmpty(t, descriptor.Payload.Entrypoint)
	require.NotContains(t, descriptor.Payload.Entrypoint, "..")
	require.NotContains(t, descriptor.Payload.Entrypoint, `\`)
	require.Equal(t, "lesser-release.json", descriptor.Compatibility.ReleaseManifestPath)
	require.Equal(t, "deploy_assembly", descriptor.Compatibility.DeployArtifactsKey)
	require.GreaterOrEqual(t, descriptor.Compatibility.ExecutorContractValue, 1)
	require.NotEmpty(t, descriptor.InstanceInputs.Required)
	require.NotEmpty(t, descriptor.Verification.IntegrityRequired)
	require.NotEmpty(t, descriptor.Verification.PreflightRequired)
}

func TestDeployAssemblyDescriptorSchemaContract(t *testing.T) {
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "contracts", "deploy-assembly-descriptor.schema.json"))
	require.NoError(t, err)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema))

	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema["$schema"])
	require.Equal(t, "Lesser Deploy Assembly Descriptor", schema["title"])
	require.ElementsMatch(t, []string{
		"kind",
		"schema_version",
		"release",
		"assembly",
		"payload",
		"compatibility",
		"instance_inputs",
		"verification",
	}, anySliceToStrings(t, schema["required"]))

	properties := anyMap(t, schema["properties"])
	require.Equal(t, "lesser.deploy_assembly_descriptor", anyMap(t, properties["kind"])["const"])
	require.Equal(t, float64(1), anyMap(t, properties["schema_version"])["const"])

	assemblyProps := anyMap(t, anyMap(t, properties["assembly"])["properties"])
	require.Equal(t, "lesser-deploy-assembly.tar.gz", anyMap(t, assemblyProps["path"])["const"])
	require.Equal(t, "tar.gz", anyMap(t, assemblyProps["format"])["const"])

	compatibilityProps := anyMap(t, anyMap(t, properties["compatibility"])["properties"])
	require.Equal(t, "lesser-release.json", anyMap(t, compatibilityProps["release_manifest_path"])["const"])
	require.Equal(t, "deploy_assembly", anyMap(t, compatibilityProps["deploy_artifacts_key"])["const"])
}

func anyMap(t *testing.T, value any) map[string]any {
	t.Helper()

	result, ok := value.(map[string]any)
	require.True(t, ok)
	return result
}

func anySliceToStrings(t *testing.T, value any) []string {
	t.Helper()

	items, ok := value.([]any)
	require.True(t, ok)

	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		require.True(t, ok)
		out = append(out, text)
	}
	return out
}
