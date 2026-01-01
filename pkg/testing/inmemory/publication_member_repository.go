// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// PublicationMemberRepository is a thread-safe in-memory implementation of interfaces.PublicationMemberRepository.
type PublicationMemberRepository struct {
	mu sync.RWMutex

	// Members by composite key: publicationID:userID -> member
	members map[string]*models.PublicationMember

	// Members by publication: publicationID -> []memberKey
	membersByPublication map[string][]string

	// Members by user: userID -> []memberKey
	membersByUser map[string][]string
}

// NewPublicationMemberRepository creates a new in-memory publication member repository
func NewPublicationMemberRepository() *PublicationMemberRepository {
	return &PublicationMemberRepository{
		members:              make(map[string]*models.PublicationMember),
		membersByPublication: make(map[string][]string),
		membersByUser:        make(map[string][]string),
	}
}

// pubMemberKey creates a composite key for a member
func pubMemberKey(publicationID, userID string) string {
	return fmt.Sprintf("%s:%s", publicationID, userID)
}

// CreateMember adds a new member to a publication
func (r *PublicationMemberRepository) CreateMember(_ context.Context, member *models.PublicationMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if member == nil || member.PublicationID == "" || member.UserID == "" {
		return storage.ErrInvalidInput
	}

	key := pubMemberKey(member.PublicationID, member.UserID)
	if _, exists := r.members[key]; exists {
		return storage.ErrAlreadyExists
	}

	// Store member
	r.members[key] = member

	// Index by publication
	r.membersByPublication[member.PublicationID] = append(r.membersByPublication[member.PublicationID], key)

	// Index by user
	r.membersByUser[member.UserID] = append(r.membersByUser[member.UserID], key)

	return nil
}

// GetMember retrieves a member by publication ID and user ID
func (r *PublicationMemberRepository) GetMember(_ context.Context, publicationID, userID string) (*models.PublicationMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := pubMemberKey(publicationID, userID)
	member, exists := r.members[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return member, nil
}

// DeleteMember removes a member from a publication
func (r *PublicationMemberRepository) DeleteMember(_ context.Context, publicationID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := pubMemberKey(publicationID, userID)
	if _, exists := r.members[key]; !exists {
		return storage.ErrNotFound
	}

	// Remove from publication index
	r.membersByPublication[publicationID] = pubMemberRemoveFromSlice(r.membersByPublication[publicationID], key)

	// Remove from user index
	r.membersByUser[userID] = pubMemberRemoveFromSlice(r.membersByUser[userID], key)

	delete(r.members, key)
	return nil
}

// ListMembers lists all members of a publication
func (r *PublicationMemberRepository) ListMembers(_ context.Context, publicationID string) ([]*models.PublicationMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.membersByPublication[publicationID]
	result := make([]*models.PublicationMember, 0, len(keys))
	for _, key := range keys {
		if member, exists := r.members[key]; exists {
			result = append(result, member)
		}
	}

	return result, nil
}

// Update updates an existing publication member
func (r *PublicationMemberRepository) Update(_ context.Context, member *models.PublicationMember) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if member == nil || member.PublicationID == "" || member.UserID == "" {
		return storage.ErrInvalidInput
	}

	key := pubMemberKey(member.PublicationID, member.UserID)
	if _, exists := r.members[key]; !exists {
		return storage.ErrNotFound
	}

	r.members[key] = member
	return nil
}

// ListMembershipsForUserPaginated lists publications a user is a member of with pagination
func (r *PublicationMemberRepository) ListMembershipsForUserPaginated(_ context.Context, userID string, limit int, cursor string) ([]*models.PublicationMember, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, "", storage.ErrInvalidInput
	}

	if limit <= 0 {
		limit = 25
	}

	// Get memberships for user
	keys := r.membersByUser[userID]
	memberships := make([]*models.PublicationMember, 0, len(keys))
	for _, key := range keys {
		if member, exists := r.members[key]; exists {
			memberships = append(memberships, member)
		}
	}

	// Sort by GSI1SK (PUBLICATION#...) ascending
	sort.Slice(memberships, func(i, j int) bool {
		return memberships[i].GSI1SK < memberships[j].GSI1SK
	})

	// Apply cursor
	startIdx := 0
	cursor = strings.TrimSpace(cursor)
	if cursor != "" {
		for i, member := range memberships {
			if member.GSI1SK > cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(memberships) {
		endIdx = len(memberships)
	}

	result := memberships[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(memberships) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI1SK
	}

	return result, nextCursor, nil
}

// Clear clears all data (test helper)
func (r *PublicationMemberRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.members = make(map[string]*models.PublicationMember)
	r.membersByPublication = make(map[string][]string)
	r.membersByUser = make(map[string][]string)
}

// pubMemberRemoveFromSlice removes an element from a slice
func pubMemberRemoveFromSlice(slice []string, element string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != element {
			result = append(result, s)
		}
	}
	return result
}

// Ensure PublicationMemberRepository implements interfaces.PublicationMemberRepository
var _ interfaces.PublicationMemberRepository = (*PublicationMemberRepository)(nil)
