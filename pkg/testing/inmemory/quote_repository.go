// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// QuoteRepository is a thread-safe in-memory implementation of interfaces.QuoteRepository.
type QuoteRepository struct {
	mu sync.RWMutex

	// Quote relationships: quoteStatusID_targetStatusID -> QuoteRelationship
	relationships map[string]*models.QuoteRelationship

	// Quotes by target status: targetStatusID -> []QuoteRelationship
	quotesByTarget map[string][]*models.QuoteRelationship

	// Quotes by user: userID -> []QuoteRelationship
	quotesByUser map[string][]*models.QuoteRelationship

	// Quote permissions: username -> QuotePermissions
	permissions map[string]*models.QuotePermissions

	// Quote counts: statusID -> count
	quoteCounts map[string]int64
}

// NewQuoteRepository creates a new in-memory quote repository
func NewQuoteRepository() *QuoteRepository {
	return &QuoteRepository{
		relationships:  make(map[string]*models.QuoteRelationship),
		quotesByTarget: make(map[string][]*models.QuoteRelationship),
		quotesByUser:   make(map[string][]*models.QuoteRelationship),
		permissions:    make(map[string]*models.QuotePermissions),
		quoteCounts:    make(map[string]int64),
	}
}

// ===== Quote Relationship Operations =====

// CreateQuoteRelationship creates a new quote relationship
func (r *QuoteRepository) CreateQuoteRelationship(_ context.Context, relationship *models.QuoteRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := relationship.QuoterNoteID + "_" + relationship.TargetNoteID
	if _, exists := r.relationships[key]; exists {
		return nil // Already exists, no-op
	}

	r.relationships[key] = relationship
	r.quotesByTarget[relationship.TargetNoteID] = append(r.quotesByTarget[relationship.TargetNoteID], relationship)
	r.quotesByUser[relationship.QuoterID] = append(r.quotesByUser[relationship.QuoterID], relationship)

	// Increment quote count
	r.quoteCounts[relationship.TargetNoteID]++

	return nil
}

// GetQuoteRelationship retrieves a quote relationship by quoter and target note IDs
func (r *QuoteRepository) GetQuoteRelationship(_ context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := quoteStatusID + "_" + targetStatusID
	relationship, exists := r.relationships[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return relationship, nil
}

// UpdateQuoteRelationship updates an existing quote relationship
func (r *QuoteRepository) UpdateQuoteRelationship(_ context.Context, relationship *models.QuoteRelationship) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := relationship.QuoterNoteID + "_" + relationship.TargetNoteID
	if _, exists := r.relationships[key]; !exists {
		return storage.ErrNotFound
	}

	r.relationships[key] = relationship

	// Update in target list
	for i, q := range r.quotesByTarget[relationship.TargetNoteID] {
		if q.QuoterNoteID == relationship.QuoterNoteID {
			r.quotesByTarget[relationship.TargetNoteID][i] = relationship
			break
		}
	}

	// Update in user list
	for i, q := range r.quotesByUser[relationship.QuoterID] {
		if q.QuoterNoteID == relationship.QuoterNoteID && q.TargetNoteID == relationship.TargetNoteID {
			r.quotesByUser[relationship.QuoterID][i] = relationship
			break
		}
	}

	return nil
}

// DeleteQuoteRelationship deletes a quote relationship
func (r *QuoteRepository) DeleteQuoteRelationship(_ context.Context, quoteStatusID, targetStatusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := quoteStatusID + "_" + targetStatusID
	relationship, exists := r.relationships[key]
	if !exists {
		return nil
	}

	delete(r.relationships, key)

	// Remove from target list
	var newTargetQuotes []*models.QuoteRelationship
	for _, q := range r.quotesByTarget[targetStatusID] {
		if q.QuoterNoteID != quoteStatusID {
			newTargetQuotes = append(newTargetQuotes, q)
		}
	}
	r.quotesByTarget[targetStatusID] = newTargetQuotes

	// Remove from user list
	var newUserQuotes []*models.QuoteRelationship
	for _, q := range r.quotesByUser[relationship.QuoterID] {
		if q.QuoterNoteID != quoteStatusID || q.TargetNoteID != targetStatusID {
			newUserQuotes = append(newUserQuotes, q)
		}
	}
	r.quotesByUser[relationship.QuoterID] = newUserQuotes

	// Decrement quote count
	if r.quoteCounts[targetStatusID] > 0 {
		r.quoteCounts[targetStatusID]--
	}

	return nil
}

// ===== Quote Query Operations =====

