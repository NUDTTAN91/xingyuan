/*
 * 星垣 - 历史数据与统计查询
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"net/http"
	"time"
	"xingyuan-monitor/database"

	"github.com/gin-gonic/gin"
)

// historyHandler 历史查询通用处理器工厂
// 4类指标（CPU/内存/磁盘/网络）的查询流程完全相同，仅查询函数不同
func (s *Server) historyHandler(query func(startTime, endTime string, sampleInterval int) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := c.DefaultQuery("start", "1970-01-01 00:00:00")
		endTime := c.DefaultQuery("end", "2099-12-31 23:59:59")

		// 计算采样间隔
		sampleInterval := calculateSampleInterval(startTime, endTime)

		metrics, err := query(startTime, endTime, sampleInterval)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusOK, metrics)
	}
}

// calculateSampleInterval 根据时间范围计算采样间隔（秒）
// 目标：返回的数据点数不超过 maxPoints
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
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, stats)
}

// handleDataTimeRange 获取数据时间范围
func (s *Server) handleDataTimeRange(c *gin.Context) {
	timeRange, err := database.GetDataTimeRange()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, timeRange)
}
