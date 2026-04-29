package releaseassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

// DeployAssemblyArchiveName is the published deploy assembly archive for
// immutable Lesser deploys.
const DeployAssemblyArchiveName = "lesser-deploy-assembly.tar.gz"

// DeployAssemblyManifestName is the published descriptor that identifies the
// deploy assembly archive and the outer executor contract.
const DeployAssemblyManifestName = "lesser-deploy-assembly.json"

// DeployAssemblyManifestKind identifies Lesser deploy assembly descriptors.
const DeployAssemblyManifestKind = "lesser.deploy_assembly_descriptor"

// DeployAssemblyManifestSchemaVersion is the current schema version for Lesser
// deploy assembly descriptors.
const DeployAssemblyManifestSchemaVersion = 1

const (
	deployAssemblyInternalManifestPath    = "manifest.json"
	deployAssemblyPayloadKind             = "lesser.cloudformation_release_assembly"
	deployAssemblyPayloadContractVersion  = 1
	deployAssemblyInternalManifestKind    = "lesser.cloudformation_release_assembly_manifest"
	deployAssemblyInternalManifestVersion = 1
	cdkBootstrapValidationRule            = "CheckBootstrapVersion"
	placeholderAppSlug                    = "appslugplaceholder"
	placeholderBaseDomain                 = "base.example.com"
	placeholderHostedZoneID               = "ZHOSTEDZONEPLACEHOLDER"
	placeholderAccountID                  = "111111111111"
	placeholderRegion                     = "us-west-2"
	placeholderLesserHostURL              = "https://lesser-host.example.invalid"
	placeholderLesserHostAttestationsURL  = "https://lesser-host-attestations.example.invalid"
	placeholderLesserHostInstanceKeyARN   = "LESSER_HOST_INSTANCE_KEY_ARN_PLACEHOLDER"
	placeholderTranslationEnabled         = "TRANSLATION_ENABLED_PLACEHOLDER"
	placeholderTipEnabled                 = "TIP_ENABLED_PLACEHOLDER"
	placeholderTipChainID                 = "TIP_CHAIN_ID_PLACEHOLDER"
	placeholderTipContractAddress         = "TIP_CONTRACT_ADDRESS_PLACEHOLDER"
	placeholderAPICORSAllowedOrigins      = "https://api-cors-placeholder.example.invalid"
)

var stageTemplateFileNames = map[naming.Stage]string{
	naming.StageDev:     "templates/lesser-managed-dev.template.json",
	naming.StageStaging: "templates/lesser-managed-staging.template.json",
	naming.StageLive:    "templates/lesser-managed-live.template.json",
}

var stageContextPlaceholders = map[string]string{
	"lesserHostUrl":             placeholderLesserHostURL,
	"lesserHostAttestationsUrl": placeholderLesserHostAttestationsURL,
	"lesserHostInstanceKeyArn":  placeholderLesserHostInstanceKeyARN,
	"translationEnabled":        placeholderTranslationEnabled,
	"tipEnabled":                placeholderTipEnabled,
	"tipChainId":                placeholderTipChainID,
	"tipContractAddress":        placeholderTipContractAddress,
	"apiCorsAllowedOrigins":     placeholderAPICORSAllowedOrigins,
}

var stageLookupParameterNames = map[string]struct{}{
	"EncryptionRoleArnParamLookupParameter":      {},
	"BasicRoleArnParamLookupParameter":           {},
	"JWTSecretArnParamLookupParameter":           {},
	"ActorKeyArnParamLookupParameter":            {},
	"LesserBodyMcpLambdaArnParamLookupParameter": {},
}

var cloudFormationLogicalIDPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// DeployAssemblyDescriptor describes the published deploy assembly archive and
// the outer executor contract.
type DeployAssemblyDescriptor struct {
	Kind           string                       `json:"kind"`
	SchemaVersion  int                          `json:"schema_version"`
	Release        LambdaBundleRelease          `json:"release"`
	Assembly       DeployAssemblyAsset          `json:"assembly"`
	Payload        DeployAssemblyPayload        `json:"payload"`
	Compatibility  DeployAssemblyCompatibility  `json:"compatibility"`
	InstanceInputs DeployAssemblyInstanceInputs `json:"instance_inputs"`
	Verification   DeployAssemblyVerification   `json:"verification"`
}

// DeployAssemblyAsset describes the published deploy assembly archive.
type DeployAssemblyAsset struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	SHA256 string `json:"sha256"`
}

