/*
 * 星垣 - Web服务器
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"xingyuan-monitor/auth"
	"xingyuan-monitor/collector"
	"xingyuan-monitor/database"
	"xingyuan-monitor/remote"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Server Web服务器
type Server struct {
	collector     *collector.Collector
	authManager   *auth.AuthManager
	remoteManager *remote.Manager
	router        *gin.Engine
	wsClients     map[*websocket.Conn]bool
	wsMutex       sync.Mutex
	upgrader      websocket.Upgrader
}

// NewServer 创建服务器实例
func NewServer() *Server {
	gin.SetMode(gin.ReleaseMode)
	
	// 获取数据库实例
	db := database.GetDB()
	
	s := &Server{
		collector:     collector.NewCollector(),
		authManager:   auth.NewAuthManager(),
		remoteManager: remote.NewManager(db),
		router:        gin.Default(),
		wsClients:     make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	// 加载网络流量基准值（用于系统重启后恢复）
	if baseline, err := database.LoadNetworkBaseline(); err == nil {
		s.collector.LoadBaseline(
			baseline.BytesRecvBaseline,
			baseline.BytesSentBaseline,
			baseline.LastRecv,
			baseline.LastSent,
		)
		log.Printf("加载网络基准值成功: 上行=%d, 下行=%d", baseline.BytesSentBaseline, baseline.BytesRecvBaseline)
	} else {
		log.Printf("加载网络基准值失败: %v，使用默认值", err)
	}

	s.setupRoutes()
	return s
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 静态文件服务 - 禁用缓存
	s.router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/static/js/history-chart.js" {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	})
	s.router.Static("/static", "./static")
	
	// 首页（无需认证，由前端 JavaScript 检查认证状态）
	s.router.GET("/", s.handleIndex)
	
	// 公开路由（无需认证）
	public := s.router.Group("/api")
	{
		public.POST("/login", s.handleLogin)
		public.POST("/refresh", s.handleRefresh)
		// WebSocket 路由（在 handler 中手动验证 Token）
		public.GET("/ws", s.handleWebSocket)
	}
	
	// 受保护的API接口（需要认证）
	api := s.router.Group("/api")
	api.Use(s.authMiddleware())
	{
		api.GET("/metrics", s.handleMetrics)
		api.GET("/docker", s.handleDocker)
		api.POST("/logout", s.handleLogout)
		api.GET("/verify", s.handleVerify)
		
		// 历史数据查询接口
		api.GET("/history/cpu", s.handleHistoryCPU)
		api.GET("/history/memory", s.handleHistoryMemory)
		api.GET("/history/disk", s.handleHistoryDisk)
		api.GET("/history/network", s.handleHistoryNetwork)
		
		// 数据库统计接口
		api.GET("/stats/database", s.handleDatabaseStats)
		api.GET("/stats/timerange", s.handleDataTimeRange)
		
		// Docker容器操作接口
		api.POST("/docker/container/stop", s.handleStopContainer)
		api.POST("/docker/container/delete", s.handleDeleteContainer)
		api.POST("/docker/container/restart", s.handleRestartContainer)
		
		// 远程主机管理接口
		api.GET("/remote/hosts", s.handleGetRemoteHosts)
		api.POST("/remote/hosts", s.handleAddRemoteHost)
		api.PUT("/remote/hosts/:id", s.handleUpdateRemoteHost)
		api.DELETE("/remote/hosts/:id", s.handleDeleteRemoteHost)
		api.GET("/remote/hosts/:id/status", s.handleCheckHostStatus)
		api.GET("/remote/hosts/status/all", s.handleCheckAllHostsStatus)
		
		// 远程数据代理接口
		api.GET("/remote/:host_id/metrics", s.handleRemoteMetrics)
		api.GET("/remote/:host_id/docker", s.handleRemoteDocker)
		api.GET("/remote/:host_id/history/cpu", s.handleRemoteHistoryCPU)
		api.GET("/remote/:host_id/history/memory", s.handleRemoteHistoryMemory)
		api.GET("/remote/:host_id/history/disk", s.handleRemoteHistoryDisk)
		api.GET("/remote/:host_id/history/network", s.handleRemoteHistoryNetwork)
	}
}

// handleIndex 首页处理
func (s *Server) handleIndex(c *gin.Context) {
	c.File("./static/index.html")
}

// handleMetrics 获取监控数据
func (s *Server) handleMetrics(c *gin.Context) {
	metrics, err := s.collector.Collect()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// handleDocker 获取Docker监控数据
func (s *Server) handleDocker(c *gin.Context) {
	dockerMetrics, err := s.collector.CollectDocker()
	if err != nil {
		log.Printf("[ERROR] CollectDocker failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	log.Printf("[DEBUG] Docker metrics collected: containers=%d, images=%d", len(dockerMetrics.Containers), len(dockerMetrics.Images))
	c.JSON(http.StatusOK, dockerMetrics)
}

// handleWebSocket WebSocket连接处理
func (s *Server) handleWebSocket(c *gin.Context) {
	// 从查询参数中获取 Token（WebSocket 不支持请求头）
	token := c.Request.URL.Query().Get("token")
	if token == "" {
		// 如果没有 token，从 Header 中获取（兼容性）
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}
	}
	
	// 验证 Token
	if token == "" {
		log.Printf("WebSocket 连接被拒绝：未提供 Token")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
		return
	}
	
	claims, err := s.authManager.ValidateToken(token, auth.TokenTypeAccess)
	if err != nil {
		log.Printf("WebSocket 连接被拒绝：Token 验证失败: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token无效或已过期"})
		return
	}
	
	log.Printf("WebSocket 连接成功，用户: %s, IP: %s", claims.Username, c.ClientIP())
	
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	s.wsMutex.Lock()
	s.wsClients[conn] = true
	s.wsMutex.Unlock()

	defer func() {
		s.wsMutex.Lock()
		delete(s.wsClients, conn)
		s.wsMutex.Unlock()
		conn.Close()
	}()

	// 保持连接
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// BroadcastMetrics 广播监控数据到所有WebSocket客户端
func (s *Server) BroadcastMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics, err := s.collector.Collect()
		if err != nil {
			log.Printf("Collect metrics error: %v", err)
			continue
		}

		// 获取数据库统计信息（实时更新）
		dbStats, err := database.GetDatabaseStats()
		if err != nil {
			log.Printf("Get database stats error: %v", err)
			// 如果获取失败，设置默认值
			metrics.DatabaseInfo = collector.DatabaseInfo{
				TotalRecords: 0,
				DataSize:     0,
			}
		} else {
			metrics.DatabaseInfo = collector.DatabaseInfo{
				TotalRecords: dbStats.TotalRecords,
				DataSize:     dbStats.DataSize,
			}
		}

		// 将监控数据存入数据库
		go s.saveMetricsToDatabase(metrics)

		s.wsMutex.Lock()
		for conn := range s.wsClients {
			if err := conn.WriteJSON(metrics); err != nil {
				log.Printf("WebSocket write error: %v", err)
				conn.Close()
				delete(s.wsClients, conn)
			}
		}
		s.wsMutex.Unlock()
	}
}

// saveMetricsToDatabase 将监控数据保存到数据库
func (s *Server) saveMetricsToDatabase(metrics *collector.SystemMetrics) {
	// 保存CPU数据
	if err := database.InsertCPUMetrics(metrics.CPU.UsagePercent); err != nil {
		log.Printf("保存CPU数据失败: %v", err)
	}

	// 保存内存数据
	if err := database.InsertMemoryMetrics(metrics.Memory.Used, metrics.Memory.Total, metrics.Memory.UsagePercent); err != nil {
		log.Printf("保存内存数据失败: %v", err)
	}

	// 保存磁盘数据
	if err := database.InsertDiskMetrics(
		metrics.Disk.Used,
		metrics.Disk.Free,
		metrics.Disk.Total,
		metrics.Disk.UsagePercent,
		metrics.Disk.ReadSpeed,
		metrics.Disk.WriteSpeed,
	); err != nil {
		log.Printf("保存磁盘数据失败: %v", err)
	}

	// 保存网络数据（包括速度和累计流量）
	if err := database.InsertNetworkMetrics(
		metrics.Network.UploadSpeed, 
		metrics.Network.DownloadSpeed,
		metrics.Network.BytesSent,
		metrics.Network.BytesRecv,
	); err != nil {
		log.Printf("保存网络数据失败: %v", err)
	}

	// 每秒保存网络基准值（用于系统重启后恢复）
	baseline := s.collector.GetBaseline()
	dbBaseline := &database.NetworkBaseline{
		ID:                1,
		BytesRecvBaseline: baseline.BytesRecvBaseline,
		BytesSentBaseline: baseline.BytesSentBaseline,
		LastRecv:          baseline.LastRecv,
		LastSent:          baseline.LastSent,
	}
	if err := database.SaveNetworkBaseline(dbBaseline); err != nil {
		log.Printf("保存网络基准值失败: %v", err)
	}
}

// Run 启动服务器
func (s *Server) Run(addr string) error {
	// 启动WebSocket广播
	go s.BroadcastMetrics()

	log.Printf("星垣启动成功，访问地址: http://%s", addr)
	log.Printf("Author: tan91 | GitHub: https://github.com/NUDTTAN91")
	
	return s.router.Run(addr)
}

// handleHistoryCPU 获取CPU历史数据
func (s *Server) handleHistoryCPU(c *gin.Context) {
	startTime := c.DefaultQuery("start", "1970-01-01 00:00:00")
	endTime := c.DefaultQuery("end", "2099-12-31 23:59:59")
	
	// 计算采样间隔
	sampleInterval := calculateSampleInterval(startTime, endTime)
	
	metrics, err := database.QueryCPUMetricsSampled(startTime, endTime, sampleInterval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// handleHistoryMemory 获取内存历史数据
func (s *Server) handleHistoryMemory(c *gin.Context) {
	startTime := c.DefaultQuery("start", "1970-01-01 00:00:00")
	endTime := c.DefaultQuery("end", "2099-12-31 23:59:59")
	
	sampleInterval := calculateSampleInterval(startTime, endTime)
	
	metrics, err := database.QueryMemoryMetricsSampled(startTime, endTime, sampleInterval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// handleHistoryDisk 获取磁盘历史数据
func (s *Server) handleHistoryDisk(c *gin.Context) {
	startTime := c.DefaultQuery("start", "1970-01-01 00:00:00")
	endTime := c.DefaultQuery("end", "2099-12-31 23:59:59")
	
	sampleInterval := calculateSampleInterval(startTime, endTime)
	
	metrics, err := database.QueryDiskMetricsSampled(startTime, endTime, sampleInterval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// handleHistoryNetwork 获取网络历史数据
func (s *Server) handleHistoryNetwork(c *gin.Context) {
	startTime := c.DefaultQuery("start", "1970-01-01 00:00:00")
	endTime := c.DefaultQuery("end", "2099-12-31 23:59:59")
	
	sampleInterval := calculateSampleInterval(startTime, endTime)
	
	metrics, err := database.QueryNetworkMetricsSampled(startTime, endTime, sampleInterval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// calculateSampleInterval 根据时间范围计算采样间隔（秒）
// 目标：保证返回的数据点数在 300-3600 之间
func calculateSampleInterval(startTime, endTime string) int {
	const maxPoints = 1800 // 最大数据点数
	
	// 解析时间
	layout := "2006-01-02 15:04:05"
	start, err1 := time.Parse(layout, startTime)
	end, err2 := time.Parse(layout, endTime)
	
	if err1 != nil || err2 != nil {
		return 1 // 解析失败，不采样
	}
	
	// 计算时间范围（秒）
	duration := int(end.Sub(start).Seconds())
	if duration <= 0 {
		return 1
	}
	
	// 计算采样间隔
	interval := duration / maxPoints
	if interval < 1 {
		interval = 1
	}
	
	return interval
}

// handleDatabaseStats 获取数据库统计信息
func (s *Server) handleDatabaseStats(c *gin.Context) {
	stats, err := database.GetDatabaseStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// handleDataTimeRange 获取数据时间范围
func (s *Server) handleDataTimeRange(c *gin.Context) {
	timeRange, err := database.GetDataTimeRange()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, timeRange)
}

// handleStopContainer 停止容器
func (s *Server) handleStopContainer(c *gin.Context) {
	var req struct {
		ContainerID string `json:"container_id"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
		})
		return
	}
	
	if req.ContainerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "container_id 不能为空",
		})
		return
	}
	
	if err := collector.StopContainer(req.ContainerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "容器停止成功",
	})
}

// handleDeleteContainer 删除容器
func (s *Server) handleDeleteContainer(c *gin.Context) {
	var req struct {
		ContainerID string `json:"container_id"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
		})
		return
	}
	
	if req.ContainerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "container_id 不能为空",
		})
		return
	}
	
	if err := collector.DeleteContainer(req.ContainerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "容器删除成功",
	})
}

// handleRestartContainer 重启容器
func (s *Server) handleRestartContainer(c *gin.Context) {
	var req struct {
		ContainerID string `json:"container_id"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
		})
		return
	}
	
	if req.ContainerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "container_id 不能为空",
		})
		return
	}
	
	if err := collector.RestartContainer(req.ContainerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "容器重启成功",
	})
}

// ==================== 认证相关 ====================

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
	// 获取 Token
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 {
			tokenString := parts[1]
			// 撤销 Token
			s.authManager.RevokeToken(tokenString)
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

// ==================== 远程主机管理 ====================

// handleGetRemoteHosts 获取远程主机列表
func (s *Server) handleGetRemoteHosts(c *gin.Context) {
	hosts, err := s.remoteManager.GetAllHosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    hosts,
	})
}

// handleAddRemoteHost 添加远程主机
func (s *Server) handleAddRemoteHost(c *gin.Context) {
	var host remote.RemoteHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}
	
	// 默认启用
	host.Enabled = true
	
	if err := s.remoteManager.AddHost(&host); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "添加主机成功",
		"data":    host,
	})
}

// handleUpdateRemoteHost 更新远程主机
func (s *Server) handleUpdateRemoteHost(c *gin.Context) {
	var host remote.RemoteHost
	if err := c.ShouldBindJSON(&host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}
	
	// 从 URL 中获取 ID
	id := c.Param("id")
	var hostID int
	if _, err := fmt.Sscanf(id, "%d", &hostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的主机 ID",
		})
		return
	}
	
	host.ID = hostID
	
	if err := s.remoteManager.UpdateHost(&host); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新主机成功",
	})
}

// handleDeleteRemoteHost 删除远程主机
func (s *Server) handleDeleteRemoteHost(c *gin.Context) {
	id := c.Param("id")
	var hostID int
	if _, err := fmt.Sscanf(id, "%d", &hostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的主机 ID",
		})
		return
	}
	
	if err := s.remoteManager.DeleteHost(hostID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除主机成功",
	})
}

// handleCheckHostStatus 检查主机状态
func (s *Server) handleCheckHostStatus(c *gin.Context) {
	id := c.Param("id")
	var hostID int
	if _, err := fmt.Sscanf(id, "%d", &hostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的主机 ID",
		})
		return
	}
	
	status, err := s.remoteManager.CheckHostStatus(hostID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// handleCheckAllHostsStatus 检查所有主机状态
func (s *Server) handleCheckAllHostsStatus(c *gin.Context) {
	statuses, err := s.remoteManager.CheckAllHostsStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    statuses,
	})
}

// ==================== 远程数据代理 ====================

// handleRemoteMetrics 代理获取远程主机的实时监控数据
func (s *Server) handleRemoteMetrics(c *gin.Context) {
	hostID := c.Param("host_id")
	var id int
	if _, err := fmt.Sscanf(hostID, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的主机 ID",
		})
		return
	}
	
	data, err := s.remoteManager.ProxyRequest(id, "/api/metrics")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	
	// 直接返回远程主机的 JSON 响应
	c.Data(http.StatusOK, "application/json", data)
}

// handleRemoteDocker 代理获取远程主机的 Docker 数据
func (s *Server) handleRemoteDocker(c *gin.Context) {
	hostID := c.Param("host_id")
	var id int
	if _, err := fmt.Sscanf(hostID, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的主机 ID",
		})
		return
	}
	
	data, err := s.remoteManager.ProxyRequest(id, "/api/docker")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	
	c.Data(http.StatusOK, "application/json", data)
}

// handleRemoteHistoryCPU 代理获取远程主机的 CPU 历史数据
func (s *Server) handleRemoteHistoryCPU(c *gin.Context) {
	hostID := c.Param("host_id")
	var id int
	if _, err := fmt.Sscanf(hostID, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的主机 ID",
		})
		return
	}
	
	// 传递查询参数
	apiPath := "/api/history/cpu"
	if len(c.Request.URL.RawQuery) > 0 {
		apiPath += "?" + c.Request.URL.RawQuery
	}
	
	data, err := s.remoteManager.ProxyRequest(id, apiPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	
	c.Data(http.StatusOK, "application/json", data)
}

// handleRemoteHistoryMemory 代理获取远程主机的内存历史数据
func (s *Server) handleRemoteHistoryMemory(c *gin.Context) {
	hostID := c.Param("host_id")
	var id int
	if _, err := fmt.Sscanf(hostID, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的主机 ID",
		})
		return
	}
	
	apiPath := "/api/history/memory"
	if len(c.Request.URL.RawQuery) > 0 {
		apiPath += "?" + c.Request.URL.RawQuery
	}
	
	data, err := s.remoteManager.ProxyRequest(id, apiPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	
	c.Data(http.StatusOK, "application/json", data)
}

// handleRemoteHistoryDisk 代理获取远程主机的磁盘历史数据
func (s *Server) handleRemoteHistoryDisk(c *gin.Context) {
	hostID := c.Param("host_id")
	var id int
	if _, err := fmt.Sscanf(hostID, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的主机 ID",
		})
		return
	}
	
	apiPath := "/api/history/disk"
	if len(c.Request.URL.RawQuery) > 0 {
		apiPath += "?" + c.Request.URL.RawQuery
	}
	
	data, err := s.remoteManager.ProxyRequest(id, apiPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	
	c.Data(http.StatusOK, "application/json", data)
}

// handleRemoteHistoryNetwork 代理获取远程主机的网络历史数据
func (s *Server) handleRemoteHistoryNetwork(c *gin.Context) {
	hostID := c.Param("host_id")
	var id int
	if _, err := fmt.Sscanf(hostID, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的主机 ID",
		})
		return
	}
	
	apiPath := "/api/history/network"
	if len(c.Request.URL.RawQuery) > 0 {
		apiPath += "?" + c.Request.URL.RawQuery
	}
	
	data, err := s.remoteManager.ProxyRequest(id, apiPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	
	c.Data(http.StatusOK, "application/json", data)
}
