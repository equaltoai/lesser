package models

import (
	"fmt"
	"time"
)

// ThreadNode represents a single node in a conversation thread tree
// Used for building and traversing thread hierarchies
type ThreadNode struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // THREAD#{rootStatusID}
	SK string `dynamorm:"sk,attr:SK" json:"-"` // NODE#{statusID}

	// GSI for querying by status
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"` // STATUS#{statusID}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"` // THREAD_NODE

	// Node data
	RootStatusID    string     `dynamorm:"attr:rootStatusID" json:"root_status_id"`         // The root of the thread
	StatusID        string     `dynamorm:"attr:statusID" json:"status_id"`                  // This status ID
	ParentID        string     `dynamorm:"attr:parentID" json:"parent_id,omitempty"`        // Direct parent (empty for root)
	Depth           int        `dynamorm:"attr:depth" json:"depth"`                         // Depth in tree (0 for root)
	Path            string     `dynamorm:"attr:path" json:"path"`                           // Path from root (e.g., "root.reply1.reply2")
	ChildIDs        []string   `dynamorm:"attr:childIDs" json:"child_ids"`                  // Direct child status IDs
	AuthorID        string     `dynamorm:"attr:authorID" json:"author_id"`                  // Author of this status
	AuthorHandle    string     `dynamorm:"attr:authorHandle" json:"author_handle"`          // For quick display
	Content         string     `dynamorm:"attr:content" json:"content"`                     // Preview of content
	CreatedAt       time.Time  `dynamorm:"attr:createdAt" json:"created_at"`                // When status was created
	Visibility      string     `dynamorm:"attr:visibility" json:"visibility"`               // public, unlisted, private, direct
	Sensitive       bool       `dynamorm:"attr:sensitive" json:"sensitive"`                 // Has content warning
	ReplyCount      int        `dynamorm:"attr:replyCount" json:"reply_count"`              // Number of direct replies
	DescendantCount int        `dynamorm:"attr:descendantCount" json:"descendant_count"`    // Total descendants in subtree
	LastReplyAt     *time.Time `dynamorm:"attr:lastReplyAt" json:"last_reply_at,omitempty"` // Most recent reply in subtree
	UpdatedAt       time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`                // Last update
	FetchedAt       time.Time  `dynamorm:"attr:fetchedAt" json:"fetched_at"`                // When we last fetched this node
	IsLocal         bool       `dynamorm:"attr:isLocal" json:"is_local"`                    // Whether this is a local status
	RemoteURL       string     `dynamorm:"attr:remoteURL" json:"remote_url,omitempty"`      // URL if remote

	// TTL for auto-cleanup (30 days after last activity)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// NewThreadNode creates a new thread node
func NewThreadNode(rootStatusID, statusID, parentID string, depth int, authorID string) *ThreadNode {
	now := time.Now()
	return &ThreadNode{
		RootStatusID:    rootStatusID,
		StatusID:        statusID,
		ParentID:        parentID,
		Depth:           depth,
		ChildIDs:        []string{},
		AuthorID:        authorID,
		ReplyCount:      0,
		DescendantCount: 0,
		CreatedAt:       now,
		UpdatedAt:       now,
		FetchedAt:       now,
		TTL:             now.Add(30 * 24 * time.Hour).Unix(),
	}
}

// UpdateKeys updates the primary and GSI keys
func (n *ThreadNode) UpdateKeys() error {
	n.PK = fmt.Sprintf("THREAD#%s", n.RootStatusID)
	n.SK = fmt.Sprintf("NODE#%s", n.StatusID)
	n.GSI1PK = fmt.Sprintf(KeyPatternStatus, n.StatusID)
	n.GSI1SK = "THREAD_NODE"

	// Update TTL based on last activity
	lastActivity := n.UpdatedAt
	if n.LastReplyAt != nil && n.LastReplyAt.After(lastActivity) {
		lastActivity = *n.LastReplyAt
	}
	n.TTL = lastActivity.Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the primary key
func (n *ThreadNode) GetPK() string {
	return n.PK
}

// GetSK returns the sort key
func (n *ThreadNode) GetSK() string {
	return n.SK
}

// AddChild adds a child status ID to the node
func (n *ThreadNode) AddChild(childID string) {
	// Check if already exists
	for _, existing := range n.ChildIDs {
		if existing == childID {
			return
		}
	}
	n.ChildIDs = append(n.ChildIDs, childID)
	n.ReplyCount = len(n.ChildIDs)
	n.UpdatedAt = time.Now()
	now := time.Now()
	n.LastReplyAt = &now
}

// IncrementDescendantCount increments the descendant count
func (n *ThreadNode) IncrementDescendantCount(count int) {
	n.DescendantCount += count
	n.UpdatedAt = time.Now()
	now := time.Now()
	n.LastReplyAt = &now
}

// IsRoot checks if this is the root of the thread
func (n *ThreadNode) IsRoot() bool {
	return n.Depth == 0 || n.ParentID == ""
}

// IsLeaf checks if this node has no children
func (n *ThreadNode) IsLeaf() bool {
	return len(n.ChildIDs) == 0
}

// UpdatePath updates the path string
func (n *ThreadNode) UpdatePath(parentPath string) {
	if n.IsRoot() {
		n.Path = n.StatusID
	} else if parentPath != "" {
		n.Path = fmt.Sprintf("%s.%s", parentPath, n.StatusID)
	} else {
		n.Path = n.StatusID
	}
}

// TableName returns the DynamoDB table backing ThreadNode.
func (ThreadNode) TableName() string {
	return MainTableName
}
