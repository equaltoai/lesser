package main

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

type verifiedReleaseAssets struct {
	files           releaseFileSet
	checksums       map[string]string
	releaseManifest releaseassets.ReleaseManifest
	lambdaBundle    releaseassets.LambdaBundleManifest
	deployAssembly  releaseassets.DeployAssemblyDescriptor
}

func loadVerifiedReleaseAssets(releaseDir string) (verifiedReleaseAssets, error) {
	files, err := requiredReleaseFiles(releaseDir)
	if err != nil {
		return verifiedReleaseAssets{}, err
	}

	checksums, err := readReleaseChecksums(files.checksumsPath)
	if err != nil {
		return verifiedReleaseAssets{}, err
	}
	if err := verifyReleaseChecksums(files, checksums); err != nil {
		return verifiedReleaseAssets{}, err
	}

	releaseManifest, err := readReleaseManifestFile(files.releaseManifestPath)
	if err != nil {
		return verifiedReleaseAssets{}, err
	}
	if err := validateReleaseManifest(releaseManifest); err != nil {
		return verifiedReleaseAssets{}, err
	}

	lambdaBundle, err := readLambdaBundleManifestFile(files.bundleManifestPath)
	if err != nil {
		return verifiedReleaseAssets{}, err
	}
	if err := validateBundleManifest(lambdaBundle, releaseManifest); err != nil {
		return verifiedReleaseAssets{}, err
	}
	if err := verifyFileChecksum(files.bundleArchivePath, lambdaBundle.Bundle.SHA256, "bundle archive"); err != nil {
		return verifiedReleaseAssets{}, err
	}

	deployAssembly, err := readDeployAssemblyDescriptorFile(files.assemblyManifestPath)
	if err != nil {
		return verifiedReleaseAssets{}, err
	}
	if err := validateDeployAssemblyDescriptor(deployAssembly, releaseManifest); err != nil {
		return verifiedReleaseAssets{}, err
	}
	if err := verifyFileChecksum(files.assemblyArchivePath, deployAssembly.Assembly.SHA256, "deploy assembly archive"); err != nil {
		return verifiedReleaseAssets{}, err
	}

	return verifiedReleaseAssets{
		files:           files,
		checksums:       checksums,
		releaseManifest: releaseManifest,
		lambdaBundle:    lambdaBundle,
		deployAssembly:  deployAssembly,
	}, nil
}

func readDeployAssemblyDescriptorFile(path string) (releaseassets.DeployAssemblyDescriptor, error) {
	var descriptor releaseassets.DeployAssemblyDescriptor
	if err := readJSONFile(path, &descriptor); err != nil {
		return releaseassets.DeployAssemblyDescriptor{}, fmt.Errorf("read deploy assembly descriptor: %w", err)
	}
	return descriptor, nil
}

func validateDeployAssemblyDescriptor(
	descriptor releaseassets.DeployAssemblyDescriptor,
	releaseManifest releaseassets.ReleaseManifest,
) error {
	if descriptor.Kind != releaseassets.DeployAssemblyManifestKind {
		return fmt.Errorf("unexpected deploy assembly kind %q", descriptor.Kind)
	}
	if descriptor.SchemaVersion != releaseassets.DeployAssemblyManifestSchemaVersion {
		return fmt.Errorf("unsupported deploy assembly schema %d", descriptor.SchemaVersion)
	}
	if descriptor.Release.Name != lesserReleaseName {
		return fmt.Errorf("unexpected deploy assembly release name %q", descriptor.Release.Name)
	}
	if descriptor.Release.Version != releaseManifest.Version {
		return fmt.Errorf(
			"deploy assembly version %q does not match release manifest %q",
			descriptor.Release.Version,
			releaseManifest.Version,
		)
	}
	if descriptor.Release.GitSHA != releaseManifest.GitSHA {
		return fmt.Errorf(
			"deploy assembly git SHA %q does not match release manifest %q",
			descriptor.Release.GitSHA,
			releaseManifest.GitSHA,
		)
	}
	if descriptor.Assembly.Path != releaseManifest.Artifacts.DeployArtifacts.DeployAssembly.Path {
		return fmt.Errorf("deploy assembly archive path %q does not match release manifest", descriptor.Assembly.Path)
	}
	if descriptor.Assembly.Format != tarGzFormat {
		return fmt.Errorf("unexpected deploy assembly archive format %q", descriptor.Assembly.Format)
	}
	if strings.TrimSpace(descriptor.Payload.Kind) == "" {
		return fmt.Errorf("deploy assembly payload kind is required")
	}
	if descriptor.Payload.ContractVersion <= 0 {
		return fmt.Errorf("deploy assembly payload contract version must be positive")
	}
	entrypoint, err := normalizeReleaseAssetPath(descriptor.Payload.Entrypoint)
	if err != nil {
		return fmt.Errorf("deploy assembly payload entrypoint %q: %w", descriptor.Payload.Entrypoint, err)
	}
	if entrypoint != descriptor.Payload.Entrypoint {
		return fmt.Errorf("deploy assembly payload entrypoint %q is not normalized", descriptor.Payload.Entrypoint)
	}
	if descriptor.Compatibility.ReleaseManifestPath != releaseassets.ReleaseManifestName {
		return fmt.Errorf(
			"unexpected deploy assembly release manifest path %q",
			descriptor.Compatibility.ReleaseManifestPath,
		)
	}
	if descriptor.Compatibility.DeployArtifactsKey != "deploy_assembly" {
		return fmt.Errorf(
			"unexpected deploy assembly compatibility key %q",
			descriptor.Compatibility.DeployArtifactsKey,
		)
	}
	if descriptor.Compatibility.ExecutorContractVersion <= 0 {
		return fmt.Errorf("deploy assembly executor contract version must be positive")
	}
	if len(descriptor.InstanceInputs.Required) == 0 {
		return fmt.Errorf("deploy assembly required instance inputs must not be empty")
	}
	if len(descriptor.Verification.IntegrityRequired) == 0 {
		return fmt.Errorf("deploy assembly integrity requirements must not be empty")
	}
	return nil
}
