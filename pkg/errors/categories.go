package errors

// ErrorCategory represents the domain/category of an error
type ErrorCategory string

const (
	// CategoryAuth represents authentication and authorization errors
	CategoryAuth ErrorCategory = "AUTH"

	// CategoryStorage represents database and storage errors
	CategoryStorage ErrorCategory = "STORAGE"

	// CategoryFederation represents ActivityPub federation errors
	CategoryFederation ErrorCategory = "FEDERATION"

	// CategoryValidation represents input validation errors
	CategoryValidation ErrorCategory = "VALIDATION"

	// CategoryAPI represents API-specific errors (REST/GraphQL)
	CategoryAPI ErrorCategory = "API"

	// CategoryLambda represents AWS Lambda-specific errors
	CategoryLambda ErrorCategory = "LAMBDA"

	// CategoryBusiness represents business logic errors
	CategoryBusiness ErrorCategory = "BUSINESS"

	// CategoryMedia represents media processing errors
	CategoryMedia ErrorCategory = "MEDIA"

	// CategoryStreaming represents WebSocket streaming errors
	CategoryStreaming ErrorCategory = "STREAMING"

	// CategoryModeration represents content moderation errors
	CategoryModeration ErrorCategory = "MODERATION"

	// CategoryInternal represents internal system errors
	CategoryInternal ErrorCategory = "INTERNAL"

	// CategoryExternal represents external service errors
	CategoryExternal ErrorCategory = "EXTERNAL"
)

// String returns the string representation of the error category
func (c ErrorCategory) String() string {
	return string(c)
}

// IsValid checks if the error category is valid
func (c ErrorCategory) IsValid() bool {
	switch c {
	case CategoryAuth, CategoryStorage, CategoryFederation, CategoryValidation,
		CategoryAPI, CategoryLambda, CategoryBusiness, CategoryMedia,
		CategoryStreaming, CategoryModeration, CategoryInternal, CategoryExternal:
		return true
	default:
		return false
	}
}

// AllCategories returns all valid error categories
func AllCategories() []ErrorCategory {
	return []ErrorCategory{
		CategoryAuth,
		CategoryStorage,
		CategoryFederation,
		CategoryValidation,
		CategoryAPI,
		CategoryLambda,
		CategoryBusiness,
		CategoryMedia,
		CategoryStreaming,
		CategoryModeration,
		CategoryInternal,
		CategoryExternal,
	}
}