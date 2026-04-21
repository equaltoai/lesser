package routing

import (
	"context"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// validateActivity preserves the legacy test seam while production code uses
// the actor/shared-inbox specific validation paths directly.
func (ih *InboxHandler) validateActivity(activity *activitypub.Activity, actor *activitypub.Actor) error {
	if err := ih.validateBasicActivity(activity); err != nil {
		return err
	}
	if err := ih.validateBasicActor(actor); err != nil {
		return err
	}
	if err := ih.validateActivityAddressing(activity); err != nil {
		return err
	}
	if err := ih.validateActorUsername(activity.Actor); err != nil {
		return err
	}
	if err := ih.validateActorPublicKey(actor); err != nil {
		return err
	}
	if err := ih.validateCreateActivityObject(activity); err != nil {
		return err
	}
	if err := ih.validateComprehensiveAddressing(activity); err != nil {
		return err
	}
	return ih.validateActivityTargeting(activity, actor)
}

// validateAddressingAndPrivacy preserves the legacy test seam while production
// code uses validateActorInboxAddressingAndPrivacy / validateSharedInboxAddressingAndPrivacy.
func (ih *InboxHandler) validateAddressingAndPrivacy(_ any, req *InboxRequest) error {
	return ih.validateActorInboxAddressingAndPrivacy(req)
}

// processActivityByType preserves the legacy test seam while production code
// routes directly to processActivityByTypeForTarget.
func (ih *InboxHandler) processActivityByType(ctx context.Context, req *InboxRequest) error {
	targetActor := req.Actor
	if targetActor == nil && len(req.TargetActors) > 0 {
		targetActor = req.TargetActors[0]
	}
	return ih.processActivityByTypeForTarget(ctx, req.Activity, targetActor, req.CostParams)
}
