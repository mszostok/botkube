package notifier

import (
	"github.com/kubeshop/botkube/internal/health"
	"github.com/kubeshop/botkube/pkg/config"
)

type Platform interface {
	GetStatus() health.PlatformStatus
	IntegrationName() config.CommPlatformIntegration
}
