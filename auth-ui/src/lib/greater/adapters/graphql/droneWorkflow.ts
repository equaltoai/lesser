import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { parse } from 'graphql';

export type DroneWorkflowPhase =
	| 'request'
	| 'review'
	| 'declaration'
	| 'signing'
	| 'graduation'
	| 'continuity';

export type DroneWorkflowState =
	| 'request.draft'
	| 'request.submitted'
	| 'review.queued'
	| 'review.approved'
	| 'review.changes_requested'
	| 'review.blocked'
	| 'declaration.ready'
	| 'signing.pending'
	| 'graduation.ready'
	| 'graduation.hold'
	| 'graduation.watch'
	| 'continuity.stable'
	| 'continuity.monitoring'
	| 'continuity.escalated';

export type DroneIdentityState = 'drone' | 'graduating' | 'souled';
export type DroneSoulBindingState = 'UNBOUND' | 'BOUND';
export type DroneLifecycleStatus = 'upcoming' | 'active' | 'complete' | 'blocked';
export type DroneReviewDecision = 'approved' | 'changes_requested' | 'blocked' | 'queued';
export type DroneGraduationReadiness = 'ready' | 'watch' | 'hold';
export type DroneSignerStatus = 'pending' | 'approved' | 'rejected';

export interface AgentSurfaceMetric {
	label: string;
	value: string;
	detail?: string | null;
}

export interface AgentSurfaceActor {
	id: string;
	name: string;
	role: string;
	handle?: string | null;
	avatarLabel?: string | null;
	statusLabel?: string | null;
}

export interface AgentSurfaceArtifact {
	id: string;
	title: string;
	description?: string | null;
	href?: string | null;
	emphasis?: string | null;
}

export interface AgentIdentityCard {
	id: string;
	name: string;
	handle?: string | null;
	summary: string;
	currentPhase: DroneWorkflowPhase;
	currentState?: DroneWorkflowState | null;
	steward?: AgentSurfaceActor | null;
	tags: ReadonlyArray<string>;
	metrics: ReadonlyArray<AgentSurfaceMetric>;
}

export interface SoulRequestCard {
	id: string;
	title: string;
	summary: string;
	requestedBy: AgentSurfaceActor;
	submittedAt?: string | null;
	constraints: ReadonlyArray<string>;
	artifacts: ReadonlyArray<AgentSurfaceArtifact>;
	routeDecision?: string | null;
	currentState?: DroneWorkflowState | null;
}

export interface ReviewFinding {
	id: string;
	title: string;
	detail: string;
	severity?: string | null;
}

export interface ReviewDecisionCard {
	id: string;
	title: string;
	decision: DroneReviewDecision;
	reviewer: AgentSurfaceActor;
	decisionSummary: string;
	findings: ReadonlyArray<ReviewFinding>;
	evidence: ReadonlyArray<AgentSurfaceArtifact>;
}

export interface DeclarationPreviewCard {
	id: string;
	title: string;
	statement: string;
	confidence: string;
	owner?: AgentSurfaceActor | null;
	declaredScope: ReadonlyArray<string>;
	risks: ReadonlyArray<string>;
	supportingArtifacts: ReadonlyArray<AgentSurfaceArtifact>;
}

export interface SignatureCheckpointSigner {
	id: string;
	name: string;
	role: string;
	status: DroneSignerStatus;
	note?: string | null;
}

export interface SignatureCheckpointCard {
	id: string;
	title: string;
	readinessLabel: string;
	approvalMemo?: string | null;
	dueAt?: string | null;
	signers: ReadonlyArray<SignatureCheckpointSigner>;
}

export interface ContinuityFollowUp {
	id: string;
	title: string;
	summary: string;
	owner: AgentSurfaceActor;
	cadence?: string | null;
}

export interface ContinuityPanel {
	id: string;
	title: string;
	objective: string;
	owner: AgentSurfaceActor;
	feedbackLoop: string;
	metrics: ReadonlyArray<AgentSurfaceMetric>;
	followUps: ReadonlyArray<ContinuityFollowUp>;
}

export interface GraduationSummaryCard {
	id: string;
	title: string;
	readiness: DroneGraduationReadiness;
	summary: string;
	launchOwner?: AgentSurfaceActor | null;
	completedMilestones: ReadonlyArray<string>;
	exitCriteria: ReadonlyArray<string>;
	nextStep?: string | null;
	metrics: ReadonlyArray<AgentSurfaceMetric>;
}

