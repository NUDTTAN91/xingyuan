/*
 * 星垣 - 认证模块
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package auth

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Token 类型
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// 错误定义
var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrTokenExpired       = errors.New("Token已过期")
	ErrTokenInvalid       = errors.New("Token无效")
	ErrTooManyAttempts    = errors.New("登录失败次数过多，请稍后再试")
)

// Claims JWT Claims
type Claims struct {
	Username  string `json:"username"`
	TokenType string `json:"type"`
	jwt.RegisteredClaims
}

// LoginAttempt 登录尝试记录
type LoginAttempt struct {
	Count      int
	LastAttempt time.Time
	LockedUntil time.Time
}

// AuthManager 认证管理器
type AuthManager struct {
	jwtSecret          []byte
	accessTokenExpire  time.Duration
	refreshTokenExpire time.Duration
	maxLoginAttempts   int
	lockDuration       time.Duration
	
	// 登录失败记录（IP -> 尝试记录）
	loginAttempts map[string]*LoginAttempt
	attemptsMu    sync.RWMutex
	
	// Token 黑名单（JTI -> 过期时间）
	tokenBlacklist map[string]time.Time
	blacklistMu    sync.RWMutex
}

// NewAuthManager 创建认证管理器
func NewAuthManager() *AuthManager {
	// 从环境变量读取配置
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "xingyuan-default-secret-please-change-in-production"
	}
	
	accessExpireMin, _ := strconv.Atoi(os.Getenv("ACCESS_TOKEN_EXPIRE_MINUTES"))
	if accessExpireMin == 0 {
		accessExpireMin = 120 // 默认2小时
	}
	
	refreshExpireDays, _ := strconv.Atoi(os.Getenv("REFRESH_TOKEN_EXPIRE_DAYS"))
	if refreshExpireDays == 0 {
		refreshExpireDays = 7 // 默认7天
	}
	
	maxAttempts, _ := strconv.Atoi(os.Getenv("MAX_LOGIN_ATTEMPTS"))
	if maxAttempts == 0 {
		maxAttempts = 5 // 默认5次
	}
	
	lockMinutes, _ := strconv.Atoi(os.Getenv("LOGIN_LOCK_MINUTES"))
	if lockMinutes == 0 {
		lockMinutes = 15 // 默认15分钟
	}
	
	am := &AuthManager{
		jwtSecret:          []byte(jwtSecret),
		accessTokenExpire:  time.Duration(accessExpireMin) * time.Minute,
		refreshTokenExpire: time.Duration(refreshExpireDays) * 24 * time.Hour,
		maxLoginAttempts:   maxAttempts,
		lockDuration:       time.Duration(lockMinutes) * time.Minute,
		loginAttempts:      make(map[string]*LoginAttempt),
		tokenBlacklist:     make(map[string]time.Time),
	}
	
	// 启动清理任务
	go am.cleanupRoutine()
	
	return am
}

// Authenticate 验证用户名和密码
func (am *AuthManager) Authenticate(username, password, clientIP string) error {
	// 检查是否被锁定
	if am.isLocked(clientIP) {
		return ErrTooManyAttempts
	}
	
	// 从环境变量获取管理员账号
	adminUser := os.Getenv("ADMIN_USERNAME")
	adminPass := os.Getenv("ADMIN_PASSWORD")
	
	if adminUser == "" {
		adminUser = "admin"
	}
	if adminPass == "" {
		adminPass = "admin123"
	}
	
	// 验证用户名
	if username != adminUser {
		am.recordFailedAttempt(clientIP)
		return ErrInvalidCredentials
	}
	
	// 验证密码（支持明文和bcrypt两种方式）
	err := bcrypt.CompareHashAndPassword([]byte(adminPass), []byte(password))
	if err != nil {
		// 如果bcrypt验证失败，尝试明文比较（兼容性）
		if password != adminPass {
			am.recordFailedAttempt(clientIP)
			return ErrInvalidCredentials
		}
	}
	
	// 登录成功，清除失败记录
	am.clearFailedAttempts(clientIP)
	return nil
}

// GenerateTokenPair 生成Access Token和Refresh Token
func (am *AuthManager) GenerateTokenPair(username string, remember bool) (accessToken, refreshToken string, err error) {
	now := time.Now()
	
	// 生成 Access Token
	accessClaims := &Claims{
		Username:  username,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(am.accessTokenExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        fmt.Sprintf("%d-access", now.UnixNano()),
		},
	}
	
	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessTokenObj.SignedString(am.jwtSecret)
	if err != nil {
		return "", "", err
	}
	
	// 生成 Refresh Token
	refreshExpire := am.refreshTokenExpire
	if remember {
		refreshExpire = 30 * 24 * time.Hour // 记住我：30天
	}
	
	refreshClaims := &Claims{
		Username:  username,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(refreshExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        fmt.Sprintf("%d-refresh", now.UnixNano()),
		},
	}
	
	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = refreshTokenObj.SignedString(am.jwtSecret)
	if err != nil {
		return "", "", err
	}
	
	return accessToken, refreshToken, nil
}

// ValidateToken 验证Token
func (am *AuthManager) ValidateToken(tokenString string, expectedType string) (*Claims, error) {
	// 解析Token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.jwtSecret, nil
	})
	
	if err != nil {
		return nil, ErrTokenInvalid
	}
	
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}
	
	// 检查Token类型
	if claims.TokenType != expectedType {
		return nil, ErrTokenInvalid
	}
	
	// 检查是否在黑名单中
	if am.isBlacklisted(claims.ID) {
		return nil, ErrTokenInvalid
	}
	
	return claims, nil
}

// RevokeToken 撤销Token（加入黑名单）
func (am *AuthManager) RevokeToken(tokenString string) error {
	// 解析Token获取过期时间
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return am.jwtSecret, nil
	})
	
	if err != nil {
		return err
	}
	
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return ErrTokenInvalid
	}
	
	// 加入黑名单直到过期
	am.blacklistMu.Lock()
	am.tokenBlacklist[claims.ID] = claims.ExpiresAt.Time
	am.blacklistMu.Unlock()
	
	return nil
}

// isBlacklisted 检查Token是否在黑名单中
func (am *AuthManager) isBlacklisted(jti string) bool {
	am.blacklistMu.RLock()
	defer am.blacklistMu.RUnlock()
	
	expireTime, exists := am.tokenBlacklist[jti]
	if !exists {
		return false
	}
	
	// 如果已过期，不在黑名单中
	return time.Now().Before(expireTime)
}

// isLocked 检查IP是否被锁定
func (am *AuthManager) isLocked(clientIP string) bool {
	am.attemptsMu.RLock()
	defer am.attemptsMu.RUnlock()
	
	attempt, exists := am.loginAttempts[clientIP]
	if !exists {
		return false
	}
	
	return time.Now().Before(attempt.LockedUntil)
}

// recordFailedAttempt 记录失败的登录尝试
func (am *AuthManager) recordFailedAttempt(clientIP string) {
	am.attemptsMu.Lock()
	defer am.attemptsMu.Unlock()
	
	now := time.Now()
	attempt, exists := am.loginAttempts[clientIP]
	
	if !exists {
		attempt = &LoginAttempt{}
		am.loginAttempts[clientIP] = attempt
	}
	
	attempt.Count++
	attempt.LastAttempt = now
	
	// 如果超过最大尝试次数，锁定账户
	if attempt.Count >= am.maxLoginAttempts {
		attempt.LockedUntil = now.Add(am.lockDuration)
	}
}

// clearFailedAttempts 清除失败记录
func (am *AuthManager) clearFailedAttempts(clientIP string) {
	am.attemptsMu.Lock()
	defer am.attemptsMu.Unlock()
	
	delete(am.loginAttempts, clientIP)
}

// cleanupRoutine 定期清理过期数据
func (am *AuthManager) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		now := time.Now()
		
		// 清理黑名单
		am.blacklistMu.Lock()
		for jti, expireTime := range am.tokenBlacklist {
			if now.After(expireTime) {
				delete(am.tokenBlacklist, jti)
			}
		}
		am.blacklistMu.Unlock()
		
		// 清理登录尝试记录
		am.attemptsMu.Lock()
		for ip, attempt := range am.loginAttempts {
			// 如果锁定已过期且超过24小时没有尝试，删除记录
			if now.After(attempt.LockedUntil) && now.Sub(attempt.LastAttempt) > 24*time.Hour {
				delete(am.loginAttempts, ip)
			}
		}
		am.attemptsMu.Unlock()
	}
}

// HashPassword 生成密码哈希（工具函数）
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
