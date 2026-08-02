package database

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// totalRecordsCount 内存中维护的总记录数计数器
// 避免每秒对4张表做全表 COUNT(*)（数据量大时非常慢）
var totalRecordsCount int64

// initTotalRecords 启动时统计一次总记录数，之后由插入操作增量维护
func initTotalRecords() error {
	tables := []string{
		"cpu_metrics", "memory_metrics", "disk_metrics", "network_metrics",
		"cpu_metrics_agg", "memory_metrics_agg", "disk_metrics_agg", "network_metrics_agg",
	}
	var total int64
	for _, table := range tables {
		var count int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			return fmt.Errorf("统计表 %s 记录数失败: %v", table, err)
		}
		total += count
	}
	atomic.StoreInt64(&totalRecordsCount, total)
	return nil
}

// InsertCPUMetrics 插入CPU监控数据
func InsertCPUMetrics(usage float64) error {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("INSERT INTO cpu_metrics (usage, timestamp) VALUES (?, ?)", usage, timestamp)
	if err != nil {
		return fmt.Errorf("插入CPU数据失败: %v", err)
	}
	atomic.AddInt64(&totalRecordsCount, 1)
	return nil
}

// InsertMemoryMetrics 插入内存监控数据
func InsertMemoryMetrics(used, total uint64, usage float64) error {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("INSERT INTO memory_metrics (used, total, usage, timestamp) VALUES (?, ?, ?, ?)",
		used, total, usage, timestamp)
	if err != nil {
		return fmt.Errorf("插入内存数据失败: %v", err)
	}
	atomic.AddInt64(&totalRecordsCount, 1)
	return nil
}

// InsertDiskMetrics 插入磁盘监控数据
func InsertDiskMetrics(used, free, total uint64, usage, readSpeed, writeSpeed float64) error {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("INSERT INTO disk_metrics (used, free, total, usage, read_speed, write_speed, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?)",
		used, free, total, usage, readSpeed, writeSpeed, timestamp)
	if err != nil {
		return fmt.Errorf("插入磁盘数据失败: %v", err)
	}
	atomic.AddInt64(&totalRecordsCount, 1)
	return nil
}

// InsertNetworkMetrics 插入网络监控数据
func InsertNetworkMetrics(uploadSpeed, downloadSpeed float64, bytesSent, bytesRecv uint64) error {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("INSERT INTO network_metrics (upload_speed, download_speed, bytes_sent, bytes_recv, timestamp) VALUES (?, ?, ?, ?, ?)",
		uploadSpeed, downloadSpeed, bytesSent, bytesRecv, timestamp)
	if err != nil {
		return fmt.Errorf("插入网络数据失败: %v", err)
	}
	atomic.AddInt64(&totalRecordsCount, 1)
	return nil
}

// CPUMetric CPU监控记录
type CPUMetric struct {
	ID        int64   `json:"id"`
	Usage     float64 `json:"usage"`
	Timestamp string  `json:"timestamp"`
}

// MemoryMetric 内存监控记录
type MemoryMetric struct {
	ID        int64   `json:"id"`
	Used      uint64  `json:"used"`
	Total     uint64  `json:"total"`
	Usage     float64 `json:"usage"`
	Timestamp string  `json:"timestamp"`
}

