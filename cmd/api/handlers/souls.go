package handlers

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

const (
	soulBindingIntegrationCallerID = "lesser-body"
	soulBindingAPIVersion          = "1"
	soulBindingStateBound          = "bound"
)

type soulHandlerService interface {
	ListMine(ctx context.Context, username string) ([]soulservice.Soul, error)
	Incorporate(ctx context.Context, principalUsername string, targetAgentUsername string, soulAgentID string) (*soulservice.Soul, error)
	ResolveBoundAgent(ctx context.Context, agentUsername string) (*soulservice.Soul, error)
	BindSoulBody(ctx context.Context, input soulservice.BindSoulBodyInput) (*soulservice.SoulBindingProjection, error)
	GetSoulBinding(ctx context.Context, agentID string, actorUsername string) (*soulservice.SoulBindingProjection, error)
}

// HandleGetMySoulsLift returns souls owned by the authenticated user's linked wallet(s) for this instance.
func (h *Handler) HandleGetMySoulsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeRead, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	souls, err := svc.ListMine(ctx.Context(), claims.Username)
	if err != nil {
		return h.respondSoulServiceError(ctx, err)
	}

	items := make([]apimodels.SoulInventoryItem, 0, len(souls))
	for _, soul := range souls {
		items = append(items, toAPISoulInventoryItem(soul))
	}

	return okJSON(apimodels.SoulsMineResponse{
		Souls: items,
		Count: len(items),
	})
}

// HandleGetBoundSoulMeLift returns the active soul bound to the authenticated local runtime-agent principal.
func (h *Handler) HandleGetBoundSoulMeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeRead, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	soul, err := svc.ResolveBoundAgent(ctx.Context(), claims.Username)
	if err != nil {
		if errors.Is(err, soulservice.ErrSoulNotAvailable) {
			return respondBoundSoulNotFound("soul_not_available", "bound soul is not available")
		}
		return h.respondSoulServiceError(ctx, err)
	}
	if soul == nil || !soul.Bound {
		return respondBoundSoulNotFound("soul_not_bound", "no bound soul for authenticated principal")
	}

	item := toAPISoulInventoryItem(*soul)
	if item.Binding == nil || item.BindingState != "bound" {
		return respondBoundSoulNotFound("soul_not_bound", "no bound soul for authenticated principal")
	}

	return okJSON(apimodels.BoundSoulResponse{
		Agent:        item.Agent,
		BindingState: item.BindingState,
		Binding:      *item.Binding,
	})
}

// HandleIncorporateSoulLift explicitly binds a soul to a local agent chosen by the authenticated principal.
func (h *Handler) HandleIncorporateSoulLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	agentID := strings.TrimSpace(ctx.Param("agentId"))
	if agentID == "" {
		return common.RespondBadRequest(ctx, "agentId is required")
	}

	var req apimodels.SoulIncorporateRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}
	targetAgentUsername := strings.TrimSpace(req.TargetAgentUsername)
	if err := common.ValidateUsernameParamID(targetAgentUsername); err != nil {
		return h.respondBadRequest(ctx, "target_agent_username is required and must be a valid username")
	}

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	soul, err := svc.Incorporate(ctx.Context(), claims.Username, targetAgentUsername, agentID)
	if err != nil {
		return h.respondSoulServiceError(ctx, err)
	}

	return okJSON(apimodels.SoulIncorporateResponse{
		Soul: toAPISoulInventoryItem(*soul),
	})
}

