package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 用 Zap 记录 panic 并返回 500，替代 gin 默认 recovery。
func Recovery(log *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorw("panic recovered",
					"path", c.Request.URL.Path,
					"error", err,
					"stack", string(debug.Stack()))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}
