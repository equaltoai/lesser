package graph

import (
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
)

func toGraphQLSoulInventoryItem(soul soulservice.Soul) *model.SoulInventoryItem {
	bindingState := model.SoulBindingStateUnbound
	available := !soul.Bound
	var binding *model.SoulAgentBinding

	if soul.Bound {
		bindingState = model.SoulBindingStateBound
		binding = &model.SoulAgentBinding{
			AgentUsername:    strings.TrimSpace(soul.BoundAgentUsername),
			PrincipalAddress: optionalString(soul.BoundPrincipalAddress),
			BoundAt:          model.Time(soul.BoundAt),
			UpdatedAt:        model.Time(soul.BoundUpdatedAt),
		}
	}

	return &model.SoulInventoryItem{
		Agent: &model.SoulAgentIdentity{
			AgentID:                strings.TrimSpace(soul.AgentID),
			Domain:                 strings.TrimSpace(soul.Domain),
			LocalID:                strings.TrimSpace(soul.LocalID),
			EnsName:                optionalTrimmedStringPtr(soul.ENSName),
			Wallet:                 strings.TrimSpace(soul.Wallet),
			PrincipalAddress:       optionalString(soul.PrincipalAddress),
			Status:                 strings.TrimSpace(soul.Status),
			LifecycleStatus:        optionalString(soul.LifecycleStatus),
			SelfDescriptionVersion: soul.SelfDescriptionVersion,
			Capabilities:           append([]string{}, soul.Capabilities...),
			MintTxHash:             optionalString(soul.MintTxHash),
			MintedAt:               optionalTimePtr(soul.MintedAt),
			UpdatedAt:              optionalTimePtr(soul.UpdatedAt),
		},
		BindingState:              bindingState,
		AvailableForIncorporation: available,
		Binding:                   binding,
	}
}

func optionalTrimmedStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return optionalString(*value)
}

func optionalTimePtr(value *time.Time) *model.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	t := model.Time(*value)
	return &t
}
