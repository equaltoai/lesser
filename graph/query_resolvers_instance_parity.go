package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/translation"
	"go.uber.org/zap"
)

// Instance returns instance information (covers Mastodon v1 + v2 instance endpoints).
func (r *queryResolver) Instance(ctx context.Context) (*model.InstanceInfo, error) {
	if r.Config == nil {
		return nil, errors.New("config is not available")
	}
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	instanceConfig := config.GetInstanceConfig()

	state, stateErr := r.Storage.Instance().GetInstanceState(ctx)
	locked := stateErr != nil || state == nil || state.Locked

	rules, err := r.Storage.Instance().GetInstanceRules(ctx)
	if err != nil {
		r.Logger.Warn("failed to get instance rules", zap.Error(err))
		rules = []storage.InstanceRule{}
	}

	ruleModels := make([]*model.InstanceRule, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		if err := common.ValidateRequiredParam("rule_id", rule.ID); err != nil {
			continue
		}
		ruleModels = append(ruleModels, &model.InstanceRule{
			ID:   rule.ID,
			Text: rule.Text,
		})
	}

	contactAccount := r.resolveInstanceContactAccount(ctx)

	userCount, err := r.Storage.Analytics().GetTotalUserCount(ctx)
	if err != nil {
		r.Logger.Warn("failed to get user count", zap.Error(err))
		userCount = 0
	}

	statusCount, err := r.Storage.Instance().GetTotalStatusCount(ctx)
	if err != nil {
		r.Logger.Warn("failed to get status count", zap.Error(err))
		statusCount = 0
	}

	domainCount, err := r.Storage.Instance().GetTotalDomainCount(ctx)
	if err != nil {
		r.Logger.Warn("failed to get domain count", zap.Error(err))
		domainCount = 0
	}

	return &model.InstanceInfo{
		Domain:            r.Config.Domain,
		Title:             instanceConfig.Title,
		ShortDescription:  optionalString(instanceConfig.ShortDescription),
		Description:       instanceConfig.Description,
		Email:             optionalString(instanceConfig.Email),
		Version:           instanceConfig.Version,
		SourceURL:         optionalString("https://github.com/equaltoai/lesser"),
		StreamingURL:      optionalString(r.Config.BaseURL()),
		ThumbnailURL:      optionalString(r.Config.BaseURL() + "/assets/thumbnail.png"),
		Languages:         instanceConfig.Languages,
		RegistrationsOpen: instanceConfig.RegistrationsOpen && !locked,
		ApprovalRequired:  instanceConfig.ApprovalRequired,
		InvitesEnabled:    instanceConfig.InvitesEnabled,
		UserCount:         userCount,
		StatusCount:       int(statusCount),
		DomainCount:       int(domainCount),
		ContactAccount:    contactAccount,
		Rules:             ruleModels,
	}, nil
}

// InstanceActivity returns weekly instance activity metrics.
func (r *queryResolver) InstanceActivity(ctx context.Context, limit *int) ([]*model.InstanceActivityEntry, error) {
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil, ErrStorageUnavailable
	}

	weeks := 12
	if limit != nil && *limit > 0 {
		if *limit > 52 {
			weeks = 52
		} else {
			weeks = *limit
		}
	}

	activity := make([]*model.InstanceActivityEntry, 0, weeks)

	now := time.Now()
	weekStart := now.Truncate(24 * time.Hour)
	for weekStart.Weekday() != time.Monday {
		weekStart = weekStart.Add(-24 * time.Hour)
	}

	for i := 0; i < weeks; i++ {
		thisWeekStart := weekStart.AddDate(0, 0, -7*i)
		weekTimestamp := thisWeekStart.Unix()

		weekActivity, err := r.Storage.Instance().GetWeeklyActivity(ctx, weekTimestamp)
		if err != nil || weekActivity == nil {
			r.Logger.Warn("failed to get weekly activity",
				zap.Int64("week", weekTimestamp),
				zap.Error(err))
			weekActivity = &storage.WeeklyActivity{}
		}

		activity = append(activity, &model.InstanceActivityEntry{
			Week:          fmt.Sprintf("%d", weekTimestamp),
			Statuses:      weekActivity.Statuses,
			Logins:        weekActivity.Logins,
			Registrations: weekActivity.Registrations,
		})
	}

	return activity, nil
}

