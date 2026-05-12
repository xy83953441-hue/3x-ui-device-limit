package job

import (
	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/web/service"
)

// CleanExpiredSessionsJob cleans up expired user sessions from the database.
type CleanExpiredSessionsJob struct {
	sessionManager service.SessionManagerService
}

// NewCleanExpiredSessionsJob creates a new expired session cleanup job.
func NewCleanExpiredSessionsJob() *CleanExpiredSessionsJob {
	return new(CleanExpiredSessionsJob)
}

// Run removes all sessions whose last_seen is older than 24 hours.
func (j *CleanExpiredSessionsJob) Run() {
	if err := j.sessionManager.CleanExpiredSessions(); err != nil {
		logger.Warning("CleanExpiredSessionsJob: failed to clean expired sessions:", err)
	}
}
