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
			TokenID:                optionalString(soul.TokenID),
			MetaURI:                optionalString(soul.MetaURI),
			Avatar:                 toGraphQLSoulAvatar(soul.Avatar),
			PrincipalAddress:       optionalString(soul.PrincipalAddress),
			PrincipalSignature:     optionalString(soul.PrincipalSignature),
			PrincipalDeclaration:   optionalString(soul.PrincipalDeclaration),
			PrincipalDeclaredAt:    optionalString(soul.PrincipalDeclaredAt),
			Status:                 strings.TrimSpace(soul.Status),
			LifecycleStatus:        optionalString(soul.LifecycleStatus),
			LifecycleReason:        optionalString(soul.LifecycleReason),
			SuccessorAgentID:       optionalString(soul.SuccessorAgentID),
			PredecessorAgentID:     optionalString(soul.PredecessorAgentID),
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

func toGraphQLSoulAvatar(value *soulservice.SoulAvatar) *model.SoulAgentAvatar {
	if value == nil {
		return nil
	}

	styles := make([]*model.SoulAgentAvatarStyle, 0, len(value.Styles))
	for _, style := range value.Styles {
		styles = append(styles, &model.SoulAgentAvatarStyle{
			StyleID:         style.StyleID,
			StyleName:       optionalString(style.StyleName),
			RendererAddress: optionalString(style.RendererAddress),
			Image:           optionalString(style.Image),
			Selected:        style.Selected,
		})
	}

	return &model.SoulAgentAvatar{
		TokenURI:               optionalString(value.TokenURI),
		Image:                  optionalString(value.Image),
		CurrentStyleID:         value.CurrentStyleID,
		CurrentStyleName:       optionalString(value.CurrentStyleName),
		CurrentRendererAddress: optionalString(value.CurrentRendererAddress),
		Styles:                 styles,
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
