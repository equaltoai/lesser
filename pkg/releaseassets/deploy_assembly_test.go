package releaseassets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestCollectDeployAssets_FileAndZip(t *testing.T) {
	root := t.TempDir()
	plainFile := filepath.Join(root, "asset.txt")
	require.NoError(t, os.WriteFile(plainFile, []byte("plain"), 0o644))

	dirAsset := filepath.Join(root, "dir-asset")
	require.NoError(t, os.MkdirAll(filepath.Join(dirAsset, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dirAsset, "nested", "index.js"), []byte("nested"), 0o644))

	manifest := cdkAssetsManifest{
		Files: map[string]cdkFileAsset{
			"file": {
				DisplayName: "plain file",
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: plainFile, Packaging: "file"},
				Destinations: map[string]struct {
					ObjectKey string `json:"objectKey"`
				}{
					"current": {ObjectKey: "assets/plain.txt"},
				},
			},
			"zip": {
				DisplayName: "dir zip",
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: dirAsset, Packaging: "zip"},
				Destinations: map[string]struct {
					ObjectKey string `json:"objectKey"`
				}{
					"current": {ObjectKey: "assets/dir.zip"},
				},
			},
			"template": {
				DisplayName: "template",
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: filepath.Join(root, "demo.template.json"), Packaging: "file"},
				Destinations: map[string]struct {
					ObjectKey string `json:"objectKey"`
				}{
					"current": {ObjectKey: "assets/demo.template.json"},
				},
			},
		},
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "demo.template.json"), []byte("{}"), 0o644))

	assets, err := collectDeployAssets("demo", manifest, nil)
	require.NoError(t, err)
	require.Len(t, assets, 2)
	require.Equal(t, "assets/dir.zip", assets[0].ObjectKey)
	require.Equal(t, "assets/plain.txt", assets[1].ObjectKey)
	require.Equal(t, "plain", string(assets[1].Data))

	zipEntries := readZipEntries(t, assets[0].Data)
	require.Equal(t, map[string]string{
		"nested/index.js": "nested",
	}, zipEntries)
}

func TestCollectDeployAssets_ErrorsWhenObjectKeyMissing(t *testing.T) {
	root := t.TempDir()
	manifest := cdkAssetsManifest{
		Files: map[string]cdkFileAsset{
			"bad": {
				DisplayName: "bad",
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: filepath.Join(root, "asset.txt"), Packaging: "file"},
			},
		},
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "asset.txt"), []byte("content"), 0o644))

	_, err := collectDeployAssets("demo", manifest, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing destination object key")
}

func TestReadAssetData_UnsupportedPackaging(t *testing.T) {
	_, err := readAssetData("/tmp/nowhere", "tar")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported deploy asset packaging")
}

func TestReadAssetData_ErrorsWhenFileMissing(t *testing.T) {
	_, err := readAssetData(filepath.Join(t.TempDir(), "missing.txt"), "file")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read deploy asset")
}

func TestReadAssetData_RejectsFileSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o644))
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := readAssetData(link, "file")
	require.Error(t, err)
	require.Contains(t, err.Error(), "symlink")
}

func TestWriteArchiveEntries_IsDeterministic(t *testing.T) {
	entries := []archiveEntry{
		{Path: "manifest.json", Data: []byte(`{"schema":1}`)},
		{Path: "templates/demo.json", Data: []byte(`{"Parameters":{}}`)},
	}

	firstPath := filepath.Join(t.TempDir(), "assembly.tar.gz")
	require.NoError(t, writeArchiveEntries(firstPath, entries))
	firstBytes, err := os.ReadFile(firstPath)
	require.NoError(t, err)

	secondPath := filepath.Join(t.TempDir(), "assembly.tar.gz")
	require.NoError(t, writeArchiveEntries(secondPath, entries))
	secondBytes, err := os.ReadFile(secondPath)
	require.NoError(t, err)

	require.Equal(t, firstBytes, secondBytes)
	require.Equal(t, []bundleEntry{
		{Name: "manifest.json", Content: `{"schema":1}`},
		{Name: "templates/demo.json", Content: `{"Parameters":{}}`},
	}, readDeployAssemblyTarGzEntries(t, firstPath))
}

