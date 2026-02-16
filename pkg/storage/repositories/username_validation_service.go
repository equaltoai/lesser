package repositories

import "context"

type usernameValidationExtractor func(model BaseModel) (userID string, username *string, ok bool)

type usernameOptionalValidationService struct {
	base    *DefaultValidationService
	extract usernameValidationExtractor
}

func newUsernameOptionalValidationService(extract usernameValidationExtractor) ValidationService {
	return &usernameOptionalValidationService{
		base:    NewDefaultValidationService(),
		extract: extract,
	}
}

func (v *usernameOptionalValidationService) ValidateModel(ctx context.Context, model BaseModel) error {
	return v.base.ValidateModel(ctx, model)
}

func (v *usernameOptionalValidationService) ValidateBusinessRules(ctx context.Context, model BaseModel, action string) error {
	return v.base.ValidateBusinessRules(ctx, model, action)
}

func (v *usernameOptionalValidationService) ValidateRequiredFields(ctx context.Context, model BaseModel) error {
	if v.extract == nil {
		return v.base.ValidateRequiredFields(ctx, model)
	}

	userID, username, ok := v.extract(model)
	if !ok {
		return v.base.ValidateRequiredFields(ctx, model)
	}

	return validateRequiredFieldsAllowAnonymousUsername(ctx, v.base, model, userID, username)
}
