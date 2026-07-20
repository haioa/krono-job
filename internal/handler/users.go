package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/haioa/krono-job/internal/middleware"
	"github.com/haioa/krono-job/internal/model"
	"github.com/haioa/krono-job/internal/repository"
	"github.com/haioa/krono-job/internal/service/auth"
)

// UsersHandler 处理管理员用户的增删改查接口（受 JWT 保护）。
type UsersHandler struct {
	repo *repository.Repository
}

// NewUsersHandler 构造用户管理 Handler。
func NewUsersHandler(repo *repository.Repository) *UsersHandler {
	return &UsersHandler{repo: repo}
}

// userView 是返回给前端的安全用户视图（不含 password_hash）。
type userView struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toUserView(u *model.SysUser) userView {
	return userView{
		ID:        u.ID,
		Username:  u.Username,
		Nickname:  u.Nickname,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// List 处理 GET /api/users，返回全部管理员（不含密码哈希）。
func (h *UsersHandler) List(c *gin.Context) {
	users, err := h.repo.ListUsers(c.Request.Context())
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}
	list := make([]userView, 0, len(users))
	for i := range users {
		list = append(list, toUserView(&users[i]))
	}
	c.JSON(http.StatusOK, gin.H{"list": list})
}

type createUserRequest struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

// Create 处理 POST /api/users，创建管理员账户。
func (h *UsersHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名必填"})
		return
	}
	if len([]rune(req.Password)) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码长度至少 6 位"})
		return
	}
	status := req.Status
	if status == "" {
		status = model.UserStatusActive
	}
	if status != model.UserStatusActive && status != model.UserStatusDisabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status 取值非法"})
		return
	}

	// 用户名唯一性校验。
	if existing, gerr := h.repo.GetUserByUsername(c.Request.Context(), req.Username); gerr != nil {
		if errors.Is(gerr, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	} else if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}

	hash, herr := auth.HashPassword(req.Password)
	if herr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}
	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		nickname = req.Username
	}
	u := &model.SysUser{
		Username:     req.Username,
		Nickname:     nickname,
		PasswordHash: hash,
		Status:       status,
	}
	if err := h.repo.CreateUser(c.Request.Context(), u); err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败"})
		return
	}
	c.JSON(http.StatusCreated, toUserView(u))
}

type updateUserRequest struct {
	Nickname string `json:"nickname"`
	Status   string `json:"status"`
	Password string `json:"password"`
}

// Update 处理 PUT /api/users/:id，更新昵称、状态与（可选）密码。用户名不可修改。
func (h *UsersHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 必填"})
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体格式错误"})
		return
	}

	existing, err := h.repo.GetUserByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		nickname = existing.Nickname
		if strings.TrimSpace(nickname) == "" {
			nickname = existing.Username
		}
	}
	status := req.Status
	if status == "" {
		status = existing.Status
	}
	if status != model.UserStatusActive && status != model.UserStatusDisabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status 取值非法"})
		return
	}

	updated := &model.SysUser{
		ID:       existing.ID,
		Nickname: nickname,
		Status:   status,
	}
	if pwd := strings.TrimSpace(req.Password); pwd != "" {
		if len([]rune(pwd)) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "密码长度至少 6 位"})
			return
		}
		hash, herr := auth.HashPassword(pwd)
		if herr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}
		updated.PasswordHash = hash
	}

	if err := h.repo.UpdateUser(c.Request.Context(), updated); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新用户失败"})
		return
	}

	updated.Username = existing.Username
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now()
	c.JSON(http.StatusOK, toUserView(updated))
}

// Delete 处理 DELETE /api/users/:id，删除指定管理员。
// 禁止删除当前登录账号，避免把自己锁在控制台外。
func (h *UsersHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 必填"})
		return
	}
	if claims, ok := middleware.ClaimsFromContext(c); ok && claims.UserID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除当前登录的账号"})
		return
	}

	affected, err := h.repo.DeleteUser(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrDBUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败"})
		return
	}
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": affected, "message": "已删除用户"})
}
