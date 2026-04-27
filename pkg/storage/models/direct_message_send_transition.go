package models

// DirectMessageSendTransition is the repository-owned DM send write set.
// It captures the status row, shared conversation metadata, and the
// per-local-user canonical state owner rows that must move together for one send.
type DirectMessageSendTransition struct {
	Conversation              *Conversation
	Status                    *Status
	ParticipantStates         []*UserConversationState
	ExpectedParticipantStates []*UserConversationState
	CreateConversation        bool
}
