package graph

import (
	"errors"

	"github.com/equaltoai/lesser/pkg/config"
)

var (
	errCMSDisabled           = errors.New("cms is disabled")
	errCMSDraftsDisabled     = errors.New("cms drafts are disabled")
	errCMSRevisionsDisabled  = errors.New("cms revision history is disabled")
	errCMSSchedulingDisabled = errors.New("cms scheduled publishing is disabled")
	errCMSSeriesDisabled     = errors.New("cms series is disabled")
	errCMSCategoriesDisabled = errors.New("cms categories is disabled")
)

func (r *Resolver) cmsConfig() *config.Config {
	if r == nil {
		return nil
	}
	if r.Config != nil {
		return r.Config
	}
	if r.Registry != nil {
		serviceCfg := r.Registry.GetConfig()
		if serviceCfg != nil {
			return serviceCfg.Config
		}
	}
	return nil
}

func (r *Resolver) cmsLongFormEnabled() bool {
	cfg := r.cmsConfig()
	if cfg == nil {
		return true
	}
	return cfg.CMSLongFormEnabled()
}

func (r *Resolver) cmsDraftsEnabled() bool {
	cfg := r.cmsConfig()
	if cfg == nil {
		return true
	}
	return cfg.CMSLongFormEnabled() && cfg.CMSDraftsEnabled()
}

func (r *Resolver) cmsRevisionsEnabled() bool {
	cfg := r.cmsConfig()
	if cfg == nil {
		return true
	}
	return cfg.CMSLongFormEnabled() && cfg.CMSRevisionsEnabled()
}

func (r *Resolver) cmsSchedulingEnabled() bool {
	cfg := r.cmsConfig()
	if cfg == nil {
		return true
	}
	return cfg.CMSLongFormEnabled() && cfg.CMSSchedulingEnabled()
}

func (r *Resolver) cmsSeriesEnabled() bool {
	cfg := r.cmsConfig()
	if cfg == nil {
		return true
	}
	return cfg.CMSLongFormEnabled() && cfg.CMSSeriesAllowed()
}

func (r *Resolver) cmsCategoriesEnabled() bool {
	cfg := r.cmsConfig()
	if cfg == nil {
		return true
	}
	return cfg.CMSLongFormEnabled() && cfg.CMSCategoriesAllowed()
}

func (r *Resolver) requireCMSLongFormEnabled() error {
	if !r.cmsLongFormEnabled() {
		return errCMSDisabled
	}
	return nil
}

func (r *Resolver) requireCMSDraftsEnabled() error {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return err
	}
	if !r.cmsDraftsEnabled() {
		return errCMSDraftsDisabled
	}
	return nil
}

func (r *Resolver) requireCMSRevisionsEnabled() error {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return err
	}
	if !r.cmsRevisionsEnabled() {
		return errCMSRevisionsDisabled
	}
	return nil
}

func (r *Resolver) requireCMSSchedulingEnabled() error {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return err
	}
	if !r.cmsSchedulingEnabled() {
		return errCMSSchedulingDisabled
	}
	return nil
}

func (r *Resolver) requireCMSSeriesEnabled() error {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return err
	}
	if !r.cmsSeriesEnabled() {
		return errCMSSeriesDisabled
	}
	return nil
}

func (r *Resolver) requireCMSCategoriesEnabled() error {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return err
	}
	if !r.cmsCategoriesEnabled() {
		return errCMSCategoriesDisabled
	}
	return nil
}
