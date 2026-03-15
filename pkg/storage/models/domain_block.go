package models

import (
	"fmt"
	"time"
)

// UserDomainBlock represents a user-level domain block
type UserDomainBlock struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK        string    `theorydb:"pk,attr:PK"`
	SK        string    `theorydb:"sk,attr:SK"`
	Username  string    `theorydb:"attr:username" json:"username"`
	Domain    string    `theorydb:"attr:domain" json:"domain"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing UserDomainBlock.
func (UserDomainBlock) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the user domain block
func (d *UserDomainBlock) UpdateKeys() error {
	d.PK = fmt.Sprintf(KeyPatternUser, d.Username)
	d.SK = fmt.Sprintf("DOMAIN_BLOCK#%s", d.Domain)
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (d *UserDomainBlock) GetPK() string {
	return d.PK
}

// GetSK returns the sort key (required by BaseModel)
func (d *UserDomainBlock) GetSK() string {
	return d.SK
}

// InstanceDomainBlock represents an instance-level domain block
type InstanceDomainBlock struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK             string    `theorydb:"pk,attr:PK"`
	SK             string    `theorydb:"sk,attr:SK"`
	GSI1PK         string    `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK         string    `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`
	ID             string    `theorydb:"attr:id" json:"ID"`
	Domain         string    `theorydb:"attr:domain" json:"Domain"`
	Severity       string    `theorydb:"attr:severity" json:"Severity"` // "silence" or "suspend"
	RejectMedia    bool      `theorydb:"attr:rejectMedia" json:"RejectMedia"`
	RejectReports  bool      `theorydb:"attr:rejectReports" json:"RejectReports"`
	PrivateComment string    `theorydb:"attr:privateComment" json:"PrivateComment"` // Admin-only notes
	PublicComment  string    `theorydb:"attr:publicComment" json:"PublicComment"`   // Public reason
	Obfuscate      bool      `theorydb:"attr:obfuscate" json:"Obfuscate"`           // Whether to obfuscate in public lists
	CreatedBy      string    `theorydb:"attr:createdBy" json:"CreatedBy"`           // Admin username who created
	CreatedByID    string    `theorydb:"attr:createdByID" json:"CreatedByID"`       // Admin actor ID
	CreatedAt      time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`
	UpdatedAt      time.Time `theorydb:"attr:updatedAt" json:"UpdatedAt"`
	Type           string    `theorydb:"attr:type" json:"Type"`
}

// TableName returns the DynamoDB table backing InstanceDomainBlock.
func (InstanceDomainBlock) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the instance domain block
func (d *InstanceDomainBlock) UpdateKeys() error {
	d.PK = fmt.Sprintf("DOMAIN_BLOCK#%s", d.Domain)
	d.SK = fmt.Sprintf("DOMAIN_BLOCK#%s", d.Domain)
	d.GSI1PK = "DOMAIN_BLOCKS"
	d.GSI1SK = fmt.Sprintf("%d#%s", d.CreatedAt.Unix(), d.Domain)
	d.Type = "INSTANCE_DOMAIN_BLOCK"
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (d *InstanceDomainBlock) GetPK() string {
	return d.PK
}

// GetSK returns the sort key (required by BaseModel)
func (d *InstanceDomainBlock) GetSK() string {
	return d.SK
}

// EmailDomainBlock represents an email domain block
type EmailDomainBlock struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK        string    `theorydb:"pk,attr:PK"`
	SK        string    `theorydb:"sk,attr:SK"`
	GSI1PK    string    `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK    string    `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`
	ID        string    `theorydb:"attr:id" json:"ID"`
	Domain    string    `theorydb:"attr:domain" json:"Domain"`
	CreatedBy string    `theorydb:"attr:createdBy" json:"CreatedBy"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`
}

// TableName returns the DynamoDB table backing EmailDomainBlock.
func (EmailDomainBlock) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the email domain block
func (d *EmailDomainBlock) UpdateKeys() error {
	d.PK = fmt.Sprintf("EMAIL_DOMAIN_BLOCK#%s", d.Domain)
	d.SK = fmt.Sprintf("EMAIL_DOMAIN_BLOCK#%s", d.Domain)
	d.GSI1PK = "EMAIL_DOMAIN_BLOCKS"
	d.GSI1SK = d.CreatedAt.Format(time.RFC3339)
	return nil
}

// GetID returns the ID of the email domain block
func (d *EmailDomainBlock) GetID() string {
	return d.ID
}

// GetPK returns the primary key of the email domain block
func (d *EmailDomainBlock) GetPK() string {
	return d.PK
}

// GetSK returns the sort key of the email domain block
func (d *EmailDomainBlock) GetSK() string {
	return d.SK
}

// GetDomain returns the domain of the email domain block
func (d *EmailDomainBlock) GetDomain() string {
	return d.Domain
}

// DomainAllow represents a domain in the allowlist
type DomainAllow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK        string    `theorydb:"pk,attr:PK"`
	SK        string    `theorydb:"sk,attr:SK"`
	GSI1PK    string    `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"`
	GSI1SK    string    `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"`
	ID        string    `theorydb:"attr:id" json:"ID"`
	Domain    string    `theorydb:"attr:domain" json:"Domain"`
	CreatedBy string    `theorydb:"attr:createdBy" json:"CreatedBy"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`
}

// TableName returns the DynamoDB table backing DomainAllow.
func (DomainAllow) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the domain allow
func (d *DomainAllow) UpdateKeys() error {
	d.PK = fmt.Sprintf("DOMAIN_ALLOW#%s", d.Domain)
	d.SK = fmt.Sprintf("DOMAIN_ALLOW#%s", d.Domain)
	d.GSI1PK = "DOMAIN_ALLOWS"
	d.GSI1SK = d.CreatedAt.Format(time.RFC3339)
	return nil
}

// GetID returns the ID of the domain allow
func (d *DomainAllow) GetID() string {
	return d.ID
}

// GetPK returns the primary key of the domain allow
func (d *DomainAllow) GetPK() string {
	return d.PK
}

// GetSK returns the sort key of the domain allow
func (d *DomainAllow) GetSK() string {
	return d.SK
}

// GetDomain returns the domain of the domain allow
func (d *DomainAllow) GetDomain() string {
	return d.Domain
}

// Domain block severity constants
const (
	DomainBlockSeveritySilence = "silence"
	DomainBlockSeveritySuspend = "suspend"
)