export interface AgentLifecycleStep {
	phase: DroneWorkflowPhase;
	title?: string | null;
	summary?: string | null;
	state?: DroneWorkflowState | null;
	status: DroneLifecycleStatus;
}

export interface AgentWorkflowConversationState {
	conversationID: string;
	folder: string;
	requestState?: string | null;
	unread: boolean;
	previewStatusID?: string | null;
	requestedAt?: string | null;
	acceptedAt?: string | null;
	declinedAt?: string | null;
	updatedAt: string;
}

export interface AgentIdentitySemantics {
	identityState: DroneIdentityState;
	identityLabel: string;
	lifecycleState: string;
	soulBindingState: DroneSoulBindingState;
	soulAgentId?: string | null;
	continuityState: string;
	continuitySummary: string;
	bodyIdentityPreserved: boolean;
	timelinePresencePreserved: boolean;
	memoryReferencesPreserved: boolean;
	attributionLabel: string;
	moderationLabel: string;
}

export interface DroneAgentCapabilities {
	canPost: boolean;
	canReply: boolean;
	canBoost: boolean;
	canFollow: boolean;
	canDM: boolean;
	maxPostsPerHour: number;
	requiresApproval: boolean;
	restrictedDomains?: ReadonlyArray<string> | null;
}

export interface AgentMCPAccess {
	mcpURL: string;
	protectedResourceURL: string;
	authorizationServerURL: string;
	registrationURL: string;
	scopes: ReadonlyArray<string>;
	guidance: ReadonlyArray<string>;
}

export interface DroneAgentState {
	id: string;
	username: string;
	displayName: string;
	bio?: string | null;
	agentType: string;
	agentVersion: string;
	agentCapabilities: DroneAgentCapabilities;
	agentOwner?: string | null;
	delegatedScopes: ReadonlyArray<string>;
	mcpAccess: AgentMCPAccess;
	verified: boolean;
	verifiedAt?: string | null;
	createdAt: string;
	activityCount: number;
	identitySemantics: AgentIdentitySemantics;
	workflow?: AgentWorkflowSurface | null;
}

export interface AgentWorkflowSurface {
	username: string;
	currentPhase: DroneWorkflowPhase;
	currentState: DroneWorkflowState;
	identity: AgentIdentityCard;
	request?: SoulRequestCard | null;
	review?: ReviewDecisionCard | null;
	declaration?: DeclarationPreviewCard | null;
	checkpoint?: SignatureCheckpointCard | null;
	graduation?: GraduationSummaryCard | null;
	continuity?: ContinuityPanel | null;
	lifecycle: ReadonlyArray<AgentLifecycleStep>;
	conversation?: AgentWorkflowConversationState | null;
	identitySemantics: AgentIdentitySemantics;
}

export interface DroneWorkflowMutationPayload {
	agent: DroneAgentState;
	workflow: AgentWorkflowSurface;
}

export interface AgentSurfaceArtifactInput {
	title: string;
	description?: string | null;
	href?: string | null;
	emphasis?: string | null;
}

export interface ReviewFindingInput {
	title: string;
	detail: string;
	severity?: string | null;
}

export interface RequestSoulPromotionInput {
	username: string;
	title?: string | null;
	summary: string;
	constraints?: ReadonlyArray<string> | null;
	artifacts?: ReadonlyArray<AgentSurfaceArtifactInput> | null;
	routeDecision?: string | null;
	conversationID?: string | null;
}

export interface ReviewSoulPromotionInput {
	username: string;
	title?: string | null;
	decision: DroneReviewDecision;
	decisionSummary: string;
	findings?: ReadonlyArray<ReviewFindingInput> | null;
	evidence?: ReadonlyArray<AgentSurfaceArtifactInput> | null;
	conversationID?: string | null;
}

