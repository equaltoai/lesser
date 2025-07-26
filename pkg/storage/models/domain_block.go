package models

import (
	"fmt"
	"time"
)

// DomainBlock represents an instance-level domain block in DynamoDB
type DomainBlock struct {
	// Primary key
	PK string `dynamorm:"pk" json:"pk"` // Format: "domain#{domain}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "block#{domain}"

	// GSI1 - by severity
	GSI1PK string `dynamorm:"index:gsi1-index,pk" json:"gsi1_pk"` // Format: "domainblock#severity#{severity}"
	GSI1SK string `dynamorm:"index:gsi1-index,sk" json:"gsi1_sk"` // Format: "{created_at}#{domain}"

	// Block data
	ID             string     `json:"id"`
	Domain         string     `json:"domain"`
	Severity       string     `json:"severity"`        // "silence" or "suspend"
	RejectMedia    bool       `json:"reject_media"`
	RejectReports  bool       `json:"reject_reports"`
	PrivateComment string     `json:"private_comment"` // Admin-only notes
	PublicComment  string     `json:"public_comment"`  // Public reason
	Obfuscate      bool       `json:"obfuscate"`       // Whether to obfuscate in public lists
	CreatedBy      string     `json:"created_by"`      // Admin username who created
	CreatedByID    string     `json:"created_by_id"`   // Admin actor ID
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"` // Optional expiration
}

// Domain block severity constants
const (
	DomainBlockSeveritySilence = "silence"
	DomainBlockSeveritySuspend = "suspend"
)

// NewDomainBlock creates a new domain block
func NewDomainBlock(domain, severity, createdBy, createdByID string) *DomainBlock {
	now := time.Now()
	id := fmt.Sprintf("domainblock-%d-%s", now.Unix(), domain)
	
	block := &DomainBlock{
		PK:          fmt.Sprintf("domain#%s", domain),
		SK:          fmt.Sprintf("block#%s", domain),
		GSI1PK:      fmt.Sprintf("domainblock#severity#%s", severity),
		GSI1SK:      fmt.Sprintf("%s#%s", now.Format(time.RFC3339), domain),
		ID:          id,
		Domain:      domain,
		Severity:    severity,
		CreatedBy:   createdBy,
		CreatedByID: createdByID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	return block
}