// GetQuotesForStatus retrieves quotes for a given status
func (r *QuoteRepository) GetQuotesForStatus(_ context.Context, statusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	quotes := r.quotesByTarget[statusID]

	// Filter out withdrawn quotes
	var activeQuotes []*models.QuoteRelationship
	for _, q := range quotes {
		if q.IsActive() {
			activeQuotes = append(activeQuotes, q)
		}
	}

	// Apply pagination
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	startIdx := 0
	if opts.Cursor != "" {
		for i, q := range activeQuotes {
			if q.PK+"#"+q.SK == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(activeQuotes) {
		endIdx = len(activeQuotes)
	}

	result := &interfaces.PaginatedResult[*models.QuoteRelationship]{
		Items:   activeQuotes[startIdx:endIdx],
		HasMore: endIdx < len(activeQuotes),
		Total:   int64(len(activeQuotes)),
	}

	if endIdx < len(activeQuotes) && len(result.Items) > 0 {
		lastQuote := result.Items[len(result.Items)-1]
		result.NextCursor = lastQuote.PK + "#" + lastQuote.SK
	}

	return result, nil
}

// GetQuotesByUser retrieves quotes created by a specific user
func (r *QuoteRepository) GetQuotesByUser(_ context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	quotes := r.quotesByUser[userID]

	// Filter out withdrawn quotes
	var activeQuotes []*models.QuoteRelationship
	for _, q := range quotes {
		if q.IsActive() {
			activeQuotes = append(activeQuotes, q)
		}
	}

	// Apply pagination
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	startIdx := 0
	if opts.Cursor != "" {
		for i, q := range activeQuotes {
			if q.PK+"#"+q.SK == opts.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + limit
	if endIdx > len(activeQuotes) {
		endIdx = len(activeQuotes)
	}

	result := &interfaces.PaginatedResult[*models.QuoteRelationship]{
		Items:   activeQuotes[startIdx:endIdx],
		HasMore: endIdx < len(activeQuotes),
		Total:   int64(len(activeQuotes)),
	}

	if endIdx < len(activeQuotes) && len(result.Items) > 0 {
		lastQuote := result.Items[len(result.Items)-1]
		result.NextCursor = lastQuote.PK + "#" + lastQuote.SK
	}

	return result, nil
}

// ===== Quote Permissions Operations =====

// CreateQuotePermissions creates new quote permissions for a user
func (r *QuoteRepository) CreateQuotePermissions(_ context.Context, permissions *models.QuotePermissions) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.permissions[permissions.Username]; exists {
		return nil // Already exists, no-op
	}

	r.permissions[permissions.Username] = permissions
	return nil
}

// GetQuotePermissions retrieves quote permissions for a user
func (r *QuoteRepository) GetQuotePermissions(_ context.Context, username string) (*models.QuotePermissions, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	permissions, exists := r.permissions[username]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return permissions, nil
}

// UpdateQuotePermissions updates existing quote permissions
func (r *QuoteRepository) UpdateQuotePermissions(_ context.Context, permissions *models.QuotePermissions) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.permissions[permissions.Username]; !exists {
		return storage.ErrNotFound
	}

	r.permissions[permissions.Username] = permissions
	return nil
}

// DeleteQuotePermissions deletes quote permissions for a user
func (r *QuoteRepository) DeleteQuotePermissions(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.permissions, username)
	return nil
}

// ===== Quote Count Operations =====

// GetQuoteCount gets the total number of quotes for a status
func (r *QuoteRepository) GetQuoteCount(_ context.Context, statusID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.quoteCounts[statusID], nil
}

// IncrementQuoteCount increments the quote count for a status
func (r *QuoteRepository) IncrementQuoteCount(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.quoteCounts[statusID]++
	return nil
}

// DecrementQuoteCount decrements the quote count for a status
func (r *QuoteRepository) DecrementQuoteCount(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.quoteCounts[statusID] > 0 {
		r.quoteCounts[statusID]--
	}
	return nil
}

// ===== Quote Withdrawal Operations =====

// WithdrawQuotes withdraws all quotes of a note created by a specific user
func (r *QuoteRepository) WithdrawQuotes(_ context.Context, noteID, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	now := time.Now()

	for _, quote := range r.quotesByUser[userID] {
		if quote.TargetNoteID == noteID && !quote.Withdrawn {
			quote.Withdrawn = true
			quote.WithdrawnAt = &now
			count++

			// Decrement quote count
			if r.quoteCounts[noteID] > 0 {
				r.quoteCounts[noteID]--
			}
		}
	}

	return count, nil
}

// Clear clears all data (test helper)
func (r *QuoteRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.relationships = make(map[string]*models.QuoteRelationship)
	r.quotesByTarget = make(map[string][]*models.QuoteRelationship)
	r.quotesByUser = make(map[string][]*models.QuoteRelationship)
	r.permissions = make(map[string]*models.QuotePermissions)
	r.quoteCounts = make(map[string]int64)
}

// Ensure QuoteRepository implements interfaces.QuoteRepository
var _ interfaces.QuoteRepository = (*QuoteRepository)(nil)
