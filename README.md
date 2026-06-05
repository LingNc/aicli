# aicli

用自然语言驱动的命令行工具。告诉 AI 你想做什么，它生成命令并执行。

## 快速开始

### 安装

**下载预构建：**

从 [Releases](https://github.com/LingNc/aicli/releases) 页面下载对应平台的二进制文件：

```bash
# 下载到本地
curl -L https://github.com/LingNc/aicli/releases/latest/download/aicli -o aicli
chmod +x aicli
# 选择安装位置并配置 shell 集成
./aicli shell install
```

**从源码编译：**

```bash
git clone https://github.com/LingNc/aicli.git
cd aicli
make build
# 选择安装位置并配置 shell 集成
./aicli shell install
```

### 配置

首次运行会自动引导配置，也可手动执行：

```bash
ai setup
```

配置文件位于 `~/.config/aicli/config.yaml`，支持 OpenAI 兼容 API。

## 使用

```bash
ai 查看内存使用情况
ai 列出占用8080端口的进程
ai 找到当前目录下大于100MB的文件
ai 清理所有 .tmp 文件
```

AI 会生成命令、分类识别（只读/读写/删除），危险操作会要求确认后执行。

## 帮助

`ai/aicli --help/-h`

## 配置说明

详细参见 [配置文档](internal/config/default.yaml)
配置文件路径 `~/.config/aicli/config.yaml`
模型配置推荐使用DeepSeek速度非常快

## Shell 集成

安装后可使用ai代替aicli

```bash
ai shell install    # 安装后执行的命令自动加入 shell 历史
```

## 安全机制

- **命令分类** — 自动识别只读/读写/删除/sudo 等操作
- **规则引擎** — 内置禁止规则（`rm -rf /`、`dd`、`mkfs` 等）
- **用户确认** — 危险命令执行前必须确认
- **白名单** — 可配置直接执行的命令

## 许可证

MIT License - 详见 [LICENSE](LICENSE)

© LingNc
