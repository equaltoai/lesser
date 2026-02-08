package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// managedProvisioningInput is a small, versioned contract that can be passed to
// the deploy runner to avoid long flag lists.
type managedProvisioningInput struct {
	Schema             int    `json:"schema"`
	Slug               string `json:"slug"`
	Stage              string `json:"stage"`
	AdminWalletAddress string `json:"admin_wallet_address"`
	AdminUsername      string `json:"admin_username"`
	AdminWalletChainID int    `json:"admin_wallet_chain_id,omitempty"`
	ConsentMessage     string `json:"consent_message,omitempty"`
	ConsentSignature   string `json:"consent_signature,omitempty"`
}

func readManagedProvisioningInput(path string) (managedProvisioningInput, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return managedProvisioningInput{}, errors.New("managed provisioning input path is empty")
	}

	data, err := os.ReadFile(path) // #nosec G304 -- CLI reads operator/runner-provided local path
	if err != nil {
		return managedProvisioningInput{}, fmt.Errorf("read managed provisioning input: %w", err)
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return managedProvisioningInput{}, errors.New("managed provisioning input is empty")
	}

	var in managedProvisioningInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return managedProvisioningInput{}, fmt.Errorf("parse managed provisioning input: %w", err)
	}

	if in.Schema == 0 {
		in.Schema = 1
	}
	if in.Schema != 1 {
		return managedProvisioningInput{}, fmt.Errorf("unsupported managed provisioning input schema %d (expected 1)", in.Schema)
	}

	in.Slug = strings.TrimSpace(in.Slug)
	in.Stage = strings.TrimSpace(in.Stage)
	in.AdminWalletAddress = strings.TrimSpace(in.AdminWalletAddress)
	in.AdminUsername = strings.TrimSpace(in.AdminUsername)
	in.ConsentMessage = strings.TrimSpace(in.ConsentMessage)
	in.ConsentSignature = strings.TrimSpace(in.ConsentSignature)
	if in.AdminUsername == "" {
		in.AdminUsername = in.Slug
	}

	if in.Slug == "" || in.Stage == "" || in.AdminWalletAddress == "" {
		return managedProvisioningInput{}, errors.New("managed provisioning input is missing required fields: slug, stage, admin_wallet_address")
	}

	return in, nil
}
