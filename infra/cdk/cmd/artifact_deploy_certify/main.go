// Package main certifies that synth consumes the provided release lambda assets.
package main

import (
	"flag"
	"fmt"
	"os"

	"cdk/stacks"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("artifact_deploy_certify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var assetRoot string
	fs.StringVar(&assetRoot, "asset-root", "", "root directory containing bin/*.zip lambda assets")
	if err := fs.Parse(argv); err != nil {
		return 2
	}

	if err := stacks.VerifyLambdaAssetRootWithSynth(assetRoot); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}

	fmt.Println("✓ synth uses the provided lambda asset root")
	return 0
}
