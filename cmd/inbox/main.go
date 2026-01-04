// Package main provides the Lambda entrypoint for the inbox handler.
package main

import "github.com/equaltoai/lesser/cmd/inbox/internal/routing"

func main() {
	routing.Run()
}