// HandleCreateSoulBindingLift handles POST /api/v1/souls/bindings.
//
// This is a server-to-server body/Ptah integration surface. It is intentionally
// route-gate reachable so ordinary OAuth middleware does not misclassify the
// dedicated integration bearer, but the handler fails closed unless the
// deployment-scoped soul-binding credential matches.
func (h *Handler) HandleCreateSoulBindingLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	callerID, resp, err := h.authenticateSoulBindingIntegration(ctx)
	if err != nil || resp != nil {
		return resp, err
	}

	idempotencyKey := strings.TrimSpace(headerValue(ctx, "Idempotency-Key"))
	if idempotencyKey == "" {
		return respondSoulBindingError(
			http.StatusBadRequest,
			"Idempotency key is required",
			"POST /api/v1/souls/bindings requires the Idempotency-Key header.",
			"SOUL_BINDING_IDEMPOTENCY_KEY_REQUIRED",
		)
	}

	req, bindErr := apptheory.BindRequest[apimodels.SoulBindingRequest](ctx, apptheory.BindConfig[apimodels.SoulBindingRequest]{
		Body:       true,
		StrictJSON: true,
	})
	if bindErr != nil {
		return respondSoulBindingError(
			http.StatusBadRequest,
			"Invalid soul-binding request",
			"Request body must be valid JSON matching the soul-binding contract.",
			"SOUL_BINDING_INVALID_REQUEST",
		)
	}

	svc := h.getSoulService()
	if svc == nil {
		return respondSoulBindingError(
			http.StatusInternalServerError,
			"Soul binding service is unavailable",
			"Lesser could not initialize the soul-binding service.",
			"SOUL_BINDING_INTERNAL",
		)
	}

	projection, serviceErr := svc.BindSoulBody(ctx.Context(), soulservice.BindSoulBodyInput{
		CallerID:             callerID,
		IdempotencyKey:       idempotencyKey,
		ActorUsername:        req.ActorUsername,
		SoulAgentID:          req.SoulAgentID,
		BodyActorID:          req.BodyActorID,
		HostRegistrationID:   req.HostRegistrationID,
		HostConversationID:   req.HostConversationID,
		AuthorityModel:       req.AuthorityModel,
		AnchorState:          req.AnchorState,
		OperationalBinding:   req.OperationalBinding,
		PrincipalAddressHint: req.PrincipalAddress,
		Evidence: soulservice.SoulBindingEvidence{
			Source:          req.Evidence.Source,
			HostRequestID:   req.Evidence.HostRequestID,
			DeclarationHash: req.Evidence.DeclarationHash,
			IssuedAt:        req.Evidence.IssuedAt,
		},
	})
	if serviceErr != nil {
		return h.respondSoulBindingServiceError(ctx, serviceErr)
	}

	return okJSON(toAPISoulBindingResponse(projection, true))
}

// HandleGetSoulBindingLift handles GET /api/v1/souls/bindings/{agentId}.
func (h *Handler) HandleGetSoulBindingLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if _, resp, err := h.authenticateSoulBindingIntegration(ctx); err != nil || resp != nil {
		return resp, err
	}

	svc := h.getSoulService()
	if svc == nil {
		return respondSoulBindingError(
			http.StatusInternalServerError,
			"Soul binding service is unavailable",
			"Lesser could not initialize the soul-binding service.",
			"SOUL_BINDING_INTERNAL",
		)
	}

	projection, serviceErr := svc.GetSoulBinding(
		ctx.Context(),
		strings.TrimSpace(ctx.Param("agentId")),
		strings.TrimSpace(queryValue(ctx, "actor_username")),
	)
	if serviceErr != nil {
		return h.respondSoulBindingServiceError(ctx, serviceErr)
	}

	return okJSON(toAPISoulBindingResponse(projection, false))
}

func (h *Handler) authenticateSoulBindingIntegration(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	if h == nil || h.cfg == nil {
		resp, err := respondSoulBindingError(
			http.StatusServiceUnavailable,
			"Soul binding is unavailable",
			"Soul-binding integration credential is not configured for this instance.",
			"SOUL_BINDING_INTERNAL",
		)
		return "", resp, err
	}

	expected, err := h.cfg.ResolveSoulBindingIntegrationKey()
	if err != nil || strings.TrimSpace(expected) == "" {
		resp, respErr := respondSoulBindingError(
			http.StatusServiceUnavailable,
			"Soul binding is unavailable",
			"Soul-binding integration credential is not configured for this instance.",
			"SOUL_BINDING_INTERNAL",
		)
		return "", resp, respErr
	}

	token, err := common.ExtractBearerToken(headerValue(ctx, "Authorization"))
	if err != nil || !constantTimeEqual(token, expected) {
		resp, respErr := respondSoulBindingError(
			http.StatusUnauthorized,
			"Soul-binding credential is required",
			"Provide the deployment-scoped body/Ptah soul-binding integration bearer credential.",
			"SOUL_BINDING_AUTH_REQUIRED",
		)
		return "", resp, respErr
	}

	return soulBindingIntegrationCallerID, nil, nil
}

