package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"
)

func TestFirstNonNilString(t *testing.T) {
	require.Nil(t, firstNonNilString(nil, aws.String(""), nil))

	value := "x"
	require.Equal(t, &value, firstNonNilString(nil, aws.String("x")))
}

type fakeSTSClient struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (f *fakeSTSClient) GetCallerIdentity(_ context.Context, _ *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

func TestResolveAWSAccountID(t *testing.T) {
	prev := newSTSClientFn
	t.Cleanup(func() { newSTSClientFn = prev })

	t.Run("success trims", func(t *testing.T) {
		newSTSClientFn = func(aws.Config) stsAPI {
			return &fakeSTSClient{out: &sts.GetCallerIdentityOutput{Account: aws.String(" 123 ")}}
		}

		accountID, err := resolveAWSAccountID(context.Background(), aws.Config{})
		require.NoError(t, err)
		require.Equal(t, "123", accountID)
	})

	t.Run("error wraps", func(t *testing.T) {
		newSTSClientFn = func(aws.Config) stsAPI {
			return &fakeSTSClient{err: errors.New("boom")}
		}

		_, err := resolveAWSAccountID(context.Background(), aws.Config{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "sts:GetCallerIdentity")
	})

	t.Run("empty account is error", func(t *testing.T) {
		newSTSClientFn = func(aws.Config) stsAPI {
			return &fakeSTSClient{out: &sts.GetCallerIdentityOutput{Account: aws.String("  ")}}
		}

		_, err := resolveAWSAccountID(context.Background(), aws.Config{})
		require.Error(t, err)
	})
}

type fakeRoute53Client struct {
	outputs []*route53.ListHostedZonesByNameOutput
	err     error

	inputs []*route53.ListHostedZonesByNameInput
	calls  int
}

func (f *fakeRoute53Client) ListHostedZonesByName(_ context.Context, input *route53.ListHostedZonesByNameInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return nil, f.err
	}
	if f.calls >= len(f.outputs) {
		return &route53.ListHostedZonesByNameOutput{}, nil
	}
	out := f.outputs[f.calls]
	f.calls++
	return out, nil
}

func TestResolveHostedZone(t *testing.T) {
	prev := newRoute53ClientFn
	t.Cleanup(func() { newRoute53ClientFn = prev })

	t.Run("single public match", func(t *testing.T) {
		fake := &fakeRoute53Client{
			outputs: []*route53.ListHostedZonesByNameOutput{{
				HostedZones: []route53types.HostedZone{{
					Id:   aws.String("/hostedzone/Z1"),
					Name: aws.String("example.com."),
				}},
				IsTruncated: false,
			}},
		}
		newRoute53ClientFn = func(aws.Config) route53API { return fake }

		zone, err := resolveHostedZone(context.Background(), aws.Config{}, "example.com")
		require.NoError(t, err)
		require.Equal(t, "Z1", zone.ID)
		require.Equal(t, "example.com", zone.Name)
		require.Len(t, fake.inputs, 1)
		require.Equal(t, aws.String("example.com."), fake.inputs[0].DNSName)
	})

	t.Run("skips private zone", func(t *testing.T) {
		newRoute53ClientFn = func(aws.Config) route53API {
			return &fakeRoute53Client{
				outputs: []*route53.ListHostedZonesByNameOutput{{
					HostedZones: []route53types.HostedZone{{
						Id:   aws.String("/hostedzone/Z1"),
						Name: aws.String("example.com."),
						Config: &route53types.HostedZoneConfig{
							PrivateZone: true,
						},
					}},
					IsTruncated: false,
				}},
			}
		}

		_, err := resolveHostedZone(context.Background(), aws.Config{}, "example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "no public Route53 hosted zone found")
	})

	t.Run("multiple matches is error", func(t *testing.T) {
		newRoute53ClientFn = func(aws.Config) route53API {
			return &fakeRoute53Client{
				outputs: []*route53.ListHostedZonesByNameOutput{{
					HostedZones: []route53types.HostedZone{
						{Id: aws.String("/hostedzone/Z1"), Name: aws.String("example.com.")},
						{Id: aws.String("/hostedzone/Z2"), Name: aws.String("example.com.")},
					},
					IsTruncated: false,
				}},
			}
		}

		_, err := resolveHostedZone(context.Background(), aws.Config{}, "example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "multiple public Route53 hosted zones")
	})

	t.Run("paginates", func(t *testing.T) {
		nextName := aws.String("example.com.")
		nextID := aws.String("ZNEXT")
		fake := &fakeRoute53Client{
			outputs: []*route53.ListHostedZonesByNameOutput{
				{
					HostedZones:      nil,
					IsTruncated:      true,
					NextDNSName:      nextName,
					NextHostedZoneId: nextID,
				},
				{
					HostedZones: []route53types.HostedZone{{
						Id:   aws.String("/hostedzone/ZFINAL"),
						Name: aws.String("example.com."),
					}},
					IsTruncated: false,
				},
			},
		}
		newRoute53ClientFn = func(aws.Config) route53API { return fake }

		zone, err := resolveHostedZone(context.Background(), aws.Config{}, "example.com.")
		require.NoError(t, err)
		require.Equal(t, "ZFINAL", zone.ID)
		require.Len(t, fake.inputs, 2)
		require.Equal(t, aws.String("example.com."), fake.inputs[0].DNSName)
		require.Equal(t, nextName, fake.inputs[1].DNSName)
		require.Equal(t, nextID, fake.inputs[1].HostedZoneId)
	})

	t.Run("list error", func(t *testing.T) {
		newRoute53ClientFn = func(aws.Config) route53API {
			return &fakeRoute53Client{err: errors.New("oops")}
		}

		_, err := resolveHostedZone(context.Background(), aws.Config{}, "example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "route53:ListHostedZonesByName")
	})

	t.Run("stops pagination when names exceed target", func(t *testing.T) {
		fake := &fakeRoute53Client{
			outputs: []*route53.ListHostedZonesByNameOutput{{
				HostedZones: []route53types.HostedZone{{
					Id:   aws.String("/hostedzone/Z1"),
					Name: aws.String("zzz.com."),
				}},
				IsTruncated: true,
			}},
		}
		newRoute53ClientFn = func(aws.Config) route53API { return fake }

		_, err := resolveHostedZone(context.Background(), aws.Config{}, "example.com")
		require.Error(t, err)
		require.Contains(t, err.Error(), "no public Route53 hosted zone found")
		require.Len(t, fake.inputs, 1)
	})
}
