/*
 * 星垣 - Docker 相关处理器
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleDocker 获取Docker监控数据
func (s *Server) handleDocker(c *gin.Context) {
	dockerMetrics, err := s.collector.CollectDocker()
	if err != nil {
		log.Printf("[ERROR] CollectDocker failed: %v", err)
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("[DEBUG] Docker metrics collected: containers=%d, images=%d", len(dockerMetrics.Containers), len(dockerMetrics.Images))
	c.JSON(http.StatusOK, dockerMetrics)
}

// isValidContainerID 校验容器ID格式（12~64位十六进制字符）
// 防止向 docker CLI 传入任意形态的参数字符串
func isValidContainerID(id string) bool {
	if len(id) < 12 || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

// containerActionHandler 容器操作通用处理器工厂
// 停止/删除/重启的处理流程完全相同，仅执行的docker操作与成功提示不同
func (s *Server) containerActionHandler(action func(containerID string) error, successMsg string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ContainerID string `json:"container_id"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "请求参数错误")
			return
		}

		if !isValidContainerID(req.ContainerID) {
			respondError(c, http.StatusBadRequest, "无效的容器ID")
			return
		}

		if err := action(req.ContainerID); err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": successMsg,
		})
	}
}
