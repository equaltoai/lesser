// Command release_assets builds the immutable release asset set published for
// Lesser releases.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/equaltoai/lesser/pkg/releaseassets"
)

func main() {
	var (
		repoRoot             string
		outDir               string
		version              string
		gitSHA               string
		goVersion            string
		cdkMajor             int
		receiptSchemaVersion int
	)

	flag.StringVar(&repoRoot, "repo-root", ".", "repository root containing bin/ and infra/cdk/inventory/lambdas.go")
	flag.StringVar(&outDir, "out-dir", filepath.Join("dist", "release"), "directory where release assets are written")
	flag.StringVar(&version, "version", "", "release version tag (example: v1.2.3)")
	flag.StringVar(&gitSHA, "git-sha", "", "40-character git sha for the release commit")
	flag.StringVar(&goVersion, "go-version", "", "Go toolchain version used for the release (example: go1.26.2)")
	flag.IntVar(&cdkMajor, "cdk-major", 0, "AWS CDK major version for the release")
	flag.IntVar(&receiptSchemaVersion, "receipt-schema-version", 0, "receipt schema version published in the release manifest")
	flag.Parse()

	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	files, err := releaseassets.WriteLambdaBundle(absRepoRoot, outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := releaseassets.WriteAuthUIBundle(absRepoRoot, outDir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if version == "" || gitSHA == "" || goVersion == "" || cdkMajor == 0 || receiptSchemaVersion == 0 {
		fmt.Fprintln(os.Stderr, "error: version, git-sha, go-version, cdk-major, and receipt-schema-version are required")
		os.Exit(2)
	}

	if _, err := releaseassets.WriteLambdaBundleManifest(outDir, version, gitSHA, files); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if _, err := releaseassets.WriteDeployAssembly(absRepoRoot, outDir, version, gitSHA); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if _, err := releaseassets.WriteReleaseManifest(outDir, releaseassets.ReleaseManifestInput{
		Version:              version,
		GitSHA:               gitSHA,
		GoVersion:            goVersion,
		CDKMajor:             cdkMajor,
		ReceiptSchemaVersion: receiptSchemaVersion,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := releaseassets.WriteChecksums(outDir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s with %d Lambda artifacts\n", filepath.Join(outDir, releaseassets.LambdaBundleArchiveName), len(files))
}
