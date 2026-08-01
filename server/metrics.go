/*
 * 星垣 - 指标采集循环与落库
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"log"
	"net/http"
	"time"
	"xingyuan-monitor/collector"
	"xingyuan-monitor/database"

	"github.com/gin-gonic/gin"
)

// handleHealth 健康检查（无需认证，仅返回服务存活状态）
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleMetrics 获取监控数据
// 直接返回广播协程每秒更新的缓存，避免重复采集与并发竞争
func (s *Server) handleMetrics(c *gin.Context) {
	s.metricsMutex.RLock()
	metrics := s.latestMetrics
	s.metricsMutex.RUnlock()

	if metrics == nil {
		// 服务刚启动、首次采集尚未完成（最多1秒窗口）
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "监控数据尚未就绪，请稍后重试",
		})
		return
	}

	c.JSON(http.StatusOK, metrics)
}

// BroadcastMetrics 广播监控数据到所有WebSocket客户端
func (s *Server) BroadcastMetrics() {
	// 启动时立即采集一次，让缓存尽快就绪
	s.collectAndBroadcast()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.collectAndBroadcast()
	}
}

// collectAndBroadcast 采集一次指标：更新缓存、异步落库并广播给WS客户端
func (s *Server) collectAndBroadcast() {
	metrics, err := s.collector.Collect()
	if err != nil {
		log.Printf("Collect metrics error: %v", err)
		return
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

	// 更新指标缓存（供 /api/metrics 直接读取）
	s.metricsMutex.Lock()
	s.latestMetrics = metrics
	s.metricsMutex.Unlock()

	// 将监控数据交给常驻写协程落库（队列满则丢弃本秒数据，避免goroutine堆积）
	// 基准值在采集协程内先取快照，避免落库协程与下一次采集并发读写
	baseline := *s.collector.GetBaseline()
	select {
	case s.saveQueue <- saveTask{metrics: metrics, baseline: baseline}:
	default:
		log.Printf("落库队列已满，丢弃本次监控数据（数据库可能繁忙）")
	}

	// 广播给所有WS客户端
	s.broadcastToClients(metrics)
}

// saveWorker 常驻落库协程：串行消费队列，与SQLite单连接模型匹配
func (s *Server) saveWorker() {
	for task := range s.saveQueue {
		s.saveMetricsToDatabase(task.metrics, task.baseline)
	}
}

// saveMetricsToDatabase 将监控数据保存到数据库（采集失败的子项跳过，避免写入假零值）
func (s *Server) saveMetricsToDatabase(metrics *collector.SystemMetrics, baseline collector.NetworkBaseline) {
	// 保存CPU数据
	if !metrics.CollectFailed("cpu") {
		if err := database.InsertCPUMetrics(metrics.CPU.UsagePercent); err != nil {
			log.Printf("保存CPU数据失败: %v", err)
		}
	}

	// 保存内存数据
	if !metrics.CollectFailed("memory") {
		if err := database.InsertMemoryMetrics(metrics.Memory.Used, metrics.Memory.Total, metrics.Memory.UsagePercent); err != nil {
			log.Printf("保存内存数据失败: %v", err)
		}
	}

	// 保存磁盘数据
	if !metrics.CollectFailed("disk") {
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
	}

	// 保存网络数据（包括速度和累计流量）
	if !metrics.CollectFailed("network") {
		if err := database.InsertNetworkMetrics(
			metrics.Network.UploadSpeed,
			metrics.Network.DownloadSpeed,
			metrics.Network.BytesSent,
			metrics.Network.BytesRecv,
		); err != nil {
			log.Printf("保存网络数据失败: %v", err)
		}
	}

	// 每秒保存网络基准值（用于系统重启后恢复）
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