func constantTimeEqual(a string, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (h *Handler) getSoulService() soulHandlerService {
	if h == nil {
		return nil
	}
	if h.soulsService != nil {
		return h.soulsService
	}
	if h.repos == nil || h.cfg == nil {
		return nil
	}
	return soulservice.NewService(h.repos.Account(), h.repos.Instance(), h.cfg, h.logger)
}

func respondBoundSoulNotFound(code string, message string) (*apptheory.Response, error) {
	return apptheory.JSON(http.StatusNotFound, common.StandardErrorResponse{
		Error:       message,
		Description: message,
		Code:        code,
	})
}

func (h *Handler) respondSoulServiceError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, soulservice.ErrTrustNotConfigured):
		return common.RespondUnprocessableEntity(ctx, "trust not configured")
	case errors.Is(err, soulservice.ErrSoulNotAvailable):
		return common.RespondNotFound(ctx, "soul")
	case errors.Is(err, soulservice.ErrTargetAgentRequired):
		return common.RespondUnprocessableEntity(ctx, "target agent is required")
	case errors.Is(err, soulservice.ErrTargetAgentNotFound):
		return common.RespondNotFound(ctx, "target agent")
	case errors.Is(err, soulservice.ErrTargetAgentNotOwned):
		return common.RespondForbidden(ctx, "target agent is not owned by authenticated principal")
	case errors.Is(err, soulservice.ErrTargetAgentMustBeAgent):
		return common.RespondUnprocessableEntity(ctx, "target account must be an agent")
	case errors.Is(err, soulservice.ErrSoulAlreadyBound):
		return common.RespondConflict(ctx, "soul already bound to another local agent")
	case errors.Is(err, soulservice.ErrTargetAgentAlreadyHasSoul):
		return common.RespondConflict(ctx, "target agent already has a soul")
	default:
		return common.RespondInternalServerError(ctx)
	}
}

