# OpenClaw 数据迁移工具

这是一个给最终用户使用的命令行工具，用来把旧版 OpenClaw 暴露到网盘中的数据迁移到新版目录结构。

## 这个工具会做什么

程序会从旧版 OpenClaw 的目录中读取数据，并复制到新版使用的位置：

- `openclaw-data` → `/home/node/.openclaw`
- `openclaw-workspace` → `/home/node/clawd`
- `openclaw-app` → `/app`

如果检测到 `openclaw-data`，程序还会创建软链：

- `~/.openclaw` → `/home/node/.openclaw`

## 什么时候使用

如果你已经把旧版 OpenClaw 的应用数据暴露到网盘，并且现在需要迁移到新版目录，就可以使用这个工具。

默认旧版包名是：

```text
iamxiaoe.lzcapp.openclaw
```

旧版数据默认位于：

```text
/lzcapp/document/AppShareCenter/<包名>/
```

程序会在这个目录下查找这些子目录：

```text
openclaw-data
openclaw-workspace
openclaw-app
```

如果这三个目录都不存在，程序会直接退出并报错。

## 使用前必看

在开始迁移前，请先确认：

1. 你已经把旧版 OpenClaw 的应用数据暴露到网盘。
2. 迁移期间不要继续使用旧版 OpenClaw，否则可能会有新数据没有被迁移。
3. 以下目标目录中的现有内容会先被清空，再复制新内容；如果你需要保留旧内容，请先备份：
   - `/home/node/.openclaw`
   - `/home/node/clawd`
   - `/app`
4. 程序会把目标目录权限修正为 `abc:abc`，请确认当前环境中存在这个用户和用户组。
5. 程序需要写入 `/home/node` 和 `/app`，通常需要足够权限。

## 如何运行

### 方式一：直接运行源码

要求：Go 1.22+

```bash
go run ./cmd/openclaw-data-migration
```

### 方式二：先构建再运行

```bash
go build -o bin/openclaw-data-migration ./cmd/openclaw-data-migration
./bin/openclaw-data-migration
```

如果当前环境没有权限写入 `/home/node`、`/app` 或执行 `chown`，可以使用：

```bash
sudo ./bin/openclaw-data-migration
```

## 运行时会询问什么

启动后，程序会依次询问你：

1. 是否已经把 OpenClaw 应用数据暴露到网盘
2. 包名是否是默认值 `iamxiaoe.lzcapp.openclaw`
3. 是否已经停止使用旧版 OpenClaw

如果包名不是默认值，程序会要求你输入完整包名。

## 目录迁移关系

| 旧目录 | 新目录 |
| --- | --- |
| `/lzcapp/document/AppShareCenter/<包名>/openclaw-data` | `/home/node/.openclaw` |
| `/lzcapp/document/AppShareCenter/<包名>/openclaw-workspace` | `/home/node/clawd` |
| `/lzcapp/document/AppShareCenter/<包名>/openclaw-app` | `/app` |

当 `openclaw-data` 存在时，还会额外创建：

```text
~/.openclaw -> /home/node/.openclaw
```

如果你是通过 `sudo` 执行，程序会优先根据 `SUDO_USER` 把软链创建到原始登录用户的家目录里。

## 运行输出示例

迁移时，程序会输出每一步的开始和完成状态，并显示复制进度，例如：

```text
openclaw-data 复制进度 [========------------] 40% (40.0 MiB/100.0 MiB)
```

## 测试

如果你是开发者，可以运行：

```bash
go test ./...
```
