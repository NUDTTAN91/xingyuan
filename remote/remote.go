package remote

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RemoteHost 远程主机配置
type RemoteHost struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"password,omitempty"` // 返回时不包含密码
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RemoteHostStatus 远程主机状态
type RemoteHostStatus struct {
	HostID       int       `json:"host_id"`
	Online       bool      `json:"online"`
	LastCheck    time.Time `json:"last_check"`
	ResponseTime int64     `json:"response_time"` // 毫秒
	Error        string    `json:"error,omitempty"`
}

// TokenCache Token 缓存
type TokenCache struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// hostCacheEntry 主机配置缓存条目
type hostCacheEntry struct {
	host     RemoteHost
	cachedAt time.Time
}

// Manager 远程主机管理器
type Manager struct {
	db          *sql.DB
	httpClient  *http.Client
	tokenCache  map[int]*TokenCache // host_id -> token
	cacheMutex  sync.RWMutex
	encryptKey  []byte // 远程主机密码的AES-256-GCM加密密钥
	// 主机配置缓存：避免每次代理请求都查库+解密（远程页每秒多次调用）
	hostCache   map[int]*hostCacheEntry
	hostCacheMu sync.RWMutex
}

// NewManager 创建远程主机管理器
func NewManager(db *sql.DB) *Manager {
	// 加载/生成密码加密密钥（持久化在数据目录）
	encryptKey, err := loadOrCreateEncryptKey("./data")
	if err != nil {
		log.Fatalf("初始化密码加密密钥失败: %v", err)
	}

	m := &Manager{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		tokenCache: make(map[int]*TokenCache),
		encryptKey: encryptKey,
		hostCache:  make(map[int]*hostCacheEntry),
	}

	// 将存量明文密码一次性加密
	m.migratePlaintextPasswords()

	return m
}

// ==================== 数据库操作 ====================

// GetAllHosts 获取所有主机
func (m *Manager) GetAllHosts() ([]RemoteHost, error) {
	query := `
		SELECT id, name, address, port, username, enabled, created_at, updated_at
		FROM remote_hosts
		ORDER BY id DESC
	`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询主机列表失败: %v", err)
	}
	defer rows.Close()

	var hosts []RemoteHost
	for rows.Next() {
		var h RemoteHost
		var createdAt, updatedAt string
		var enabled int

		err := rows.Scan(
			&h.ID, &h.Name, &h.Address, &h.Port,
			&h.Username, &enabled, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描主机数据失败: %v", err)
		}

		h.Enabled = enabled == 1
		h.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		h.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历主机列表失败: %v", err)
	}

	return hosts, nil
}

// hostCacheTTL 主机配置缓存有效期（兜底失效，正常由增删改主动失效）
const hostCacheTTL = 60 * time.Second

// invalidateHostCache 使指定主机的配置缓存失效（增删改时调用）
func (m *Manager) invalidateHostCache(id int) {
	m.hostCacheMu.Lock()
	delete(m.hostCache, id)
	m.hostCacheMu.Unlock()
}

// GetHostByID 根据 ID 获取主机（带内存缓存，避免每次代理请求都查库+解密）
func (m *Manager) GetHostByID(id int) (*RemoteHost, error) {
	m.hostCacheMu.RLock()
	if entry, ok := m.hostCache[id]; ok && time.Since(entry.cachedAt) < hostCacheTTL {
		host := entry.host // 返回副本，避免调用方修改缓存
		m.hostCacheMu.RUnlock()
		return &host, nil
	}
	m.hostCacheMu.RUnlock()

	host, err := m.getHostByIDFromDB(id)
	if err != nil {
		return nil, err
	}

	m.hostCacheMu.Lock()
	m.hostCache[id] = &hostCacheEntry{host: *host, cachedAt: time.Now()}
	m.hostCacheMu.Unlock()

	return host, nil
}

// getHostByIDFromDB 从数据库读取主机配置（含密码解密）
func (m *Manager) getHostByIDFromDB(id int) (*RemoteHost, error) {
	query := `
		SELECT id, name, address, port, username, password, enabled, created_at, updated_at
		FROM remote_hosts
		WHERE id = ?
	`

	var h RemoteHost
	var createdAt, updatedAt string
	var enabled int

	err := m.db.QueryRow(query, id).Scan(
		&h.ID, &h.Name, &h.Address, &h.Port,
		&h.Username, &h.Password, &enabled, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("主机不存在")
		}
		return nil, fmt.Errorf("查询主机失败: %v", err)
	}

	h.Enabled = enabled == 1
	h.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	h.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	// 解密密码（存量明文数据原样返回）
	password, err := m.decryptPassword(h.Password)
	if err != nil {
		return nil, fmt.Errorf("解密主机密码失败: %v", err)
	}
	h.Password = password

	return &h, nil
}

// AddHost 添加主机
func (m *Manager) AddHost(h *RemoteHost) error {
	now := time.Now().Format("2006-01-02 15:04:05")

	// 密码加密后入库
	encrypted, err := m.encryptPassword(h.Password)
	if err != nil {
		return fmt.Errorf("加密密码失败: %v", err)
	}

	query := `
		INSERT INTO remote_hosts (name, address, port, username, password, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	enabled := 0
	if h.Enabled {
		enabled = 1
	}

	result, err := m.db.Exec(query, h.Name, h.Address, h.Port, h.Username, encrypted, enabled, now, now)
	if err != nil {
		return fmt.Errorf("添加主机失败: %v", err)
	}

	id, _ := result.LastInsertId()
	h.ID = int(id)

	log.Printf("添加远程主机成功: %s (%s:%d)", h.Name, h.Address, h.Port)
	return nil
}

