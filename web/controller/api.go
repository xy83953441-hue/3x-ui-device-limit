package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/web/entity"
	"github.com/mhsanaei/3x-ui/v3/web/middleware"
	"github.com/mhsanaei/3x-ui/v3/web/service"
	"github.com/mhsanaei/3x-ui/v3/web/session"

	"github.com/gin-gonic/gin"
)

// APIController handles the main API routes for the 3x-ui panel, including inbounds and server management.
type APIController struct {
	BaseController
	inboundController *InboundController
	serverController  *ServerController
	nodeController    *NodeController
	settingService    service.SettingService
	userService       service.UserService
	sessionManager    service.SessionManagerService
	Tgbot             service.Tgbot
}

// NewAPIController creates a new APIController instance and initializes its routes.
func NewAPIController(g *gin.RouterGroup, customGeo *service.CustomGeoService) *APIController {
	a := &APIController{}
	a.initRouter(g, customGeo)
	return a
}

// checkAPIAuth is a middleware that returns 404 for unauthenticated API requests
// to hide the existence of API endpoints from unauthorized users.
//
// Two auth paths are accepted:
//  1. Authorization: Bearer <apiToken> — used by remote central panels
//     polling this instance as a node. Matches via constant-time compare.
//     Sets c.Set("api_authed", true) so CSRFMiddleware can short-circuit.
//  2. Existing session cookie — used by browsers logged into the panel UI.
//
// Anything else falls through to a 404 so the API endpoints remain hidden.
func (a *APIController) checkAPIAuth(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		tok := strings.TrimPrefix(auth, "Bearer ")
		if a.settingService.MatchApiToken(tok) {
			// Handlers like InboundController.addInbound assume a logged-in
			// user (inbound.UserId = user.Id). Bearer callers have no
			// session, so attach the first user as a fallback. Single-user
			// panels are the norm here.
			if u, err := a.userService.GetFirstUser(); err == nil {
				session.SetAPIAuthUser(c, u)
			}
			c.Set("api_authed", true)
			c.Next()
			return
		}
	}
	if !session.IsLogin(c) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Next()
}

// initRouter sets up the API routes for inbounds, server, and other endpoints.
func (a *APIController) initRouter(g *gin.RouterGroup, customGeo *service.CustomGeoService) {
	// Main API group
	api := g.Group("/panel/api")
	api.Use(a.checkAPIAuth)
	api.Use(middleware.CSRFMiddleware())

	// Inbounds API
	inbounds := api.Group("/inbounds")
	a.inboundController = NewInboundController(inbounds)

	// Server API
	server := api.Group("/server")
	a.serverController = NewServerController(server)

	// Nodes API — multi-panel management
	nodes := api.Group("/nodes")
	a.nodeController = NewNodeController(nodes)

	NewCustomGeoController(api.Group("/custom-geo"), customGeo)

	// Extra routes
	api.GET("/backuptotgbot", a.BackuptoTgbot)

	// User sessions API - 设备限制功能
	api.GET("/user/sessions", a.getUserSessions)
	api.PUT("/user/max-devices", a.setMaxDevices)
	api.GET("/user/max-devices", a.getMaxDevices)
	api.DELETE("/user/sessions/:sessionId", a.kickSession)
	api.GET("/user/current-session", a.getCurrentSession)
	api.POST("/user/register-session", a.registerSession)
	api.DELETE("/user/session", a.removeCurrentSession)
}

// BackuptoTgbot sends a backup of the panel data to Telegram bot admins.
func (a *APIController) BackuptoTgbot(c *gin.Context) {
	a.Tgbot.SendBackupToAdmins()
}