// DeployAssemblyPayload describes the inner deploy assembly payload.
type DeployAssemblyPayload struct {
	Kind            string `json:"kind"`
	ContractVersion int    `json:"contract_version"`
	Entrypoint      string `json:"entrypoint"`
}

// DeployAssemblyCompatibility describes how the deploy assembly hooks into the
// top-level release manifest and executor contract.
type DeployAssemblyCompatibility struct {
	ReleaseManifestPath     string `json:"release_manifest_path"`
	DeployArtifactsKey      string `json:"deploy_artifacts_key"`
	ExecutorContractVersion int    `json:"executor_contract_version"`
}

// DeployAssemblyInstanceInputs describes the canonical input-category contract
// for the deploy assembly.
type DeployAssemblyInstanceInputs struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

// DeployAssemblyVerification describes integrity and preflight requirements for
// the deploy assembly.
type DeployAssemblyVerification struct {
	IntegrityRequired []string `json:"integrity_required"`
	PreflightRequired []string `json:"preflight_required"`
}

type deployAssemblyPayloadManifest struct {
	Kind          string                       `json:"kind"`
	SchemaVersion int                          `json:"schema_version"`
	Release       LambdaBundleRelease          `json:"release"`
	Stacks        []deployAssemblyStack        `json:"stacks"`
	Assets        []deployAssemblyUploadedFile `json:"assets"`
}

type deployAssemblyStack struct {
	Name         string `json:"name"`
	Stage        string `json:"stage,omitempty"`
	TemplatePath string `json:"template_path"`
	SHA256       string `json:"sha256"`
}

