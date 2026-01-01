/*
 * 星垣 - 数据结构定义
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

// SystemMetrics 系统监控指标
type SystemMetrics struct {
	Timestamp    int64          `json:"timestamp"`
	CPU          CPUMetrics     `json:"cpu"`
	Memory       MemoryMetrics  `json:"memory"`
	Disk         DiskMetrics    `json:"disk"`
	Network      NetworkMetrics `json:"network"`
	Processes    []ProcessInfo  `json:"processes"`
	SystemInfo   SystemInfo     `json:"system_info"`
	DatabaseInfo DatabaseInfo   `json:"database_info"` // 数据库统计信息
}

// CPUMetrics CPU指标
type CPUMetrics struct {
	UsagePercent  float64 `json:"usage_percent"`
	Cores         int     `json:"cores"`
	UserPercent   float64 `json:"user_percent"`
	SystemPercent float64 `json:"system_percent"`
	IdlePercent   float64 `json:"idle_percent"`
}

// MemoryMetrics 内存指标
type MemoryMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	Available    uint64  `json:"available"`
}

// DiskMetrics 磁盘指标
type DiskMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	ReadSpeed    float64 `json:"read_speed"`
	WriteSpeed   float64 `json:"write_speed"`
}

// NetworkMetrics 网络指标
type NetworkMetrics struct {
	DownloadSpeed float64 `json:"download_speed"`
	UploadSpeed   float64 `json:"upload_speed"`
	BytesRecv     uint64  `json:"bytes_recv"`
	BytesSent     uint64  `json:"bytes_sent"`
}

// ProcessInfo 进程信息
type ProcessInfo struct {
	PID         int32   `json:"pid"`
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float32 `json:"mem_percent"`
	MemoryUsage uint64  `json:"memory_usage"`
	Status      string  `json:"status"`
}

// SystemInfo 系统信息
type SystemInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Platform string `json:"platform"`
	Uptime   uint64 `json:"uptime"`
}

// DatabaseInfo 数据库统计信息
type DatabaseInfo struct {
	TotalRecords int64 `json:"total_records"` // 总数据条数
	DataSize     int64 `json:"data_size"`     // data目录总大小（字节）
}
