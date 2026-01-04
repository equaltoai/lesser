package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
)

const (
	adminAccountIDPrefix = "user-"
	adminRoleAdmin       = "admin"
	adminRoleModerator   = "moderator"
	adminRoleUser        = "user"
)

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeAdminAccountID(id string) (accountID string, username string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ""
	}

	if strings.HasPrefix(id, adminAccountIDPrefix) {
		username = strings.TrimPrefix(id, adminAccountIDPrefix)
		return id, username
	}

	username = id
	return fmt.Sprintf("%s%s", adminAccountIDPrefix, username), username
}

func adminRoleID(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case adminRoleAdmin:
		return "3"
	case adminRoleModerator:
		return "2"
	default:
		return "1"
	}
}

func adminRolePermissions(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case adminRoleAdmin:
		return 0xFFFFFFFF
	case adminRoleModerator:
		return 0x0000FFFF
	default:
		return 0x00000001
	}
}

func adminRoleFromName(role string) *model.AdminRole {
	role = strings.TrimSpace(role)
	if role == "" {
		role = adminRoleUser
	}
	return &model.AdminRole{
		ID:          adminRoleID(role),
		Name:        role,
		Permissions: adminRolePermissions(role),
	}
}

func deriveAdminIPInfo(sessions []*storage.Session) (*string, []*model.AdminIP) {
	if len(sessions) == 0 {
		return nil, []*model.AdminIP{}
	}

	filtered := make([]*storage.Session, 0, len(sessions))
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		filtered = append(filtered, sess)
	}
	if len(filtered) == 0 {
		return nil, []*model.AdminIP{}
	}

	sessions = filtered

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActivity.After(sessions[j].LastActivity)
	})

	var lastIP *string
	if sessions[0] != nil && strings.TrimSpace(sessions[0].IPAddress) != "" {
		ip := strings.TrimSpace(sessions[0].IPAddress)
		lastIP = &ip
	}

	ipLastSeen := make(map[string]time.Time)
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		ip := strings.TrimSpace(sess.IPAddress)
		if ip == "" {
			continue
		}
		if existing, ok := ipLastSeen[ip]; ok && existing.After(sess.LastActivity) {
			continue
		}
		ipLastSeen[ip] = sess.LastActivity
	}

	ips := make([]*model.AdminIP, 0, len(ipLastSeen))
	for ip, usedAt := range ipLastSeen {
		ips = append(ips, &model.AdminIP{
			IP:     ip,
			UsedAt: model.Time(usedAt),
		})
	}

	sort.Slice(ips, func(i, j int) bool {
		return time.Time(ips[i].UsedAt).After(time.Time(ips[j].UsedAt))
	})

	return lastIP, ips
}

func cleanDomainValue(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")

	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	domain = strings.TrimSuffix(domain, ":443")
	domain = strings.TrimSuffix(domain, ":80")

	return strings.ToLower(strings.TrimSpace(domain))
}

func toJSONPointer(value any) *string {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := strings.TrimSpace(string(data))
	if out == "" || out == "null" {
		return nil
	}
	return &out
}