func (h *Handler) respondSoulBindingServiceError(_ *apptheory.Context, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, soulservice.ErrSoulBindingIdempotencyKeyRequired):
		return respondSoulBindingError(
			http.StatusBadRequest,
			"Idempotency key is required",
			"POST /api/v1/souls/bindings requires the Idempotency-Key header.",
			"SOUL_BINDING_IDEMPOTENCY_KEY_REQUIRED",
		)
	case errors.Is(err, soulservice.ErrSoulBindingInvalidRequest),
		errors.Is(err, soulservice.ErrTargetAgentRequired):
		return respondSoulBindingError(
			http.StatusBadRequest,
			"Invalid soul-binding request",
			"Request fields must identify a local agent actor and hosted bound soul.",
			"SOUL_BINDING_INVALID_REQUEST",
		)
	case errors.Is(err, soulservice.ErrSoulBindingAgentIDInvalid):
		return respondSoulBindingError(
			http.StatusBadRequest,
			"Invalid soul agent ID",
			"soul_agent_id must be canonical lowercase 0x plus 64 hexadecimal characters.",
			"SOUL_BINDING_AGENT_ID_INVALID",
		)
	case errors.Is(err, soulservice.ErrTargetAgentNotFound):
		return respondSoulBindingError(
			http.StatusNotFound,
			"Target local actor was not found",
			"Lesser could not find the requested local agent actor.",
			"SOUL_BINDING_ACTOR_NOT_FOUND",
		)
	case errors.Is(err, soulservice.ErrSoulBindingNotFound):
		return respondSoulBindingError(
			http.StatusNotFound,
			"Soul binding was not found",
			"No Lesser-local soul/body binding exists for the requested soul agent ID.",
			"SOUL_BINDING_NOT_FOUND",
		)
	case errors.Is(err, soulservice.ErrSoulBindingIdempotencyMismatch):
		return respondSoulBindingError(
			http.StatusConflict,
			"Idempotency key was reused with a different soul-binding request",
			"Reuse the original request payload or send a new idempotency key for a new ceremony attempt.",
			"SOUL_BINDING_IDEMPOTENCY_MISMATCH",
		)
	case errors.Is(err, soulservice.ErrSoulAlreadyBound):
		return respondSoulBindingError(
			http.StatusConflict,
			"Soul is already bound to another local actor",
			"The requested soul agent ID is already bound to a different local actor.",
			"SOUL_BINDING_SOUL_CONFLICT",
		)
	case errors.Is(err, soulservice.ErrTargetAgentAlreadyHasSoul):
		return respondSoulBindingError(
			http.StatusConflict,
			"Local actor is already bound to another soul",
			"The requested local actor already has a different soul binding.",
			"SOUL_BINDING_BODY_CONFLICT",
		)
	case errors.Is(err, soulservice.ErrSoulBindingActorMismatch):
		return respondSoulBindingError(
			http.StatusConflict,
			"Soul binding is for a different local actor",
			"The requested actor_username does not match the existing Lesser-local binding.",
			"SOUL_BINDING_ACTOR_MISMATCH",
		)
	case errors.Is(err, soulservice.ErrTargetAgentMustBeAgent):
		return respondSoulBindingError(
			http.StatusUnprocessableEntity,
			"Target local actor is not eligible for soul binding",
			"The requested local actor exists but is not an agent account.",
			"SOUL_BINDING_ACTOR_INVALID",
		)
	case errors.Is(err, soulservice.ErrSoulBindingHostRegistryRejected):
		return respondSoulBindingError(
			http.StatusUnprocessableEntity,
			"Host registry did not confirm the requested soul binding",
			"The requested soul was not active for this Lesser domain and local agent.",
			"SOUL_BINDING_HOST_REGISTRY_REJECTED",
		)
	case errors.Is(err, soulservice.ErrSoulBindingHostRegistryUnavailable):
		return respondSoulBindingError(
			http.StatusServiceUnavailable,
			"Host registry is unavailable",
			"Lesser could not verify Host source truth for the requested hosted soul.",
			"SOUL_BINDING_HOST_REGISTRY_UNAVAILABLE",
		)
	default:
		return respondSoulBindingError(
			http.StatusInternalServerError,
			"Soul binding failed",
			"Lesser encountered an unexpected error while processing the soul-binding request.",
			"SOUL_BINDING_INTERNAL",
		)
	}
}

func respondSoulBindingError(status int, message string, description string, code string) (*apptheory.Response, error) {
	return apptheory.JSON(status, common.StandardErrorResponse{
		Error:       message,
		Description: description,
		Code:        code,
	})
}

