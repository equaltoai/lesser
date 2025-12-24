package main

import (
	"fmt"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func stageURLs(stage naming.Stage, baseDomain string) map[string]string {
	stageDomain := naming.StageDomain(stage, baseDomain)

	return map[string]string{
		"setup":        fmt.Sprintf("https://auth.%s/setup", stageDomain),
		"setup_status": fmt.Sprintf("https://api.%s/setup/status", stageDomain),
		"client":       fmt.Sprintf("https://%s", stageDomain),
		"api":          fmt.Sprintf("https://api.%s", stageDomain),
		"ws":           fmt.Sprintf("wss://ws.%s", stageDomain),
		"media":        fmt.Sprintf("https://media.%s", stageDomain),
	}
}

func printStageURLs(stages []naming.Stage, baseDomain string) {
	fmt.Println("\nStage URLs:")
	for _, stage := range stages {
		urls := stageURLs(stage, baseDomain)
		fmt.Printf("  %s:\n", stage)
		fmt.Printf("    setup:        %s\n", urls["setup"])
		fmt.Printf("    setup_status: %s\n", urls["setup_status"])
		fmt.Printf("    client:       %s\n", urls["client"])
		fmt.Printf("    api:          %s\n", urls["api"])
		fmt.Printf("    ws:           %s\n", urls["ws"])
	}
}
