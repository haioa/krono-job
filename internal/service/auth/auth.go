package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/haioa/krono-job/internal/config"
	"github.com/haioa/krono-job/internal/model"
	"github.com/haioa/krono-job/internal/repository"
)

// Claims 是 JWT 载荷，携带登录用户身份。
type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Service 封装登录鉴权与令牌签发逻辑。
type Service struct {
	repo         *repository.Repository
	secret       []byte
	expireHours  int
	bootstrapUser string
	bootstrapPass string
}

// New 构造鉴权服务。
// secret 为空时回退为随机临时密钥（仅本进程有效，生产应通过 KRONO_JWT_SECRET 注入）。
func New(repo *repository.Repository, cfg *config.Config) *Service {
	secret := cfg.JWT.Secret
	if secret == "" {
		secret = generateRandomSecret()
	}
	exp := cfg.JWT.ExpireHours
	if exp <= 0 {
		exp = 24
	}
	return &Service{
		repo:          repo,
		secret:        []byte(secret),
		expireHours:   exp,
		bootstrapUser: cfg.Bootstrap.AdminUser,
		bootstrapPass: cfg.Bootstrap.AdminPass,
	}
}

// HashPassword 使用 bcrypt 对明文密码加盐哈希。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(b), nil
}

// VerifyPassword 校验明文密码与 bcrypt 哈希是否匹配。
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateToken 为指定用户签发 HS256 JWT，并返回过期时间。
func (s *Service) GenerateToken(user *model.SysUser) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(time.Duration(s.expireHours) * time.Hour)
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// ParseToken 校验并解析 JWT，返回其 Claims。
func (s *Service) ParseToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// 业务错误，便于 handler 映射 HTTP 状态码。
var (
	ErrInvalidCredential = errors.New("用户名或密码错误")
	ErrUserDisabled      = errors.New("账号已被禁用")
	ErrWeakPassword      = errors.New("新密码长度至少 6 位")
)

// ChangePassword 直接为指定用户设置新密码（无需校验原密码）。
func (s *Service) ChangePassword(ctx context.Context, userID, newPassword string) error {
	if len([]rune(newPassword)) < 6 {
		return ErrWeakPassword
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if err := s.repo.UpdateUserPassword(ctx, userID, hash); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// Login 校验用户名/密码，成功返回用户、令牌与过期时间。
func (s *Service) Login(ctx context.Context, username, password string) (*model.SysUser, string, time.Time, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("query user: %w", err)
	}
	if user == nil {
		return nil, "", time.Time{}, ErrInvalidCredential
	}
	if user.Status != model.UserStatusActive {
		return nil, "", time.Time{}, ErrUserDisabled
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return nil, "", time.Time{}, ErrInvalidCredential
	}
	token, exp, err := s.GenerateToken(user)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return user, token, exp, nil
}

// BootstrapAdmin 在 sys_user 为空时，按环境变量配置插入首个管理员（决策 4）。
// 未配置 bootstrap 凭据或表非空则跳过。
func (s *Service) BootstrapAdmin(ctx context.Context) error {
	count, err := s.repo.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	user, pass := s.bootstrapUser, s.bootstrapPass
	if user == "" || pass == "" {
		return nil
	}
	hash, err := HashPassword(pass)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	u := &model.SysUser{
		Username:     user,
		PasswordHash: hash,
		Nickname:     "admin",
		Status:       model.UserStatusActive,
	}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}
	return nil
}

func generateRandomSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
