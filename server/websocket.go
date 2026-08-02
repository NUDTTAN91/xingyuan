/*
 * 星垣 - WebSocket 连接管理
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package server

import (
	"log"
	"net/http"
	"strings"
	"time"
	"xingyuan-monitor/auth"
	"xingyuan-monitor/collector"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WebSocket 连接参数
const (
	wsMaxClients   = 100              // 最大并发连接数
	wsWriteTimeout = 10 * time.Second // 单次写超时
	wsPongTimeout  = 90 * time.Second // 读超时：超过该时长未收到pong/消息判定为死连接
	wsPingInterval = 30 * time.Second // 心跳间隔
)

// handleWebSocket WebSocket连接处理
func (s *Server) handleWebSocket(c *gin.Context) {
	// 优先从 Sec-WebSocket-Protocol 子协议中获取 Token（不进访问日志，比URL参数安全）
	// 前端格式: new WebSocket(url, ["xingyuan-auth", token])
	var token string
	usedSubprotocol := false
	if protoHeader := c.GetHeader("Sec-WebSocket-Protocol"); protoHeader != "" {
		for _, p := range strings.Split(protoHeader, ",") {
			p = strings.TrimSpace(p)
			if p != "" && p != "xingyuan-auth" {
				token = p
				usedSubprotocol = true
				break
			}
		}
	}
	// 兼容旧客户端：从查询参数获取 Token
	if token == "" {
		token = c.Request.URL.Query().Get("token")
	}
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
		respondError(c, http.StatusUnauthorized, "未提供认证信息")
		return
	}

	claims, err := s.authManager.ValidateToken(token, auth.TokenTypeAccess)
	if err != nil {
		log.Printf("WebSocket 连接被拒绝：Token 验证失败: %v", err)
		respondError(c, http.StatusUnauthorized, "Token无效或已过期")
		return
	}

	log.Printf("WebSocket 连接成功，用户: %s, IP: %s", claims.Username, c.ClientIP())

	// 连接数上限保护
	s.wsMutex.Lock()
	clientCount := len(s.wsClients)
	s.wsMutex.Unlock()
	if clientCount >= wsMaxClients {
		log.Printf("WebSocket 连接被拒绝：已达连接数上限 %d", wsMaxClients)
		respondError(c, http.StatusServiceUnavailable, "连接数已达上限")
		return
	}

	// 若客户端通过子协议传递Token，握手响应需回选一个客户端提供的子协议
	var responseHeader http.Header
	if usedSubprotocol {
		responseHeader = http.Header{"Sec-WebSocket-Protocol": {"xingyuan-auth"}}
	}

	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, responseHeader)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// 每连接独立发送队列 + 写协程：网络写不在广播循环中进行
	sendCh := make(chan *collector.SystemMetrics, 4)
	s.wsMutex.Lock()
	s.wsClients[conn] = sendCh
	s.wsMutex.Unlock()

	go s.wsWriter(conn, sendCh)

	defer func() {
		s.wsMutex.Lock()
		delete(s.wsClients, conn)
		s.wsMutex.Unlock()
		// 从map移除后广播协程不会再向该channel发送，可安全关闭以退出写协程
		close(sendCh)
		conn.Close()
	}()

	// 读超时 + pong心跳：死连接在 wsPongTimeout 内被回收，不再等TCP层超时
	conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
		return nil
	})

	// 保持连接
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
		// 收到任意消息也视为存活
		conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	}
}

// wsWriter 单个WS连接的发送协程：带写超时与ping心跳，慢客户端只影响自己
func (s *Server) wsWriter(conn *websocket.Conn, sendCh chan *collector.SystemMetrics) {
	pingTicker := time.NewTicker(wsPingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case metrics, ok := <-sendCh:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
			if err := conn.WriteJSON(metrics); err != nil {
				log.Printf("WebSocket write error: %v", err)
				// 关闭连接使读循环退出，触发注销清理
				conn.Close()
				return
			}
		case <-pingTicker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteTimeout)); err != nil {
				conn.Close()
				return
			}
		}
	}
}

// broadcastToClients 将指标非阻塞投递到各连接的发送队列，网络写由各自的写协程完成
func (s *Server) broadcastToClients(metrics *collector.SystemMetrics) {
	s.wsMutex.Lock()
	for _, sendCh := range s.wsClients {
		select {
		case sendCh <- metrics:
		default:
			// 慢客户端队列已满：丢弃本次数据，不拖累其他客户端
		}
	}
	s.wsMutex.Unlock()
}
