/*
 * 星垣 - 系统信息采集
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/host"
)

// collectSystemInfo 采集系统信息
func (c *Collector) collectSystemInfo() (*SystemInfo, error) {
	// 从宿主机挂载点读取主机名
	hostname := "unknown"
	hostRoot := os.Getenv("HOST_ROOT")
	if hostRoot != "" {
		// 从宿主机的 /etc/hostname 读取
		if data, err := os.ReadFile(hostRoot + "/etc/hostname"); err == nil {
			hostname = strings.TrimSpace(string(data))
		}
	}
	if hostname == "unknown" || hostname == "" {
		hostname, _ = os.Hostname()
	}

	// 从宿主机读取系统信息
	var osName, platform string
	var uptime uint64

	if hostRoot != "" {
		// 读取 /etc/os-release
		if data, err := os.ReadFile(hostRoot + "/etc/os-release"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					platform = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				}
				if strings.HasPrefix(line, "ID=") && !strings.HasPrefix(line, "ID_LIKE=") {
					osName = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
				}
			}
		}

		// 读取 uptime
		if data, err := os.ReadFile(hostRoot + "/proc/uptime"); err == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				if uptimeSec, err := strconv.ParseFloat(parts[0], 64); err == nil {
					uptime = uint64(uptimeSec)
				}
			}
		}
	}

	// 如果从宿主机读取失败，使用 gopsutil 库
	if platform == "" {
		info, err := host.Info()
		if err == nil {
			osName = info.OS
			platform = info.Platform + " " + info.PlatformVersion
			uptime = info.Uptime
		}
	}

	return &SystemInfo{
		Hostname: hostname,
		OS:       osName,
		Platform: platform,
		Uptime:   uptime,
	}, nil
}
