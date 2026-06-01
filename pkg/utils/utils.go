package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// PathSeparator 路径分隔符
var PathSeparator = string(os.PathSeparator)

// ConvertPath 转换路径分隔符（百度网盘用/，本地系统可能不同）
func ConvertPath(remotePath string) string {
	if runtime.GOOS == "windows" {
		// 将/转换为\
		return filepath.FromSlash(remotePath)
	}
	return remotePath
}

// GetParentDir 获取父目录路径
func GetParentDir(path string) string {
	// 移除末尾的斜杠
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	
	// 查找最后一个斜杠
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i+1]
		}
	}
	
	return "/"
}

// GetFileName 获取文件名
func GetFileName(path string) string {
	// 移除末尾的斜杠
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	
	// 查找最后一个斜杠
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	
	return path
}

// FileExists 检查文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// IsDir 检查是否为目录
func IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// FormatSize 格式化文件大小
func FormatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	
	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/float64(TB))
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// FormatDuration 格式化时间长度
func FormatDuration(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟%d秒", hours, minutes, secs)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分钟%d秒", minutes, secs)
	} else {
		return fmt.Sprintf("%d秒", secs)
	}
}

// CreateDirIfNotExist 如果目录不存在则创建
func CreateDirIfNotExist(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

// GetAbsolutePath 获取绝对路径
func GetAbsolutePath(path string) (string, error) {
	return filepath.Abs(path)
}