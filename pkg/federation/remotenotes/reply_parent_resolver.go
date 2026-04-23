// Package remotenotes resolves and materializes remote note parents for
// request-scoped write-path flows such as create-status reply creation.
package remotenotes

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// AuthorizedFetcher fetches a remote ActivityPub object in the local
// replying-actor context when protected parent acquisition is required.
type AuthorizedFetcher interface {
	FetchObject(ctx context.Context, objectURL string, signingActor *activitypub.Actor) (any, error)
}

type statusRepository interface {
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	GetStatusByURL(ctx context.Context, url string) (*models.Status, error)
	CreateStatus(ctx context.Context, status *models.Status) error
}

type objectRepository interface {
	CreateObject(ctx context.Context, object any) error
}

type domainBlockRepository interface {
	IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error)
}

// Resolver resolves reply parents locally first and materializes unresolved
// canonical remote URLs on the create-status write path.
type Resolver struct {
	statusRepo      statusRepository
	objectRepo      objectRepository
	domainBlockRepo domainBlockRepository
	fetcher         AuthorizedFetcher
	localDomain     string
	logger          *zap.Logger
}

var _ notes.ReplyParentResolver = (*Resolver)(nil)

// NewReplyParentResolver constructs a create-status reply-parent resolver over
// storage-first lookup plus optional authorized remote acquisition.
func NewReplyParentResolver(
	statusRepo statusRepository,
	objectRepo objectRepository,
	domainBlockRepo domainBlockRepository,
	fetcher AuthorizedFetcher,
	localDomain string,
	logger *zap.Logger,
) *Resolver {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Resolver{
		statusRepo:      statusRepo,
		objectRepo:      objectRepo,
		domainBlockRepo: domainBlockRepo,
		fetcher:         fetcher,
		localDomain:     strings.TrimSpace(localDomain),
		logger:          logger,
	}
}

// ResolveReplyParent resolves a reply parent for a single create-note request,
// materializing a canonical remote parent when it is not yet stored locally.
func (r *Resolver) ResolveReplyParent(
	ctx context.Context,
	author *storage.Account,
	rawInReplyTo string,
	requestedVisibility string,
) (*notes.ResolvedReplyParent, error) {
	rawInReplyTo = strings.TrimSpace(rawInReplyTo)
	if rawInReplyTo == "" {
		return nil, nil
	}

	if status, err := r.resolveStoredParent(ctx, rawInReplyTo); err == nil && status != nil {
		if err := validateReplyParentVisibility(status, requestedVisibility); err != nil {
			r.logOutcome("visibility_rejected", rawInReplyTo, status, false, err)
			return nil, err
		}
		r.logOutcome("stored", rawInReplyTo, status, false, nil)
		return replyParentResultFromStatus(status, false, r.localDomain), nil
	} else if err != nil && !statusLookupNotFound(err) {
		return nil, err
	}

	if !isReplyParentURL(rawInReplyTo) {
		return nil, unusableReplyParent(rawInReplyTo, "reply parent is not materialized locally")
	}

	parentURL, err := normalizeReplyParentURL(rawInReplyTo)
	if err != nil {
		return nil, invalidReplyParentReference(rawInReplyTo, err)
	}

	if requestedVisibility == models.VisibilityDirect {
		return nil, unusableReplyParent(parentURL, "direct replies are handled by conversations")
	}

	if err := r.ensureFetchAllowed(ctx, parentURL); err != nil {
		return nil, err
	}

	signingActor, err := replyParentSigningActor(author, r.localDomain)
	if err != nil {
		return nil, err
	}

	obj, err := r.fetcher.FetchObject(ctx, parentURL, signingActor)
	if err != nil {
		mappedErr := mapReplyParentFetchError(parentURL, err)
		r.logOutcome("fetch_failed", rawInReplyTo, nil, true, mappedErr)
		return nil, mappedErr
	}

	note, err := fetchedReplyParentNote(obj)
	if err != nil {
		unusableErr := unusableReplyParent(parentURL, err.Error())
		r.logOutcome("unusable", rawInReplyTo, nil, true, unusableErr)
		return nil, unusableErr
	}

	status, err := federation.MaterializeRemoteNote(ctx, r.objectRepo, r.statusRepo, note, r.localDomain)
	if err != nil {
		mappedErr := materializeReplyParentFailure(parentURL, err)
		r.logOutcome("materialize_failed", rawInReplyTo, nil, true, mappedErr)
		return nil, mappedErr
	}

	if err := validateReplyParentVisibility(status, requestedVisibility); err != nil {
		r.logOutcome("visibility_rejected", rawInReplyTo, status, true, err)
		return nil, err
	}

	r.logOutcome("materialized", rawInReplyTo, status, true, nil)
	return replyParentResultFromStatus(status, true, r.localDomain), nil
}

