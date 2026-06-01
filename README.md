# 百度网盘自动下载工具

一个使用Go语言编写的百度网盘自动下载工具，支持批量下载所有文件到本地，保持文件夹结构，使用aria2c作为下载引擎。

## 功能特性

- ✅ **批量获取文件列表**：递归获取百度网盘所有文件，缓存到本地
- ✅ **并发下载**：支持多并发下载（默认3个并发）
- ✅ **断点续传**：下载状态持久化，中断后可继续下载
- ✅ **保持目录结构**：按照网盘文件夹结构下载到本地
- ✅ **简单直接**：通过启动参数注入access_token，无需复杂OAuth流程

## 快速开始

### 1. 安装aria2c

#### Windows

```bash
# 使用winget
winget install aria2.aria2

# 或下载预编译版本：https://github.com/aria2/aria2/releases
```

#### macOS

```bash
brew install aria2
```

#### Linux

```bash
# Ubuntu/Debian
sudo apt install aria2

# CentOS/RHEL
sudo yum install aria2
```

### 2. 启动aria2c服务

```bash
aria2c --enable-rpc --rpc-listen-port=6800 --rpc-allow-origin-all
```

如果需要修改端口，使用 `-aria2-port`参数指定：

### 3. 编译或下载程序

#### 从源码编译

```bash
git clone https://github.com/mogumc/baidunetdisk-dl.git
cd baidunetdisk-dl
go build -o baidunetdisk-dl .
```

#### 直接下载

从 [Releases](https://github.com/mogumc/baidunetdisk-dl/releases) 下载预编译版本。

### 4. 运行程序

```bash
./baidunetdisk-dl -token "your_access_token"
```

## 使用方法

### 基本用法

```bash
./baidunetdisk-dl -token "your_access_token"
```

### 完整参数

```bash
./baidunetdisk-dl \
  -token "your_access_token" \
  -output "./downloads" \
  -aria2-port 6800 \
  -concurrent 3 \
  -root-path "/" \
  -retry 3
```

### 参数说明

| 参数             | 默认值                    | 说明                 |
| ---------------- | ------------------------- | -------------------- |
| `-token`       | 必需                      | 百度网盘access_token |
| `-output`      | `./downloads`           | 本地输出目录         |
| `-aria2-port`  | `6800`                  | aria2c RPC端口       |
| `-aria2-token` | 空                        | aria2c授权令牌       |
| `-concurrent`  | `3`                     | 最大并发下载数       |
| `-root-path`   | `/`                     | 网盘根路径           |
| `-cache-path`  | `./filelist.json`       | 文件列表缓存路径     |
| `-state-path`  | `./download_state.json` | 下载状态文件路径     |
| `-retry`       | `3`                     | 下载失败重试次数     |

## 获取access_token

### 方式一：使用百度网盘开放平台（推荐）

1. 访问 [百度网盘开放平台](https://pan.baidu.com/union)
2. 注册并创建应用
3. 使用OAuth 2.0授权码模式获取access_token

### 方式二：使用第三方工具

可以使用 [https://api.oplist.org/](https://api.oplist.org/) 等工具获取access_token。

## 使用示例

### 示例1：下载所有文件到默认目录

```bash
./baidunetdisk-dl -token "your_access_token"
```

### 示例2：下载到指定目录，5个并发

```bash
./baidunetdisk-dl \
  -token "your_access_token" \
  -output "D:\百度网盘备份" \
  -concurrent 5
```

### 示例3：只下载特定目录

```bash
./baidunetdisk-dl \
  -token "your_access_token" \
  -root-path "/我的文档"
```

## 中断与续传

### 中断下载

按 `Ctrl+C` 可以中断下载，程序会自动保存当前状态。

### 继续下载

再次运行程序即可自动继续下载，已完成的文件会被跳过。

### 查看进度

程序运行时会显示实时进度：

```
进度: 150/1000 (完成: 150, 下载中: 3, 待下载: 847, 失败: 0)
```

## 错误处理

### 常见错误

1. **Invalid Bduss**：access_token无效或过期，请重新获取
2. **aria2c连接失败**：请确保aria2c已启动
3. **磁盘空间不足**：请检查输出目录的磁盘空间
4. **网络错误**：程序会自动重试3次

### 日志文件

程序会输出详细的日志信息，包括：

- 文件列表获取进度
- 下载任务分配
- 下载进度
- 错误信息

## 技术架构

```
├── main.go                    # 主程序入口
├── pkg/
│   ├── filelist/              # 文件列表管理
│   │   └── filelist.go
│   ├── state/                 # 下载状态管理
│   │   └── state.go
│   ├── downloader/            # 下载执行器
│   │   └── worker.go
│   ├── coordinator/           # 下载协调器
│   │   └── coordinator.go
│   └── utils/                 # 工具函数
│       └── utils.go
├── .github/workflows/         # GitHub Actions CI/CD
│   └── build.yml
└── go.mod
```

## CI/CD

本项目使用GitHub Actions进行自动编译，支持以下平台：

| 平台 | 架构 | 文件名 |
|------|------|--------|
| Linux | amd64 | baidunetdisk-dl-linux-amd64 |
| Linux | arm64 | baidunetdisk-dl-linux-arm64 |
| Windows | amd64 | baidunetdisk-dl-windows-amd64.exe |
| macOS | amd64 | baidunetdisk-dl-darwin-amd64 |
| macOS | arm64 | baidunetdisk-dl-darwin-arm64 |

### 使用预编译版本

1. 访问 [Releases](https://github.com/mogumc/baidunetdisk-dl/releases)
2. 下载对应平台的二进制文件
3. 直接运行即可

## 依赖

- **Go 1.21+**
- **aria2c**：下载引擎
- **百度网盘开放API**：获取文件列表和下载链接

## 许可证

MIT License

## 免责声明

本工具仅供学习交流使用，请遵守百度网盘使用条款和相关法律法规。
