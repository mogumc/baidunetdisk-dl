package coordinator

import (
	"fmt"
	"sync"
	"time"

	"github.com/mogumc/baidunetdisk-dl/pkg/downloader"
	"github.com/mogumc/baidunetdisk-dl/pkg/state"
)

type DownloadCoordinator struct {
	MaxConcurrent int
	Workers       []*downloader.DownloadWorker
	TaskQueue     chan *downloader.DownloadTask
	StateManager  *state.StateManager

	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func NewDownloadCoordinator(maxConcurrent int, workers []*downloader.DownloadWorker, stateManager *state.StateManager) *DownloadCoordinator {
	return &DownloadCoordinator{
		MaxConcurrent: maxConcurrent,
		Workers:       workers,
		TaskQueue:     make(chan *downloader.DownloadTask, 100),
		StateManager:  stateManager,
	}
}

func (c *DownloadCoordinator) Start(tasks []*downloader.DownloadTask) error {
	fmt.Printf("启动下载协调器，并发数: %d\n", c.MaxConcurrent)

	for i := 0; i < c.MaxConcurrent; i++ {
		c.wg.Add(1)
		go c.workerLoop(c.Workers[i], i)
	}

	for _, task := range tasks {
		c.TaskQueue <- task
	}

	close(c.TaskQueue)
	c.wg.Wait()
	return nil
}

func (c *DownloadCoordinator) workerLoop(worker *downloader.DownloadWorker, workerID int) {
	defer c.wg.Done()

	for task := range c.TaskQueue {
		err := c.executeTask(worker, task, workerID)

		if err != nil {
			fmt.Printf("[Worker %d] 下载失败: %s - %v\n", workerID, task.Path, err)
			c.StateManager.MarkFailed(task.FID, err.Error())
		} else {
			fmt.Printf("[Worker %d] 下载完成: %s\n", workerID, task.Path)
			c.StateManager.MarkCompleted(task.FID, task.LocalPath, true)
		}

		if err := c.StateManager.SaveState(); err != nil {
			fmt.Printf("[Worker %d] 保存状态失败: %v\n", workerID, err)
		}

		c.showProgress()
	}
}

func (c *DownloadCoordinator) executeTask(worker *downloader.DownloadWorker, task *downloader.DownloadTask, workerID int) error {
	if err := c.StateManager.MarkDownloading(task.FID); err != nil {
		return fmt.Errorf("标记下载中失败: %v", err)
	}

	var lastErr error
	for retry := 0; retry <= worker.MaxRetry; retry++ {
		if retry > 0 {
			fmt.Printf("[Worker %d] 第 %d 次重试: %s\n", workerID, retry, task.Path)
			time.Sleep(time.Duration(retry) * time.Second)
		}

		lastErr = worker.ExecuteDownload(task)
		if lastErr == nil {
			return nil
		}

		fmt.Printf("[Worker %d] 下载失败: %v\n", workerID, lastErr)
	}

	return fmt.Errorf("重试 %d 次后仍失败: %v", worker.MaxRetry, lastErr)
}

func (c *DownloadCoordinator) showProgress() {
	completed, total := c.StateManager.GetProgress()
	pending, downloading, _, failed := c.StateManager.GetStats()

	fmt.Printf("进度: %d/%d (下载中: %d, 待下载: %d, 失败: %d)\n",
		completed, total, downloading, pending, failed)
}

func (c *DownloadCoordinator) AddTask(task *downloader.DownloadTask) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.TaskQueue <- task
}

func (c *DownloadCoordinator) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	close(c.TaskQueue)
	c.wg.Wait()
}

func (c *DownloadCoordinator) GetStats() (completed, total, pending, downloading, failed int) {
	completed, total = c.StateManager.GetProgress()
	pending, downloading, _, failed = c.StateManager.GetStats()
	return
}