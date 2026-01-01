package remote

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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

// Manager 远程主机管理器
type Manager struct {
	db          *sql.DB
	httpClient  *http.Client
	tokenCache  map[int]*TokenCache // host_id -> token
	cacheMutex  sync.RWMutex
}

// NewManager 创建远程主机管理器
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		tokenCache: make(map[int]*TokenCache),
	}
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

	return hosts, nil
}

// GetHostByID 根据 ID 获取主机
func (m *Manager) GetHostByID(id int) (*RemoteHost, error) {
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

	return &h, nil
}

// AddHost 添加主机
func (m *Manager) AddHost(h *RemoteHost) error {
	now := time.Now().Format("2006-01-02 15:04:05")

	query := `
		INSERT INTO remote_hosts (name, address, port, username, password, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	enabled := 0
	if h.Enabled {
		enabled = 1
	}

	result, err := m.db.Exec(query, h.Name, h.Address, h.Port, h.Username, h.Password, enabled, now, now)
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

	query := `
		UPDATE remote_hosts
		SET name = ?, address = ?, port = ?, username = ?, password = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`

	enabled := 0
	if h.Enabled {
		enabled = 1
	}

	_, err := m.db.Exec(query, h.Name, h.Address, h.Port, h.Username, h.Password, enabled, now, h.ID)
	if err != nil {
		return fmt.Errorf("更新主机失败: %v", err)
	}

	// 清除 Token 缓存
	m.cacheMutex.Lock()
	delete(m.tokenCache, h.ID)
	m.cacheMutex.Unlock()

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

	// 清除 Token 缓存
	m.cacheMutex.Lock()
	delete(m.tokenCache, id)
	m.cacheMutex.Unlock()

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

	// 缓存 Token（Access Token 默认 2 小时有效）
	m.cacheMutex.Lock()
	m.tokenCache[host.ID] = &TokenCache{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(110 * time.Minute), // 提前 10 分钟过期
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

	return status, nil
}

// CheckAllHostsStatus 检查所有主机状态
func (m *Manager) CheckAllHostsStatus() (map[int]*RemoteHostStatus, error) {
	hosts, err := m.GetAllHosts()
	if err != nil {
		return nil, err
	}

	statuses := make(map[int]*RemoteHostStatus)

	for _, host := range hosts {
		if !host.Enabled {
			continue
		}

		status, _ := m.CheckHostStatus(host.ID)
		if status != nil {
			statuses[host.ID] = status
		}
	}

	return statuses, nil
}