// UpdateHost 更新主机
func (m *Manager) UpdateHost(h *RemoteHost) error {
	now := time.Now().Format("2006-01-02 15:04:05")

	// 密码加密后入库；若未提供新密码则保留原密码
	var encrypted string
	if h.Password == "" {
		var stored string
		if err := m.db.QueryRow("SELECT password FROM remote_hosts WHERE id = ?", h.ID).Scan(&stored); err != nil {
			return fmt.Errorf("查询原密码失败: %v", err)
		}
		encrypted = stored
	} else {
		var err error
		encrypted, err = m.encryptPassword(h.Password)
		if err != nil {
			return fmt.Errorf("加密密码失败: %v", err)
		}
	}

	query := `
		UPDATE remote_hosts
		SET name = ?, address = ?, port = ?, username = ?, password = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`

	enabled := 0
	if h.Enabled {
		enabled = 1
	}

	_, err := m.db.Exec(query, h.Name, h.Address, h.Port, h.Username, encrypted, enabled, now, h.ID)
	if err != nil {
		return fmt.Errorf("更新主机失败: %v", err)
	}

	// 清除 Token 缓存与主机配置缓存
	m.cacheMutex.Lock()
	delete(m.tokenCache, h.ID)
	m.cacheMutex.Unlock()
	m.invalidateHostCache(h.ID)

	log.Printf("更新远程主机成功: %s (%s:%d)", h.Name, h.Address, h.Port)
	return nil
}

// DeleteHost 删除主机
func (m *Manager) DeleteHost(id int) error {
	query := `DELETE FROM remote_hosts WHERE id = ?`

	_, err := m.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("删除主机失败: %v", err)
	}

	// 清除 Token 缓存与主机配置缓存
	m.cacheMutex.Lock()
	delete(m.tokenCache, id)
	m.cacheMutex.Unlock()
	m.invalidateHostCache(id)

	log.Printf("删除远程主机成功: ID=%d", id)
	return nil
}

// ==================== Token 管理 ====================

// getToken 获取主机的 Access Token（自动登录和刷新）
func (m *Manager) getToken(host *RemoteHost) (string, error) {
	m.cacheMutex.RLock()
	cache, exists := m.tokenCache[host.ID]
	m.cacheMutex.RUnlock()

	// 如果缓存存在且未过期，直接返回
	if exists && time.Now().Before(cache.ExpiresAt) {
		return cache.AccessToken, nil
	}

	// 需要重新登录
	return m.login(host)
}

// parseJWTExpiry 解析JWT的exp声明（不校验签名，仅用于估算本地缓存有效期）
func parseJWTExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("非标准JWT格式")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("解码JWT载荷失败: %v", err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("JWT缺少exp声明")
	}
	return time.Unix(claims.Exp, 0), nil
}