func TestWriteArchiveEntries_ErrorsWhenOutputDirBlocked(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "blocked", "assembly.tar.gz")
	require.NoError(t, os.WriteFile(filepath.Dir(archivePath), []byte("blocked"), 0o644))

	err := writeArchiveEntries(archivePath, []archiveEntry{{Path: "manifest.json", Data: []byte(`{}`)}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create deploy assembly archive")
}

func TestWriteArchiveEntries_ErrorsWhenArchivePathIsDirectory(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "assembly.tar.gz")
	require.NoError(t, os.MkdirAll(archivePath, 0o755))

	err := writeArchiveEntries(archivePath, []archiveEntry{{Path: "manifest.json", Data: []byte(`{}`)}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "finalize deploy assembly archive")
}

func TestWriteArchiveEntries_ErrorsWhenHeaderInvalid(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "assembly.tar.gz")

	err := writeArchiveEntries(archivePath, []archiveEntry{{Path: "bad\x00name", Data: []byte(`{}`)}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "write deploy assembly header")
}

func TestTemplateTransformHelpers(t *testing.T) {
	template := map[string]any{
		"Parameters": map[string]any{
			"BootstrapVersion": map[string]any{"Type": "String"},
			"EncryptionRoleArnParamLookupParameter": map[string]any{
				"Type":    "String",
				"Default": placeholderAppSlug,
			},
		},
		"Resources": map[string]any{
			"Bucket": map[string]any{
				"Properties": map[string]any{
					"BucketName": placeholderAppSlug + "-assets",
					"Domain":     stagePlaceholderDomain(naming.StageDev),
					"Lookup":     map[string]any{"Ref": "EncryptionRoleArnParamLookupParameter"},
				},
			},
		},
		"Rules": map[string]any{
			cdkBootstrapValidationRule: map[string]any{
				"Assertions": []any{
					map[string]any{
						"Assert":            map[string]any{"Fn::Contains": []any{[]any{"1", "2"}, map[string]any{"Ref": "BootstrapVersion"}}},
						"AssertDescription": "CDK bootstrap version 6 required",
					},
				},
			},
		},
	}

	stripCDKBootstrapValidation(template)
	deleteStageLookupParameters(template)
	addStringParameter(template, "AppSlug", "app slug", false, "")
	addStringParameter(template, "HostedZoneId", "zone", true, "ZDEFAULT")

	transformed, err := transformTemplateValues(template, func(path []string) bool {
		return len(path) == 3 && path[0] == "Parameters" && path[2] == "Default"
	}, orderedPlaceholderReplacements(map[string]string{
		placeholderAppSlug:                      "${AppSlug}",
		stagePlaceholderDomain(naming.StageDev): stageDomainSub(naming.StageDev),
	}))
	require.NoError(t, err)

	replaceStageLookupRefs(transformed, naming.StageDev)

	parameters := transformed["Parameters"].(map[string]any)
	require.NotContains(t, parameters, "BootstrapVersion")
	require.NotContains(t, transformed, "Rules")
	require.Equal(t, map[string]any{
		"Description": "app slug",
		"Type":        "String",
	}, parameters["AppSlug"])
	require.Equal(t, map[string]any{
		"Default":     "ZDEFAULT",
		"Description": "zone",
		"Type":        "String",
	}, parameters["HostedZoneId"])

	resources := transformed["Resources"].(map[string]any)
	props := resources["Bucket"].(map[string]any)["Properties"].(map[string]any)
	require.Equal(t, map[string]any{"Fn::Sub": "${AppSlug}-assets"}, props["BucketName"])
	require.Equal(t, map[string]any{"Fn::Sub": "dev.${BaseDomain}"}, props["Domain"])
	require.Equal(t, map[string]any{
		"Fn::Sub": "{{resolve:ssm:/${AppSlug}/shared/iam/lambda-encryption-role-arn}}",
	}, props["Lookup"])
}

func TestTransformNestedStageTemplateAssetAddsReleaseParameters(t *testing.T) {
	nested := map[string]any{
		"Resources": map[string]any{
			"Alarm": map[string]any{
				"Type": "AWS::CloudWatch::Alarm",
				"Properties": map[string]any{
					"AlarmName":  placeholderAppSlug + "-dev-metrics-processor-critical-errors",
					"MetricName": "Errors",
				},
			},
		},
	}
	data, err := json.Marshal(nested)
	require.NoError(t, err)

	out, err := transformNestedStageTemplateAsset(data, naming.StageDev, stageTemplateReplacements(naming.StageDev))
	require.NoError(t, err)

	var transformed map[string]any
	require.NoError(t, json.Unmarshal(out, &transformed))
	parameters := transformed["Parameters"].(map[string]any)
	require.Contains(t, parameters, "AppSlug")
	require.Contains(t, parameters, "SoulBindingIntegrationKeyArn")

	props := transformed["Resources"].(map[string]any)["Alarm"].(map[string]any)["Properties"].(map[string]any)
	require.Equal(t, map[string]any{"Fn::Sub": "${AppSlug}-dev-metrics-processor-critical-errors"}, props["AlarmName"])
}

func TestTransformNestedStageTemplateAssetErrors(t *testing.T) {
	_, err := transformNestedStageTemplateAsset([]byte("{"), naming.StageDev, stageTemplateReplacements(naming.StageDev))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse nested template")

	nested := map[string]any{
		"Resources": map[string]any{
			"BadDependsOn": map[string]any{
				"Type":      "Custom::Example",
				"DependsOn": map[string]any{"Fn::GetAtt": []any{"Other", "Arn"}},
			},
		},
	}
	data, err := json.Marshal(nested)
	require.NoError(t, err)

	_, err = transformNestedStageTemplateAsset(data, naming.StageDev, stageTemplateReplacements(naming.StageDev))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported intrinsic")
}

func TestInjectNestedStackReleaseParameters(t *testing.T) {
	template := map[string]any{
		"Resources": map[string]any{
			"Nested": map[string]any{
				"Type":       "AWS::CloudFormation::Stack",
				"Properties": map[string]any{"TemplateURL": "https://example.invalid/nested.json"},
			},
			"Bucket": map[string]any{"Type": "AWS::S3::Bucket"},
		},
	}

	injectNestedStackReleaseParameters(template)

	nestedProps := template["Resources"].(map[string]any)["Nested"].(map[string]any)["Properties"].(map[string]any)
	params := nestedProps["Parameters"].(map[string]any)
	require.Equal(t, map[string]any{"Ref": "AppSlug"}, params["AppSlug"])
	require.Equal(t, map[string]any{"Ref": "ReleaseAssetBucketName"}, params["ReleaseAssetBucketName"])
}

func TestInjectNestedStackReleaseParametersHandlesMissingSections(t *testing.T) {
	noResources := map[string]any{"Description": "empty"}
	injectNestedStackReleaseParameters(noResources)
	require.Equal(t, map[string]any{"Description": "empty"}, noResources)

	template := map[string]any{
		"Resources": map[string]any{
			"Nested": map[string]any{
				"Type": "AWS::CloudFormation::Stack",
			},
			"Malformed": "not-a-resource-map",
		},
	}

	injectNestedStackReleaseParameters(template)

	nested := template["Resources"].(map[string]any)["Nested"].(map[string]any)
	props := nested["Properties"].(map[string]any)
	params := props["Parameters"].(map[string]any)
	require.Equal(t, map[string]any{"Ref": "BaseDomain"}, params["BaseDomain"])
}

func TestNormalizeTemplateDependsOn(t *testing.T) {
	template := map[string]any{
		"Resources": map[string]any{
			"ScalarSub": map[string]any{
				"Type": "Custom::Example",
				"DependsOn": map[string]any{
					"Fn::Sub": "ImportedBasicRolePolicy${AppSlug}devImportedBasicRole2EE626BABA11CB8E",
				},
			},
			"NestedList": map[string]any{
				"Type": "Custom::Example",
				"DependsOn": []any{
					[]any{"ImportedEncryptionRolePolicyappslugplaceholderdevImportedEncryptionRole173ECB6CE41A2D5A"},
					[]any{
						map[string]any{
							"Fn::Sub": "ImportedBasicRolePolicy${AppSlug}devImportedBasicRole2EE626BABA11CB8E",
						},
					},
				},
			},
			"EmptyList": map[string]any{
				"Type":      "Custom::Example",
				"DependsOn": []any{},
			},
		},
	}

	require.NoError(t, normalizeTemplateDependsOn(template))

	resources := template["Resources"].(map[string]any)
	require.Equal(t,
		"ImportedBasicRolePolicyappslugplaceholderdevImportedBasicRole2EE626BABA11CB8E",
		resources["ScalarSub"].(map[string]any)["DependsOn"],
	)
	require.Equal(t, []any{
		"ImportedEncryptionRolePolicyappslugplaceholderdevImportedEncryptionRole173ECB6CE41A2D5A",
		"ImportedBasicRolePolicyappslugplaceholderdevImportedBasicRole2EE626BABA11CB8E",
	}, resources["NestedList"].(map[string]any)["DependsOn"])
	require.NotContains(t, resources["EmptyList"].(map[string]any), "DependsOn")
}

func TestNormalizeTemplateDependsOn_ErrorsOnUnsupportedValues(t *testing.T) {
	template := map[string]any{
		"Resources": map[string]any{
			"Bad": map[string]any{
				"Type":      "Custom::Example",
				"DependsOn": map[string]any{"Fn::GetAtt": []any{"Other", "Arn"}},
			},
		},
	}

	err := normalizeTemplateDependsOn(template)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported intrinsic")
}

func TestNormalizeDependsOnSub_ArrayFormResolvesVariables(t *testing.T) {
	resolved, err := normalizeDependsOnSub(map[string]any{
		"Fn::Sub": []any{
			"ImportedBasicRolePolicy${AppSlug}${StageDependency}",
			map[string]any{
				"StageDependency": "devImportedBasicRole2EE626BABA11CB8E",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t,
		"ImportedBasicRolePolicyappslugplaceholderdevImportedBasicRole2EE626BABA11CB8E",
		resolved,
	)
}

func TestNormalizeDependsOnValue_FlattensNilAndNestedValues(t *testing.T) {
	normalized, keep, err := normalizeDependsOnValue([]any{
		nil,
		[]any{
			"ImportedEncryptionRolePolicyappslugplaceholderdevImportedEncryptionRole173ECB6CE41A2D5A",
			map[string]any{
				"Fn::Sub": "ImportedBasicRolePolicy${AppSlug}devImportedBasicRole2EE626BABA11CB8E",
			},
		},
	})
	require.NoError(t, err)
	require.True(t, keep)
	require.Equal(t, []any{
		"ImportedEncryptionRolePolicyappslugplaceholderdevImportedEncryptionRole173ECB6CE41A2D5A",
		"ImportedBasicRolePolicyappslugplaceholderdevImportedBasicRole2EE626BABA11CB8E",
	}, normalized)
}

func TestNormalizeDependsOnHelpers_ErrorBranches(t *testing.T) {
	_, err := normalizeDependsOnSub(map[string]any{
		"Fn::Sub": []any{
			"ImportedBasicRolePolicy${AppSlug}${StageDependency}",
			map[string]any{
				"StageDependency": 17,
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")

	_, err = normalizeDependsOnString("ImportedBasicRolePolicy${AppSlug}")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unresolved substitutions")

	_, err = normalizeDependsOnString("not-a-logical-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a valid logical ID")

	_, _, err = normalizeDependsOnValue(17)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported DependsOn value type")
}

func TestNormalizeDependsOnSub_ErrorsOnInvalidShapes(t *testing.T) {
	testCases := []struct {
		name    string
		value   map[string]any
		wantErr string
	}{
		{
			name: "unsupported intrinsic wrapper",
			value: map[string]any{
				"Fn::Sub": "ImportedBasicRolePolicy${AppSlug}",
				"Ref":     "OtherResource",
			},
			wantErr: "unsupported intrinsic",
		},
		{
			name: "empty sub array",
			value: map[string]any{
				"Fn::Sub": []any{},
			},
			wantErr: "is empty",
		},
		{
			name: "non string template",
			value: map[string]any{
				"Fn::Sub": []any{17},
			},
			wantErr: "template has unsupported type",
		},
		{
			name: "variables not map",
			value: map[string]any{
				"Fn::Sub": []any{"ImportedBasicRolePolicy${AppSlug}", "bad-vars"},
			},
			wantErr: "variables have unsupported type",
		},
		{
			name: "raw type unsupported",
			value: map[string]any{
				"Fn::Sub": 17,
			},
			wantErr: "unsupported type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeDependsOnSub(tc.value)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestNormalizeDependsOnString_ErrorsOnEmptyValue(t *testing.T) {
	_, err := normalizeDependsOnString("   ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "DependsOn value is empty")
}

func TestAddStringParameter_CreatesParametersMapAndPreservesExisting(t *testing.T) {
	template := map[string]any{}
	addStringParameter(template, "AppSlug", "app slug", true, "demo")
	addStringParameter(template, "AppSlug", "changed", false, "")

	parameters := template["Parameters"].(map[string]any)
	require.Equal(t, map[string]any{
		"Default":     "demo",
		"Description": "app slug",
		"Type":        "String",
	}, parameters["AppSlug"])
}

func TestDeleteTemplateParameter_NoParametersSection(t *testing.T) {
	template := map[string]any{
		"Resources": map[string]any{},
	}

	deleteTemplateParameter(template, "Missing")
	deleteTemplateRule(template, cdkBootstrapValidationRule)
	require.Equal(t, map[string]any{
		"Resources": map[string]any{},
	}, template)
}

func TestTransformTemplateValues_RootTypeError(t *testing.T) {
	_, err := transformTemplateValues([]any{"nope"}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected type")
}

func TestReplaceLookupRefValues_TraversesNestedCollections(t *testing.T) {
	value := map[string]any{
		"Items": []any{
			map[string]any{"Ref": "LookupValue"},
			map[string]any{"Nested": map[string]any{"Ref": "LookupValue"}},
		},
	}

	replaced := replaceLookupRefValues(value, map[string]any{
		"LookupValue": map[string]any{"Fn::Sub": "resolved"},
	}).(map[string]any)

	items := replaced["Items"].([]any)
	require.Equal(t, map[string]any{"Fn::Sub": "resolved"}, items[0])
	require.Equal(t, map[string]any{
		"Nested": map[string]any{"Fn::Sub": "resolved"},
	}, items[1])
}

func TestAbsolutizeAssetPathsAndEnvHelpers(t *testing.T) {
	manifest := cdkAssetsManifest{
		Files: map[string]cdkFileAsset{
			"a": {
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: "nested/file.txt", Packaging: "file"},
			},
			"b": {
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: "", Packaging: "file"},
			},
			"c": {
				Source: struct {
					Path      string `json:"path"`
					Packaging string `json:"packaging"`
				}{Path: "/already/absolute.txt", Packaging: "file"},
			},
		},
	}

	updated := absolutizeAssetPaths("/tmp/synth", manifest)
	require.Equal(t, "/tmp/synth/nested/file.txt", updated.Files["a"].Source.Path)
	require.Equal(t, "", updated.Files["b"].Source.Path)
	require.Equal(t, "/already/absolute.txt", updated.Files["c"].Source.Path)

	env := setCmdEnv([]string{"PATH=/bin"}, "AWS_REGION", "us-east-1")
	require.True(t, envHasKey(env, "AWS_REGION"))
	env = setCmdEnv(env, "AWS_REGION", "us-west-2")
	require.Contains(t, env, "AWS_REGION=us-west-2")
	require.False(t, envHasKey(env, "AWS_PROFILE"))
}

func TestWriteDeployAssembly_RequiresMetadata(t *testing.T) {
	_, err := WriteDeployAssembly(t.TempDir(), t.TempDir(), "", "abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "release version is required")

	_, err = WriteDeployAssembly(t.TempDir(), t.TempDir(), "v1.2.3", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "release git SHA is required")
}

func TestWriteDeployAssembly_ErrorsWhenOutputDirBlocked(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "release")
	require.NoError(t, os.WriteFile(outDir, []byte("blocked"), 0o644))

	_, err := WriteDeployAssembly(t.TempDir(), outDir, "v1.2.3", "0123456789abcdef0123456789abcdef01234567")
	require.Error(t, err)
	require.Contains(t, err.Error(), "create release dir")
}

func TestWriteDeployAssembly(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))

	binDir := t.TempDir()
	sharedStack := naming.SharedStackName(placeholderAppSlug)
	devStack := naming.StageStackName(placeholderAppSlug, naming.StageDev)
	stagingStack := naming.StageStackName(placeholderAppSlug, naming.StageStaging)
	liveStack := naming.StageStackName(placeholderAppSlug, naming.StageLive)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "cdk"), []byte(fakeCDKScript(sharedStack, devStack, stagingStack, liveStack)), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	outDir := t.TempDir()
	descriptor, err := WriteDeployAssembly(repoRoot, outDir, "v1.2.3", "0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)

	require.Equal(t, DeployAssemblyManifestKind, descriptor.Kind)
	require.Equal(t, DeployAssemblyArchiveName, descriptor.Assembly.Path)
	require.Equal(t, "tar.gz", descriptor.Assembly.Format)
	require.Equal(t, []string{
		"app_identity",
		"aws_target",
		"base_domain",
		"hosted_zone",
		"stage_plan",
	}, descriptor.InstanceInputs.Required)
	require.Equal(t, []string{
		"feature_config",
		"managed_service_urls",
		"provisioning_input",
		"bootstrap_io",
		"binding_secrets",
	}, descriptor.InstanceInputs.Optional)

	archiveEntries := readTarGzEntryBytes(t, filepath.Join(outDir, DeployAssemblyArchiveName))
	require.Contains(t, archiveEntries, deployAssemblyInternalManifestPath)
	require.Contains(t, archiveEntries, "templates/lesser-shared.template.json")
	require.Contains(t, archiveEntries, stageTemplateFileNames[naming.StageDev])
	require.Contains(t, archiveEntries, stageTemplateFileNames[naming.StageStaging])
	require.Contains(t, archiveEntries, stageTemplateFileNames[naming.StageLive])
	require.Contains(t, archiveEntries, "assets/plain.txt")
	require.Contains(t, archiveEntries, "assets/site.zip")

	var payload deployAssemblyPayloadManifest
	require.NoError(t, json.Unmarshal(archiveEntries[deployAssemblyInternalManifestPath], &payload))
	require.Len(t, payload.Stacks, 4)
	require.Len(t, payload.Assets, 2)

	devTemplate := string(archiveEntries[stageTemplateFileNames[naming.StageDev]])
	require.Contains(t, devTemplate, `"AppSlug"`)
	require.Contains(t, devTemplate, `"ReleaseAssetBucketName"`)
	require.Contains(t, devTemplate, `{{resolve:ssm:/${AppSlug}/shared/iam/lambda-encryption-role-arn}}`)
	require.Contains(t, devTemplate, `{{resolve:ssm:/${AppSlug}/dev/lesser-body/exports/v1/mcp_lambda_arn}}`)
	require.Contains(t, devTemplate, `dev.${BaseDomain}`)
	require.Contains(t, devTemplate, `"Fn::Sub": "${LesserHostUrl}"`)
	require.Contains(t, devTemplate, `"SoulBindingIntegrationKeyArn"`)
	require.Contains(t, devTemplate, `"Fn::Sub": "${SoulBindingIntegrationKeyArn}"`)
	require.Contains(t, devTemplate, `"ApiCorsAllowedOrigins"`)
	require.Contains(t, devTemplate, `"Fn::Sub": "${ReleaseAssetBucketName}"`)

	for stage, templatePath := range stageTemplateFileNames {
		templateJSON := string(archiveEntries[templatePath])
		require.Contains(t, templateJSON, `/instance/ptah/mcp`, "stage %s", stage)
		require.Contains(t, templateJSON, `/instance/ba/mcp`, "stage %s", stage)
		require.Contains(t, templateJSON, `/.well-known/oauth-protected-resource/instance/ptah/mcp`, "stage %s", stage)
		require.Contains(t, templateJSON, `/.well-known/oauth-protected-resource/instance/ba/mcp`, "stage %s", stage)
		require.Contains(t, templateJSON, `/instance/downloads/installer-grants/{grantId}`, "stage %s", stage)
		require.Contains(t, templateJSON, fmt.Sprintf(`{{resolve:ssm:/${AppSlug}/%s/lesser-body/exports/v1/instance_mcp_lambda_arn}}`, stage), "stage %s", stage)
		require.NotContains(t, templateJSON, `LesserBodyInstanceMcpLambdaArnParamLookupParameter`, "stage %s", stage)
	}

	zipEntries := readZipEntries(t, archiveEntries["assets/site.zip"])
	require.Equal(t, map[string]string{
		"nested/index.js": "console.log('stage')",
	}, zipEntries)

	sharedTemplate := string(archiveEntries["templates/lesser-shared.template.json"])
	require.Contains(t, sharedTemplate, `"AppSlug"`)
	require.Contains(t, sharedTemplate, `${AWS::AccountId}`)
	for _, templatePath := range []string{
		"templates/lesser-shared.template.json",
		stageTemplateFileNames[naming.StageDev],
		stageTemplateFileNames[naming.StageStaging],
		stageTemplateFileNames[naming.StageLive],
	} {
		templateJSON := string(archiveEntries[templatePath])
		require.NotContains(t, templateJSON, `"BootstrapVersion"`)
		require.NotContains(t, templateJSON, `"CheckBootstrapVersion"`)
	}

	var devTemplateJSON map[string]any
	require.NoError(t, json.Unmarshal(archiveEntries[stageTemplateFileNames[naming.StageDev]], &devTemplateJSON))
	for _, resource := range devTemplateJSON["Resources"].(map[string]any) {
		resMap := resource.(map[string]any)
		dependsOn, ok := resMap["DependsOn"]
		if !ok {
			continue
		}
		switch v := dependsOn.(type) {
		case string:
			require.NotContains(t, v, "${")
		case []any:
			require.NotEmpty(t, v)
			for _, item := range v {
				itemStr, ok := item.(string)
				require.True(t, ok)
				require.NotContains(t, itemStr, "${")
			}
		default:
			t.Fatalf("unexpected DependsOn type %T", dependsOn)
		}
	}
}

func TestRunCDKSynthJSON_ErrorsWhenCommandFails(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))

	binDir := t.TempDir()
	script := "#!/usr/bin/env bash\nset -euo pipefail\necho synth failed >&2\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "cdk"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, _, err := runCDKSynthJSON(repoRoot, "demo", map[string]string{"app": "demo"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cdk synth demo")
	require.Contains(t, err.Error(), "synth failed")
}

func TestRunCDKSynthJSON_ErrorsOnInvalidTemplateJSON(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))

	binDir := t.TempDir()
	script := "#!/usr/bin/env bash\nset -euo pipefail\necho '{'\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "cdk"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, _, err := runCDKSynthJSON(repoRoot, "demo", map[string]string{"app": "demo"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse synthesized template")
}

func TestRunCDKSynthJSON_PrefersSynthesizedTemplateFileOverNoisyStdout(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))

	binDir := t.TempDir()
	script := `#!/usr/bin/env bash
set -euo pipefail
stack="$2"
shift 2
output=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --context)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
mkdir -p "$output"
cat > "$output/$stack.template.json" <<'JSON'
{
  "Resources": {
    "Example": {
      "Type": "AWS::S3::Bucket"
    }
  }
}
JSON
printf '7 feature flags are not configured.\n'
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "cdk"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	template, _, synthDir, err := runCDKSynthJSON(repoRoot, "demo", map[string]string{"app": "demo"})
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(synthDir) }()

	resources, ok := template["Resources"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, resources, "Example")
}

func TestRunCDKSynthJSON_ErrorsOnInvalidAssetsManifest(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk"), 0o755))

	binDir := t.TempDir()
	script := "#!/usr/bin/env bash\nset -euo pipefail\nstack=\"$2\"\noutput=\"\"\nshift 2\nwhile [[ $# -gt 0 ]]; do\n  case \"$1\" in\n    --output) output=\"$2\"; shift 2 ;;\n    --context) shift 2 ;;\n    *) shift ;;\n  esac\ndone\nmkdir -p \"$output\"\nprintf '{' > \"$output/$stack.assets.json\"\necho '{}'\n"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "cdk"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, _, err := runCDKSynthJSON(repoRoot, "demo", map[string]string{"app": "demo"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse asset manifest")
}

func TestMarshalTemplateJSON_AppendsTrailingNewline(t *testing.T) {
	data, err := marshalTemplateJSON(map[string]any{"Resources": map[string]any{}})
	require.NoError(t, err)
	require.Equal(t, byte('\n'), data[len(data)-1])

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
}

func readZipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		entries[file.Name] = string(content)
	}
	return entries
}

func readTarGzEntryBytes(t *testing.T, archivePath string) map[string][]byte {
	t.Helper()

	f, err := os.Open(archivePath) // #nosec G304 -- test reads temp fixture path
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	entries := map[string][]byte{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		require.NoError(t, err)

		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		entries[header.Name] = content
	}
}

func readDeployAssemblyTarGzEntries(t *testing.T, archivePath string) []bundleEntry {
	t.Helper()

	f, err := os.Open(archivePath) // #nosec G304 -- test reads temp fixture path
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var entries []bundleEntry
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		require.NoError(t, err)

		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		entries = append(entries, bundleEntry{
			Name:    header.Name,
			Content: string(content),
		})
	}
}

func fakeCDKScript(sharedStack, devStack, stagingStack, liveStack string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

stack="$2"
shift 2
output=""
body_enabled=""
instance_plane_enabled=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --context)
      case "$2" in
        bodyEnabled=*) body_enabled="${2#*=}" ;;
        instancePlaneEnabled=*) instance_plane_enabled="${2#*=}" ;;
      esac
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

mkdir -p "$output"

write_stage_assets() {
  printf 'plain' > "$output/plain.txt"
  mkdir -p "$output/site/nested"
  printf "console.log('stage')" > "$output/site/nested/index.js"
  cat > "$output/$stack.assets.json" <<'JSON'
{
  "files": {
    "plain": {
      "displayName": "plain",
      "source": { "path": "plain.txt", "packaging": "file" },
      "destinations": { "current": { "objectKey": "assets/plain.txt" } }
    },
    "site": {
      "displayName": "site",
      "source": { "path": "site", "packaging": "zip" },
      "destinations": { "current": { "objectKey": "assets/site.zip" } }
    }
  }
}
JSON
}

case "$stack" in
 %q)
    cat <<'JSON'
{
  "Parameters": {
    "BootstrapVersion": { "Type": "String" }
  },
  "Rules": {
    "CheckBootstrapVersion": {
      "Assertions": [
        {
          "Assert": { "Fn::Contains": [["6"], { "Ref": "BootstrapVersion" }] },
          "AssertDescription": "CDK bootstrap version 6 required"
        }
      ]
    }
  },
  "Resources": {
    "Example": {
      "Properties": {
        "Name": "appslugplaceholder-111111111111-us-west-2"
      }
    }
  }
}
JSON
    ;;
  %q|%q|%q)
    write_stage_assets
    if [[ "$stack" == %q ]]; then
      stage_domain="dev.base.example.com"
      body_path="dev"
    elif [[ "$stack" == %q ]]; then
      stage_domain="staging.base.example.com"
      body_path="staging"
    else
      stage_domain="base.example.com"
      body_path="live"
    fi
    instance_parameter=""
    instance_resources=""
    if [[ "$body_enabled" == "true" && "$instance_plane_enabled" == "true" ]]; then
      instance_parameter=',
    "LesserBodyInstanceMcpLambdaArnParamLookupParameter": { "Type": "String", "Default": "appslugplaceholder" }'
      instance_resources=',
    "ImportedLesserBodyInstanceMcpLambda": {
      "Type": "AWS::Lambda::Function",
      "Properties": {
        "FunctionName": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    },
    "InstanceMcpLambdaInvokeFromApiGateway": {
      "Type": "AWS::Lambda::Permission",
      "Properties": {
        "FunctionName": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    },
    "InstancePtahMcpRoute": {
      "Type": "AWS::ApiGateway::Method",
      "Properties": {
        "RoutePath": "/instance/ptah/mcp",
        "Target": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    },
    "InstanceBaMcpRoute": {
      "Type": "AWS::ApiGateway::Method",
      "Properties": {
        "RoutePath": "/instance/ba/mcp",
        "Target": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    },
    "InstancePtahProtectedResourceRoute": {
      "Type": "AWS::ApiGateway::Method",
      "Properties": {
        "RoutePath": "/.well-known/oauth-protected-resource/instance/ptah/mcp",
        "Target": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    },
    "InstanceBaProtectedResourceRoute": {
      "Type": "AWS::ApiGateway::Method",
      "Properties": {
        "RoutePath": "/.well-known/oauth-protected-resource/instance/ba/mcp",
        "Target": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    },
    "InstanceInstallerGrantDownloadRoute": {
      "Type": "AWS::ApiGateway::Method",
      "Properties": {
        "RoutePath": "/instance/downloads/installer-grants/{grantId}",
        "Target": { "Ref": "LesserBodyInstanceMcpLambdaArnParamLookupParameter" }
      }
    }'
    fi
    cat <<JSON
{
  "Parameters": {
    "BootstrapVersion": { "Type": "String" },
    "EncryptionRoleArnParamLookupParameter": { "Type": "String", "Default": "appslugplaceholder" },
    "BasicRoleArnParamLookupParameter": { "Type": "String", "Default": "appslugplaceholder" },
    "JWTSecretArnParamLookupParameter": { "Type": "String", "Default": "appslugplaceholder" },
    "ActorKeyArnParamLookupParameter": { "Type": "String", "Default": "appslugplaceholder" },
    "LesserBodyMcpLambdaArnParamLookupParameter": { "Type": "String", "Default": "appslugplaceholder" }$instance_parameter
  },
  "Rules": {
    "CheckBootstrapVersion": {
      "Assertions": [
        {
          "Assert": { "Fn::Contains": [["6"], { "Ref": "BootstrapVersion" }] },
          "AssertDescription": "CDK bootstrap version 6 required"
        }
      ]
    }
  },
  "Resources": {
    "ImportedBasicRolePolicyappslugplaceholderdevImportedBasicRole2EE626BABA11CB8E": {
      "Type": "AWS::IAM::Policy"
    },
    "ImportedEncryptionRolePolicyappslugplaceholderdevImportedEncryptionRole173ECB6CE41A2D5A": {
      "Type": "AWS::IAM::Policy"
    },
    "Example": {
      "DependsOn": [
        "ImportedBasicRolePolicyappslugplaceholderdevImportedBasicRole2EE626BABA11CB8E"
      ],
      "Properties": {
        "Domain": "$stage_domain",
        "WildcardDomain": "*.$stage_domain",
        "BaseDomain": "base.example.com",
        "HostedZone": "ZHOSTEDZONEPLACEHOLDER",
        "App": "appslugplaceholder",
        "Account": "111111111111",
        "Region": "us-west-2",
        "ManagedUrl": "https://lesser-host.example.invalid",
        "ManagedAttestations": "https://lesser-host-attestations.example.invalid",
        "ManagedKey": "LESSER_HOST_INSTANCE_KEY_ARN_PLACEHOLDER",
        "SoulBindingKey": "SOUL_BINDING_INTEGRATION_KEY_ARN_PLACEHOLDER",
        "Translation": "TRANSLATION_ENABLED_PLACEHOLDER",
        "TipEnabled": "TIP_ENABLED_PLACEHOLDER",
        "TipChain": "TIP_CHAIN_ID_PLACEHOLDER",
        "TipContract": "TIP_CONTRACT_ADDRESS_PLACEHOLDER",
        "ReleaseBucket": "cdk-hnb659fds-assets-111111111111-us-west-2",
        "EncryptionRole": { "Ref": "EncryptionRoleArnParamLookupParameter" },
        "BasicRole": { "Ref": "BasicRoleArnParamLookupParameter" },
        "JWTSecret": { "Ref": "JWTSecretArnParamLookupParameter" },
        "ActorKey": { "Ref": "ActorKeyArnParamLookupParameter" },
        "BodyLambda": { "Ref": "LesserBodyMcpLambdaArnParamLookupParameter" }
      }
    },
    "NestedDependsOnExample": {
      "DependsOn": [
        [
          "ImportedEncryptionRolePolicyappslugplaceholderdevImportedEncryptionRole173ECB6CE41A2D5A"
        ]
      ],
      "Type": "Custom::Example"
    },
    "EmptyDependsOnExample": {
      "DependsOn": [],
      "Type": "Custom::Example"
    }$instance_resources
  }
}
JSON
    ;;
  *)
    echo "unexpected stack: $stack" >&2
    exit 1
    ;;
esac
`, sharedStack, devStack, stagingStack, liveStack, devStack, stagingStack)
}
