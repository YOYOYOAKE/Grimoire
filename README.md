# Grimoire v2

Grimoire v2 是一个只服务 `telegram.admin_user_id` 的 Telegram 绘图机器人：

- 接收 Telegram 文本消息
- 调用 OpenAI 兼容接口翻译提示词
- 调用 Xianyun NovelAI 中继生成图片
- 直接把图片发送回 Telegram
- 通过 `/img` 维护全局默认尺寸和画师串

运行时不使用 SQLite，不做任务恢复。绘图任务只保存在内存里，进程重启后会清空。`/img` 的全局偏好会保存在可执行文件同目录下的 `runtime.json` 中。

## 构建

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
```

## 配置

```bash
./bin/grimoire-bot
```

首次启动时，如果可执行文件同级目录下不存在 `config.yaml`，程序会自动生成模板配置并退出。

编辑生成的 `config.yaml`，填入 `telegram`、`llms`、`nai` 三组配置。`llms` 是按顺序回退的 OpenAI-compatible 模型列表：前一个连续失败 3 次后会切到下一个。

## 运行

```bash
./bin/grimoire-bot
```

也可以显式指定配置文件：

```bash
./bin/grimoire-bot /path/to/config.yaml
```

## Docker 部署

项目已提供 [Dockerfile](/home/YOAKE/dev/Grimoire/Dockerfile) 和 [docker-compose.yaml](/home/YOAKE/dev/Grimoire/docker-compose.yaml)。

首次启动：

```bash
docker compose up --build
```

容器会把程序复制到 `./data/` 后运行。第一次启动时，如果 `./data/config.yaml` 不存在，程序会自动生成模板配置并退出。

编辑 `./data/config.yaml` 后，再次启动：

```bash
docker compose up -d
```

查看日志：

```bash
docker compose logs -f
```

Docker 部署下的持久化文件都在 `./data/`：

- `config.yaml`：主配置
- `runtime.json`：`/img` 维护的全局偏好
- `grimoire-bot`：容器启动时复制出的可执行文件
