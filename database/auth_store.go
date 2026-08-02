package database

import (
	"fmt"
	"time"
)

// 认证状态持久化：Token黑名单与登录锁定落库，服务重启后不丢失。
// 所有函数在数据库未初始化时安全降级为no-op（兼容auth包独立单测）。

const timeLayout = "2006-01-02 15:04:05"

// BlacklistEntry 黑名单条目
type BlacklistEntry struct {
	JTI       string
	ExpiresAt time.Time
}

// LoginAttemptEntry 登录失败记录条目
type LoginAttemptEntry struct {
	IP          string
	Count       int
	LastAttempt time.Time
	LockedUntil time.Time
}

// SaveBlacklistToken 持久化被撤销的Token
func SaveBlacklistToken(jti string, expiresAt time.Time) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO token_blacklist (jti, expires_at) VALUES (?, ?)",
		jti, expiresAt.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("保存黑名单Token失败: %v", err)
	}
	return nil
}

// LoadBlacklist 加载未过期的黑名单条目（启动时恢复）
func LoadBlacklist() ([]BlacklistEntry, error) {
	if db == nil {
		return nil, nil
	}
	now := time.Now().Format(timeLayout)
	rows, err := db.Query("SELECT jti, expires_at FROM token_blacklist WHERE expires_at > ?", now)
	if err != nil {
		return nil, fmt.Errorf("加载黑名单失败: %v", err)
	}
	defer rows.Close()

	var entries []BlacklistEntry
	for rows.Next() {
		var jti, expiresStr string
		if err := rows.Scan(&jti, &expiresStr); err != nil {
			return nil, err
		}
		expiresAt, err := time.ParseInLocation(timeLayout, expiresStr, time.Local)
		if err != nil {
			continue
		}
		entries = append(entries, BlacklistEntry{JTI: jti, ExpiresAt: expiresAt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历黑名单失败: %v", err)
	}
	return entries, nil
}

// CleanExpiredBlacklist 清理已过期的黑名单条目
func CleanExpiredBlacklist() error {
	if db == nil {
		return nil
	}
	now := time.Now().Format(timeLayout)
	_, err := db.Exec("DELETE FROM token_blacklist WHERE expires_at <= ?", now)
	return err
}

// SaveLoginAttempt 持久化登录失败记录
func SaveLoginAttempt(ip string, count int, lastAttempt, lockedUntil time.Time) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("INSERT OR REPLACE INTO login_attempts (ip, count, last_attempt, locked_until) VALUES (?, ?, ?, ?)",
		ip, count, lastAttempt.Format(timeLayout), lockedUntil.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("保存登录失败记录失败: %v", err)
	}
	return nil
}

// DeleteLoginAttempt 删除登录失败记录（登录成功时调用）
func DeleteLoginAttempt(ip string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec("DELETE FROM login_attempts WHERE ip = ?", ip)
	return err
}

// LoadLoginAttempts 加载登录失败记录（启动时恢复锁定状态）
func LoadLoginAttempts() ([]LoginAttemptEntry, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT ip, count, last_attempt, locked_until FROM login_attempts")
	if err != nil {
		return nil, fmt.Errorf("加载登录失败记录失败: %v", err)
	}
	defer rows.Close()

	var entries []LoginAttemptEntry
	for rows.Next() {
		var e LoginAttemptEntry
		var lastStr, lockedStr string
		if err := rows.Scan(&e.IP, &e.Count, &lastStr, &lockedStr); err != nil {
			return nil, err
		}
		e.LastAttempt, _ = time.ParseInLocation(timeLayout, lastStr, time.Local)
		e.LockedUntil, _ = time.ParseInLocation(timeLayout, lockedStr, time.Local)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历登录失败记录失败: %v", err)
	}
	return entries, nil
}