func (r *Resolver) resolveStoredParent(ctx context.Context, raw string) (*models.Status, error) {
	if r.statusRepo == nil {
		return nil, notes.ErrStatusRepositoryUnavailable
	}

	if isReplyParentURL(raw) {
		status, err := r.statusRepo.GetStatusByURL(ctx, raw)
		if err == nil && status != nil {
			return status, nil
		}
		if err != nil && !statusLookupNotFound(err) {
			return nil, err
		}
	}

	var lastErr error
	for _, candidate := range models.StatusLookupCandidatesForDomain(raw, r.localDomain) {
		status, err := r.statusRepo.GetStatus(ctx, candidate)
		if err == nil && status != nil {
			return status, nil
		}
		if err != nil && !statusLookupNotFound(err) {
			return nil, err
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = storage.ErrNotFound
	}
	return nil, lastErr
}

func (r *Resolver) ensureFetchAllowed(ctx context.Context, parentURL string) error {
	if r.fetcher == nil {
		return serviceUnavailableReplyParent(parentURL, stdErrors.New("remote reply parent fetch unavailable"))
	}

	domain := replyParentDomain(parentURL)
	if domain == "" || r.domainBlockRepo == nil {
		return nil
	}

	blocked, _, err := r.domainBlockRepo.IsDomainBlocked(ctx, domain)
	if err != nil {
		return serviceUnavailableReplyParent(parentURL, err)
	}
	if blocked {
		return unusableReplyParent(parentURL, fmt.Sprintf("reply parent domain %s is blocked", domain))
	}

	return nil
}

func replyParentSigningActor(author *storage.Account, localDomain string) (*activitypub.Actor, error) {
	if author == nil || author.User == nil || strings.TrimSpace(author.User.Username) == "" {
		return nil, serviceUnavailableReplyParent("", stdErrors.New("reply author identity unavailable"))
	}

	if author.Actor != nil && strings.TrimSpace(author.Actor.ID) != "" {
		if author.Actor.PublicKey == nil {
			authorCopy := *author.Actor
			authorCopy.PublicKey = &activitypub.PublicKey{
				ID:    strings.TrimRight(authorCopy.ID, "/") + "#main-key",
				Owner: authorCopy.ID,
			}
			return &authorCopy, nil
		}
		return author.Actor, nil
	}

	if localDomain == "" {
		return nil, serviceUnavailableReplyParent("", stdErrors.New("reply author actor unavailable"))
	}

	actorID := fmt.Sprintf("https://%s/users/%s", localDomain, author.User.Username)
	return &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: actorID},
		PreferredUsername: author.User.Username,
		PublicKey: &activitypub.PublicKey{
			ID:    actorID + "#main-key",
			Owner: actorID,
		},
	}, nil
}

func normalizeReplyParentURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", stdErrors.New("reply parent reference is empty")
	}
	if !isReplyParentURL(raw) {
		return "", stdErrors.New("reply parent reference must be a canonical remote status URL")
	}

	if err := common.ValidateURL(raw, "in_reply_to_id"); err != nil {
		return "", err
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Host == "" {
		return "", stdErrors.New("reply parent URL is invalid")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func fetchedReplyParentNote(obj any) (*activitypub.Note, error) {
	objMap, ok := obj.(map[string]any)
	if !ok {
		return nil, stdErrors.New("fetched parent object is not a valid ActivityPub object")
	}

	common.SanitizeActivityPubObjectDefault(objMap)

	if noteType, _ := objMap["type"].(string); strings.TrimSpace(noteType) != activitypub.NoteType {
		return nil, stdErrors.New("fetched parent object is not a Note")
	}

	if err := common.ValidateActivityPubNote(objMap); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(objMap)
	if err != nil {
		return nil, err
	}

	var note activitypub.Note
	if err := common.ParseActivityPubObject(raw, &note); err != nil {
		return nil, err
	}
	if strings.TrimSpace(note.ID) == "" {
		return nil, stdErrors.New("fetched parent note is empty")
	}

	return &note, nil
}

func validateReplyParentVisibility(status *models.Status, requestedVisibility string) error {
	if status == nil {
		return unusableReplyParent("", "reply parent is unavailable")
	}

	parentRank, parentOK := replyVisibilityRank(status.Visibility)
	requestedRank, requestedOK := replyVisibilityRank(requestedVisibility)
	if !parentOK || !requestedOK {
		return unusableReplyParent(status.StatusID, "reply visibility is unsupported")
	}

	if requestedRank < parentRank {
		return unusableReplyParent(status.StatusID, "reply visibility cannot broaden beyond the parent")
	}

	return nil
}

func replyVisibilityRank(visibility string) (int, bool) {
	switch strings.TrimSpace(visibility) {
	case models.VisibilityPublic:
		return 0, true
	case models.VisibilityUnlisted:
		return 1, true
	case models.VisibilityPrivate:
		return 2, true
	case models.VisibilityDirect:
		return 3, true
	default:
		return 0, false
	}
}

func replyParentResultFromStatus(status *models.Status, fetched bool, localDomain string) *notes.ResolvedReplyParent {
	if status == nil {
		return nil
	}

	objectURL := strings.TrimSpace(replyParentObjectURL(status, localDomain))
	return &notes.ResolvedReplyParent{
		Status:             status,
		CanonicalObjectURL: objectURL,
		CanonicalStatusID:  strings.TrimSpace(status.StatusID),
		Fetched:            fetched,
		Remote:             replyParentLooksRemote(status, objectURL, localDomain),
		Visibility:         strings.TrimSpace(status.Visibility),
	}
}

func replyParentObjectURL(status *models.Status, localDomain string) string {
	if status == nil {
		return ""
	}
	if status.Note != nil && strings.TrimSpace(status.Note.ID) != "" {
		return strings.TrimSpace(status.Note.ID)
	}
	for _, raw := range status.URLs {
		if isReplyParentURL(raw) {
			return strings.TrimSpace(raw)
		}
	}
	if isReplyParentURL(status.StatusID) {
		return strings.TrimSpace(status.StatusID)
	}
	if strings.TrimSpace(localDomain) == "" || strings.TrimSpace(status.AuthorUsername) == "" || strings.TrimSpace(status.StatusID) == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/users/%s/statuses/%s", localDomain, status.AuthorUsername, status.StatusID)
}

func replyParentLooksRemote(status *models.Status, objectURL, localDomain string) bool {
	if status == nil {
		return false
	}
	if models.IsCanonicalRemoteStatusID(status.StatusID) {
		return true
	}
	if actorID := strings.TrimSpace(status.AuthorID); actorID != "" {
		if parsed, err := url.Parse(actorID); err == nil && parsed != nil {
			host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
			return host != "" && !strings.EqualFold(host, localDomain)
		}
	}
	if parsed, err := url.Parse(objectURL); err == nil && parsed != nil {
		host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		return host != "" && !strings.EqualFold(host, localDomain)
	}
	return false
}

func isReplyParentURL(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

func replyParentDomain(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func mapReplyParentFetchError(parentURL string, err error) error {
	if err == nil {
		return nil
	}

	if stdErrors.Is(err, context.DeadlineExceeded) {
		return timeoutReplyParent(parentURL, err)
	}

	var netErr net.Error
	if stdErrors.As(err, &netErr) && netErr.Timeout() {
		return timeoutReplyParent(parentURL, err)
	}

	if appErr, ok := commonerrors.AsAppError(err); ok {
		switch appErr.Code {
		case commonerrors.CodeTimeout, commonerrors.CodeExternalServiceTimeout:
			return timeoutReplyParent(parentURL, err)
		case commonerrors.CodeExternalServiceUnavailable, commonerrors.CodeRateLimited:
			return serviceUnavailableReplyParent(parentURL, err)
		case commonerrors.CodeUnauthorized, commonerrors.CodeNotFound, commonerrors.CodeGone,
			commonerrors.CodeUnprocessableEntity, commonerrors.CodeValidationFailed,
			commonerrors.CodeInvalidInput, commonerrors.CodeInvalidFormat,
			commonerrors.CodeActivityParsingFailed, commonerrors.CodeRemoteFetchFailed:
			return serviceUnavailableReplyParent(parentURL, err)
		}
	}

	return serviceUnavailableReplyParent(parentURL, err)
}

func materializeReplyParentFailure(parentURL string, err error) error {
	if err == nil {
		return nil
	}
	return serviceUnavailableReplyParent(parentURL, err)
}

func invalidReplyParentReference(parentURL string, err error) error {
	appErr := commonerrors.NewAppError(commonerrors.CodeBadRequest, commonerrors.CategoryValidation, "Invalid in_reply_to_id")
	appErr = appErr.WithInternalError(err).WithMetadata("reply_parent", parentURL)
	return appErr
}

func timeoutReplyParent(parentURL string, err error) error {
	appErr := commonerrors.NewAppError(commonerrors.CodeTimeout, commonerrors.CategoryExternal, "Remote reply parent fetch timed out")
	appErr = appErr.WithInternalError(err).WithMetadata("reply_parent", parentURL).AsRetryable()
	return appErr
}

func serviceUnavailableReplyParent(parentURL string, err error) error {
	appErr := commonerrors.NewAppError(commonerrors.CodeExternalServiceUnavailable, commonerrors.CategoryExternal, "Remote reply parent is temporarily unavailable")
	appErr = appErr.WithInternalError(err).WithMetadata("reply_parent", parentURL).AsRetryable()
	return appErr
}

func unusableReplyParent(parentURL string, reason string) error {
	appErr := commonerrors.NewAppError(commonerrors.CodeUnprocessableEntity, commonerrors.CategoryValidation, "Remote reply parent is not usable")
	appErr = appErr.WithMetadata("reply_parent", parentURL)
	if reason != "" {
		appErr = appErr.WithMetadata("reason", reason)
	}
	return appErr
}

func (r *Resolver) logOutcome(outcome, requested string, status *models.Status, fetched bool, err error) {
	if r == nil || r.logger == nil {
		return
	}

	fields := []zap.Field{
		zap.String("outcome", outcome),
		zap.String("requested_parent", requested),
		zap.Bool("fetched", fetched),
	}

	if status != nil {
		fields = append(fields,
			zap.String("resolved_status_id", status.StatusID),
			zap.String("resolved_parent_url", replyParentObjectURL(status, r.localDomain)),
			zap.String("parent_visibility", status.Visibility),
			zap.Bool("remote_parent", replyParentLooksRemote(status, replyParentObjectURL(status, r.localDomain), r.localDomain)),
		)
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		r.logger.Warn("remote reply parent acquisition", fields...)
		return
	}

	r.logger.Info("remote reply parent acquisition", fields...)
}

func statusLookupNotFound(err error) bool {
	if err == nil {
		return false
	}
	if common.IsNotFound(err) {
		return true
	}
	if appErr, ok := commonerrors.AsAppError(err); ok {
		return appErr.Code == commonerrors.CodeNotFound
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
