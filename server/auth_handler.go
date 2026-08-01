/*
 * 星垣 - 认证相关处理器
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"log"
	"net/http"
	"strings"
	"xingyuan-monitor/auth"

	"github.com/gin-gonic/gin"
)

// authMiddleware 认证中间件
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取 Authorization 头
		authHeader := c.GetHeader("Authorization")

		// 如果没有 Token，返回 401
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "未提供认证信息",
			})
			c.Abort()
			return
		}

		// 解析 Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的认证格式",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 验证 Access Token
		claims, err := s.authManager.ValidateToken(tokenString, auth.TokenTypeAccess)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Token无效或已过期",
			})
			c.Abort()
			return
		}

		// 将用户名存入上下文
		c.Set("username", claims.Username)
		c.Next()
	}
}

// handleLogin 处理登录
func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Remember bool   `json:"remember"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}

	// 获取客户端IP
	clientIP := c.ClientIP()

	// 验证用户名和密码
	err := s.authManager.Authenticate(req.Username, req.Password, clientIP)
	if err != nil {
		if err == auth.ErrTooManyAttempts {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "登录失败次数过多，请稍后再试",
			})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "用户名或密码错误",
			})
		}
		return
	}

	// 生成 Token 对
	accessToken, refreshToken, err := s.authManager.GenerateTokenPair(req.Username, req.Remember)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "生成Token失败",
		})
		return
	}

	log.Printf("用户 %s 登录成功，IP: %s", req.Username, clientIP)

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "登录成功",
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// handleLogout 处理登出
func (s *Server) handleLogout(c *gin.Context) {
	// 撤销 Access Token
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 {
			tokenString := parts[1]
			// 撤销 Token
			s.authManager.RevokeToken(tokenString)
		}
	}

	// 同时撤销请求体中的 Refresh Token（否则登出后仍可用它换取新Token）
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
		if err := s.authManager.RevokeToken(req.RefreshToken); err != nil {
			log.Printf("撤销 Refresh Token 失败: %v", err)
		}
	}

	username, _ := c.Get("username")
	log.Printf("用户 %v 登出", username)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "登出成功",
	})
}

// handleRefresh 刷新 Access Token
func (s *Server) handleRefresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}

	// 验证 Refresh Token
	claims, err := s.authManager.ValidateToken(req.RefreshToken, auth.TokenTypeRefresh)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Refresh Token无效或已过期",
		})
		return
	}

	// 生成新的 Token 对
	accessToken, refreshToken, err := s.authManager.GenerateTokenPair(claims.Username, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "生成Token失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// handleVerify 验证 Token 是否有效
func (s *Server) handleVerify(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"valid": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":    true,
		"username": username,
	})
}
