/*
 * 星垣 - 远程主机管理与数据代理处理器
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"net/http"
	"strconv"
	"xingyuan-monitor/remote"

	"github.com/gin-gonic/gin"
)

// parseIntParam 解析路径参数为整数
// 使用 strconv.Atoi 严格解析（拒绝 "12abc" 这类前缀合法的脏输入）
func parseIntParam(c *gin.Context, name string) (int, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil {
		return 0, false
	}
	return id, true
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
	hostID, ok := parseIntParam(c, "id")
	if !ok {
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
	hostID, ok := parseIntParam(c, "id")
	if !ok {
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
	hostID, ok := parseIntParam(c, "id")
	if !ok {
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

// remoteProxyHandler 远程数据代理通用处理器工厂
// metrics/docker/history 的代理流程完全相同，仅远端API路径与是否透传查询参数不同
func (s *Server) remoteProxyHandler(apiPath string, forwardQuery bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseIntParam(c, "host_id")
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的主机 ID",
			})
			return
		}

		// 透传查询参数（历史查询需要 start/end）
		path := apiPath
		if forwardQuery && len(c.Request.URL.RawQuery) > 0 {
			path += "?" + c.Request.URL.RawQuery
		}

		data, err := s.remoteManager.ProxyRequest(id, path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// 直接返回远程主机的 JSON 响应
		c.Data(http.StatusOK, "application/json", data)
	}
}
