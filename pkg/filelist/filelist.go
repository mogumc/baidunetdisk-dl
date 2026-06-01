package filelist

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type FileItem struct {
	FID      int64  `json:"fs_id"`
	Path     string `json:"path"`
	IsDir    int    `json:"isdir"`
	Size     int64  `json:"size"`
	MD5      string `json:"md5"`
	Filename string `json:"server_filename"`
}

func (f *FileItem) IsDirectory() bool {
	return f.IsDir == 1
}

type FileCache struct {
	Files      []FileItem `json:"files"`
	FetchedAt  int64      `json:"fetched_at"`
	TotalCount int        `json:"total_count"`
}

type FileListManager struct {
	Token     string
	RootPath  string
	CachePath string
	client    *http.Client
	cached    *FileCache
	mu        sync.RWMutex
}

func NewFileListManager(token, rootPath, cachePath string) *FileListManager {
	return &FileListManager{
		Token:     token,
		RootPath:  rootPath,
		CachePath: cachePath,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *FileListManager) FetchAll() (*FileCache, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Println("开始获取文件列表...")

	var allFiles []FileItem
	cursor := 0
	page := 1

	for {
		fmt.Printf("获取第 %d 页文件列表...\n", page)

		files, nextCursor, hasMore, err := m.fetchPage(cursor)
		if err != nil {
			return nil, fmt.Errorf("获取文件列表失败: %v", err)
		}

		allFiles = append(allFiles, files...)
		fmt.Printf("获取到 %d 个文件，累计 %d 个\n", len(files), len(allFiles))

		if !hasMore {
			break
		}

		cursor = nextCursor
		page++

		time.Sleep(8 * time.Second)
	}

	cache := &FileCache{
		Files:      allFiles,
		FetchedAt:  time.Now().Unix(),
		TotalCount: len(allFiles),
	}

	if err := m.SaveCache(cache); err != nil {
		fmt.Printf("保存缓存失败: %v\n", err)
	}

	m.cached = cache

	fmt.Printf("文件列表获取完成，共 %d 个文件\n", len(allFiles))
	return cache, nil
}

func (m *FileListManager) fetchPage(cursor int) ([]FileItem, int, bool, error) {
	baseURL := "https://pan.baidu.com/rest/2.0/xpan/multimedia"
	params := url.Values{}
	params.Set("method", "listall")
	params.Set("path", m.RootPath)
	params.Set("recursion", "1")
	params.Set("access_token", m.Token)
	params.Set("start", strconv.Itoa(cursor))
	params.Set("limit", "1000")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := m.client.Get(reqURL)
	if err != nil {
		return nil, 0, false, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, false, fmt.Errorf("读取响应失败: %v", err)
	}

	var result struct {
		Errno   int        `json:"errno"`
		Errmsg  string     `json:"errmsg"`
		List    []FileItem `json:"list"`
		HasMore int        `json:"has_more"`
		Cursor  int        `json:"cursor"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, false, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.Errno != 0 {
		return nil, 0, false, fmt.Errorf("API错误: %d - %s", result.Errno, result.Errmsg)
	}

	hasMore := result.HasMore == 1
	return result.List, result.Cursor, hasMore, nil
}

func (m *FileListManager) LoadCache() (*FileCache, error) {
	m.mu.RLock()
	if m.cached != nil {
		defer m.mu.RUnlock()
		return m.cached, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cached != nil {
		return m.cached, nil
	}

	if _, err := os.Stat(m.CachePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("缓存文件不存在: %s", m.CachePath)
	}

	data, err := os.ReadFile(m.CachePath)
	if err != nil {
		return nil, fmt.Errorf("读取缓存文件失败: %v", err)
	}

	var cache FileCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("解析缓存文件失败: %v", err)
	}

	m.cached = &cache
	return &cache, nil
}

func (m *FileListManager) SaveCache(cache *FileCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化缓存失败: %v", err)
	}

	if err := os.WriteFile(m.CachePath, data, 0644); err != nil {
		return fmt.Errorf("写入缓存文件失败: %v", err)
	}

	fmt.Printf("缓存已保存到: %s\n", m.CachePath)
	return nil
}

func (m *FileListManager) GetFile(fid int64, path string) (*FileItem, error) {
	if m.cached == nil {
		return nil, fmt.Errorf("缓存未加载")
	}

	for _, file := range m.cached.Files {
		if fid > 0 && file.FID == fid {
			return &file, nil
		}
		if path != "" && file.Path == path {
			return &file, nil
		}
	}

	if path != "" {
		return nil, fmt.Errorf("未找到文件: %s", path)
	}
	return nil, fmt.Errorf("未找到文件: fid=%d", fid)
}

func (m *FileListManager) GetFilesByDirectory(dirPath string) ([]FileItem, error) {
	if m.cached == nil {
		return nil, fmt.Errorf("缓存未加载")
	}

	normalizedDir := strings.TrimSuffix(dirPath, "/") + "/"
	var files []FileItem
	for _, file := range m.cached.Files {
		if strings.HasPrefix(file.Path, normalizedDir) {
			files = append(files, file)
		}
	}

	return files, nil
}

func (m *FileListManager) GetFilesByExtension(ext string) ([]FileItem, error) {
	if m.cached == nil {
		return nil, fmt.Errorf("缓存未加载")
	}

	var files []FileItem
	for _, file := range m.cached.Files {
		if !file.IsDirectory() && strings.HasSuffix(file.Filename, ext) {
			files = append(files, file)
		}
	}

	return files, nil
}

func (m *FileListManager) GetDirectoryTree() (map[string][]FileItem, error) {
	if m.cached == nil {
		return nil, fmt.Errorf("缓存未加载")
	}

	tree := make(map[string][]FileItem)
	for _, file := range m.cached.Files {
		parentDir := getParentDir(file.Path)
		tree[parentDir] = append(tree[parentDir], file)
	}

	return tree, nil
}

func getParentDir(path string) string {
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i+1]
		}
	}

	return "/"
}

func (m *FileListManager) GetStats() (totalFiles, totalDirs, totalSize int64, err error) {
	if m.cached == nil {
		if _, loadErr := m.LoadCache(); loadErr != nil {
			return 0, 0, 0, loadErr
		}
	}

	for _, file := range m.cached.Files {
		if file.IsDirectory() {
			totalDirs++
		} else {
			totalFiles++
			totalSize += file.Size
		}
	}

	return
}
