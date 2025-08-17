package services

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
)

// validationService implements ValidationService
type validationService struct {
	config *ServiceConfig
}

// NewValidationService creates a new validation service
func NewValidationService(config *ServiceConfig) ValidationService {
	return &validationService{
		config: config,
	}
}

// ValidateCreatePost validates post creation input
func (v *validationService) ValidateCreatePost(input *CreatePostInput) error {
	if err := common.ValidateRequiredParam("content", input.Content); err != nil {
		return NewValidationError("Post content is required")
	}

	if len(input.Content) > 500 {
		return NewValidationError("Post content must not exceed 500 characters")
	}

	if input.Visibility != "" && !isValidVisibility(input.Visibility) {
		return NewValidationError("Invalid visibility setting")
	}

	if len(input.MediaIDs) > 4 {
		return NewValidationError("Maximum 4 media attachments allowed")
	}

	return nil
}

// ValidateFollowInput validates follow operation input
func (v *validationService) ValidateFollowInput(input *FollowInput) error {
	if err := common.ValidateRequiredParam("target_actor_id", input.TargetActorID); err != nil {
		return NewValidationError("Target actor ID is required")
	}

	if err := common.ValidateRequiredParam("target_actor_id_trimmed", strings.TrimSpace(input.TargetActorID)); err != nil {
		return NewValidationError("Target actor ID cannot be empty")
	}

	return nil
}

// ValidateLikeInput validates like operation input
func (v *validationService) ValidateLikeInput(input *LikeInput) error {
	if err := common.ValidateRequiredParam("object_id", input.ObjectID); err != nil {
		return NewValidationError("Object ID is required")
	}

	if err := common.ValidateRequiredParam("object_id_trimmed", strings.TrimSpace(input.ObjectID)); err != nil {
		return NewValidationError("Object ID cannot be empty")
	}

	return nil
}

// ValidateDeletePost validates post deletion input
func (v *validationService) ValidateDeletePost(input *DeletePostInput) error {
	if err := common.ValidateRequiredParam("object_id", input.ObjectID); err != nil {
		return NewValidationError("Object ID is required")
	}

	if err := common.ValidateRequiredParam("object_id_trimmed", strings.TrimSpace(input.ObjectID)); err != nil {
		return NewValidationError("Object ID cannot be empty")
	}

	return nil
}

// ValidateUpdatePost validates post update input
func (v *validationService) ValidateUpdatePost(input *UpdatePostInput) error {
	if err := common.ValidateRequiredParam("object_id", input.ObjectID); err != nil {
		return NewValidationError("Object ID is required")
	}

	if input.Content != "" && len(input.Content) > 500 {
		return NewValidationError("Post content must not exceed 500 characters")
	}

	if input.Visibility != "" && !isValidVisibility(input.Visibility) {
		return NewValidationError("Invalid visibility setting")
	}

	return nil
}

// Helper functions
func isValidVisibility(visibility string) bool {
	validVisibilities := map[string]bool{
		"public":   true,
		"unlisted": true,
		"private":  true,
		"direct":   true,
	}
	return validVisibilities[visibility]
}
