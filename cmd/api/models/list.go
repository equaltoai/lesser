package models

// List represents a Mastodon-compatible list
type List struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	RepliesPolicy string `json:"replies_policy"` // "followed", "list", or "none"
}

// CreateListRequest represents a request to create a list
type CreateListRequest struct {
	Title         string `json:"title"`
	RepliesPolicy string `json:"replies_policy,omitempty"` // defaults to "list"
}

// UpdateListRequest represents a request to update a list
type UpdateListRequest struct {
	Title         string `json:"title,omitempty"`
	RepliesPolicy string `json:"replies_policy,omitempty"`
}

// AddAccountsRequest represents a request to add accounts to a list
type AddAccountsRequest struct {
	AccountIDs []string `json:"account_ids"`
}

// RemoveAccountsRequest represents a request to remove accounts from a list
type RemoveAccountsRequest struct {
	AccountIDs []string `json:"account_ids"`
}
