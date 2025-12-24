package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type hostedZone struct {
	ID   string
	Name string
}

func resolveAWSAccountID(ctx context.Context, cfg aws.Config) (string, error) {
	client := sts.NewFromConfig(cfg)
	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("sts:GetCallerIdentity: %w", err)
	}
	accountID := strings.TrimSpace(aws.ToString(out.Account))
	if accountID == "" {
		return "", errors.New("sts:GetCallerIdentity returned empty account ID")
	}
	return accountID, nil
}

func resolveHostedZone(ctx context.Context, cfg aws.Config, baseDomain string) (hostedZone, error) {
	dnsName := strings.TrimSuffix(strings.TrimSpace(baseDomain), ".") + "."
	client := route53.NewFromConfig(cfg)

	var found []hostedZone
	var startDNSName *string
	var startHostedZoneID *string

	for {
		out, err := client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
			DNSName:      firstNonNilString(startDNSName, aws.String(dnsName)),
			HostedZoneId: startHostedZoneID,
			MaxItems:     aws.Int32(100),
		})
		if err != nil {
			return hostedZone{}, fmt.Errorf("route53:ListHostedZonesByName: %w", err)
		}

		seenBeyondTarget := false
		for _, zone := range out.HostedZones {
			name := aws.ToString(zone.Name)
			if name > dnsName {
				seenBeyondTarget = true
				break
			}
			if name != dnsName {
				continue
			}
			if zone.Config != nil && zone.Config.PrivateZone {
				continue
			}
			id := strings.TrimPrefix(aws.ToString(zone.Id), "/hostedzone/")
			found = append(found, hostedZone{ID: id, Name: strings.TrimSuffix(name, ".")})
		}

		if !out.IsTruncated || seenBeyondTarget {
			break
		}

		startDNSName = out.NextDNSName
		startHostedZoneID = out.NextHostedZoneId
	}

	if len(found) == 0 {
		return hostedZone{}, fmt.Errorf("no public Route53 hosted zone found for %q", baseDomain)
	}
	if len(found) > 1 {
		return hostedZone{}, fmt.Errorf("multiple public Route53 hosted zones found for %q; unable to choose automatically", baseDomain)
	}
	return found[0], nil
}

func firstNonNilString(candidates ...*string) *string {
	for _, candidate := range candidates {
		if candidate != nil && strings.TrimSpace(*candidate) != "" {
			return candidate
		}
	}
	return nil
}