// login 登录远程主机
func (m *Manager) login(host *RemoteHost) (string, error) {
	url := fmt.Sprintf("http://%s:%d/api/login", host.Address, host.Port)

	loginData := map[string]interface{}{
		"username": host.Username,
		"password": host.Password,
		"remember": false,
	}

	jsonData, _ := json.Marshal(loginData)

	resp, err := m.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("登录请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("登录失败 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success      bool   `json:"success"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Message      string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %v", err)
	}

	if !result.Success {
		return "", fmt.Errorf("登录失败: %s", result.Message)
	}

	// 缓存Token：从JWT的exp声明推算真实有效期并提前10%过期，
	// 兼容远端任意的 ACCESS_TOKEN_EXPIRE_MINUTES 配置（不再假设固定2小时）
	expiresAt := time.Now().Add(110 * time.Minute) // 解析失败时的兜底值
	if exp, err := parseJWTExpiry(result.AccessToken); err == nil {
		ttl := time.Until(exp)
		if ttl > time.Minute {
			expiresAt = time.Now().Add(ttl * 9 / 10)
		}
	}

	m.cacheMutex.Lock()
	m.tokenCache[host.ID] = &TokenCache{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    expiresAt,
	}
	m.cacheMutex.Unlock()

	log.Printf("远程主机登录成功: %s (%s:%d)", host.Name, host.Address, host.Port)
	return result.AccessToken, nil
}

// ==================== 数据代理 ====================

// ProxyRequest 代理请求到远程主机
func (m *Manager) ProxyRequest(hostID int, apiPath string) ([]byte, error) {
	// 获取主机配置
	host, err := m.GetHostByID(hostID)
	if err != nil {
		log.Printf("[ERROR] 获取主机ID=%d失败: %v", hostID, err)
		return nil, err
	}

	if !host.Enabled {
		log.Printf("[ERROR] 主机 %s (ID=%d) 已禁用", host.Name, hostID)
		return nil, fmt.Errorf("主机已禁用")
	}

	// 获取 Token
	token, err := m.getToken(host)
	if err != nil {
		log.Printf("[ERROR] 主机 %s (ID=%d) 获取Token失败: %v", host.Name, hostID, err)
		return nil, fmt.Errorf("获取认证 Token 失败: %v", err)
	}

	// 构建请求
	url := fmt.Sprintf("http://%s:%d%s", host.Address, host.Port, apiPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[ERROR] 主机 %s (ID=%d) 创建请求失败: %v", host.Name, hostID, err)
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	// 发送请求
	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Printf("[ERROR] 主机 %s (ID=%d) 请求失败: %v", host.Name, hostID, err)
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ERROR] 主机 %s (ID=%d) 读取响应失败: %v", host.Name, hostID, err)
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 如果返回 401，清除缓存并重试一次
	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("[WARN] 主机 %s (ID=%d) 返回401，重新登录", host.Name, hostID)
		m.cacheMutex.Lock()
		delete(m.tokenCache, hostID)
		m.cacheMutex.Unlock()

		// 重新登录
		token, err = m.login(host)
		if err != nil {
			log.Printf("[ERROR] 主机 %s (ID=%d) 重新登录失败: %v", host.Name, hostID, err)
			return nil, fmt.Errorf("重新登录失败: %v", err)
		}

		// 重试请求
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = m.httpClient.Do(req)
		if err != nil {
			log.Printf("[ERROR] 主机 %s (ID=%d) 重试请求失败: %v", host.Name, hostID, err)
			return nil, fmt.Errorf("重试请求失败: %v", err)
		}
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[ERROR] 主机 %s (ID=%d) 读取重试响应失败: %v", host.Name, hostID, err)
			return nil, fmt.Errorf("读取重试响应失败: %v", err)
		}
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ERROR] 远程主机 %s (%s:%d) API返回错误: HTTP %d, Body: %s", host.Name, host.Address, host.Port, resp.StatusCode, string(body))
		return nil, fmt.Errorf("远程 API 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// CheckHostStatus 检查主机在线状态
func (m *Manager) CheckHostStatus(hostID int) (*RemoteHostStatus, error) {
	host, err := m.GetHostByID(hostID)
	if err != nil {
		return nil, err
	}

	status := &RemoteHostStatus{
		HostID:    hostID,
		LastCheck: time.Now(),
	}

	// 尝试访问 /api/verify 端点
	url := fmt.Sprintf("http://%s:%d/api/verify", host.Address, host.Port)

	start := time.Now()
	resp, err := m.httpClient.Get(url)
	responseTime := time.Since(start).Milliseconds()

	if err != nil {
		status.Online = false
		status.Error = err.Error()
		return status, nil
	}
	defer resp.Body.Close()

	status.Online = true
	status.ResponseTime = responseTime

	// 可达后进一步校验凭据有效性（命中Token缓存时无额外开销）
	if _, err := m.getToken(host); err != nil {
		status.Error = "认证失败: 用户名或密码错误"
	}

	return status, nil
}

// CheckAllHostsStatus 检查所有主机状态（并发探测，避免单台超时拖慢整体）
func (m *Manager) CheckAllHostsStatus() (map[int]*RemoteHostStatus, error) {
	hosts, err := m.GetAllHosts()
	if err != nil {
		return nil, err
	}

	statuses := make(map[int]*RemoteHostStatus)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, host := range hosts {
		if !host.Enabled {
			continue
		}

		wg.Add(1)
		go func(hostID int) {
			defer wg.Done()
			status, _ := m.CheckHostStatus(hostID)
			if status != nil {
				mu.Lock()
				statuses[hostID] = status
				mu.Unlock()
			}
		}(host.ID)
	}
	wg.Wait()

	return statuses, nil
}