export interface FinalizeSoulPromotionInput {
	username: string;
	readiness: DroneGraduationReadiness;
	summary: string;
	declarationTitle?: string | null;
	declarationStatement: string;
	declarationConfidence: string;
	declaredScope?: ReadonlyArray<string> | null;
	declarationRisks?: ReadonlyArray<string> | null;
	supportingArtifacts?: ReadonlyArray<AgentSurfaceArtifactInput> | null;
	completedMilestones?: ReadonlyArray<string> | null;
	exitCriteria?: ReadonlyArray<string> | null;
	nextStep?: string | null;
	continuityObjective?: string | null;
	continuityFeedbackLoop?: string | null;
	conversationID?: string | null;
	soulAgentID?: string | null;
}

export interface DroneAgentStateQuery {
	agent?: DroneAgentState | null;
}

export interface DroneAgentStateQueryVariables {
	username: string;
}

export interface DroneWorkflowQuery {
	droneWorkflow?: AgentWorkflowSurface | null;
}

export interface DroneWorkflowQueryVariables {
	username: string;
}

export interface MyDroneRequestsQuery {
	myDroneRequests: ReadonlyArray<SoulRequestCard>;
}

export interface MyDroneReviewsQuery {
	myDroneReviews: ReadonlyArray<ReviewDecisionCard>;
}

export interface RequestSoulPromotionMutation {
	requestSoulPromotion: DroneWorkflowMutationPayload;
}

export interface RequestSoulPromotionMutationVariables {
	input: RequestSoulPromotionInput;
}

export interface ReviewSoulPromotionMutation {
	reviewSoulPromotion: DroneWorkflowMutationPayload;
}

export interface ReviewSoulPromotionMutationVariables {
	input: ReviewSoulPromotionInput;
}

export interface FinalizeSoulPromotionMutation {
	finalizeSoulPromotion: DroneWorkflowMutationPayload;
}

export interface FinalizeSoulPromotionMutationVariables {
	input: FinalizeSoulPromotionInput;
}

const AGENT_SURFACE_METRIC_FIELDS = `
	label
	value
	detail
`;

const AGENT_SURFACE_ACTOR_FIELDS = `
	id
	name
	role
	handle
	avatarLabel
	statusLabel
`;

const AGENT_SURFACE_ARTIFACT_FIELDS = `
	id
	title
	description
	href
	emphasis
`;

const AGENT_IDENTITY_SEMANTICS_FIELDS = `
	identityState
	identityLabel
	lifecycleState
	soulBindingState
	soulAgentId
	continuityState
	continuitySummary
	bodyIdentityPreserved
	timelinePresencePreserved
	memoryReferencesPreserved
	attributionLabel
	moderationLabel
`;

const AGENT_WORKFLOW_FIELDS = `
	username
	currentPhase
	currentState
	identity {
		id
		name
		handle
		summary
		currentPhase
		currentState
		steward {
			${AGENT_SURFACE_ACTOR_FIELDS}
		}
		tags
		metrics {
			${AGENT_SURFACE_METRIC_FIELDS}
		}
	}
	request {
		id
		title
		summary
		requestedBy {
			${AGENT_SURFACE_ACTOR_FIELDS}
		}
		submittedAt
		constraints
		artifacts {
			${AGENT_SURFACE_ARTIFACT_FIELDS}
		}
		routeDecision
		currentState
	}
	review {
		id
		title
		decision
		reviewer {
			${AGENT_SURFACE_ACTOR_FIELDS}
		}
		decisionSummary
		findings {
			id
			title
			detail
			severity
		}
		evidence {
			${AGENT_SURFACE_ARTIFACT_FIELDS}
		}
	}
	declaration {
		id
		title
		statement
		confidence
		owner {
			${AGENT_SURFACE_ACTOR_FIELDS}
		}
		declaredScope
		risks
		supportingArtifacts {
			${AGENT_SURFACE_ARTIFACT_FIELDS}
		}
	}
	checkpoint {
		id
		title
		readinessLabel
		approvalMemo
		dueAt
		signers {
			id
			name
			role
			status
			note
		}
	}
	graduation {
		id
		title
		readiness
		summary
		launchOwner {
			${AGENT_SURFACE_ACTOR_FIELDS}
		}
		completedMilestones
		exitCriteria
		nextStep
		metrics {
			${AGENT_SURFACE_METRIC_FIELDS}
		}
	}
	continuity {
		id
		title
		objective
		owner {
			${AGENT_SURFACE_ACTOR_FIELDS}
		}
		feedbackLoop
		metrics {
			${AGENT_SURFACE_METRIC_FIELDS}
		}
		followUps {
			id
			title
			summary
			owner {
				${AGENT_SURFACE_ACTOR_FIELDS}
			}
			cadence
		}
	}
	lifecycle {
		phase
		title
		summary
		state
		status
	}
	conversation {
		conversationID
		folder
		requestState
		unread
		previewStatusID
		requestedAt
		acceptedAt
		declinedAt
		updatedAt
	}
	identitySemantics {
		${AGENT_IDENTITY_SEMANTICS_FIELDS}
	}
`;