// DiskMetric 磁盘监控记录
type DiskMetric struct {
	ID         int64   `json:"id"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	Total      uint64  `json:"total"`
	Usage      float64 `json:"usage"`
	ReadSpeed  float64 `json:"read_speed"`
	WriteSpeed float64 `json:"write_speed"`
	Timestamp  string  `json:"timestamp"`
}

// NetworkMetric 网络监控记录
type NetworkMetric struct {
	ID            int64   `json:"id"`
	UploadSpeed   float64 `json:"upload_speed"`
	DownloadSpeed float64 `json:"download_speed"`
	BytesSent     uint64  `json:"bytes_sent"`      // 累计上行流量（字节）
	BytesRecv     uint64  `json:"bytes_recv"`      // 累计下行流量（字节）
	Timestamp     string  `json:"timestamp"`
}

// NetworkBaseline 网络流量基准数据
type NetworkBaseline struct {
	ID                int    `json:"id"`
	BytesRecvBaseline uint64 `json:"bytes_recv_baseline"` // 下行基准值
	BytesSentBaseline uint64 `json:"bytes_sent_baseline"` // 上行基准值
	LastRecv          uint64 `json:"last_recv"`          // 上次读取的下行值
	LastSent          uint64 `json:"last_sent"`          // 上次读取的上行值
	UpdatedAt         string `json:"updated_at"`
}

// QueryCPUMetrics 查询CPU监控数据
func QueryCPUMetrics(startTime, endTime string, limit int) ([]CPUMetric, error) {
	query := "SELECT id, usage, timestamp FROM cpu_metrics WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC LIMIT ?"
	rows, err := db.Query(query, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("查询CPU数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []CPUMetric
	for rows.Next() {
		var m CPUMetric
		if err := rows.Scan(&m.ID, &m.Usage, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryCPUMetricsSampled 查询CPU监控数据（带采样）
// 联合原始表与分钟级聚合表，保证超过保留期的历史数据依然可查
func QueryCPUMetricsSampled(startTime, endTime string, sampleInterval int) ([]CPUMetric, error) {
	// 使用子查询实现采样：按时间分组，每组取平均值
	query := `
		SELECT 
			MIN(id) as id,
			AVG(usage) as usage,
			MIN(timestamp) as timestamp
		FROM (
			SELECT id, usage, timestamp FROM cpu_metrics WHERE timestamp >= ? AND timestamp <= ?
			UNION ALL
			SELECT id, usage, timestamp FROM cpu_metrics_agg WHERE timestamp >= ? AND timestamp <= ?
		)
		GROUP BY strftime('%s', timestamp) / ?
		ORDER BY timestamp ASC
	`
	rows, err := db.Query(query, startTime, endTime, startTime, endTime, sampleInterval)
	if err != nil {
		return nil, fmt.Errorf("查询CPU数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []CPUMetric
	for rows.Next() {
		var m CPUMetric
		if err := rows.Scan(&m.ID, &m.Usage, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryMemoryMetrics 查询内存监控数据
func QueryMemoryMetrics(startTime, endTime string, limit int) ([]MemoryMetric, error) {
	query := "SELECT id, used, total, usage, timestamp FROM memory_metrics WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC LIMIT ?"
	rows, err := db.Query(query, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("查询内存数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []MemoryMetric
	for rows.Next() {
		var m MemoryMetric
		if err := rows.Scan(&m.ID, &m.Used, &m.Total, &m.Usage, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryMemoryMetricsSampled 查询内存监控数据（带采样）
// 联合原始表与分钟级聚合表，保证超过保留期的历史数据依然可查
func QueryMemoryMetricsSampled(startTime, endTime string, sampleInterval int) ([]MemoryMetric, error) {
	query := `
		SELECT 
			MIN(id) as id,
			CAST(AVG(used) AS INTEGER) as used,
			CAST(AVG(total) AS INTEGER) as total,
			AVG(usage) as usage,
			MIN(timestamp) as timestamp
		FROM (
			SELECT id, used, total, usage, timestamp FROM memory_metrics WHERE timestamp >= ? AND timestamp <= ?
			UNION ALL
			SELECT id, used, total, usage, timestamp FROM memory_metrics_agg WHERE timestamp >= ? AND timestamp <= ?
		)
		GROUP BY strftime('%s', timestamp) / ?
		ORDER BY timestamp ASC
	`
	rows, err := db.Query(query, startTime, endTime, startTime, endTime, sampleInterval)
	if err != nil {
		return nil, fmt.Errorf("查询内存数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []MemoryMetric
	for rows.Next() {
		var m MemoryMetric
		if err := rows.Scan(&m.ID, &m.Used, &m.Total, &m.Usage, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryDiskMetrics 查询磁盘监控数据
func QueryDiskMetrics(startTime, endTime string, limit int) ([]DiskMetric, error) {
	query := "SELECT id, used, free, total, usage, read_speed, write_speed, timestamp FROM disk_metrics WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC LIMIT ?"
	rows, err := db.Query(query, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("查询磁盘数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []DiskMetric
	for rows.Next() {
		var m DiskMetric
		if err := rows.Scan(&m.ID, &m.Used, &m.Free, &m.Total, &m.Usage, &m.ReadSpeed, &m.WriteSpeed, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryDiskMetricsSampled 查询磁盘监控数据（带采样）
// 联合原始表与分钟级聚合表，保证超过保留期的历史数据依然可查
func QueryDiskMetricsSampled(startTime, endTime string, sampleInterval int) ([]DiskMetric, error) {
	query := `
		SELECT 
			MIN(id) as id,
			CAST(AVG(used) AS INTEGER) as used,
			CAST(AVG(free) AS INTEGER) as free,
			CAST(AVG(total) AS INTEGER) as total,
			AVG(usage) as usage,
			AVG(read_speed) as read_speed,
			AVG(write_speed) as write_speed,
			MIN(timestamp) as timestamp
		FROM (
			SELECT id, used, free, total, usage, read_speed, write_speed, timestamp FROM disk_metrics WHERE timestamp >= ? AND timestamp <= ?
			UNION ALL
			SELECT id, used, free, total, usage, read_speed, write_speed, timestamp FROM disk_metrics_agg WHERE timestamp >= ? AND timestamp <= ?
		)
		GROUP BY strftime('%s', timestamp) / ?
		ORDER BY timestamp ASC
	`
	rows, err := db.Query(query, startTime, endTime, startTime, endTime, sampleInterval)
	if err != nil {
		return nil, fmt.Errorf("查询磁盘数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []DiskMetric
	for rows.Next() {
		var m DiskMetric
		if err := rows.Scan(&m.ID, &m.Used, &m.Free, &m.Total, &m.Usage, &m.ReadSpeed, &m.WriteSpeed, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryNetworkMetrics 查询网络监控数据
func QueryNetworkMetrics(startTime, endTime string, limit int) ([]NetworkMetric, error) {
	query := "SELECT id, upload_speed, download_speed, bytes_sent, bytes_recv, timestamp FROM network_metrics WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC LIMIT ?"
	rows, err := db.Query(query, startTime, endTime, limit)
	if err != nil {
		return nil, fmt.Errorf("查询网络数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []NetworkMetric
	for rows.Next() {
		var m NetworkMetric
		if err := rows.Scan(&m.ID, &m.UploadSpeed, &m.DownloadSpeed, &m.BytesSent, &m.BytesRecv, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// QueryNetworkMetricsSampled 查询网络监控数据（带采样）
// 联合原始表与分钟级聚合表，保证超过保留期的历史数据依然可查
func QueryNetworkMetricsSampled(startTime, endTime string, sampleInterval int) ([]NetworkMetric, error) {
	query := `
		SELECT 
			MIN(id) as id,
			AVG(upload_speed) as upload_speed,
			AVG(download_speed) as download_speed,
			MAX(bytes_sent) as bytes_sent,
			MAX(bytes_recv) as bytes_recv,
			MIN(timestamp) as timestamp
		FROM (
			SELECT id, upload_speed, download_speed, bytes_sent, bytes_recv, timestamp FROM network_metrics WHERE timestamp >= ? AND timestamp <= ?
			UNION ALL
			SELECT id, upload_speed, download_speed, bytes_sent, bytes_recv, timestamp FROM network_metrics_agg WHERE timestamp >= ? AND timestamp <= ?
		)
		GROUP BY strftime('%s', timestamp) / ?
		ORDER BY timestamp ASC
	`
	rows, err := db.Query(query, startTime, endTime, startTime, endTime, sampleInterval)
	if err != nil {
		return nil, fmt.Errorf("查询网络数据失败: %v", err)
	}
	defer rows.Close()

	var metrics []NetworkMetric
	for rows.Next() {
		var m NetworkMetric
		if err := rows.Scan(&m.ID, &m.UploadSpeed, &m.DownloadSpeed, &m.BytesSent, &m.BytesRecv, &m.Timestamp); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历查询结果失败: %v", err)
	}
	return metrics, nil
}

// LoadNetworkBaseline 加载网络流量基准值
func LoadNetworkBaseline() (*NetworkBaseline, error) {
	query := "SELECT id, bytes_recv_baseline, bytes_sent_baseline, last_recv, last_sent, updated_at FROM network_baseline WHERE id = 1"
	row := db.QueryRow(query)
	
	var baseline NetworkBaseline
	if err := row.Scan(&baseline.ID, &baseline.BytesRecvBaseline, &baseline.BytesSentBaseline, 
		&baseline.LastRecv, &baseline.LastSent, &baseline.UpdatedAt); err != nil {
		return nil, fmt.Errorf("加载网络基准值失败: %v", err)
	}
	return &baseline, nil
}

// SaveNetworkBaseline 保存网络流量基准值
func SaveNetworkBaseline(baseline *NetworkBaseline) error {
	query := `UPDATE network_baseline SET 
		bytes_recv_baseline = ?, 
		bytes_sent_baseline = ?, 
		last_recv = ?, 
		last_sent = ?, 
		updated_at = datetime('now') 
		WHERE id = 1`
	
	_, err := db.Exec(query, baseline.BytesRecvBaseline, baseline.BytesSentBaseline, 
		baseline.LastRecv, baseline.LastSent)
	if err != nil {
		return fmt.Errorf("保存网络基准值失败: %v", err)
	}
	return nil
}

// DatabaseStats 数据库统计信息
type DatabaseStats struct {
	TotalRecords int64 `json:"total_records"` // 总数据条数
	DataSize     int64 `json:"data_size"`     // data目录总大小（字节）
}

// dirSizeCache 目录大小缓存（递归扫描目录较重，60秒刷新一次即可）
var (
	dirSizeMu       sync.Mutex
	dirSizeCached   int64
	dirSizeCachedAt time.Time
)

// GetDatabaseStats 获取数据库统计信息
// 总记录数使用内存计数器，目录大小使用60秒缓存，避免每秒全表扫描
func GetDatabaseStats() (*DatabaseStats, error) {
	stats := &DatabaseStats{
		TotalRecords: atomic.LoadInt64(&totalRecordsCount),
	}

	dirSizeMu.Lock()
	if time.Since(dirSizeCachedAt) > 60*time.Second {
		// 统计data目录的总大小
		// 优先尝试 /app/data（容器中），然后尝试 ./data（本地开发）
		dataSize, err := getDirSize("/app/data")
		if err != nil {
			dataSize, err = getDirSize("./data")
			if err != nil {
				// 目录大小获取失败不影响总记录数返回
				dataSize = 0
			}
		}
		dirSizeCached = dataSize
		dirSizeCachedAt = time.Now()
	}
	stats.DataSize = dirSizeCached
	dirSizeMu.Unlock()

	return stats, nil
}

// getDirSize 计算目录总大小（递归）
func getDirSize(path string) (int64, error) {
	var size int64
	
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}
	
	for _, entry := range entries {
		fullPath := path + "/" + entry.Name()
		
		if entry.IsDir() {
			// 递归计算子目录大小
			subSize, err := getDirSize(fullPath)
			if err != nil {
				continue // 跳过无法访问的目录
			}
			size += subSize
		} else {
			// 获取文件大小
			info, err := os.Stat(fullPath)
			if err != nil {
				continue // 跳过无法访问的文件
			}
			size += info.Size()
		}
	}
	
	return size, nil
}

// DataTimeRange 数据时间范围
type DataTimeRange struct {
	MinTime string `json:"min_time"` // 最早时间戳
	MaxTime string `json:"max_time"` // 最晚时间戳
}

// GetDataTimeRange 获取数据时间范围（从所有表中查询）
func GetDataTimeRange() (*DataTimeRange, error) {
	var minTime, maxTime string
	
	// 查询所有表的最早和最晚时间（含分钟级聚合表，保证清理后时间范围不缩水）
	query := `
		SELECT 
			MIN(min_ts) as min_time,
			MAX(max_ts) as max_time
		FROM (
			SELECT MIN(timestamp) as min_ts, MAX(timestamp) as max_ts FROM cpu_metrics
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM memory_metrics
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM disk_metrics
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM network_metrics
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM cpu_metrics_agg
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM memory_metrics_agg
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM disk_metrics_agg
			UNION ALL
			SELECT MIN(timestamp), MAX(timestamp) FROM network_metrics_agg
		)
	`
	
	err := db.QueryRow(query).Scan(&minTime, &maxTime)
	if err != nil {
		return nil, fmt.Errorf("查询数据时间范围失败: %v", err)
	}
	
	return &DataTimeRange{
		MinTime: minTime,
		MaxTime: maxTime,
	}, nil
}
