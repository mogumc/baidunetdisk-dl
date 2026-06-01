package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/siku2/arigo"
)

type DownloadTask struct {
	FID       int64
	Path      string
	LocalPath string
	FileSize  int64
	Filename  string
}

type DownloadWorker struct {
	Token      string
	Aria2RPC   string
	Aria2Token string
	OutputDir  string
	MaxRetry   int
	Timeout    time.Duration

	client *http.Client
	mu     sync.Mutex
}

func NewDownloadWorker(token, aria2RPC, aria2Token, outputDir string, maxRetry int, timeout time.Duration) *DownloadWorker {
	return &DownloadWorker{
		Token:      token,
		Aria2RPC:   aria2RPC,
		Aria2Token: aria2Token,
		OutputDir:  outputDir,
		MaxRetry:   maxRetry,
		Timeout:    timeout,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *DownloadWorker) ExecuteDownload(task *DownloadTask) error {
	fmt.Printf("开始下载: %s\n", task.Path)

	localPath := filepath.Join(w.OutputDir, task.Path)

	dlink, err := w.GetDownloadLink(task.FID)
	if err != nil {
		return fmt.Errorf("获取下载链接失败: %v", err)
	}

	localDir := filepath.Join(w.OutputDir, filepath.Dir(task.Path))
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	if err := w.DownloadWithAria2(dlink, localPath, task.Filename); err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}

	if err := w.VerifyDownload(localPath, task.FileSize); err != nil {
		return fmt.Errorf("文件验证失败: %v", err)
	}

	fmt.Printf("下载完成: %s -> %s\n", task.Path, localPath)
	return nil
}

func (w *DownloadWorker) GetDownloadLink(fid int64) (string, error) {
	baseURL := "https://pan.baidu.com/rest/2.0/xpan/multimedia"
	params := url.Values{}
	params.Set("method", "filemetas")
	params.Set("fsids", fmt.Sprintf("[%d]", fid))
	params.Set("dlink", "1")
	params.Set("access_token", w.Token)

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := w.client.Get(reqURL)
	if err != nil {
		return "", fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	var result struct {
		Errno  int    `json:"errno"`
		Errmsg string `json:"errmsg"`
		List   []struct {
			Dlink string `json:"dlink"`
		} `json:"list"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("JSON解析失败: %v, 原始响应: %s", err, string(body))
	}

	if result.Errno != 0 {
		return "", fmt.Errorf("百度网盘API错误 (errno=%d): %s", result.Errno, result.Errmsg)
	}

	if len(result.List) == 0 {
		return "", fmt.Errorf("未找到文件信息 (fid=%d)", fid)
	}

	dlink := result.List[0].Dlink
	if dlink == "" {
		return "", fmt.Errorf("下载链接为空 (fid=%d)", fid)
	}

	return dlink, nil
}

func (w *DownloadWorker) DownloadWithAria2(dlink, localPath, filename string) error {
	client, err := arigo.Dial(w.Aria2RPC, w.Aria2Token)
	if err != nil {
		return fmt.Errorf("连接aria2c RPC失败: %v", err)
	}
	defer client.Close()

	dlink = w.appendAccessToken(dlink)

	absDir, err := filepath.Abs(filepath.Dir(localPath))
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}

	options := &arigo.Options{
		Dir:    absDir,
		Out:    filename,
		Header: "User-Agent: pan.baidu.com",
		Split:  6,
		MaxConnectionPerServer: 16,
		MinSplitSize: "1M",
	}

	gid, err := client.AddURI([]string{dlink}, options)
	if err != nil {
		return fmt.Errorf("添加下载任务失败: %v", err)
	}

	fmt.Printf("下载任务已提交，GID: %s, 文件: %s\n", gid.String(), filename)

	return w.monitorDownload(client, gid.String(), filename)
}

func (w *DownloadWorker) appendAccessToken(dlink string) string {
	if strings.Contains(dlink, "?") {
		return dlink + "&access_token=" + w.Token
	}
	return dlink + "?access_token=" + w.Token
}

func (w *DownloadWorker) monitorDownload(client *arigo.Client, gid, filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("下载超时 (%v)", w.Timeout)
		case <-ticker.C:
			status, err := client.TellStatus(gid, "status", "totalLength", "completedLength", "errorCode", "errorMessage")
			if err != nil {
				return fmt.Errorf("获取下载状态失败: %v", err)
			}

			switch status.Status {
			case "complete":
				fmt.Printf("下载完成: %s\n", filename)
				return nil
			case "error":
				return fmt.Errorf("下载失败: %s - %s", status.ErrorCode, status.ErrorMessage)
			case "active":
				if status.TotalLength > 0 {
					progress := float64(status.CompletedLength) / float64(status.TotalLength) * 100
					fmt.Printf("下载进度: %s - %.1f%%\n", filename, progress)
				}
			case "waiting":
			case "paused":
				return fmt.Errorf("下载已暂停")
			case "removed":
				return fmt.Errorf("下载任务已被移除")
			}
		}
	}
}

func (w *DownloadWorker) VerifyDownload(localPath string, expectedSize int64) error {
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("文件不存在: %v", err)
	}

	if expectedSize == 0 {
		return nil
	}

	if fileInfo.Size() != expectedSize {
		return fmt.Errorf("文件大小不匹配: 期望 %d, 实际 %d", expectedSize, fileInfo.Size())
	}

	return nil
}

func (w *DownloadWorker) BatchGetDownloadLinks(fids []int64) (map[int64]string, error) {
	if len(fids) == 0 {
		return make(map[int64]string), nil
	}

	baseURL := "https://pan.baidu.com/rest/2.0/xpan/multimedia"

	fidsStr := "["
	for i, fid := range fids {
		if i > 0 {
			fidsStr += ","
		}
		fidsStr += strconv.FormatInt(fid, 10)
	}
	fidsStr += "]"

	params := url.Values{}
	params.Set("method", "filemetas")
	params.Set("fsids", fidsStr)
	params.Set("dlink", "1")
	params.Set("access_token", w.Token)

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := w.client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result struct {
		Errno  int    `json:"errno"`
		Errmsg string `json:"errmsg"`
		List   []struct {
			FID   int64  `json:"fs_id"`
			Dlink string `json:"dlink"`
		} `json:"list"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.Errno != 0 {
		return nil, fmt.Errorf("API错误: %d - %s", result.Errno, result.Errmsg)
	}

	dlinks := make(map[int64]string)
	for _, item := range result.List {
		if item.Dlink != "" {
			dlinks[item.FID] = item.Dlink
		}
	}

	return dlinks, nil
}

func (w *DownloadWorker) CheckAria2Connection() error {
	client, err := arigo.Dial(w.Aria2RPC, w.Aria2Token)
	if err != nil {
		return fmt.Errorf("连接aria2c RPC失败: %v\n请确保aria2c已启动且RPC地址正确: %s", err, w.Aria2RPC)
	}
	defer client.Close()

	_, err = client.GetVersion()
	if err != nil {
		return fmt.Errorf("aria2c RPC连接验证失败: %v", err)
	}

	return nil
}