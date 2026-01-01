package database

import (
	"fmt"
)

// createTables 创建所有监控表
func createTables() error {
	// CPU监控表
	cpuTable := `
	CREATE TABLE IF NOT EXISTS cpu_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		usage REAL NOT NULL,
		timestamp TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_cpu_timestamp ON cpu_metrics(timestamp);
	`

	// 内存监控表
	memoryTable := `
	CREATE TABLE IF NOT EXISTS memory_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		used BIGINT NOT NULL,
		total BIGINT NOT NULL,
		usage REAL NOT NULL,
		timestamp TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_memory_timestamp ON memory_metrics(timestamp);
	`

	// 磁盘监控表
	diskTable := `
	CREATE TABLE IF NOT EXISTS disk_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		used BIGINT NOT NULL,
		free BIGINT NOT NULL,
		total BIGINT NOT NULL,
		usage REAL NOT NULL,
		read_speed REAL NOT NULL,
		write_speed REAL NOT NULL,
		timestamp TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_disk_timestamp ON disk_metrics(timestamp);
	`

	// 网络监控表
	networkTable := `
	CREATE TABLE IF NOT EXISTS network_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		upload_speed REAL NOT NULL,
		download_speed REAL NOT NULL,
		bytes_sent BIGINT NOT NULL,
		bytes_recv BIGINT NOT NULL,
		timestamp TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_network_timestamp ON network_metrics(timestamp);
	`

	// 网络流量基准表（用于系统重启后的累计计算）
	networkBaselineTable := `
	CREATE TABLE IF NOT EXISTS network_baseline (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		bytes_recv_baseline BIGINT NOT NULL DEFAULT 0,
		bytes_sent_baseline BIGINT NOT NULL DEFAULT 0,
		last_recv BIGINT NOT NULL DEFAULT 0,
		last_sent BIGINT NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL
	);
	-- 插入默认记录
	INSERT OR IGNORE INTO network_baseline (id, bytes_recv_baseline, bytes_sent_baseline, last_recv, last_sent, updated_at) 
	VALUES (1, 0, 0, 0, 0, datetime('now'));
	`

	// 远程主机配置表
	remoteHostsTable := `
	CREATE TABLE IF NOT EXISTS remote_hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		address TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 80,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_remote_hosts_enabled ON remote_hosts(enabled);
	`

	// 执行建表语句
	tables := []string{cpuTable, memoryTable, diskTable, networkTable, networkBaselineTable, remoteHostsTable}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return fmt.Errorf("创建表失败: %v", err)
		}
	}

	return nil
}