// InstancePeers returns connected domains (federation peers).
func (r *queryResolver) InstancePeers(ctx context.Context, limit *int) ([]string, error) {
	if r.Config == nil {
		return []string{}, nil
	}
	if r.Storage == nil || r.Storage.Search() == nil {
		return nil, ErrStorageUnavailable
	}

	lim := 100
	if limit != nil && *limit > 0 {
		if *limit > 200 {
			lim = 200
		} else {
			lim = *limit
		}
	}

	actors, err := r.Storage.Search().SearchAccounts(ctx, "@", lim, false, 0)
	if err != nil {
		r.Logger.Warn("failed to search for remote actors", zap.Error(err))
		return []string{}, nil
	}

	domainMap := make(map[string]bool)
	for _, actor := range actors {
		if actor == nil {
			continue
		}
		if strings.Contains(actor.ID, "https://") && !strings.Contains(actor.ID, r.Config.Domain) {
			parts := strings.Split(actor.ID, "/")
			if len(parts) >= 3 {
				domain := strings.Replace(parts[2], "www.", "", 1)
				if domain != "" && domain != r.Config.Domain {
					domainMap[domain] = true
				}
			}
		}
	}

	peers := make([]string, 0, len(domainMap))
	for domain := range domainMap {
		peers = append(peers, domain)
	}
	sort.Strings(peers)
	return peers, nil
}

// InstanceDomainBlocks returns public domain blocks.
func (r *queryResolver) InstanceDomainBlocks(ctx context.Context, limit *int) ([]*model.InstanceDomainBlock, error) {
	if r.Storage == nil || r.Storage.DomainBlock() == nil {
		return nil, ErrStorageUnavailable
	}

	lim := 100
	if limit != nil && *limit > 0 {
		if *limit > 200 {
			lim = 200
		} else {
			lim = *limit
		}
	}

	domainBlocks, _, err := r.Storage.DomainBlock().ListInstanceDomainBlocks(ctx, lim, "")
	if err != nil {
		r.Logger.Warn("failed to get instance domain blocks", zap.Error(err))
		return []*model.InstanceDomainBlock{}, nil
	}

	blocks := make([]*model.InstanceDomainBlock, 0, len(domainBlocks))
	for _, block := range domainBlocks {
		if block == nil {
			continue
		}
		if block.Obfuscate || block.PublicComment == "" {
			continue
		}

		hash := sha256.Sum256([]byte(block.Domain))
		digest := hex.EncodeToString(hash[:])

		blocks = append(blocks, &model.InstanceDomainBlock{
			Domain:   block.Domain,
			Digest:   digest,
			Severity: block.Severity,
			Comment:  block.PublicComment,
		})
	}

	return blocks, nil
}

// TranslationLanguages returns supported translation languages.
func (r *queryResolver) TranslationLanguages(ctx context.Context) ([]*model.TranslationLanguage, error) {
	if r.Config == nil || !r.Config.TranslationEnabled {
		return nil, errors.New("translation service is not enabled")
	}
	if r.Storage == nil {
		return nil, ErrStorageUnavailable
	}

	translationSvc, err := translation.NewService(ctx, r.Config, r.Storage, r.Logger, true)
	if err != nil {
		r.Logger.Error("failed to initialize translation service", zap.Error(err))
		return nil, errors.Join(errors.New("translation service initialization failed"), err)
	}

	supportedLangs, err := translationSvc.GetSupportedLanguages(ctx)
	if err != nil {
		r.Logger.Error("failed to get supported languages", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get supported languages"), err)
	}

	languages := make([]*model.TranslationLanguage, 0, len(supportedLangs))
	for _, lang := range supportedLangs {
		languages = append(languages, &model.TranslationLanguage{
			Code: lang.Code,
			Name: lang.Name,
		})
	}

	return languages, nil
}

func (r *queryResolver) resolveInstanceContactAccount(ctx context.Context) *activitypub.Actor {
	if r.Storage == nil || r.Storage.Instance() == nil {
		return nil
	}

	contact, err := r.Storage.Instance().GetContactAccount(ctx)
	if err != nil || contact == nil || contact.Username == "" {
		return nil
	}

	if r.Registry != nil && r.Registry.Accounts() != nil {
		account, err := r.Registry.Accounts().GetAccount(ctx, contact.Username)
		if err == nil && account != nil {
			return r.convertAccountToActor(account)
		}
	}

	return nil
}
