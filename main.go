package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mogumc/baidunetdisk-dl/pkg/coordinator"
	"github.com/mogumc/baidunetdisk-dl/pkg/downloader"
	"github.com/mogumc/baidunetdisk-dl/pkg/filelist"
	"github.com/mogumc/baidunetdisk-dl/pkg/state"
)

func main() {
	token := flag.String("token", "", "百度网盘access_token")
	output := flag.String("output", "./downloads", "本地输出目录")
	aria2Port := flag.Int("aria2-port", 6800, "aria2c RPC端口")
	aria2Token := flag.String("aria2-token", "", "aria2c授权令牌")
	concurrent := flag.Int("concurrent", 3, "最大并发下载数")
	rootPath := flag.String("root-path", "/", "网盘根路径")
	cachePath := flag.String("cache-path", "./filelist.json", "文件列表缓存路径")
	statePath := flag.String("state-path", "./download_state.json", "下载状态文件路径")
	retry := flag.Int("retry", 3, "下载失败重试次数")
	timeout := flag.Duration("timeout", 30*time.Minute, "单个文件下载超时时间")

	flag.Parse()

	aria2RPC := fmt.Sprintf("ws://localhost:%d/jsonrpc", *aria2Port)

	if *token == "" {
		fmt.Println("错误: 必须提供-token参数")
		flag.Usage()
		os.Exit(1)
	}

	if err := os.MkdirAll(*output, 0755); err != nil {
		fmt.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("百度网盘自动下载工具启动")
	fmt.Printf("输出目录: %s\n", *output)
	fmt.Printf("并发数: %d\n", *concurrent)
	fmt.Printf("网盘根路径: %s\n", *rootPath)

	fmt.Println("\n=== 初始化文件列表管理器 ===")
	fileListMgr := filelist.NewFileListManager(*token, *rootPath, *cachePath)

	var fileCache *filelist.FileCache
	var err error

	fileCache, err = fileListMgr.LoadCache()
	if err != nil {
		fmt.Println("缓存不存在或加载失败，开始获取文件列表...")
		fileCache, err = fileListMgr.FetchAll()
		if err != nil {
			fmt.Printf("获取文件列表失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("从缓存加载了 %d 个文件\n", fileCache.TotalCount)
	}

	totalFiles, totalDirs, totalSize, err := fileListMgr.GetStats()
	if err != nil {
		fmt.Printf("获取统计信息失败: %v\n", err)
	} else {
		fmt.Printf("文件统计: %d 个文件, %d 个目录, 总大小: %.2f MB\n",
			totalFiles, totalDirs, float64(totalSize)/1024/1024)
	}

	fmt.Println("\n=== 初始化下载状态管理器 ===")
	stateMgr := state.NewStateManager(*statePath)

	_, err = stateMgr.LoadState()
	if err != nil {
		fmt.Printf("加载状态文件失败: %v\n", err)
	}

	if err := stateMgr.ResetDownloadingStates(); err != nil {
		fmt.Printf("重置下载状态失败: %v\n", err)
	}

	for _, file := range fileCache.Files {
		if !file.IsDirectory() {
			if err := stateMgr.AddFile(file.FID, file.Path, file.Size); err != nil {
				fmt.Printf("添加文件到状态管理器失败: %v\n", err)
			}
		}
	}

	pending, downloading, completed, failed := stateMgr.GetStats()
	fmt.Printf("状态统计: 待下载 %d, 下载中 %d, 已完成 %d, 失败 %d\n",
		pending, downloading, completed, failed)

	fmt.Println("\n=== 初始化下载执行器 ===")
	workers := make([]*downloader.DownloadWorker, *concurrent)
	for i := 0; i < *concurrent; i++ {
		workers[i] = downloader.NewDownloadWorker(*token, aria2RPC, *aria2Token, *output, *retry, *timeout)
	}

	if err := workers[0].CheckAria2Connection(); err != nil {
		fmt.Printf("aria2c连接检查失败: %v\n", err)
		fmt.Printf("请确保aria2c已安装并启动，且RPC端口为 %d\n", *aria2Port)
		fmt.Printf("启动命令: aria2c --enable-rpc --rpc-listen-port=%d --rpc-allow-origin-all\n", *aria2Port)
		os.Exit(1)
	}

	fmt.Println("\n=== 初始化下载协调器 ===")
	coord := coordinator.NewDownloadCoordinator(*concurrent, workers, stateMgr)

	fmt.Println("\n=== 准备下载任务 ===")
	pendingFiles := stateMgr.GetPendingFiles()
	fmt.Printf("待下载文件数: %d\n", len(pendingFiles))

	tasks := make([]*downloader.DownloadTask, 0, len(pendingFiles))
	for _, fileState := range pendingFiles {
		task := &downloader.DownloadTask{
			FID:       fileState.FID,
			Path:      fileState.Path,
			LocalPath: filepath.Join(*output, fileState.Path),
			FileSize:  fileState.FileSize,
			Filename:  filepath.Base(fileState.Path),
		}
		tasks = append(tasks, task)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n=== 开始下载 ===")
	fmt.Println("按 Ctrl+C 可以中断下载，下次启动将继续下载")

	downloadDone := make(chan error, 1)
	go func() {
		downloadDone <- coord.Start(tasks)
	}()

	select {
	case err := <-downloadDone:
		if err != nil {
			fmt.Printf("下载过程出错: %v\n", err)
		}
		fmt.Println("\n=== 下载完成 ===")
	case <-sigChan:
		fmt.Println("\n收到中断信号，正在保存状态...")
		if err := stateMgr.SaveState(); err != nil {
			fmt.Printf("保存状态失败: %v\n", err)
		} else {
			fmt.Println("状态已保存，下次启动将继续下载")
		}
	}

	completed, total, pending, downloading, failed := coord.GetStats()
	fmt.Printf("\n最终统计:\n")
	fmt.Printf("  总文件数: %d\n", total)
	fmt.Printf("  已完成: %d\n", completed)
	fmt.Printf("  下载中: %d\n", downloading)
	fmt.Printf("  待下载: %d\n", pending)
	fmt.Printf("  失败: %d\n", failed)

	os.Exit(0)
}