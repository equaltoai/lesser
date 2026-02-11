package main

import "testing"

func TestCLIAPIClient_Close_IsNilSafe(t *testing.T) {
	var nilClient *cliAPIClient
	nilClient.Close()

	(&cliAPIClient{}).Close()
}
