package services

import (
	"go.uber.org/zap"
)

// ServiceFactory creates and manages service instances
type ServiceFactory struct {
	deps *ServiceDependencies
}

// NewServiceFactory creates a new service factory
func NewServiceFactory(repos interface{}, config *ServiceConfig, logger *zap.Logger) *ServiceFactory {
	deps := &ServiceDependencies{
		Repos:  repos,
		Config: config,
		Logger: logger,
	}

	return &ServiceFactory{
		deps: deps,
	}
}

// CreateBusinessLogicService creates a fully configured business logic service
func (f *ServiceFactory) CreateBusinessLogicService() BusinessLogicService {
	// Create dependency services
	validation := f.CreateValidationService()
	auth := f.CreateAuthenticationService()
	federation := f.CreateFederationService()
	timeline := f.CreateTimelineService()
	analytics := f.CreateAnalyticsService()

	// Create and return the business logic service
	// Create job queue service
	jobQueue, err := NewJobQueueService(f.deps.Config.Config, f.deps.Logger.(*zap.Logger))
	if err != nil {
		f.deps.Logger.(*zap.Logger).Warn("failed to create job queue service, using nil", zap.Error(err))
		jobQueue = nil
	}

	return NewBusinessLogicService(
		f.deps,
		validation,
		auth,
		federation,
		timeline,
		analytics,
		nil, // Factory doesn't have publisher access, events won't be emitted
		jobQueue,
	)
}

// CreateValidationService creates a validation service
func (f *ServiceFactory) CreateValidationService() ValidationService {
	return NewValidationService(f.deps.Config)
}

// CreateAuthenticationService creates an authentication service
func (f *ServiceFactory) CreateAuthenticationService() AuthenticationService {
	return NewAuthenticationService(f.deps.Config.JWTSecret, f.deps.Config.Config, f.deps.Repos)
}

// CreateFederationService creates a federation service
func (f *ServiceFactory) CreateFederationService() FederationService {
	return NewFederationService(f.deps)
}

// CreateTimelineService creates a timeline service
func (f *ServiceFactory) CreateTimelineService() TimelineService {
	return NewTimelineService(f.deps)
}

// CreateAnalyticsService creates an analytics service
func (f *ServiceFactory) CreateAnalyticsService() AnalyticsService {
	return NewAnalyticsService(f.deps)
}

// CreateNotificationService creates a notification service
func (f *ServiceFactory) CreateNotificationService() NotificationService {
	return NewNotificationService(f.deps)
}

// GetServiceDependencies returns the service dependencies for custom service creation
func (f *ServiceFactory) GetServiceDependencies() *ServiceDependencies {
	return f.deps
}
