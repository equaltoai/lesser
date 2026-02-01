// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
)

// PublicationRepository is a thread-safe in-memory implementation of interfaces.PublicationRepository.
type PublicationRepository struct {
	mu sync.RWMutex

	// Publications by ID
	publications map[string]*models.Publication
}

// NewPublicationRepository creates a new in-memory publication repository
func NewPublicationRepository() *PublicationRepository {
	return &PublicationRepository{
		publications: make(map[string]*models.Publication),
	}
}

// GetDB returns the underlying DynamoDB connection.
// For in-memory implementation, this returns nil.
func (r *PublicationRepository) GetDB() dynamormcore.DB {
	return nil
}

// CreatePublication creates a new publication
func (r *PublicationRepository) CreatePublication(_ context.Context, publication *models.Publication) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if publication == nil || publication.ID == "" {
		return storage.ErrInvalidInput
	}

	if _, exists := r.publications[publication.ID]; exists {
		return storage.ErrAlreadyExists
	}

	r.publications[publication.ID] = publication
	return nil
}

// GetPublication retrieves a publication by ID
func (r *PublicationRepository) GetPublication(_ context.Context, id string) (*models.Publication, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	publication, exists := r.publications[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return publication, nil
}

// Update updates an existing publication
func (r *PublicationRepository) Update(_ context.Context, publication *models.Publication) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if publication == nil || publication.ID == "" {
		return storage.ErrInvalidInput
	}

	if _, exists := r.publications[publication.ID]; !exists {
		return storage.ErrNotFound
	}

	r.publications[publication.ID] = publication
	return nil
}

// Delete deletes a publication by PK and SK
func (r *PublicationRepository) Delete(_ context.Context, pk, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract publicationID from PK (format: PUBLICATION#<publicationID>)
	pk = strings.TrimSpace(pk)
	if !strings.HasPrefix(pk, "PUBLICATION#") {
		return storage.ErrInvalidInput
	}
	publicationID := strings.TrimPrefix(pk, "PUBLICATION#")

	if _, exists := r.publications[publicationID]; !exists {
		return storage.ErrNotFound
	}

	delete(r.publications, publicationID)
	return nil
}

// Clear clears all data (test helper)
func (r *PublicationRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.publications = make(map[string]*models.Publication)
}

// Ensure PublicationRepository implements interfaces.PublicationRepository
var _ interfaces.PublicationRepository = (*PublicationRepository)(nil)
