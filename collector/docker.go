/*
 * 星垣 - Docker监控采集
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DockerContainerInfo Docker容器信息
type DockerContainerInfo struct {
	ID      string   `json:"id"`
	Image   string   `json:"image"`
	Name    string   `json:"name"`
	State   string   `json:"state"`
	Ports   []string `json:"ports"`
	Created string   `json:"created"`
}

// DockerImageInfo Docker镜像信息
type DockerImageInfo struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	ID         string `json:"id"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

// DockerMetrics Docker监控指标
type DockerMetrics struct {
	Containers     []DockerContainerInfo `json:"containers"`
	Images         []DockerImageInfo     `json:"images"`
	ContainerCount int                   `json:"container_count"`
	RunningCount   int                   `json:"running_count"`
	ImageCount     int                   `json:"image_count"`
}

// dockerContainer Docker命令返回的容器结构
type dockerContainer struct {
	ID      string `json:"ID"`
	Image   string `json:"Image"`
	Names   string `json:"Names"`
	State   string `json:"State"`
	Ports   string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
}

// dockerImage Docker命令返回的镜像结构
type dockerImage struct {
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	Size       string `json:"Size"`
	CreatedAt  string `json:"CreatedAt"`
}

// collectDocker 采集Docker信息（使用命令行方式）
func (c *Collector) collectDocker() (*DockerMetrics, error) {
	metrics := &DockerMetrics{
		Containers: []DockerContainerInfo{},
		Images:     []DockerImageInfo{},
	}

	// 采集容器信息
	containerCmd := exec.Command("docker", "ps", "-a", "--format", "json")
	containerOutput, err := containerCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute docker ps: %v", err)
	}

	// 解析容器数据（每行一个JSON对象）
	runningCount := 0
	lines := strings.Split(strings.TrimSpace(string(containerOutput)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var container dockerContainer
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}

		// 处理端口映射
		ports := parsePorts(container.Ports)

		// 格式化创建时间为 YYYY-MM-DD HH:mm:ss
		created := formatDockerTime(container.CreatedAt)

		// 统计运行中的容器
		if container.State == "running" {
			runningCount++
		}

		containerInfo := DockerContainerInfo{
			ID:      container.ID[:12], // 只取前12位
			Image:   container.Image,
			Name:    container.Names,
			State:   container.State,
			Ports:   ports,
			Created: created,
		}
		metrics.Containers = append(metrics.Containers, containerInfo)
	}

	metrics.ContainerCount = len(metrics.Containers)
	metrics.RunningCount = runningCount

	// 采集镜像信息
	imageCmd := exec.Command("docker", "images", "--format", "json")
	imageOutput, err := imageCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute docker images: %v", err)
	}

	// 解析镜像数据
	lines = strings.Split(strings.TrimSpace(string(imageOutput)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var image dockerImage
		if err := json.Unmarshal([]byte(line), &image); err != nil {
			continue
		}

		// 格式化创建时间
		created := formatDockerTime(image.CreatedAt)

		imageInfo := DockerImageInfo{
			Repository: image.Repository,
			Tag:        image.Tag,
			ID:         image.ID,
			Size:       image.Size,
			Created:    created,
		}
		metrics.Images = append(metrics.Images, imageInfo)
	}

	metrics.ImageCount = len(metrics.Images)

	return metrics, nil
}

// parsePorts 解析端口映射字符串为数组
func parsePorts(portStr string) []string {
	if portStr == "" {
		return []string{}
	}
	// Docker输出格式: "0.0.0.0:8080->8080/tcp, 0.0.0.0:443->443/tcp"
	ports := []string{}
	parts := strings.Split(portStr, ", ")
	for _, part := range parts {
		// 提取端口映射 "0.0.0.0:8080->8080/tcp" => "8080:8080"
		if strings.Contains(part, "->") {
			mapping := strings.Split(part, "->")
			if len(mapping) == 2 {
				public := strings.Split(mapping[0], ":") // ["0.0.0.0", "8080"]
				private := strings.Split(mapping[1], "/") // ["8080", "tcp"]
				if len(public) == 2 && len(private) >= 1 {
					ports = append(ports, public[1]+":"+private[0])
				}
			}
		}
	}
	return ports
}

// formatDockerTime 格式化Docker时间为 YYYY-MM-DD HH:mm:ss
func formatDockerTime(timeStr string) string {
	// Docker时间格式: "2024-12-06 09:30:45 +0800 CST"
	// 目标格式: "2024-12-06 09:30:45"
	if timeStr == "" {
		return ""
	}

	// 尝试多种时间格式解析
	formats := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}

	// 如果解析失败，直接截取前19个字符
	if len(timeStr) >= 19 {
		return timeStr[:19]
	}

	return timeStr
}

// StopContainer 停止容器
func StopContainer(containerID string) error {
	// 设置超时时间为 3 秒，避免等待太久
	cmd := exec.Command("docker", "stop", "-t", "3", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop container: %v, output: %s", err, string(output))
	}
	return nil
}

// DeleteContainer 删除容器
func DeleteContainer(containerID string) error {
	// 先停止容器（如果正在运行），3秒超时
	exec.Command("docker", "stop", "-t", "3", containerID).Run() // 忽略错误，容器可能已停止
	
	// 删除容器
	cmd := exec.Command("docker", "rm", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete container: %v, output: %s", err, string(output))
	}
	return nil
}

// RestartContainer 重启容器
func RestartContainer(containerID string) error {
	cmd := exec.Command("docker", "restart", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart container: %v, output: %s", err, string(output))
	}
	return nil
}
