# Grimoire

Grimoire 是一个 Telegram 绘图机器人，当前采用“会话 -> request -> 确认 -> 绘图”的交互流程：

- 接收 Telegram 文本消息，先进入会话式需求对话
- 基于最近会话和全局偏好生成待确认 request
- 用户确认后才创建绘图任务，并把任务状态持久化到 SQLite
- 调用 OpenAI 兼容接口翻译提示词
- 调用官方 NovelAI 图像 API 同步生成图片
- 把图片保存到本地目录后再发送回 Telegram
- 支持查看 prompt、重新翻译并绘图、不翻译并重新绘图
- 支持任务停止与进程重启后的任务恢复
- 通过 `/img` 维护全局绘图偏好，通过 `/balance` 查询 NAI 余额

## 配置文件

不传参数启动时，程序会默认在“可执行文件同级目录”的 `config/config.yaml` 查找配置文件。

如果默认配置文件不存在，程序会自动生成模板并退出：

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
./bin/grimoire-bot
```

首次启动后会生成 `./bin/config/config.yaml`，填写完配置再重新启动。

也可以显式指定配置文件：

```bash
./bin/grimoire-bot /path/to/config.yaml
```

配置项说明：

- `telegram`
  - `bot_token`: Telegram Bot Token
  - `admin_user_id`: 管理员 Telegram ID，启动成功后会向该用户发送启动通知
  - `proxy`: Telegram 请求代理，可留空
  - `timeout_sec`: Telegram 请求超时，默认 `60`
- `storage`
  - `data_dir`: 数据根目录，可留空
  - `sqlite_path`: SQLite 文件路径，可留空
  - `image_dir`: 图片目录，可留空
  - 三者都留空时，默认解析为：
    - 数据目录：`<config 根目录>/data`
    - 数据库：`<config 根目录>/data/grimoire.db`
    - 图片目录：`<config 根目录>/data/images`
- `conversation`
  - `recent_message_limit`: 生成 request 时回看最近消息条数，默认 `15`
- `recovery`
  - `enabled`: 是否在启动时重新入队 `queued / translating / drawing` 任务，默认 `true`
- `llms`
  - OpenAI 兼容模型列表，会按顺序做 failover
  - 当前同时承担会话回复、request 生成、prompt 翻译三类能力
- `nai`
  - NovelAI 图像 API 配置
  - 当前支持余额查询和同步出图

## 交互流程

### 会话与 request

1. 用户直接发送自然语言需求。
2. Bot 先进入会话式补充对话，而不是立即绘图。
3. Bot 基于当前会话和全局偏好生成“待确认 request”。
4. 用户点击“确认执行”后才会创建任务；点击“继续修改”则回到会话补充。

### 偏好、停止与重试

- `/img` 可设置全局尺寸和画师串，这些偏好会写入 SQLite。
- 任务进行中会显示进度消息，并提供“停止任务”按钮。
- 结果图会提供三个按钮：
  - `查看 prompt`
  - `重新翻译并绘图`
  - `不翻译并重新绘图`

### 恢复机制

- 任务状态存储在 SQLite 中。
- 进程重启时，如果 `recovery.enabled=true`，系统会自动扫描 `queued / translating / drawing` 任务并重新入队。
- 启动日志会输出数据库路径、图片目录、恢复开关和最近消息裁剪参数。

## 部署

### 从源码安装并部署

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
./bin/grimoire-bot
```

运维建议：

- 持久化 `config/` 和 `data/`
- 至少备份：
  - `data/grimoire.db`
  - `data/images/`
- 如果需要关闭重启恢复，可将 `recovery.enabled` 显式设为 `false`

### 使用 Docker 部署

项目已提供 [Dockerfile](/home/YOAKE/dev/Grimoire/docker/Dockerfile)。

如果需要本地构建镜像，请先编译 Linux 二进制：

```bash
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/grimoire-bot ./cmd/grimoire-bot
docker build -f docker/Dockerfile -t grimoire:local .
```

首次启动时需要生成配置文件：

```bash
mkdir -p config data
docker run --rm \
  -v "$(pwd)/config:/opt/grimoire/config" \
  -v "$(pwd)/data:/opt/grimoire/data" \
  ghcr.io/yoyoyoake/grimoire:latest
```

填写配置文件 `./config/config.yaml` 后，再启动常驻容器：

```bash
mkdir -p config data
docker run -d \
  --name grimoire-bot \
  --restart unless-stopped \
  -v "$(pwd)/config:/opt/grimoire/config" \
  -v "$(pwd)/data:/opt/grimoire/data" \
  ghcr.io/yoyoyoake/grimoire:latest
```

容器内默认配置路径为 `/opt/grimoire/config/config.yaml`，默认数据目录为 `/opt/grimoire/data`。

### 使用 Docker Compose 部署

首次启动时，程序会自动生成模板配置并退出。

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
