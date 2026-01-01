/*
 * 星垣 - 主程序
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"xingyuan-monitor/database"
	"xingyuan-monitor/server"
)

func main() {
	// 从环境变量获取端口，如果未设置则使用命令行参数
	defaultPort := "8080"
	if port := os.Getenv("SERVER_PORT"); port != "" {
		defaultPort = port
	}

	defaultAddr := fmt.Sprintf("0.0.0.0:%s", defaultPort)
	addr := flag.String("addr", defaultAddr, "监听地址和端口")
	flag.Parse()

	// 初始化数据库
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	dbPath := filepath.Join(dataDir, "monitor.db")
	if err := database.Init(dataDir); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer database.Close()
	log.Printf("数据库已初始化: %s", dbPath)

	srv := server.NewServer()
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
