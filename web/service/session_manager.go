package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/logger"
)

type SessionManagerService struct{}

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
		// Fallback to a partial ID with timestamp
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

// GetActiveSessionCount gets the count of active sessions for a user
func (s *SessionManagerService) GetActiveSessionCount(userId int) (int64, error) {
	db := database.GetDB()
	var count int64
	err := db.Model(&model.UserSession{}).Where("user_id = ? AND last_seen > ?",
		userId, time.Now().Add(-24*time.Hour).Unix()).Count(&count).Error
	return count, err
}

// CheckDeviceLimit checks if the device limit has been reached
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
	var sessions []model.UserSession

	query := db.Where("user_id = ? AND last_seen > ?", userId, time.Now().Add(-24*time.Hour).Unix())
	if excludeSessionId != "" {
		query = query.Where("session_id != ?", excludeSessionId)
	}

	if err := query.Find(&sessions).Error; err != nil {
		return err
	}

	if int64(len(sessions)) >= int64(maxDevices) {
		return errors.New("已达到最大设备数限制，请先退出其他设备")
	}

	return nil
}

// RegisterSession registers a new session
func (s *SessionManagerService) RegisterSession(userId int, sessionId, deviceId, userAgent, ip string) error {
	db := database.GetDB()

	var existingSession model.UserSession
	err := db.Where("user_id = ? AND device_id = ? AND last_seen > ?",
		userId, deviceId, time.Now().Add(-24*time.Hour).Unix()).First(&existingSession).Error

	if err == nil {
		existingSession.SessionId = sessionId
		existingSession.UserAgent = userAgent
		existingSession.IpAddress = ip
		existingSession.LastSeen = time.Now().Unix()
		return db.Save(&existingSession).Error
	}

	if err := s.CheckDeviceLimit(userId, ""); err != nil {
		return err
	}

	newSession := &model.UserSession{
		UserId:    userId,
		SessionId: sessionId,
		DeviceId:  deviceId,
		UserAgent: userAgent,
		IpAddress: ip,
		LoginAt:   time.Now().Unix(),
		LastSeen:  time.Now().Unix(),
	}

	return db.Create(newSession).Error
}

// UpdateSessionLastSeen updates the last seen time for a session
func (s *SessionManagerService) UpdateSessionLastSeen(sessionId string) error {
	db := database.GetDB()
	return db.Model(&model.UserSession{}).Where("session_id = ?", sessionId).
		Update("last_seen", time.Now().Unix()).Error
}

// RemoveSession removes a session
func (s *SessionManagerService) RemoveSession(sessionId string) error {
	db := database.GetDB()
	return db.Where("session_id = ?", sessionId).Delete(&model.UserSession{}).Error
}

// RemoveUserAllSessions removes all sessions for a user
func (s *SessionManagerService) RemoveUserAllSessions(userId int) error {
	db := database.GetDB()
	return db.Where("user_id = ?", userId).Delete(&model.UserSession{}).Error
}

// GetUserSessions gets all sessions for a user
func (s *SessionManagerService) GetUserSessions(userId int, currentSessionId string) ([]model.UserSession, error) {
	db := database.GetDB()
	var sessions []model.UserSession

	err := db.Where("user_id = ? AND last_seen > ?", userId, time.Now().Add(-24*time.Hour).Unix()).
		Order("last_seen DESC").Find(&sessions).Error

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// CleanExpiredSessions cleans up expired sessions
func (s *SessionManagerService) CleanExpiredSessions() error {
	db := database.GetDB()
	return db.Where("last_seen < ?", time.Now().Add(-24*time.Hour).Unix()).Delete(&model.UserSession{}).Error
}

// KickSession kicks a specific session
func (s *SessionManagerService) KickSession(sessionId string) error {
	return s.RemoveSession(sessionId)
}
