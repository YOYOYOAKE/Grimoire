# Grimoire

Grimoire 是一个 Telegram 绘图机器人：

- 接收 Telegram 文本消息
- 调用 OpenAI 兼容接口翻译提示词
- 调用官方 NovelAI 图像 API 同步生成图片
- 把图片发送回 Telegram
- 通过 `/img` 维护绘图偏好

## 配置文件

首次启动时，如果不存在 `config/config.yaml`，程序会自动生成模板配置并退出。

编辑生成的 `config/config.yaml`，填入 `telegram`、`llms`、`nai` 三组配置。

`llms` 是按顺序回退的 OpenAI 兼容 API 模型列表，当连续请求失败 3 次后会切换到下一个 API。

## 部署

### 从源码安装并部署

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
./bin/grimoire-bot
```

第一次启动时，如果 `./bin/config/config.yaml` 不存在，程序会自动生成模板配置并退出。

或显式指定配置文件：

```bash
./bin/grimoire-bot /path/to/config.yaml
```

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
mkdir -p config
docker run --rm \
  -v "$(pwd)/config:/opt/grimoire/config" \
  ghcr.io/yoyoyoake/grimoire:latest
```

填写配置文件 `./config/config.yaml` 后，再启动常驻容器：

```bash
docker run -d \
  --name grimoire-bot \
  --restart unless-stopped \
  -v "$(pwd)/config:/opt/grimoire/config" \
  ghcr.io/yoyoyoake/grimoire:latest
```

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
```
