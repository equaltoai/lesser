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
		if err := runUp(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case "client":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(2)
		}
		switch os.Args[2] {
		case "deploy":
			if err := runClientDeploy(os.Args[3:]); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		case "-h", "--help", "help":
			printUsage()
			return
		default:
			printUsage()
			fmt.Fprintln(os.Stderr, "\nUnknown client command:", os.Args[2])
			os.Exit(2)
		}
	case "-h", "--help", "help":
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
}
