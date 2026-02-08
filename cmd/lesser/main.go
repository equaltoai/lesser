package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(runCLI(os.Args, os.Stderr))
}

var (
	runUpFn           = runUp
	runDownFn         = runDown
	runClientDeployFn = runClientDeploy
	runInitAdminFn    = runInitAdmin
	runBuildFn        = runBuild
	runGenerateFn     = runGenerate
	runVerifyFn       = runVerify
	runTestFn         = runTest
	runCoverageFn     = runCoverage
	runDevFn          = runDev
	runFmtFn          = runFmt
	runLintFn         = runLint
	runSecScanFn      = runSecScan
	runVulnCheckFn    = runVulnCheck
	runGqlgenFn       = runGqlgen
	runTidyFn         = runTidy
	runSchemaFn       = runSchema
	runExportSchemaFn = runExportSchema
	runLogsFn         = runLogs
	runMetricsFn      = runMetrics
	runErrorsFn       = runErrors
	runDashboardFn    = runDashboard
	runSmokeFn        = runSmoke
)

func runCLI(args []string, stderr io.Writer) int {
	if len(args) < 2 {
		printUsageTo(stderr)
		return 2
	}

	switch args[1] {
	case "up":
		return exitCodeFromErr(runUpFn(args[2:]), stderr)
	case "down", "destroy":
		return exitCodeFromErr(runDownFn(args[2:]), stderr)
	case "init-admin":
		return exitCodeFromErr(runInitAdminFn(args[2:]), stderr)
	case "client":
		if len(args) < 3 {
			printUsageTo(stderr)
			return 2
		}
		switch args[2] {
		case "deploy":
			return exitCodeFromErr(runClientDeployFn(args[3:]), stderr)
		case helpFlagShort, helpFlagLong, helpCommand:
			printUsageTo(stderr)
			return 0
		default:
			printUsageTo(stderr)
			_, _ = fmt.Fprintln(stderr, "\nUnknown client command:", args[2])
			return 2
		}
	case "build":
		return exitCodeFromErr(runBuildFn(args[2:]), stderr)
	case "generate":
		return exitCodeFromErr(runGenerateFn(args[2:]), stderr)
	case "verify":
		return exitCodeFromErr(runVerifyFn(args[2:]), stderr)
	case "test":
		return exitCodeFromErr(runTestFn(args[2:]), stderr)
	case "coverage":
		return exitCodeFromErr(runCoverageFn(args[2:]), stderr)
	case valueDev:
		return exitCodeFromErr(runDevFn(args[2:]), stderr)
	case "fmt":
		return exitCodeFromErr(runFmtFn(args[2:]), stderr)
	case "lint":
		return exitCodeFromErr(runLintFn(args[2:]), stderr)
	case "sec-scan":
		return exitCodeFromErr(runSecScanFn(args[2:]), stderr)
	case "vuln-check":
		return exitCodeFromErr(runVulnCheckFn(args[2:]), stderr)
	case "gqlgen":
		return exitCodeFromErr(runGqlgenFn(args[2:]), stderr)
	case "tidy":
		return exitCodeFromErr(runTidyFn(args[2:]), stderr)
	case valueSchema:
		return exitCodeFromErr(runSchemaFn(args[2:]), stderr)
	case "export-schema":
		return exitCodeFromErr(runExportSchemaFn(args[2:]), stderr)
	case "logs":
		return exitCodeFromErr(runLogsFn(args[2:]), stderr)
	case "metrics":
		return exitCodeFromErr(runMetricsFn(args[2:]), stderr)
	case "errors":
		return exitCodeFromErr(runErrorsFn(args[2:]), stderr)
	case "dashboard":
		return exitCodeFromErr(runDashboardFn(args[2:]), stderr)
	case "smoke":
		return exitCodeFromErr(runSmokeFn(args[2:]), stderr)
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsageTo(stderr)
		return 0
	default:
		printUsageTo(stderr)
		_, _ = fmt.Fprintln(stderr, "\nUnknown command:", args[1])
		return 2
	}
}

func printUsage() {
	printUsageTo(os.Stderr)
}

func printUsageTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  lesser up --app <slug> --base-domain <example.com> [--aws-profile <profile>] [--stage dev|staging|live] [--provisioning-input <path>] [--bootstrap-wallet-address <0x...>] [--with-staging] [--out <path>] [--rebuild-lambdas]")
	_, _ = fmt.Fprintln(w, "  lesser init-admin --app <slug> --base-domain <example.com> [--aws-profile <profile>] --stage dev|staging|live [--provisioning-input <path>] --wallet-address <0x...> --signature <0x...> [--message <string> | --message-file <path>] [--username <username>] [--chain-id <n>]")
	_, _ = fmt.Fprintln(w, "  lesser down --app <slug> --base-domain <example.com> --aws-profile <profile> [--state <path>] [--purge-artifacts]")
	_, _ = fmt.Fprintln(w, "  lesser client deploy --app <slug> --base-domain <example.com> --aws-profile <profile> --dist <dir> [--stage dev|live|staging|both|all] [--state <path>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "  lesser build [--rebuild-lambdas]                # rebuild deployment artifacts (lambdas, auth-ui, go build)")
	_, _ = fmt.Fprintln(w, "  lesser build lambdas [--rebuild]               # (re)build bin/*.zip")
	_, _ = fmt.Fprintln(w, "  lesser build lambda <name>                     # build a single bin/<name>.zip")
	_, _ = fmt.Fprintln(w, "  lesser generate openapi|graphql-coverage|inventory|schema")
	_, _ = fmt.Fprintln(w, "  lesser schema [--out <path>] | lesser export-schema")
	_, _ = fmt.Fprintln(w, "  lesser verify [all|ci|docs|ai-training|schema|audit|supply-chain|graphql-coverage|openapi|inventory|lambda-set|unit|smoke|cdk] [--smoke] [--cdk]")
	_, _ = fmt.Fprintln(w, "  lesser test [all|unit|integration|race]")
	_, _ = fmt.Fprintln(w, "  lesser test coverage [--scope all|pkg] [--exclude-generated=true|false] [--include-testing] [--include-tools]")
	_, _ = fmt.Fprintln(w, "  lesser coverage scoreboard [--profile <path>] [--mode package|file] [--package <prefix>] [--top <n>] [--min-total <pct>] [--exclude-generated=true|false]")
	_, _ = fmt.Fprintln(w, "  lesser dev [init|dynamodb|seed-and-validate]   # local development")
	_, _ = fmt.Fprintln(w, "  lesser fmt | lesser lint [--fix] | lesser tidy")
	_, _ = fmt.Fprintln(w, "  lesser sec-scan | lesser vuln-check | lesser gqlgen")
	_, _ = fmt.Fprintln(w, "  lesser logs --app <slug> --function <name> [--env dev|staging|live] [--aws-profile <profile>]")
	_, _ = fmt.Fprintln(w, "  lesser metrics|errors|dashboard --app <slug> [--env dev|staging|live] [--aws-profile <profile>] [--region <aws-region>]")
	_, _ = fmt.Fprintln(w, "  lesser smoke core|federation [flags...]")
}

func exitCodeFromErr(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "Error:", err)
	return 1
}