const DRONE_AGENT_FIELDS = `
	id
	username
	displayName
	bio
	agentType
	agentVersion
	agentCapabilities {
		canPost
		canReply
		canBoost
		canFollow
		canDM
		maxPostsPerHour
		requiresApproval
		restrictedDomains
	}
	agentOwner
	delegatedScopes
	mcpAccess {
		mcpURL
		protectedResourceURL
		authorizationServerURL
		registrationURL
		scopes
		guidance
	}
	verified
	verifiedAt
	createdAt
	activityCount
	identitySemantics {
		${AGENT_IDENTITY_SEMANTICS_FIELDS}
	}
`;

export const DroneAgentStateDocument = parse(`
	query DroneAgentState($username: String!) {
		agent(username: $username) {
			${DRONE_AGENT_FIELDS}
			workflow {
				${AGENT_WORKFLOW_FIELDS}
			}
		}
	}
`) as unknown as TypedDocumentNode<DroneAgentStateQuery, DroneAgentStateQueryVariables>;

export const DroneWorkflowDocument = parse(`
	query DroneWorkflow($username: String!) {
		droneWorkflow(username: $username) {
			${AGENT_WORKFLOW_FIELDS}
		}
	}
`) as unknown as TypedDocumentNode<DroneWorkflowQuery, DroneWorkflowQueryVariables>;

export const MyDroneRequestsDocument = parse(`
	query MyDroneRequests {
		myDroneRequests {
			id
			title
			summary
			requestedBy {
				${AGENT_SURFACE_ACTOR_FIELDS}
			}
			submittedAt
			constraints
			artifacts {
				${AGENT_SURFACE_ARTIFACT_FIELDS}
			}
			routeDecision
			currentState
		}
	}
`) as unknown as TypedDocumentNode<MyDroneRequestsQuery, Record<string, never>>;

export const MyDroneReviewsDocument = parse(`
	query MyDroneReviews {
		myDroneReviews {
			id
			title
			decision
			reviewer {
				${AGENT_SURFACE_ACTOR_FIELDS}
			}
			decisionSummary
			findings {
				id
				title
				detail
				severity
			}
			evidence {
				${AGENT_SURFACE_ARTIFACT_FIELDS}
			}
		}
	}
`) as unknown as TypedDocumentNode<MyDroneReviewsQuery, Record<string, never>>;

export const RequestSoulPromotionDocument = parse(`
	mutation RequestSoulPromotion($input: RequestSoulPromotionInput!) {
		requestSoulPromotion(input: $input) {
			agent {
				${DRONE_AGENT_FIELDS}
			}
			workflow {
				${AGENT_WORKFLOW_FIELDS}
			}
		}
	}
`) as unknown as TypedDocumentNode<
	RequestSoulPromotionMutation,
	RequestSoulPromotionMutationVariables
>;

export const ReviewSoulPromotionDocument = parse(`
	mutation ReviewSoulPromotion($input: ReviewSoulPromotionInput!) {
		reviewSoulPromotion(input: $input) {
			agent {
				${DRONE_AGENT_FIELDS}
			}
			workflow {
				${AGENT_WORKFLOW_FIELDS}
			}
		}
	}
`) as unknown as TypedDocumentNode<
	ReviewSoulPromotionMutation,
	ReviewSoulPromotionMutationVariables
>;

export const FinalizeSoulPromotionDocument = parse(`
	mutation FinalizeSoulPromotion($input: FinalizeSoulPromotionInput!) {
		finalizeSoulPromotion(input: $input) {
			agent {
				${DRONE_AGENT_FIELDS}
			}
			workflow {
				${AGENT_WORKFLOW_FIELDS}
			}
		}
	}
`) as unknown as TypedDocumentNode<
	FinalizeSoulPromotionMutation,
	FinalizeSoulPromotionMutationVariables
>;
