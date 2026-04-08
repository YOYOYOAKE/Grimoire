# Grimoire

Grimoire V3 是一个面向 Telegram 的多轮对话式辅助绘图机器人。

## 功能

- 通过多轮对话确认绘图需求
- 使用 SQLite 持久化用户、会话、消息和任务
- 使用本地文件系统保存生成图片
- 支持任务停止、结果重试和重启恢复
- 支持 `/new` 新建当前会话
- 支持 `/img` 调整默认尺寸与画师串
- 支持 `/balance` 查询 NovelAI 余额

## 部署

### 从源码运行

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
./bin/grimoire-bot
```

首次启动时，如果默认配置不存在，程序会在可执行文件同级生成 `config/config.yaml` 模板并退出。填好配置后重新启动即可。

也可以显式指定配置文件：

```bash
./bin/grimoire-bot /path/to/config.yaml
```

### Docker

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

或使用 Docker Compose 管理容器：

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
