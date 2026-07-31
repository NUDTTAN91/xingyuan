/*
 * 星垣 - 网络监控采集
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

// updateBaseline 更新网络流量基准值并返回真实累计值
// 当计数器回绕（当前值 < 上次值，说明宿主机重启）时，把上次累计值滚入基准，
// 从而实现"历史累计流量"跨重启不清零
func updateBaseline(baseline *NetworkBaseline, totalRecv, totalSent uint64) (finalRecv, finalSent uint64) {
	// 检测系统是否重启（当前值 < 上次值）
	if baseline.LastRecv > 0 && (totalRecv < baseline.LastRecv || totalSent < baseline.LastSent) {
		// 系统重启了，将上次的累计值加入基准
		baseline.BytesRecvBaseline += baseline.LastRecv
		baseline.BytesSentBaseline += baseline.LastSent
	}

	// 更新上次读取的值
	baseline.LastRecv = totalRecv
	baseline.LastSent = totalSent

	// 计算真实的累计值（基准值 + 当前系统值）
	return baseline.BytesRecvBaseline + totalRecv, baseline.BytesSentBaseline + totalSent
}

// collectNetwork 采集网络指标（host网络模式下直接读取宿主机网卡）
func (c *Collector) collectNetwork() (*NetworkMetrics, error) {
	// 使用 host 网络模式，可以直接读取宿主机网卡数据
	ioCounters, err := net.IOCounters(true) // true = 每个网卡分别统计
	if err != nil {
		return nil, err
	}

	var totalRecv, totalSent uint64
	for _, io := range ioCounters {
		// 过滤回环接口
		if io.Name == "lo" {
			continue
		}
		totalRecv += io.BytesRecv
		totalSent += io.BytesSent
	}

	finalRecv, finalSent := updateBaseline(c.networkBaseline, totalRecv, totalSent)

	metrics := &NetworkMetrics{
		BytesRecv: finalRecv,
		BytesSent: finalSent,
	}

	// 计算网络速度
	now := time.Now()
	if c.lastNetIO != nil {
		duration := now.Sub(c.lastNetTime).Seconds()
		if duration > 0 {
			metrics.DownloadSpeed = roundFloat(float64(totalRecv-c.lastNetIO.BytesRecv)/duration, 2)
			metrics.UploadSpeed = roundFloat(float64(totalSent-c.lastNetIO.BytesSent)/duration, 2)
		}
	}

	c.lastNetIO = &NetworkIOStat{
		BytesRecv: totalRecv,
		BytesSent: totalSent,
	}
	c.lastNetTime = now

	return metrics, nil
}
