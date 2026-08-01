/*
 * 星垣 - Web服务器
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"xingyuan-monitor/auth"
	"xingyuan-monitor/collector"
	"xingyuan-monitor/database"
	"xingyuan-monitor/remote"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// saveTask 待落库的一次采集数据
type saveTask struct {
	metrics  *collector.SystemMetrics
	baseline collector.NetworkBaseline
}

// Server Web服务器
type Server struct {
	collector     *collector.Collector
	authManager   *auth.AuthManager
	remoteManager *remote.Manager
	router        *gin.Engine
	// WS客户端 -> 每连接发送队列（网络写在独立协程中完成，慢客户端不拖累广播）
	wsClients map[*websocket.Conn]chan *collector.SystemMetrics
	wsMutex   sync.Mutex
	upgrader  websocket.Upgrader
	// 最新一次采集的指标缓存（由广播协程每秒更新）
	// /api/metrics 直接读缓存，避免每个请求都重复采集（CPU采样需阻塞100ms）
	latestMetrics *collector.SystemMetrics
	metricsMutex  sync.RWMutex
	// 落库有界队列：由单个常驻写协程消费，避免SQLite阻塞时goroutine无界堆积
	saveQueue chan saveTask
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
		wsClients:     make(map[*websocket.Conn]chan *collector.SystemMetrics),
		// 队列容量60（约1分钟数据）：SQLite长时间阻塞时丢弃新数据而非无界堆积
		saveQueue: make(chan saveTask, 60),
		upgrader: websocket.Upgrader{
			// 同源校验：拒绝其他网站页面发起的跨源WS连接
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					// 非浏览器客户端（无Origin头）放行，仍需通过Token验证
					return true
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return u.Host == r.Host
			},
		},
	}

	// 可信代理配置：默认不信任任何代理头，防止伪造 X-Forwarded-For 绕过登录锁定
	// 部署在反向代理后时通过 TRUSTED_PROXIES 环境变量指定代理IP/CIDR（逗号分隔）
	var trustedProxies []string
	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				trustedProxies = append(trustedProxies, p)
			}
		}
	}
	if err := s.router.SetTrustedProxies(trustedProxies); err != nil {
		log.Printf("设置可信代理失败: %v", err)
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
	// 静态资源缓存策略：第三方库长缓存，业务资源协商缓存（依赖304）
	s.router.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/static/") {
			if strings.HasSuffix(p, "/chart.umd.min.js") {
				// Chart.js 等第三方库几乎不变，缓存7天
				c.Header("Cache-Control", "public, max-age=604800")
			} else {
				// 业务JS/CSS/HTML：每次协商验证（文件未变时返回304，不重复传输）
				c.Header("Cache-Control", "no-cache")
			}
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
		// 健康检查（供 Docker healthcheck 使用，不暴露任何监控数据）
		public.GET("/health", s.handleHealth)
	}

	// 受保护的API接口（需要认证）
	api := s.router.Group("/api")
	api.Use(s.authMiddleware())
	{
		api.GET("/metrics", s.handleMetrics)
		api.GET("/docker", s.handleDocker)
		api.POST("/logout", s.handleLogout)
		api.GET("/verify", s.handleVerify)

		// 历史数据查询接口（统一由 historyHandler 工厂生成）
		api.GET("/history/cpu", s.historyHandler(func(start, end string, interval int) (any, error) {
			return database.QueryCPUMetricsSampled(start, end, interval)
		}))
		api.GET("/history/memory", s.historyHandler(func(start, end string, interval int) (any, error) {
			return database.QueryMemoryMetricsSampled(start, end, interval)
		}))
		api.GET("/history/disk", s.historyHandler(func(start, end string, interval int) (any, error) {
			return database.QueryDiskMetricsSampled(start, end, interval)
		}))
		api.GET("/history/network", s.historyHandler(func(start, end string, interval int) (any, error) {
			return database.QueryNetworkMetricsSampled(start, end, interval)
		}))

		// 数据库统计接口
		api.GET("/stats/database", s.handleDatabaseStats)
		api.GET("/stats/timerange", s.handleDataTimeRange)

		// Docker容器操作接口（统一由 containerActionHandler 工厂生成）
		api.POST("/docker/container/stop", s.containerActionHandler(collector.StopContainer, "容器停止成功"))
		api.POST("/docker/container/delete", s.containerActionHandler(collector.DeleteContainer, "容器删除成功"))
		api.POST("/docker/container/restart", s.containerActionHandler(collector.RestartContainer, "容器重启成功"))

		// 远程主机管理接口
		api.GET("/remote/hosts", s.handleGetRemoteHosts)
		api.POST("/remote/hosts", s.handleAddRemoteHost)
		api.PUT("/remote/hosts/:id", s.handleUpdateRemoteHost)
		api.DELETE("/remote/hosts/:id", s.handleDeleteRemoteHost)
		api.GET("/remote/hosts/:id/status", s.handleCheckHostStatus)
		api.GET("/remote/hosts/status/all", s.handleCheckAllHostsStatus)

		// 远程数据代理接口（统一由 remoteProxyHandler 工厂生成，历史接口透传查询参数）
		api.GET("/remote/:host_id/metrics", s.remoteProxyHandler("/api/metrics", false))
		api.GET("/remote/:host_id/docker", s.remoteProxyHandler("/api/docker", false))
		api.GET("/remote/:host_id/history/cpu", s.remoteProxyHandler("/api/history/cpu", true))
		api.GET("/remote/:host_id/history/memory", s.remoteProxyHandler("/api/history/memory", true))
		api.GET("/remote/:host_id/history/disk", s.remoteProxyHandler("/api/history/disk", true))
		api.GET("/remote/:host_id/history/network", s.remoteProxyHandler("/api/history/network", true))
	}
}

// handleIndex 首页处理
func (s *Server) handleIndex(c *gin.Context) {
	c.File("./static/index.html")
}

// Run 启动服务器（支持优雅停机：收到SIGINT/SIGTERM后停止接收新请求并等待处理完成）
func (s *Server) Run(addr string) error {
	// 启动常驻落库协程
	go s.saveWorker()
	// 启动WebSocket广播
	go s.BroadcastMetrics()

	log.Printf("星垣启动成功，访问地址: http://%s", addr)
	log.Printf("Author: tan91 | GitHub: https://github.com/NUDTTAN91")

	httpServer := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		log.Printf("收到信号 %v，开始优雅停机...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP服务停机异常: %v", err)
		}
		log.Printf("服务已停止")
		return nil
	}
}