// getUserSessions 获取当前用户的会话列表
func (a *APIController) getUserSessions(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, entity.Msg{Success: false, Msg: "未登录"})
		return
	}

	sessionId := session.GetSessionId(c)
	sessions, err := a.sessionManager.GetUserSessions(user.Id, sessionId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, entity.Msg{Success: false, Msg: err.Error()})
		return
	}

	type SessionInfo struct {
		SessionId string `json:"sessionId"`
		UserAgent string `json:"userAgent"`
		IpAddress string `json:"ipAddress"`
		LoginAt   string `json:"loginAt"`
		LastSeen  string `json:"lastSeen"`
		IsCurrent bool   `json:"isCurrent"`
	}

	var sessionList []SessionInfo
	for _, s := range sessions {
		isCurrent := s.SessionId == sessionId
		sessionList = append(sessionList, SessionInfo{
			SessionId: s.SessionId,
			UserAgent: s.UserAgent,
			IpAddress: s.IpAddress,
			LoginAt:   time.Unix(s.LoginAt, 0).Format("2006-01-02 15:04:05"),
			LastSeen:  time.Unix(s.LastSeen, 0).Format("2006-01-02 15:04:05"),
			IsCurrent: isCurrent,
		})
	}

	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: sessionList})
}

// setMaxDevices 设置最大设备数
func (a *APIController) setMaxDevices(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, entity.Msg{Success: false, Msg: "未登录"})
		return
	}

	var req struct {
		MaxDevices int `json:"maxDevices"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, entity.Msg{Success: false, Msg: "参数错误"})
		return
	}

	if req.MaxDevices < 0 || req.MaxDevices > 20 {
		c.JSON(http.StatusBadRequest, entity.Msg{Success: false, Msg: "设备数必须在0-20之间，0表示不限制"})
		return
	}

	err := a.sessionManager.SetUserMaxDevices(user.Id, req.MaxDevices)
	if err != nil {
		c.JSON(http.StatusInternalServerError, entity.Msg{Success: false, Msg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, entity.Msg{Success: true, Msg: "设置成功"})
}

// getMaxDevices 获取最大设备数
func (a *APIController) getMaxDevices(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, entity.Msg{Success: false, Msg: "未登录"})
		return
	}

	maxDevices, err := a.sessionManager.GetUserMaxDevices(user.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, entity.Msg{Success: false, Msg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: maxDevices})
}

// kickSession 踢出指定会话
func (a *APIController) kickSession(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, entity.Msg{Success: false, Msg: "未登录"})
		return
	}

	sessionId := c.Param("sessionId")
	if sessionId == "" {
		c.JSON(http.StatusBadRequest, entity.Msg{Success: false, Msg: "会话ID不能为空"})
		return
	}

	currentSessionId := session.GetSessionId(c)
	if sessionId == currentSessionId {
		c.JSON(http.StatusBadRequest, entity.Msg{Success: false, Msg: "不能踢出自己的会话"})
		return
	}

	err := a.sessionManager.KickSession(sessionId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, entity.Msg{Success: false, Msg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, entity.Msg{Success: true, Msg: "已踢出该设备"})
}

// getCurrentSession 获取当前会话ID
func (a *APIController) getCurrentSession(c *gin.Context) {
	sessionId := session.GetSessionId(c)
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: sessionId})
}

// registerSession 注册会话
func (a *APIController) registerSession(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, entity.Msg{Success: false, Msg: "未登录"})
		return
	}

	sessionId := session.GetSessionId(c)
	deviceId := session.GetDeviceId(c)
	userAgent := c.Request.UserAgent()
	ip := c.ClientIP()

	err := a.sessionManager.RegisterSession(user.Id, sessionId, deviceId, userAgent, ip)
	if err != nil {
		c.JSON(http.StatusForbidden, entity.Msg{Success: false, Msg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, entity.Msg{Success: true, Msg: "会话已注册"})
}

// removeCurrentSession 删除当前会话
func (a *APIController) removeCurrentSession(c *gin.Context) {
	sessionId := session.GetSessionId(c)
	if sessionId != "" {
		a.sessionManager.RemoveSession(sessionId)
	}
	session.ClearSession(c)
	c.JSON(http.StatusOK, entity.Msg{Success: true, Msg: "已登出"})
}
