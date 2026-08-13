package models

// AgentAccessResponse is the REST representation of an actor-admission decision.
//
// It is returned only when the caller is authorized. Unauthorized callers receive
// a uniform 403 with no actor, ownership, or grant detail.
type AgentAccessResponse struct {
	Actor        string `json:"actor"`
	Relationship string `json:"relationship"`
	Authorized   bool   `json:"authorized"`
	ActedBy      string `json:"acted_by"`
}
