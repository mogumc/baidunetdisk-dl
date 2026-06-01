package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type DownloadStatus int

const (
	StatusPending     DownloadStatus = iota
	StatusDownloading
	StatusCompleted
	StatusFailed
)

type FileDownloadState struct {
	FID        int64          `json:"fs_id"`
	Path       string         `json:"path"`
	Status     DownloadStatus `json:"status"`
	LocalPath  string         `json:"local_path"`
	FileSize   int64          `json:"file_size"`
	Downloaded int64          `json:"downloaded"`
	LastUpdate int64          `json:"last_update"`
	RetryCount int            `json:"retry_count"`
	ErrorMsg   string         `json:"error_msg"`
	IsVerified bool           `json:"is_verified"`
}

type DownloadState struct {
	States     map[int64]*FileDownloadState `json:"states"`
	UpdatedAt  int64                        `json:"updated_at"`
	TotalFiles int                          `json:"total_files"`
}

type StateManager struct {
	StatePath string
	state     *DownloadState
	mu        sync.RWMutex
}

func NewStateManager(statePath string) *StateManager {
	return &StateManager{
		StatePath: statePath,
	}
}

func (s *StateManager) LoadState() (*DownloadState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.StatePath); os.IsNotExist(err) {
		s.state = &DownloadState{
			States:     make(map[int64]*FileDownloadState),
			UpdatedAt:  time.Now().Unix(),
			TotalFiles: 0,
		}
		return s.state, nil
	}

	data, err := os.ReadFile(s.StatePath)
	if err != nil {
		return nil, fmt.Errorf("读取状态文件失败: %v", err)
	}

	var state DownloadState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析状态文件失败: %v", err)
	}

	s.state = &state
	fmt.Printf("从状态文件加载了 %d 个文件状态\n", len(state.States))
	return s.state, nil
}

func (s *StateManager) SaveState() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveStateLocked()
}

// saveStateLocked 在已持有写锁时调用，避免 Reset() 等场景死锁
func (s *StateManager) saveStateLocked() error {
	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	s.state.UpdatedAt = time.Now().Unix()

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态失败: %v", err)
	}

	if err := os.WriteFile(s.StatePath, data, 0644); err != nil {
		return fmt.Errorf("写入状态文件失败: %v", err)
	}

	return nil
}

func (s *StateManager) GetPendingFiles() []*FileDownloadState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return nil
	}

	var pending []*FileDownloadState
	for _, state := range s.state.States {
		if state.Status == StatusPending || state.Status == StatusFailed {
			pending = append(pending, state)
		}
	}

	return pending
}

func (s *StateManager) GetPendingFilesCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return 0
	}

	count := 0
	for _, state := range s.state.States {
		if state.Status == StatusPending || state.Status == StatusFailed {
			count++
		}
	}

	return count
}

func (s *StateManager) MarkDownloading(fid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	state, exists := s.state.States[fid]
	if !exists {
		return fmt.Errorf("未找到文件状态: %d", fid)
	}

	state.Status = StatusDownloading
	state.LastUpdate = time.Now().Unix()

	return nil
}

func (s *StateManager) MarkCompleted(fid int64, localPath string, isVerified bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	state, exists := s.state.States[fid]
	if !exists {
		return fmt.Errorf("未找到文件状态: %d", fid)
	}

	state.Status = StatusCompleted
	state.LocalPath = localPath
	state.IsVerified = isVerified
	state.LastUpdate = time.Now().Unix()

	return nil
}

func (s *StateManager) MarkFailed(fid int64, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	state, exists := s.state.States[fid]
	if !exists {
		return fmt.Errorf("未找到文件状态: %d", fid)
	}

	state.Status = StatusFailed
	state.ErrorMsg = errMsg
	state.RetryCount++
	state.LastUpdate = time.Now().Unix()

	return nil
}

func (s *StateManager) UpdateProgress(fid int64, downloaded int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	state, exists := s.state.States[fid]
	if !exists {
		return fmt.Errorf("未找到文件状态: %d", fid)
	}

	state.Downloaded = downloaded
	state.LastUpdate = time.Now().Unix()

	return nil
}

func (s *StateManager) GetProgress() (completed, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return 0, 0
	}

	for _, state := range s.state.States {
		if state.Status == StatusCompleted {
			completed++
		}
	}

	return completed, len(s.state.States)
}

func (s *StateManager) AddFile(fid int64, path string, fileSize int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	if _, exists := s.state.States[fid]; exists {
		return nil
	}

	s.state.States[fid] = &FileDownloadState{
		FID:        fid,
		Path:       path,
		Status:     StatusPending,
		FileSize:   fileSize,
		LastUpdate: time.Now().Unix(),
	}
	s.state.TotalFiles++

	return nil
}

func (s *StateManager) ResetDownloadingStates() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("状态未初始化")
	}

	timeout := time.Now().Unix() - 300

	for _, state := range s.state.States {
		if state.Status == StatusDownloading && state.LastUpdate < timeout {
			state.Status = StatusPending
			state.LastUpdate = time.Now().Unix()
		}
	}

	return nil
}

func (s *StateManager) GetFileState(fid int64) (*FileDownloadState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return nil, fmt.Errorf("状态未初始化")
	}

	state, exists := s.state.States[fid]
	if !exists {
		return nil, fmt.Errorf("未找到文件状态: %d", fid)
	}

	return state, nil
}

func (s *StateManager) IsCompleted(fid int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return false
	}

	state, exists := s.state.States[fid]
	if !exists {
		return false
	}

	return state.Status == StatusCompleted
}

func (s *StateManager) GetStats() (pending, downloading, completed, failed int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return 0, 0, 0, 0
	}

	for _, state := range s.state.States {
		switch state.Status {
		case StatusPending:
			pending++
		case StatusDownloading:
			downloading++
		case StatusCompleted:
			completed++
		case StatusFailed:
			failed++
		}
	}

	return
}

func (s *StateManager) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = &DownloadState{
		States:     make(map[int64]*FileDownloadState),
		UpdatedAt:  time.Now().Unix(),
		TotalFiles: 0,
	}

	return s.saveStateLocked()
}