type deployAssemblyUploadedFile struct {
	ObjectKey   string `json:"object_key"`
	ArchivePath string `json:"archive_path"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
}

type cdkAssetsManifest struct {
	Files map[string]cdkFileAsset `json:"files"`
}

type cdkFileAsset struct {
	DisplayName string `json:"displayName"`
	Source      struct {
		Path      string `json:"path"`
		Packaging string `json:"packaging"`
	} `json:"source"`
	Destinations map[string]struct {
		ObjectKey string `json:"objectKey"`
	} `json:"destinations"`
}

type archiveEntry struct {
	Path string
	Data []byte
}

type deployAsset struct {
	ObjectKey   string
	ArchivePath string
	Data        []byte
}

// WriteDeployAssembly writes the deploy assembly archive and descriptor into
// outDir.
func WriteDeployAssembly(repoRoot string, outDir string, version string, gitSHA string) (DeployAssemblyDescriptor, error) {
	if version == "" {
		return DeployAssemblyDescriptor{}, fmt.Errorf("release version is required")
	}
	if gitSHA == "" {
		return DeployAssemblyDescriptor{}, fmt.Errorf("release git SHA is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return DeployAssemblyDescriptor{}, fmt.Errorf("create release dir: %w", err)
	}

	sharedTemplate, err := synthesizeSharedTemplate(repoRoot)
	if err != nil {
		return DeployAssemblyDescriptor{}, err
	}

	stageAssets := map[string]deployAsset{}
	stageEntries := make([]deployAssemblyStack, 0, 3)
	archiveEntries := []archiveEntry{
		{
			Path: "templates/lesser-shared.template.json",
			Data: sharedTemplate,
		},
	}

	for _, stage := range []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive} {
		templateBytes, assets, err := synthesizeStageTemplate(repoRoot, stage)
		if err != nil {
			return DeployAssemblyDescriptor{}, err
		}

		templatePath := stageTemplateFileNames[stage]
		archiveEntries = append(archiveEntries, archiveEntry{
			Path: templatePath,
			Data: templateBytes,
		})
		templateSHA := sha256.Sum256(templateBytes)
		stageEntries = append(stageEntries, deployAssemblyStack{
			Name:         string(stage),
			Stage:        string(stage),
			TemplatePath: templatePath,
			SHA256:       hex.EncodeToString(templateSHA[:]),
		})

		for _, asset := range assets {
			if existing, ok := stageAssets[asset.ObjectKey]; ok {
				if !bytes.Equal(existing.Data, asset.Data) {
					return DeployAssemblyDescriptor{}, fmt.Errorf("deploy assembly asset collision for %s", asset.ObjectKey)
				}
				continue
			}
			stageAssets[asset.ObjectKey] = asset
		}
	}

	sharedSHA := sha256.Sum256(sharedTemplate)
	stackEntries := append([]deployAssemblyStack{{
		Name:         string(naming.StageShared),
		TemplatePath: "templates/lesser-shared.template.json",
		SHA256:       hex.EncodeToString(sharedSHA[:]),
	}}, stageEntries...)

	assetKeys := make([]string, 0, len(stageAssets))
	for key := range stageAssets {
		assetKeys = append(assetKeys, key)
	}
	sort.Strings(assetKeys)

	payloadAssets := make([]deployAssemblyUploadedFile, 0, len(assetKeys))
	for _, key := range assetKeys {
		asset := stageAssets[key]
		archiveEntries = append(archiveEntries, archiveEntry{
			Path: asset.ArchivePath,
			Data: asset.Data,
		})
		sum := sha256.Sum256(asset.Data)
		payloadAssets = append(payloadAssets, deployAssemblyUploadedFile{
			ObjectKey:   asset.ObjectKey,
			ArchivePath: asset.ArchivePath,
			SHA256:      hex.EncodeToString(sum[:]),
			SizeBytes:   int64(len(asset.Data)),
		})
	}

	payloadManifest := deployAssemblyPayloadManifest{
		Kind:          deployAssemblyInternalManifestKind,
		SchemaVersion: deployAssemblyInternalManifestVersion,
		Release: LambdaBundleRelease{
			Name:    "lesser",
			Version: version,
			GitSHA:  gitSHA,
		},
		Stacks: stackEntries,
		Assets: payloadAssets,
	}
	payloadData, err := json.MarshalIndent(payloadManifest, "", "  ")
	if err != nil {
		return DeployAssemblyDescriptor{}, fmt.Errorf("marshal deploy assembly payload manifest: %w", err)
	}
	payloadData = append(payloadData, '\n')
	archiveEntries = append(archiveEntries, archiveEntry{
		Path: deployAssemblyInternalManifestPath,
		Data: payloadData,
	})

	sort.Slice(archiveEntries, func(i, j int) bool {
		return archiveEntries[i].Path < archiveEntries[j].Path
	})

	archivePath := filepath.Join(outDir, DeployAssemblyArchiveName)
	if err := writeArchiveEntries(archivePath, archiveEntries); err != nil {
		return DeployAssemblyDescriptor{}, err
	}

	archiveSHA, err := fileSHA256(archivePath)
	if err != nil {
		return DeployAssemblyDescriptor{}, fmt.Errorf("hash deploy assembly archive: %w", err)
	}

	descriptor := DeployAssemblyDescriptor{
		Kind:          DeployAssemblyManifestKind,
		SchemaVersion: DeployAssemblyManifestSchemaVersion,
		Release: LambdaBundleRelease{
			Name:    "lesser",
			Version: version,
			GitSHA:  gitSHA,
		},
		Assembly: DeployAssemblyAsset{
			Path:   DeployAssemblyArchiveName,
			Format: "tar.gz",
			SHA256: archiveSHA,
		},
		Payload: DeployAssemblyPayload{
			Kind:            deployAssemblyPayloadKind,
			ContractVersion: deployAssemblyPayloadContractVersion,
			Entrypoint:      deployAssemblyInternalManifestPath,
		},
		Compatibility: DeployAssemblyCompatibility{
			ReleaseManifestPath:     ReleaseManifestName,
			DeployArtifactsKey:      "deploy_assembly",
			ExecutorContractVersion: 1,
		},
		InstanceInputs: DeployAssemblyInstanceInputs{
			Required: []string{
				"app_identity",
				"aws_target",
				"base_domain",
				"hosted_zone",
				"stage_plan",
			},
			Optional: []string{
				"feature_config",
				"managed_service_urls",
				"provisioning_input",
				"bootstrap_io",
			},
		},
		Verification: DeployAssemblyVerification{
			IntegrityRequired: []string{
				"assembly.sha256",
				"checksums.txt",
			},
			PreflightRequired: []string{
				"instance_input_validation",
				"release_manifest_compatibility",
			},
		},
	}

	descriptorData, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return DeployAssemblyDescriptor{}, fmt.Errorf("marshal deploy assembly descriptor: %w", err)
	}
	descriptorData = append(descriptorData, '\n')
	if err := os.WriteFile(filepath.Join(outDir, DeployAssemblyManifestName), descriptorData, 0o644); err != nil {
		return DeployAssemblyDescriptor{}, fmt.Errorf("write deploy assembly descriptor: %w", err)
	}

	return descriptor, nil
}

func synthesizeSharedTemplate(repoRoot string) ([]byte, error) {
	stackName := naming.SharedStackName(placeholderAppSlug)
	template, _, synthDir, err := runCDKSynthJSON(repoRoot, stackName, map[string]string{
		"app":   placeholderAppSlug,
		"stage": string(naming.StageShared),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(synthDir) }()

	stripCDKBootstrapValidation(template)
	addStringParameter(template, "AppSlug", "app slug / stack prefix for this installation", false, "")

	replacements := orderedPlaceholderReplacements(map[string]string{
		placeholderAppSlug:   "${AppSlug}",
		placeholderAccountID: "${AWS::AccountId}",
		placeholderRegion:    "${AWS::Region}",
	})

	transformed, err := transformTemplateValues(template, nil, replacements)
	if err != nil {
		return nil, err
	}
	if err := normalizeTemplateDependsOn(transformed); err != nil {
		return nil, err
	}
	return marshalTemplateJSON(transformed)
}

func synthesizeStageTemplate(repoRoot string, stage naming.Stage) ([]byte, []deployAsset, error) {
	stackName := naming.StageStackName(placeholderAppSlug, stage)
	contexts := map[string]string{
		"app":          placeholderAppSlug,
		"baseDomain":   placeholderBaseDomain,
		"hostedZoneId": placeholderHostedZoneID,
		"stage":        string(stage),
		"bodyEnabled":  "true",
	}
	for key, value := range stageContextPlaceholders {
		contexts[key] = value
	}

	template, assetsManifest, synthDir, err := runCDKSynthJSON(repoRoot, stackName, contexts)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.RemoveAll(synthDir) }()

	stripCDKBootstrapValidation(template)
	deleteStageLookupParameters(template)
	addStringParameter(template, "AppSlug", "app slug / stack prefix for this installation", false, "")
	addStringParameter(template, "BaseDomain", "base domain for this installation", false, "")
	addStringParameter(template, "HostedZoneId", "route53 hosted zone id for the base domain", false, "")
	addStringParameter(template, "ReleaseAssetBucketName", "shared release asset bucket used for deploy assembly uploads", false, "")
	addStringParameter(template, "LesserHostUrl", "managed Lesser host base URL", true, "")
	addStringParameter(template, "LesserHostAttestationsUrl", "managed Lesser host attestations URL", true, "")
	addStringParameter(template, "LesserHostInstanceKeyArn", "managed Lesser host instance key secret ARN", true, "")
	addStringParameter(template, "TranslationEnabled", "per-install translation toggle", true, "")
	addStringParameter(template, "TipEnabled", "per-install tips toggle", true, "")
	addStringParameter(template, "TipChainId", "per-install tip chain id", true, "")
	addStringParameter(template, "TipContractAddress", "per-install tip contract address", true, "")
	addStringParameter(template, "ApiCorsAllowedOrigins", "per-install API browser CORS origins", true, "")

	replacements := orderedPlaceholderReplacements(map[string]string{
		stagePlaceholderDomain(stage):        stageDomainSub(stage),
		"*." + stagePlaceholderDomain(stage): "*." + stageDomainSub(stage),
		placeholderBaseDomain:                "${BaseDomain}",
		placeholderHostedZoneID:              "${HostedZoneId}",
		placeholderAppSlug:                   "${AppSlug}",
		placeholderAccountID:                 "${AWS::AccountId}",
		placeholderRegion:                    "${AWS::Region}",
		placeholderLesserHostURL:             "${LesserHostUrl}",
		placeholderLesserHostAttestationsURL: "${LesserHostAttestationsUrl}",
		placeholderLesserHostInstanceKeyARN:  "${LesserHostInstanceKeyArn}",
		placeholderTranslationEnabled:        "${TranslationEnabled}",
		placeholderTipEnabled:                "${TipEnabled}",
		placeholderTipChainID:                "${TipChainId}",
		placeholderTipContractAddress:        "${TipContractAddress}",
		placeholderAPICORSAllowedOrigins:     "${ApiCorsAllowedOrigins}",
		assetBucketPlaceholder():             "${ReleaseAssetBucketName}",
	})

	transformed, err := transformTemplateValues(template, func(path []string) bool {
		return len(path) == 3 && path[0] == "Parameters" && path[2] == "Default" && generatedParameterName(path[1])
	}, replacements)
	if err != nil {
		return nil, nil, err
	}

	replaceStageLookupRefs(transformed, stage)
	if err := normalizeTemplateDependsOn(transformed); err != nil {
		return nil, nil, err
	}

	templateBytes, err := marshalTemplateJSON(transformed)
	if err != nil {
		return nil, nil, err
	}

	assets, err := collectDeployAssets(stackName, assetsManifest)
	if err != nil {
		return nil, nil, err
	}
	return templateBytes, assets, nil
}

func runCDKSynthJSON(repoRoot string, stackName string, contexts map[string]string) (map[string]any, cdkAssetsManifest, string, error) {
	synthDir, err := os.MkdirTemp("", "lesser-release-synth.")
	if err != nil {
		return nil, cdkAssetsManifest{}, "", fmt.Errorf("create synth dir: %w", err)
	}

	args := []string{"synth", stackName, "--json", "--output", synthDir}
	keys := make([]string, 0, len(contexts))
	for key := range contexts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--context", fmt.Sprintf("%s=%s", key, contexts[key]))
	}

	cmd := exec.CommandContext(context.Background(), "cdk", args...) //nolint:gosec // local trusted tool invocation
	cmd.Dir = filepath.Join(repoRoot, "infra", "cdk")
	cmd.Env = append([]string(nil), os.Environ()...)
	cmd.Env = setCmdEnv(cmd.Env, "CDK_DEFAULT_ACCOUNT", placeholderAccountID)
	cmd.Env = setCmdEnv(cmd.Env, "CDK_DEFAULT_REGION", placeholderRegion)
	cmd.Env = setCmdEnv(cmd.Env, "AWS_REGION", placeholderRegion)
	cmd.Env = setCmdEnv(cmd.Env, "AWS_DEFAULT_REGION", placeholderRegion)
	if !envHasKey(cmd.Env, "GOTOOLCHAIN") {
		cmd.Env = append(cmd.Env, "GOTOOLCHAIN=auto")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(synthDir)
		return nil, cdkAssetsManifest{}, "", fmt.Errorf("cdk synth %s: %w\n%s", stackName, err, strings.TrimSpace(stderr.String()))
	}

	templateData, err := readSynthesizedTemplateFile(synthDir, stackName, stdout.Bytes())
	if err != nil {
		_ = os.RemoveAll(synthDir)
		return nil, cdkAssetsManifest{}, "", err
	}

	var template map[string]any
	if err := json.Unmarshal(templateData, &template); err != nil {
		_ = os.RemoveAll(synthDir)
		return nil, cdkAssetsManifest{}, "", fmt.Errorf("parse synthesized template for %s: %w", stackName, err)
	}

	var assets cdkAssetsManifest
	assetsPath := filepath.Join(synthDir, stackName+".assets.json")
	if data, err := os.ReadFile(assetsPath); err == nil {
		if err := json.Unmarshal(data, &assets); err != nil {
			_ = os.RemoveAll(synthDir)
			return nil, cdkAssetsManifest{}, "", fmt.Errorf("parse asset manifest %s: %w", assetsPath, err)
		}
	}

	assets = absolutizeAssetPaths(synthDir, assets)
	return template, assets, synthDir, nil
}

func readSynthesizedTemplateFile(synthDir string, stackName string, stdout []byte) ([]byte, error) {
	templatePath := filepath.Join(synthDir, stackName+".template.json")
	templateData, err := os.ReadFile(templatePath)
	if err == nil {
		return templateData, nil
	}

	if len(bytes.TrimSpace(stdout)) != 0 {
		return stdout, nil
	}

	return nil, fmt.Errorf("read synthesized template %s: %w", templatePath, err)
}

func absolutizeAssetPaths(synthDir string, manifest cdkAssetsManifest) cdkAssetsManifest {
	for key, fileAsset := range manifest.Files {
		if fileAsset.Source.Path == "" || filepath.IsAbs(fileAsset.Source.Path) {
			continue
		}
		fileAsset.Source.Path = filepath.Join(synthDir, fileAsset.Source.Path)
		manifest.Files[key] = fileAsset
	}
	return manifest
}

func collectDeployAssets(stackName string, manifest cdkAssetsManifest) ([]deployAsset, error) {
	assets := make([]deployAsset, 0, len(manifest.Files))
	for _, fileAsset := range manifest.Files {
		if strings.HasSuffix(fileAsset.Source.Path, stackName+".template.json") {
			continue
		}
		objectKey := firstObjectKey(fileAsset.Destinations)
		if strings.TrimSpace(objectKey) == "" {
			return nil, fmt.Errorf("deploy asset %q missing destination object key", fileAsset.DisplayName)
		}

		data, err := readAssetData(fileAsset.Source.Path, fileAsset.Source.Packaging)
		if err != nil {
			return nil, err
		}

		assets = append(assets, deployAsset{
			ObjectKey:   objectKey,
			ArchivePath: filepath.ToSlash(filepath.Join("assets", filepath.Base(objectKey))),
			Data:        data,
		})
	}

	sort.Slice(assets, func(i, j int) bool {
		return assets[i].ObjectKey < assets[j].ObjectKey
	})
	return assets, nil
}

func firstObjectKey(destinations map[string]struct {
	ObjectKey string `json:"objectKey"`
}) string {
	keys := make([]string, 0, len(destinations))
	for key := range destinations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if objectKey := strings.TrimSpace(destinations[key].ObjectKey); objectKey != "" {
			return objectKey
		}
	}
	return ""
}

func readAssetData(sourcePath string, packaging string) ([]byte, error) {
	switch packaging {
	case "file":
		data, err := readReleaseAssetFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read deploy asset %s: %w", sourcePath, err)
		}
		return data, nil
	case "zip":
		tmpZip, err := os.CreateTemp("", "deploy-asset-*.zip")
		if err != nil {
			return nil, fmt.Errorf("create temp zip for %s: %w", sourcePath, err)
		}
		tmpPath := tmpZip.Name()
		_ = tmpZip.Close()
		defer func() { _ = os.Remove(tmpPath) }()

		if err := zipDirectory(sourcePath, tmpPath); err != nil {
			return nil, fmt.Errorf("zip deploy asset %s: %w", sourcePath, err)
		}
		data, err := os.ReadFile(tmpPath) // #nosec G304 -- temp file created in repo-local tmp dir
		if err != nil {
			return nil, fmt.Errorf("read zipped deploy asset %s: %w", sourcePath, err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported deploy asset packaging %q for %s", packaging, sourcePath)
	}
}

func writeArchiveEntries(archivePath string, entries []archiveEntry) error {
	tmpPath := archivePath + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) // #nosec G304 -- caller controls output dir
	if err != nil {
		return fmt.Errorf("create deploy assembly archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("create deploy assembly gzip writer: %w", err)
	}
	gz.ModTime = deterministicArchiveTime
	gz.OS = 255

	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.Path,
			Mode:     0o644,
			Size:     int64(len(entry.Data)),
			ModTime:  deterministicArchiveTime,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = f.Close()
			return fmt.Errorf("write deploy assembly header %s: %w", entry.Path, err)
		}
		if _, err := io.Copy(tw, bytesReader(entry.Data)); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = f.Close()
			return fmt.Errorf("write deploy assembly entry %s: %w", entry.Path, err)
		}
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = f.Close()
		return fmt.Errorf("finalize deploy assembly tar stream: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return fmt.Errorf("finalize deploy assembly gzip stream: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return fmt.Errorf("finalize deploy assembly archive: %w", err)
	}
	return nil
}

func deleteTemplateParameter(template map[string]any, name string) {
	parameters, ok := template["Parameters"].(map[string]any)
	if !ok {
		return
	}
	delete(parameters, name)
	if len(parameters) == 0 {
		delete(template, "Parameters")
	}
}

func deleteTemplateRule(template map[string]any, name string) {
	rules, ok := template["Rules"].(map[string]any)
	if !ok {
		return
	}
	delete(rules, name)
	if len(rules) == 0 {
		delete(template, "Rules")
	}
}

func stripCDKBootstrapValidation(template map[string]any) {
	deleteTemplateParameter(template, "BootstrapVersion")
	deleteTemplateRule(template, cdkBootstrapValidationRule)
}

func addStringParameter(template map[string]any, name string, description string, withDefault bool, defaultValue string) {
	parameters, ok := template["Parameters"].(map[string]any)
	if !ok {
		parameters = map[string]any{}
		template["Parameters"] = parameters
	}
	if _, exists := parameters[name]; exists {
		return
	}
	param := map[string]any{
		"Type":        "String",
		"Description": description,
	}
	if withDefault {
		param["Default"] = defaultValue
	}
	parameters[name] = param
}

func marshalTemplateJSON(template map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal transformed template: %w", err)
	}
	return append(data, '\n'), nil
}

func transformTemplateValues(value any, skip func(path []string) bool, replacements []placeholderReplacement) (map[string]any, error) {
	transformed, err := transformValue(value, nil, skip, replacements)
	if err != nil {
		return nil, err
	}
	root, ok := transformed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("transformed template root has unexpected type %T", transformed)
	}
	return root, nil
}

type placeholderReplacement struct {
	needle      string
	replacement string
}

func orderedPlaceholderReplacements(values map[string]string) []placeholderReplacement {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	result := make([]placeholderReplacement, 0, len(keys))
	for _, key := range keys {
		result = append(result, placeholderReplacement{
			needle:      key,
			replacement: values[key],
		})
	}
	return result
}

func transformValue(value any, path []string, skip func(path []string) bool, replacements []placeholderReplacement) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, err := transformValue(v[key], append(path, key), skip, replacements)
			if err != nil {
				return nil, err
			}
			out[key] = child
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i := range v {
			child, err := transformValue(v[i], append(path, fmt.Sprintf("[%d]", i)), skip, replacements)
			if err != nil {
				return nil, err
			}
			out[i] = child
		}
		return out, nil
	case string:
		if skip != nil && skip(path) {
			return v, nil
		}
		templateValue, changed := applyPlaceholderReplacements(v, replacements)
		if !changed {
			return v, nil
		}
		return map[string]any{"Fn::Sub": templateValue}, nil
	default:
		return value, nil
	}
}

func normalizeTemplateDependsOn(value any) error {
	switch v := value.(type) {
	case map[string]any:
		if dependsOn, ok := v["DependsOn"]; ok {
			normalized, keep, err := normalizeDependsOnValue(dependsOn)
			if err != nil {
				return err
			}
			if keep {
				v["DependsOn"] = normalized
			} else {
				delete(v, "DependsOn")
			}
		}
		for _, child := range v {
			if err := normalizeTemplateDependsOn(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := normalizeTemplateDependsOn(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeDependsOnValue(value any) (any, bool, error) {
	items, err := flattenDependsOnValues(value)
	if err != nil {
		return nil, false, err
	}
	if len(items) == 0 {
		return nil, false, nil
	}
	if len(items) == 1 {
		return items[0], true, nil
	}

	normalized := make([]any, len(items))
	for idx, item := range items {
		normalized[idx] = item
	}
	return normalized, true, nil
}

func flattenDependsOnValues(value any) ([]string, error) {
	switch v := value.(type) {
	case string:
		resolved, err := normalizeDependsOnString(v)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	case map[string]any:
		resolved, err := normalizeDependsOnSub(v)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	case []any:
		flattened := make([]string, 0, len(v))
		for _, child := range v {
			items, err := flattenDependsOnValues(child)
			if err != nil {
				return nil, err
			}
			flattened = append(flattened, items...)
		}
		return flattened, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported DependsOn value type %T", value)
	}
}

func normalizeDependsOnSub(value map[string]any) (string, error) {
	raw, ok := value["Fn::Sub"]
	if !ok || len(value) != 1 {
		return "", fmt.Errorf("DependsOn contains unsupported intrinsic %#v", value)
	}

	switch v := raw.(type) {
	case string:
		return normalizeDependsOnString(resolveDependsOnPlaceholders(v))
	case []any:
		if len(v) == 0 {
			return "", fmt.Errorf("DependsOn Fn::Sub is empty")
		}
		template, ok := v[0].(string)
		if !ok {
			return "", fmt.Errorf("DependsOn Fn::Sub template has unsupported type %T", v[0])
		}
		resolved := resolveDependsOnPlaceholders(template)
		if len(v) == 2 {
			vars, ok := v[1].(map[string]any)
			if !ok {
				return "", fmt.Errorf("DependsOn Fn::Sub variables have unsupported type %T", v[1])
			}
			for key, rawValue := range vars {
				strValue, ok := rawValue.(string)
				if !ok {
					return "", fmt.Errorf("DependsOn Fn::Sub variable %s has unsupported type %T", key, rawValue)
				}
				resolved = strings.ReplaceAll(resolved, "${"+key+"}", strValue)
			}
		}
		return normalizeDependsOnString(resolved)
	default:
		return "", fmt.Errorf("DependsOn Fn::Sub has unsupported type %T", raw)
	}
}

func resolveDependsOnPlaceholders(value string) string {
	replacements := orderedPlaceholderReplacements(map[string]string{
		"${AppSlug}":                   placeholderAppSlug,
		"${AWS::AccountId}":            placeholderAccountID,
		"${AWS::Region}":               placeholderRegion,
		"${BaseDomain}":                placeholderBaseDomain,
		"${HostedZoneId}":              placeholderHostedZoneID,
		"${ReleaseAssetBucketName}":    assetBucketPlaceholder(),
		"${LesserHostUrl}":             placeholderLesserHostURL,
		"${LesserHostAttestationsUrl}": placeholderLesserHostAttestationsURL,
		"${LesserHostInstanceKeyArn}":  placeholderLesserHostInstanceKeyARN,
		"${TranslationEnabled}":        placeholderTranslationEnabled,
		"${TipEnabled}":                placeholderTipEnabled,
		"${TipChainId}":                placeholderTipChainID,
		"${TipContractAddress}":        placeholderTipContractAddress,
		"${ApiCorsAllowedOrigins}":     placeholderAPICORSAllowedOrigins,
	})
	resolved, _ := applyPlaceholderReplacements(value, replacements)
	return resolved
}

func normalizeDependsOnString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("DependsOn value is empty")
	}
	if strings.Contains(value, "${") {
		return "", fmt.Errorf("DependsOn value %q has unresolved substitutions", value)
	}
	if !cloudFormationLogicalIDPattern.MatchString(value) {
		return "", fmt.Errorf("DependsOn value %q is not a valid logical ID", value)
	}
	return value, nil
}

func applyPlaceholderReplacements(value string, replacements []placeholderReplacement) (string, bool) {
	updated := value
	changed := false
	for _, replacement := range replacements {
		if strings.Contains(updated, replacement.needle) {
			updated = strings.ReplaceAll(updated, replacement.needle, replacement.replacement)
			changed = true
		}
	}
	return updated, changed
}

func generatedParameterName(name string) bool {
	_, ok := stageLookupParameterNames[name]
	return ok
}

func deleteStageLookupParameters(template map[string]any) {
	for name := range stageLookupParameterNames {
		deleteTemplateParameter(template, name)
	}
}

func replaceStageLookupRefs(template map[string]any, stage naming.Stage) {
	replacements := map[string]any{
		"EncryptionRoleArnParamLookupParameter": map[string]any{
			"Fn::Sub": "{{resolve:ssm:/${AppSlug}/shared/iam/lambda-encryption-role-arn}}",
		},
		"BasicRoleArnParamLookupParameter": map[string]any{
			"Fn::Sub": "{{resolve:ssm:/${AppSlug}/shared/iam/lambda-basic-role-arn}}",
		},
		"JWTSecretArnParamLookupParameter": map[string]any{
			"Fn::Sub": "{{resolve:ssm:/${AppSlug}/shared/secrets/jwt-secret-arn}}",
		},
		"ActorKeyArnParamLookupParameter": map[string]any{
			"Fn::Sub": "{{resolve:ssm:/${AppSlug}/shared/secrets/actor-private-key-arn}}",
		},
		"LesserBodyMcpLambdaArnParamLookupParameter": map[string]any{
			"Fn::Sub": fmt.Sprintf("{{resolve:ssm:/${AppSlug}/%s/lesser-body/exports/v1/mcp_lambda_arn}}", stage),
		},
	}
	replaceLookupRefValues(template, replacements)
}

func replaceLookupRefValues(value any, replacements map[string]any) any {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 1 {
			if refName, ok := v["Ref"].(string); ok {
				if replacement, exists := replacements[refName]; exists {
					return replacement
				}
			}
		}
		for key, child := range v {
			v[key] = replaceLookupRefValues(child, replacements)
		}
		return v
	case []any:
		for idx, child := range v {
			v[idx] = replaceLookupRefValues(child, replacements)
		}
		return v
	default:
		return value
	}
}

func stagePlaceholderDomain(stage naming.Stage) string {
	return naming.StageDomain(stage, placeholderBaseDomain)
}

func stageDomainSub(stage naming.Stage) string {
	if stage == naming.StageLive {
		return "${BaseDomain}"
	}
	return fmt.Sprintf("%s.${BaseDomain}", stage)
}

func assetBucketPlaceholder() string {
	return fmt.Sprintf("cdk-hnb659fds-assets-%s-%s", placeholderAccountID, placeholderRegion)
}

func setCmdEnv(env []string, key, value string) []string {
	prefix := key + "="
	for idx, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[idx] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
