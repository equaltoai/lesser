package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "up":
		exitOnError(runUp(os.Args[2:]))
	case "client":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(2)
		}
		switch os.Args[2] {
		case "deploy":
			exitOnError(runClientDeploy(os.Args[3:]))
		case helpFlagShort, helpFlagLong, helpCommand:
			printUsage()
			return
		default:
			printUsage()
			fmt.Fprintln(os.Stderr, "\nUnknown client command:", os.Args[2])
			os.Exit(2)
		}
	case "build":
		exitOnError(runBuild(os.Args[2:]))
	case "generate":
		exitOnError(runGenerate(os.Args[2:]))
	case "verify":
		exitOnError(runVerify(os.Args[2:]))
	case "test":
		exitOnError(runTest(os.Args[2:]))
	case "coverage":
		exitOnError(runCoverage(os.Args[2:]))
	case valueDev:
		exitOnError(runDev(os.Args[2:]))
	case "fmt":
		exitOnError(runFmt(os.Args[2:]))
	case "lint":
		exitOnError(runLint(os.Args[2:]))
	case "sec-scan":
		exitOnError(runSecScan(os.Args[2:]))
	case "vuln-check":
		exitOnError(runVulnCheck(os.Args[2:]))
	case "gqlgen":
		exitOnError(runGqlgen(os.Args[2:]))
	case "tidy":
		exitOnError(runTidy(os.Args[2:]))
	case valueSchema:
		exitOnError(runSchema(os.Args[2:]))
	case "export-schema":
		exitOnError(runExportSchema(os.Args[2:]))
	case "logs":
		exitOnError(runLogs(os.Args[2:]))
	case "metrics":
		exitOnError(runMetrics(os.Args[2:]))
	case "errors":
		exitOnError(runErrors(os.Args[2:]))
	case "dashboard":
		exitOnError(runDashboard(os.Args[2:]))
	case "smoke":
		exitOnError(runSmoke(os.Args[2:]))
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return
	default:
		printUsage()
		fmt.Fprintln(os.Stderr, "\nUnknown command:", os.Args[1])
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  lesser up --app <slug> --base-domain <example.com> --aws-profile <profile> [--with-staging] [--out <path>] [--rebuild-lambdas]")
	fmt.Fprintln(os.Stderr, "  lesser client deploy --app <slug> --base-domain <example.com> --aws-profile <profile> --dist <dir> [--stage dev|live|staging|both|all] [--state <path>]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  lesser build [--rebuild-lambdas]                # rebuild deployment artifacts (lambdas, auth-ui, go build)")
	fmt.Fprintln(os.Stderr, "  lesser build lambdas [--rebuild]               # (re)build bin/*.zip")
	fmt.Fprintln(os.Stderr, "  lesser build lambda <name>                     # build a single bin/<name>.zip")
	fmt.Fprintln(os.Stderr, "  lesser generate openapi|graphql-coverage|inventory|schema")
	fmt.Fprintln(os.Stderr, "  lesser schema [--out <path>] | lesser export-schema")
	fmt.Fprintln(os.Stderr, "  lesser verify [all|docs|ai-training|schema|graphql-coverage|openapi|inventory|lambda-set|unit|smoke|cdk] [--smoke] [--cdk]")
	fmt.Fprintln(os.Stderr, "  lesser test [all|unit|integration|race]")
	fmt.Fprintln(os.Stderr, "  lesser test coverage [--scope all|pkg] [--include-testing] [--include-tools]")
	fmt.Fprintln(os.Stderr, "  lesser coverage scoreboard [--profile <path>] [--mode package|file] [--package <prefix>] [--top <n>]")
	fmt.Fprintln(os.Stderr, "  lesser dev [init|dynamodb|seed-and-validate]   # local development")
	fmt.Fprintln(os.Stderr, "  lesser fmt | lesser lint [--fix] | lesser tidy")
	fmt.Fprintln(os.Stderr, "  lesser sec-scan | lesser vuln-check | lesser gqlgen")
	fmt.Fprintln(os.Stderr, "  lesser logs --app <slug> --function <name> [--env dev|staging|live] [--aws-profile <profile>]")
	fmt.Fprintln(os.Stderr, "  lesser metrics|errors|dashboard --app <slug> [--env dev|staging|live] [--aws-profile <profile>] [--region <aws-region>]")
	fmt.Fprintln(os.Stderr, "  lesser smoke core|federation [flags...]")
}

func exitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