func toAPISoulBindingResponse(projection *soulservice.SoulBindingProjection, includeIdempotency bool) apimodels.SoulBindingResponse {
	if projection == nil {
		return apimodels.SoulBindingResponse{
			Version:      soulBindingAPIVersion,
			Status:       soulBindingStateBound,
			BindingState: soulBindingStateBound,
		}
	}

	soul := projection.Soul
	out := apimodels.SoulBindingResponse{
		Version:      soulBindingAPIVersion,
		Status:       soulBindingStateBound,
		BindingState: soulBindingStateBound,
		Agent: apimodels.SoulBindingAgent{
			AgentID:            soul.AgentID,
			Domain:             soul.Domain,
			LocalID:            soul.LocalID,
			AuthorityModel:     soul.AuthorityModel,
			AnchorState:        soul.AnchorState,
			OperationalBinding: soul.OperationalBinding,
			LifecycleStatus:    soul.LifecycleStatus,
			PublishedVersion:   soul.PublishedVersion,
		},
		Binding: apimodels.SoulAgentBinding{
			AgentUsername:    soul.BoundAgentUsername,
			PrincipalAddress: soul.BoundPrincipalAddress,
			BoundAt:          soul.BoundAt,
			UpdatedAt:        soul.BoundUpdatedAt,
		},
	}
	if includeIdempotency {
		out.Idempotency = &apimodels.SoulBindingIdempotency{
			Key:         projection.IdempotencyKey,
			Replayed:    projection.Replayed,
			PayloadHash: projection.PayloadHash,
		}
		out.Links = &apimodels.SoulBindingLinks{
			Status: "/api/v1/souls/bindings/" + soul.AgentID,
		}
	}
	return out
}

func toAPISoulInventoryItem(soul soulservice.Soul) apimodels.SoulInventoryItem {
	var binding *apimodels.SoulAgentBinding
	bindingState := "unbound"
	available := !soul.Bound

	if soul.Bound {
		bindingState = "bound"
		binding = &apimodels.SoulAgentBinding{
			AgentUsername:    soul.BoundAgentUsername,
			PrincipalAddress: soul.BoundPrincipalAddress,
			BoundAt:          soul.BoundAt,
			UpdatedAt:        soul.BoundUpdatedAt,
		}
	}

	return apimodels.SoulInventoryItem{
		Agent: apimodels.SoulAgentIdentity{
			AgentID:                soul.AgentID,
			Domain:                 soul.Domain,
			LocalID:                soul.LocalID,
			ENSName:                soul.ENSName,
			Wallet:                 soul.Wallet,
			AuthorityModel:         soul.AuthorityModel,
			AnchorState:            soul.AnchorState,
			OperationalBinding:     soul.OperationalBinding,
			TokenID:                soul.TokenID,
			MetaURI:                soul.MetaURI,
			Avatar:                 toAPISoulAvatar(soul.Avatar),
			PrincipalAddress:       soul.PrincipalAddress,
			PrincipalSignature:     soul.PrincipalSignature,
			PrincipalDeclaration:   soul.PrincipalDeclaration,
			PrincipalDeclaredAt:    soul.PrincipalDeclaredAt,
			Status:                 soul.Status,
			LifecycleStatus:        soul.LifecycleStatus,
			LifecycleReason:        soul.LifecycleReason,
			SuccessorAgentID:       soul.SuccessorAgentID,
			PredecessorAgentID:     soul.PredecessorAgentID,
			SelfDescriptionVersion: soul.SelfDescriptionVersion,
			Capabilities:           append([]string(nil), soul.Capabilities...),
			MintTxHash:             soul.MintTxHash,
			MintedAt:               soul.MintedAt,
			UpdatedAt:              soul.UpdatedAt,
			PublishedVersion:       soul.PublishedVersion,
		},
		BindingState:              bindingState,
		AvailableForIncorporation: available,
		Binding:                   binding,
	}
}

func toAPISoulAvatar(value *soulservice.SoulAvatar) *apimodels.SoulAgentAvatar {
	if value == nil {
		return nil
	}

	styles := make([]apimodels.SoulAgentAvatarStyle, 0, len(value.Styles))
	for _, style := range value.Styles {
		styles = append(styles, apimodels.SoulAgentAvatarStyle{
			StyleID:         style.StyleID,
			StyleName:       style.StyleName,
			RendererAddress: style.RendererAddress,
			Image:           style.Image,
			Selected:        style.Selected,
		})
	}

	return &apimodels.SoulAgentAvatar{
		TokenURI:               value.TokenURI,
		Image:                  value.Image,
		CurrentStyleID:         value.CurrentStyleID,
		CurrentStyleName:       value.CurrentStyleName,
		CurrentRendererAddress: value.CurrentRendererAddress,
		Styles:                 styles,
	}
}
