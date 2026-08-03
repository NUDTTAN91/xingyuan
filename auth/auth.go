/*
 * 星垣 - 认证模块
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"xingyuan-monitor/database"

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
	
	// 管理员凭据（启动时从环境变量一次性加载，运行期不可变更，
	// 有且只能通过 docker-compose.yml 修改后重建容器生效）
	adminUsername string
	adminPassword string
	
	// 登录失败记录（IP -> 尝试记录）
	loginAttempts map[string]*LoginAttempt
	attemptsMu    sync.RWMutex
	
	// Token 黑名单（JTI -> 过期时间）
	tokenBlacklist map[string]time.Time
	blacklistMu    sync.RWMutex
}

// NewAuthManager 创建认证管理器
func NewAuthManager() *AuthManager {
	// 管理员凭据启动时一次性加载（唯一配置入口为 docker-compose.yml 的环境变量），
	// 必须显式配置密码，禁止使用内置默认值
	adminUser := os.Getenv("ADMIN_USERNAME")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass == "" {
		log.Fatalf("安全检查失败: 未配置 ADMIN_PASSWORD 环境变量，拒绝启动。" +
			"请在 docker-compose.yml 或环境变量中设置强密码后重试")
	}
	// 常见弱密码警告（不阻止启动，避免影响存量部署）
	weakPasswords := []string{"admin", "admin123", "root", "123456", "12345678", "password"}
	for _, weak := range weakPasswords {
		if adminPass == weak {
			log.Printf("【安全警告】ADMIN_PASSWORD 当前为常见弱密码 %q，强烈建议修改为强密码！", weak)
			break
		}
	}

	// 从环境变量读取配置
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" || jwtSecret == "xingyuan-default-secret-please-change-in-production" ||
		jwtSecret == "xingyuan-secret-key-please-change-this-in-production" {
		// 未配置或仍为示例值：随机生成密钥（重启后所有Token失效，需重新登录）
		randomKey := make([]byte, 32)
		if _, err := rand.Read(randomKey); err != nil {
			log.Fatalf("生成随机 JWT 密钥失败: %v", err)
		}
		jwtSecret = hex.EncodeToString(randomKey)
		log.Printf("【安全警告】JWT_SECRET 未配置或仍为示例值，已使用随机密钥（重启后需重新登录）。" +
			"建议在环境变量中配置固定的强密钥")
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
		adminUsername:      adminUser,
		adminPassword:      adminPass,
		loginAttempts:      make(map[string]*LoginAttempt),
		tokenBlacklist:     make(map[string]time.Time),
	}
	
	// 从数据库恢复黑名单与登录锁定状态（重启后不丢失；数据库未初始化时自动跳过）
	if entries, err := database.LoadBlacklist(); err != nil {
		log.Printf("恢复Token黑名单失败: %v", err)
	} else if len(entries) > 0 {
		for _, e := range entries {
			am.tokenBlacklist[e.JTI] = e.ExpiresAt
		}
		log.Printf("已恢复 %d 条Token黑名单记录", len(entries))
	}
	if entries, err := database.LoadLoginAttempts(); err != nil {
		log.Printf("恢复登录锁定状态失败: %v", err)
	} else if len(entries) > 0 {
		for _, e := range entries {
			am.loginAttempts[e.IP] = &LoginAttempt{
				Count:       e.Count,
				LastAttempt: e.LastAttempt,
				LockedUntil: e.LockedUntil,
			}
		}
		log.Printf("已恢复 %d 条登录失败记录", len(entries))
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
	
	// 使用启动时加载的管理员凭据（运行期不可变更）
	adminUser := am.adminUsername
	adminPass := am.adminPassword
	
	// 验证用户名
	if username != adminUser {
		am.recordFailedAttempt(clientIP)
		return ErrInvalidCredentials
	}
	
	// 验证密码：配置为bcrypt哈希（$2a$/$2b$/$2y$前缀）时只走bcrypt校验，
	// 否则按明文恒定时间比较（避免输入哈希串本身即可登录的漏洞）
	if strings.HasPrefix(adminPass, "$2a$") || strings.HasPrefix(adminPass, "$2b$") || strings.HasPrefix(adminPass, "$2y$") {
		if err := bcrypt.CompareHashAndPassword([]byte(adminPass), []byte(password)); err != nil {
			am.recordFailedAttempt(clientIP)
			return ErrInvalidCredentials
		}
	} else {
		if subtle.ConstantTimeCompare([]byte(password), []byte(adminPass)) != 1 {
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
	
	// 持久化到数据库（重启后仍然失效）
	if err := database.SaveBlacklistToken(claims.ID, claims.ExpiresAt.Time); err != nil {
		log.Printf("持久化黑名单Token失败: %v", err)
	}
	
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

// maxLoginAttemptEntries 登录失败记录的容量上限
// 防止攻击者伪造海量来源IP撑爆内存
const maxLoginAttemptEntries = 10000

// recordFailedAttempt 记录失败的登录尝试
func (am *AuthManager) recordFailedAttempt(clientIP string) {
	// 锁内只更新内存并取快照，锁外再落库，避免持锁期间做SQLite I/O
	// （SQLite单连接+busy_timeout，锁内慢写会阻塞所有登录/锁定检查）
	am.attemptsMu.Lock()
	now := time.Now()
	attempt, exists := am.loginAttempts[clientIP]

	if !exists {
		// 达到容量上限时先清理过期条目；仍满则放弃记录（不影响正常校验）
		if len(am.loginAttempts) >= maxLoginAttemptEntries {
			am.pruneExpiredAttemptsLocked(now)
			if len(am.loginAttempts) >= maxLoginAttemptEntries {
				am.attemptsMu.Unlock()
				return
			}
		}
		attempt = &LoginAttempt{}
		am.loginAttempts[clientIP] = attempt
	}

	attempt.Count++
	attempt.LastAttempt = now

	// 如果超过最大尝试次数，锁定账户
	if attempt.Count >= am.maxLoginAttempts {
		attempt.LockedUntil = now.Add(am.lockDuration)
	}

	// 快照当前值供锁外落库
	count, lastAttempt, lockedUntil := attempt.Count, attempt.LastAttempt, attempt.LockedUntil
	am.attemptsMu.Unlock()

	// 持久化（重启后锁定状态不丢失）
	if err := database.SaveLoginAttempt(clientIP, count, lastAttempt, lockedUntil); err != nil {
		log.Printf("持久化登录失败记录失败: %v", err)
	}
}

// pruneExpiredAttemptsLocked 清理已过锁定期且1小时无活动的记录（调用方需持有attemptsMu写锁）
func (am *AuthManager) pruneExpiredAttemptsLocked(now time.Time) {
	for ip, attempt := range am.loginAttempts {
		if now.After(attempt.LockedUntil) && now.Sub(attempt.LastAttempt) > time.Hour {
			delete(am.loginAttempts, ip)
		}
	}
}

// clearFailedAttempts 清除失败记录
func (am *AuthManager) clearFailedAttempts(clientIP string) {
	am.attemptsMu.Lock()
	delete(am.loginAttempts, clientIP)
	am.attemptsMu.Unlock()

	// 锁外落库
	if err := database.DeleteLoginAttempt(clientIP); err != nil {
		log.Printf("清除登录失败记录失败: %v", err)
	}
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
		
		// 清理登录尝试记录（锁内收集待删IP，锁外落库）
		am.attemptsMu.Lock()
		var expiredIPs []string
		for ip, attempt := range am.loginAttempts {
			// 如果锁定已过期且超过24小时没有尝试，删除记录
			if now.After(attempt.LockedUntil) && now.Sub(attempt.LastAttempt) > 24*time.Hour {
				delete(am.loginAttempts, ip)
				expiredIPs = append(expiredIPs, ip)
			}
		}
		am.attemptsMu.Unlock()
		
		for _, ip := range expiredIPs {
			if err := database.DeleteLoginAttempt(ip); err != nil {
				log.Printf("清理过期登录记录失败: %v", err)
			}
		}
		
		// 同步清理数据库中的过期黑名单
		if err := database.CleanExpiredBlacklist(); err != nil {
			log.Printf("清理过期黑名单失败: %v", err)
		}
	}
}

// HashPassword 生成密码哈希（工具函数）
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
