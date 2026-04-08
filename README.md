# Grimoire

Grimoire 是一个面向 Telegram 的多轮对话式绘图机器人。当前实现已经完成以下主链路：

- 通过多轮对话收敛绘图需求
- 基于会话上下文生成待确认的 request
- 用户确认后进入 `request -> prompt -> drawing` 执行链路
- 使用 SQLite 持久化用户、会话、消息和任务
- 使用本地文件系统持久化生成图片
- 支持任务停止、结果重试和重启恢复
- 支持 `/img` 调整默认尺寸与画师串，支持 `/balance` 查询 NAI 余额

## 交互流程

1. 用户发送普通文本，进入会话式需求收敛流程。
2. Bot 返回自然语言回复，并基于当前会话生成一条“待确认 request”消息。
3. 用户点击“确认执行”后，系统创建任务并进入异步执行；点击“继续修改”则回到会话阶段。
4. 任务执行期间，Bot 会发送一条进度消息，并持续更新“已入队 / 正在翻译提示词 / 正在绘图”等状态；进度消息带“停止任务”按钮。
5. 绘图完成后，Bot 发送结果图片消息，并提供“查看 prompt / 重新翻译并绘图 / 不翻译并重新绘图”按钮；原进度消息会被删除。

## 配置文件

首次启动时，如果默认配置文件不存在，程序会自动生成模板并退出：

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
./bin/grimoire-bot
```

默认配置路径为可执行文件同级的 `config/config.yaml`。也可以显式指定配置文件：

```bash
./bin/grimoire-bot /path/to/config.yaml
```

只有在使用默认启动路径时，缺失配置才会自动生成模板；如果显式传入一个不存在的配置文件路径，程序会直接报错退出。

当前配置项包括：

- `telegram.bot_token`: Telegram Bot Token。
- `telegram.admin_user_id`: 启动时会自动确保该 Telegram ID 对应的用户存在并可访问系统。
- `telegram.proxy`, `telegram.timeout_sec`: Telegram HTTP 代理与超时。
- `storage.data_dir`: 数据根目录，可留空。
- `storage.sqlite_path`: SQLite 文件路径，可留空。
- `storage.image_dir`: 图片目录，可留空。
- `conversation.recent_message_limit`: 每次生成会话回复和 request 时读取的最近消息条数，默认 `15`。
- `recovery.enabled`: 是否在启动时自动恢复 `queued / translating / drawing` 任务，默认开启。
- `llms`: OpenAI 兼容模型列表；`llms[0]` 用于会话回复和 request 生成，整组 `llms` 用于 prompt 翻译 failover，请至少配置一个可用模型。
- `nai`: 官方 NovelAI 图像接口配置；当前 `nai.model` 必须为 `nai-diffusion-4-5-full`。

### 存储路径默认值

如果 `storage` 三个字段都留空，系统会基于配置文件位置推导运行目录：

- 当配置文件位于 `<root>/config/config.yaml` 时：

```text
<root>/data/grimoire.db
<root>/data/images/
```

- 当配置文件不在 `config/` 目录下时，默认数据目录会落在“配置文件所在目录”下。

SQLite 中保存业务事实数据，图片文件保存到本地文件系统；恢复、重试和结果展示都会依赖这两部分数据。

## 部署

### 从源码运行

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
./bin/grimoire-bot
```

首次启动会生成模板配置并退出；填好配置后重新启动即可。

### Docker

如果需要本地构建镜像：

```bash
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/grimoire-bot ./cmd/grimoire-bot
docker build -f docker/Dockerfile -t grimoire:local .
```

首次启动生成配置文件：

```bash
mkdir -p config data
docker run --rm \
  -v "$(pwd)/config:/opt/grimoire/config" \
  -v "$(pwd)/data:/opt/grimoire/data" \
  ghcr.io/yoyoyoake/grimoire:latest
```

填好 `./config/config.yaml` 后，再启动常驻容器：

```bash
docker run -d \
  --name grimoire-bot \
  --restart unless-stopped \
  -v "$(pwd)/config:/opt/grimoire/config" \
  -v "$(pwd)/data:/opt/grimoire/data" \
  ghcr.io/yoyoyoake/grimoire:latest
```

### Docker Compose

```yaml
services:
  grimoire-bot:
    image: ghcr.io/yoyoyoake/grimoire:latest
    container_name: grimoire-bot
    restart: unless-stopped
    volumes:
      - ./config:/opt/grimoire/config
      - ./data:/opt/grimoire/data
```

## 运维说明

### 访问控制

系统当前以 SQLite `users` 表作为访问白名单。启动时只会自动确保 `telegram.admin_user_id` 对应用户存在；如需开放给其他用户，需要额外维护 `users` 表。

### 数据备份

请同时备份以下两部分：

- `data/grimoire.db`
- `data/images/`

只备份其中一部分会导致任务记录与图片文件不一致。

### 恢复行为

当 `recovery.enabled=true` 时，系统启动顺序为：

1. 打开并迁移 SQLite
2. 启动单并发任务 worker
3. 查询 `queued / translating / drawing` 任务
4. 将这些任务重新入队

因此，重启后可以继续推进未终态任务。

### 并发约束

当前 draw worker 固定为单并发。原因是 NAI 侧不接受并发绘图任务；如果同时触发多个任务，它们会在本地队列中串行执行。
