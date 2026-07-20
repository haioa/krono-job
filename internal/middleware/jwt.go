package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/haioa/krono-job/internal/service/auth"
)

// JWT 返回校验 Bearer Token 的 Gin 中间件。
// 校验失败直接 401；成功将 *auth.Claims 存入 context 供后续 handler 使用。
func JWT(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少 Authorization 请求头"})
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization 格式应为 Bearer <token>"})
			return
		}
		claims, err := svc.ParseToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "令牌无效或已过期"})
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

// ClaimsFromContext 从 gin.Context 取出 JWT Claims。
func ClaimsFromContext(c *gin.Context) (*auth.Claims, bool) {
	v, ok := c.Get("claims")
	if !ok {
		return nil, false
	}
	claims, ok := v.(*auth.Claims)
	return claims, ok
}
