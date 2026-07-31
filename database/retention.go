package database

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

// aggregateTask 单张原始表的压缩任务定义
type aggregateTask struct {
	rawTable string // 原始表名
	aggSQL   string // 将超期原始数据按分钟聚合写入聚合表的SQL（参数：cutoff）
}

// 各表的分钟级聚合SQL：按 timestamp 前16位（精确到分钟）分组
var aggregateTasks = []aggregateTask{
	{
		rawTable: "cpu_metrics",
		aggSQL: `INSERT INTO cpu_metrics_agg (usage, timestamp)
			SELECT AVG(usage), substr(timestamp, 1, 16) || ':00'
			FROM cpu_metrics WHERE timestamp < ?
			GROUP BY substr(timestamp, 1, 16)`,
	},
	{
		rawTable: "memory_metrics",
		aggSQL: `INSERT INTO memory_metrics_agg (used, total, usage, timestamp)
			SELECT CAST(AVG(used) AS INTEGER), CAST(AVG(total) AS INTEGER), AVG(usage), substr(timestamp, 1, 16) || ':00'
			FROM memory_metrics WHERE timestamp < ?
			GROUP BY substr(timestamp, 1, 16)`,
	},
	{
		rawTable: "disk_metrics",
		aggSQL: `INSERT INTO disk_metrics_agg (used, free, total, usage, read_speed, write_speed, timestamp)
			SELECT CAST(AVG(used) AS INTEGER), CAST(AVG(free) AS INTEGER), CAST(AVG(total) AS INTEGER),
				AVG(usage), AVG(read_speed), AVG(write_speed), substr(timestamp, 1, 16) || ':00'
			FROM disk_metrics WHERE timestamp < ?
			GROUP BY substr(timestamp, 1, 16)`,
	},
	{
		rawTable: "network_metrics",
		aggSQL: `INSERT INTO network_metrics_agg (upload_speed, download_speed, bytes_sent, bytes_recv, timestamp)
			SELECT AVG(upload_speed), AVG(download_speed), MAX(bytes_sent), MAX(bytes_recv), substr(timestamp, 1, 16) || ':00'
			FROM network_metrics WHERE timestamp < ?
			GROUP BY substr(timestamp, 1, 16)`,
	},
}

// StartRetentionRoutine 启动数据保留后台任务
// 每天将超过 retentionDays 的原始秒级数据压缩为分钟级聚合数据后删除，
// 防止数据库无限增长导致系统越来越慢
func StartRetentionRoutine(retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}

	go func() {
		// 启动1分钟后先执行一次（处理历史积压），之后每24小时执行一次
		time.Sleep(1 * time.Minute)
		runRetention(retentionDays)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			runRetention(retentionDays)
		}
	}()

	log.Printf("数据保留任务已启动: 原始数据保留 %d 天，超期数据自动压缩为分钟级", retentionDays)
}

// runRetention 执行一次压缩清理
func runRetention(retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01-02 15:04:05")
	start := time.Now()
	var totalDeleted int64

	for _, task := range aggregateTasks {
		deleted, err := aggregateAndDelete(task, cutoff)
		if err != nil {
			log.Printf("压缩表 %s 失败: %v", task.rawTable, err)
			continue
		}
		totalDeleted += deleted
	}

	if totalDeleted == 0 {
		return
	}

	// 重新校准总记录数计数器
	if err := initTotalRecords(); err != nil {
		log.Printf("清理后重新统计记录数失败: %v", err)
	}

	// 收缩 WAL 文件
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("WAL checkpoint 失败: %v", err)
	}

	// 首次清理大量历史积压时执行 VACUUM 回收磁盘空间
	// （日常清理量小，空闲页会被新数据复用，无需每次 VACUUM）
	if totalDeleted > 500000 {
		log.Printf("本次清理 %d 条记录，开始 VACUUM 回收磁盘空间（可能需要一段时间）...", totalDeleted)
		if _, err := db.Exec("VACUUM"); err != nil {
			log.Printf("VACUUM 失败: %v", err)
		}
	}

	log.Printf("数据清理完成: 压缩并删除 %d 条超期原始记录，当前总记录数 %d，耗时 %v",
		totalDeleted, atomic.LoadInt64(&totalRecordsCount), time.Since(start).Round(time.Millisecond))
}

// aggregateAndDelete 在同一事务中完成：聚合写入 -> 删除原始数据
// 保证不会出现"删了原始数据但聚合数据没写入"的情况
func aggregateAndDelete(task aggregateTask, cutoff string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("开启事务失败: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(task.aggSQL, cutoff); err != nil {
		return 0, fmt.Errorf("聚合数据失败: %v", err)
	}

	result, err := tx.Exec("DELETE FROM "+task.rawTable+" WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("删除超期数据失败: %v", err)
	}

	deleted, _ := result.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %v", err)
	}
	return deleted, nil
}
