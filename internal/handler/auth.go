package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/haioa/krono-job/internal/middleware"
	"github.com/haioa/krono-job/internal/service/auth"
)

// AuthHandler 处理认证相关 HTTP 接口。
type AuthHandler struct {
	auth *auth.Service
}

// NewAuthHandler 构造认证 Handler。
func NewAuthHandler(svc *auth.Service) *AuthHandler {
	return &AuthHandler{auth: svc}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 处理 POST /api/auth/login。
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码必填"})
		return
	}

	user, token, exp, err := h.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		switch err {
		case auth.ErrInvalidCredential, auth.ErrUserDisabled:
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_at": exp,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
		},
	})
}

type changePasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ChangePassword 处理 POST /api/auth/change-password。
// 无需原密码，直接为当前登录用户设置新密码。
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	claims, ok := middleware.ClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	if req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码必填"})
		return
	}

	if err := h.auth.ChangePassword(c.Request.Context(), claims.UserID, req.NewPassword); err != nil {
		switch err {
		case auth.ErrWeakPassword:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}
