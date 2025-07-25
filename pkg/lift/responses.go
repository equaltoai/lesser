package lift

import (
	"net/http"

	"github.com/pay-theory/lift/pkg/lift"
)

// Response helpers for consistent API responses

// PaginatedResponse represents a paginated collection response
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	NextCursor string      `json:"next_cursor,omitempty"`
	PrevCursor string      `json:"prev_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
	Total      int         `json:"total,omitempty"`
}

// ListResponse represents a simple list response
type ListResponse struct {
	Data  interface{} `json:"data"`
	Count int         `json:"count"`
}

// CreatedResponse represents a resource creation response
type CreatedResponse struct {
	ID       string      `json:"id"`
	Resource interface{} `json:"resource,omitempty"`
}

// UpdatedResponse represents a resource update response
type UpdatedResponse struct {
	ID       string      `json:"id"`
	Resource interface{} `json:"resource,omitempty"`
	Updated  []string    `json:"updated_fields,omitempty"`
}

// DeletedResponse represents a resource deletion response
type DeletedResponse struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// Response helper functions

// OK returns a successful response with data
func OK(ctx *lift.Context, data interface{}) error {
	return ctx.JSON(data)
}

// Created returns a 201 response for resource creation
func Created(ctx *lift.Context, id string, resource interface{}) error {
	ctx.Status(http.StatusCreated)
	return ctx.JSON(CreatedResponse{
		ID:       id,
		Resource: resource,
	})
}

// Updated returns a response for resource updates
func Updated(ctx *lift.Context, id string, resource interface{}, updatedFields ...string) error {
	return ctx.JSON(UpdatedResponse{
		ID:       id,
		Resource: resource,
		Updated:  updatedFields,
	})
}

// Deleted returns a response for resource deletion
func Deleted(ctx *lift.Context, id string) error {
	return ctx.JSON(DeletedResponse{
		ID:      id,
		Deleted: true,
	})
}

// NoContent returns a 204 No Content response
func NoContent(ctx *lift.Context) error {
	ctx.Status(http.StatusNoContent)
	return nil
}

// Accepted returns a 202 Accepted response for async operations
func Accepted(ctx *lift.Context, data interface{}) error {
	ctx.Status(http.StatusAccepted)
	if data != nil {
		return ctx.JSON(data)
	}
	return nil
}

// List returns a list response
func List(ctx *lift.Context, data interface{}, count int) error {
	return ctx.JSON(ListResponse{
		Data:  data,
		Count: count,
	})
}

// Paginated returns a paginated response
func Paginated(ctx *lift.Context, data interface{}, nextCursor, prevCursor string, hasMore bool) error {
	return ctx.JSON(PaginatedResponse{
		Data:       data,
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
		HasMore:    hasMore,
	})
}

// PaginatedWithTotal returns a paginated response with total count
func PaginatedWithTotal(ctx *lift.Context, data interface{}, nextCursor, prevCursor string, hasMore bool, total int) error {
	return ctx.JSON(PaginatedResponse{
		Data:       data,
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
		HasMore:    hasMore,
		Total:      total,
	})
}

// ActivityPub response helpers

// ActivityPubResponse sets the correct content type and returns JSON
func ActivityPubResponse(ctx *lift.Context, data interface{}) error {
	ctx.Response.Header("Content-Type", "application/activity+json")
	return ctx.JSON(data)
}

// WebFingerResponse sets the correct content type for WebFinger
func WebFingerResponse(ctx *lift.Context, data interface{}) error {
	ctx.Response.Header("Content-Type", "application/jrd+json")
	return ctx.JSON(data)
}

// NodeInfoResponse sets the correct content type for NodeInfo
func NodeInfoResponse(ctx *lift.Context, data interface{}) error {
	ctx.Response.Header("Content-Type", "application/json")
	ctx.Response.Header("Access-Control-Allow-Origin", "*")
	return ctx.JSON(data)
}

// Redirect helpers

// Redirect returns a redirect response
func Redirect(ctx *lift.Context, url string, permanent bool) error {
	if permanent {
		ctx.Status(http.StatusMovedPermanently)
	} else {
		ctx.Status(http.StatusFound)
	}
	ctx.Response.Header("Location", url)
	return nil
}

// SeeOther returns a 303 See Other redirect
func SeeOther(ctx *lift.Context, url string) error {
	ctx.Status(http.StatusSeeOther)
	ctx.Response.Header("Location", url)
	return nil
}

// TemporaryRedirect returns a 307 Temporary Redirect
func TemporaryRedirect(ctx *lift.Context, url string) error {
	ctx.Status(http.StatusTemporaryRedirect)
	ctx.Response.Header("Location", url)
	return nil
}