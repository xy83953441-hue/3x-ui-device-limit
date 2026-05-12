package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/logger"
	"gorm.io/gorm"
)

type SessionManagerService struct{}

// lastSeenThrottle tracks the last time we updated last_seen per session
// to avoid hammering the database on every request.
var lastSeenThrottle sync.Map

// GenerateSessionId generates a unique session ID
func (s *SessionManagerService) GenerateSessionId() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateDeviceId generates a device ID
func (s *SessionManagerService) GenerateDeviceId() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		logger.Warning("Failed to generate device ID:", err)
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// GetUserMaxDevices gets the maximum devices allowed for a user
func (s *SessionManagerService) GetUserMaxDevices(userId int) (int, error) {
	db := database.GetDB()
	var user model.User
	err := db.First(&user, userId).Error
	if err != nil {
		return 5, err
	}
	if user.MaxDevices <= 0 {
		return 0, nil
	}
	return user.MaxDevices, nil
}

// SetUserMaxDevices sets the maximum devices for a user
func (s *SessionManagerService) SetUserMaxDevices(userId int, maxDevices int) error {
	db := database.GetDB()
	return db.Model(&model.User{}).Where("id = ?", userId).Update("max_devices", maxDevices).Error
}

// nowUnix is a variable so it can be replaced in tests.
var nowUnix = func() int64 { return time.Now().Unix() }

// CheckDeviceLimit checks if the device limit has been reached.
// It counts sessions (excluding the optionally-provided one) whose
// last_seen falls within the active window.
func (s *SessionManagerService) CheckDeviceLimit(userId int, excludeSessionId string) error {
	maxDevices, err := s.GetUserMaxDevices(userId)
	if err != nil {
		logger.Warning("Failed to get user max devices:", err)
		return nil
	}

	if maxDevices == 0 {
		return nil
	}

	db := database.GetDB()
	var count int64

	query := db.Model(&model.UserSession{}).
		Where("user_id = ? AND last_seen > ?", userId, nowUnix()-24*3600)
	if excludeSessionId != "" {
		query = query.Where("session_id != ?", excludeSessionId)
	}

	if err := query.Count(&count).Error; err != nil {
		return err
	}

	if count >= int64(maxDevices) {
		return errors.New("已达到最大设备数限制，请先退出其他设备")
	}

	return nil
}

// RegisterSession registers a new session under a transaction so the
// device limit check and row creation are atomic (prevents TOCTOU races
// under concurrent login requests).
func (s *SessionManagerService) RegisterSession(userId int, sessionId, deviceId, userAgent, ip string) error {
	db := database.GetDB()
	now := nowUnix()

	return db.Transaction(func(tx *gorm.DB) error {
		// Reuse existing session row for the same device (within the active window).
		var existing model.UserSession
		err := tx.Where("user_id = ? AND device_id = ? AND last_seen > ?",
			userId, deviceId, now-24*3600).First(&existing).Error

		if err == nil {
			existing.SessionId = sessionId
			existing.UserAgent = userAgent
			existing.IpAddress = ip
			existing.LastSeen = now
			return tx.Save(&existing).Error
		}

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Count active sessions (excluding the new one we are about to insert).
		var count int64
		if err := tx.Model(&model.UserSession{}).
			Where("user_id = ? AND last_seen > ?", userId, now-24*3600).
			Count(&count).Error; err != nil {
			return err
		}

		maxDevices, err := s.GetUserMaxDevices(userId)
		if err != nil {
			logger.Warning("Failed to get user max devices:", err)
			// On lookup failure, allow the session (defensive default).
		} else if maxDevices > 0 && count >= int64(maxDevices) {
			return errors.New("已达到最大设备数限制，请先退出其他设备")
		}

		newSession := &model.UserSession{
			UserId:    userId,
			SessionId: sessionId,
			DeviceId:  deviceId,
			UserAgent: userAgent,
			IpAddress: ip,
			LoginAt:   now,
			LastSeen:  now,
		}

		return tx.Create(newSession).Error
	})
}

// UpdateSessionLastSeen updates the last seen time for a session.
// It throttles writes so the same session is persisted at most once per minute.
func (s *SessionManagerService) UpdateSessionLastSeen(sessionId string) error {
	if sessionId == "" {
		return nil
	}

	now := nowUnix()

	// Throttle: skip if we updated within the last 60 seconds.
	if last, ok := lastSeenThrottle.Load(sessionId); ok {
		if lt, ok2 := last.(int64); ok2 && now-lt < 60 {
			return nil
		}
	}

	db := database.GetDB()
	err := db.Model(&model.UserSession{}).Where("session_id = ?", sessionId).
		Update("last_seen", now).Error
	if err != nil {
		return err
	}

	lastSeenThrottle.Store(sessionId, now)
	return nil
}

// RemoveSession removes a session
func (s *SessionManagerService) RemoveSession(sessionId string) error {
	if sessionId == "" {
		return nil
	}
	lastSeenThrottle.Delete(sessionId)
	db := database.GetDB()
	return db.Where("session_id = ?", sessionId).Delete(&model.UserSession{}).Error
}

// RemoveUserAllSessions removes all sessions for a user
func (s *SessionManagerService) RemoveUserAllSessions(userId int) error {
	db := database.GetDB()
	return db.Where("user_id = ?", userId).Delete(&model.UserSession{}).Error
}

// GetUserSessions gets all active sessions for a user
func (s *SessionManagerService) GetUserSessions(userId int) ([]model.UserSession, error) {
	db := database.GetDB()
	var sessions []model.UserSession

	err := db.Where("user_id = ? AND last_seen > ?", userId, nowUnix()-24*3600).
		Order("last_seen DESC").Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// CleanExpiredSessions cleans up expired sessions
func (s *SessionManagerService) CleanExpiredSessions() error {
	db := database.GetDB()
	return db.Where("last_seen < ?", nowUnix()-24*3600).Delete(&model.UserSession{}).Error
}

// KickSession kicks a specific session
func (s *SessionManagerService) KickSession(sessionId string) error {
	return s.RemoveSession(sessionId)
}
