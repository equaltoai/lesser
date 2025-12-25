package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

type upReceipt struct {
	Version       int                      `json:"version"`
	App           string                   `json:"app"`
	BaseDomain    string                   `json:"base_domain"`
	AWSProfile    string                   `json:"aws_profile"`
	AccountID     string                   `json:"account_id"`
	Region        string                   `json:"region"`
	HostedZone    hostedZoneReceipt        `json:"hosted_zone"`
	SharedStack   string                   `json:"shared_stack"`
	SharedOutputs map[string]string        `json:"shared_outputs,omitempty"`
	Stages        map[string]*stageReceipt `json:"stages"`
	CreatedAt     time.Time                `json:"created_at"`
}

type hostedZoneReceipt struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type stageReceipt struct {
	Stage            string            `json:"stage"`
	Domain           string            `json:"domain"`
	StackName        string            `json:"stack_name"`
	TableName        string            `json:"table_name"`
	BootstrapAddress string            `json:"bootstrap_address"`
	Locked           bool              `json:"locked"`
	URLs             map[string]string `json:"urls"`
	StackOutputs     map[string]string `json:"stack_outputs,omitempty"`
	BootstrappedAt   time.Time         `json:"bootstrapped_at"`
}

func newUpReceipt(app, baseDomain, awsProfile, accountID, region string, stages []naming.Stage, hz hostedZone) *upReceipt {
	stageMap := map[string]*stageReceipt{}
	for _, stage := range stages {
		stageMap[string(stage)] = &stageReceipt{
			Stage: string(stage),
		}
	}

	return &upReceipt{
		Version:     1,
		App:         app,
		BaseDomain:  baseDomain,
		AWSProfile:  awsProfile,
		AccountID:   accountID,
		Region:      region,
		HostedZone:  hostedZoneReceipt(hz),
		SharedStack: naming.SharedStackName(app),
		Stages:      stageMap,
		CreatedAt:   time.Now().UTC(),
	}
}

func writeReceipt(path string, receipt *upReceipt) error {
	if receipt == nil {
		return fmt.Errorf("receipt is nil")
	}

	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readReceipt(path string) (*upReceipt, error) {
	data, err := os.ReadFile(path) //nolint:gosec // CLI reads an operator-provided local receipt path
	if err != nil {
		return nil, fmt.Errorf("read receipt %s: %w", path, err)
	}

	var receipt upReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, fmt.Errorf("parse receipt %s: %w", path, err)
	}

	if strings.TrimSpace(receipt.App) == "" || strings.TrimSpace(receipt.BaseDomain) == "" {
		return nil, fmt.Errorf("receipt %s is missing required fields", path)
	}

	return &receipt, nil
}
