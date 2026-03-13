# Grimoire v2

Grimoire v2 是一个只服务 `telegram.admin_user_id` 的 Telegram 绘图机器人：

- 接收 Telegram 文本消息
- 调用 OpenAI 兼容接口翻译提示词
- 调用官方 NovelAI 图像 API 同步生成图片
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

## GitHub Actions 部署

仓库已提供 [deploy.yml](/home/YOAKE/dev/Grimoire/.github/workflows/deploy.yml)。推送到 `main` 后会自动：

- 运行测试
- 在 GitHub Actions 上构建 Linux 二进制
- 把二进制上传到服务器的 `/opt/grimoire`
- 通过 GitHub Action 的 SSH action 激活新二进制
- 重启服务

服务器需要提前满足这些条件：

- `/opt/grimoire` 目录已经存在
- `DEPLOY_USER` 对 `/opt/grimoire` 有写权限
- `config.yaml` 和 `runtime.json` 放在 `/opt/grimoire` 中
- 如果需要自动安装 systemd unit，`DEPLOY_USER` 还需要有免密码 `sudo`

需要在 GitHub 仓库 Secrets 中配置：

- `DEPLOY_HOST`：服务器地址
- `DEPLOY_PORT`：SSH 端口，可留空，默认 `22`
- `DEPLOY_USER`：SSH 用户
- `DEPLOY_SSH_PRIVATE_KEY`：用于发布的私钥
- `DEPLOY_HOST_FINGERPRINT`：可选，服务器 SSH host key 的 SHA256 指纹，建议配置
- `DEPLOY_RESTART_CMD`：发布后执行的重启命令，例如 `sudo systemctl restart grimoire-bot`

发布逻辑分成两段：workflow 先在 GitHub Actions 上构建 `dist/grimoire-bot.new`，并附带上传 [grimoire-bot.service](/home/YOAKE/dev/Grimoire/deploy/grimoire-bot.service)，再通过 `appleboy/scp-action` 一并传到服务器 `/opt/grimoire`，最后由 [deploy-remote-install.sh](/home/YOAKE/dev/Grimoire/scripts/deploy-remote-install.sh) 原子替换成 `/opt/grimoire/grimoire-bot` 并执行重启命令。如果服务器上还没有 `/etc/systemd/system/grimoire-bot.service`，脚本会自动把上传的 unit 安装过去并执行 `systemctl daemon-reload`。部署目录固定为 `/opt/grimoire`，不再提供配置项。

## systemd 管理

仓库已提供 [grimoire-bot.service](/home/YOAKE/dev/Grimoire/deploy/grimoire-bot.service)。该文件默认约定：

- 部署目录为 `/opt/grimoire`
- 运行用户为 `root`
- 二进制、`config.yaml`、`runtime.json` 位于同一目录

安装示例：

```bash
sudo mkdir -p /opt/grimoire
sudo cp bin/config.yaml /opt/grimoire/config.yaml
sudo cp bin/runtime.json /opt/grimoire/runtime.json

sudo cp deploy/grimoire-bot.service /etc/systemd/system/grimoire-bot.service
sudo systemctl daemon-reload
sudo systemctl enable grimoire-bot
```

查看状态与日志：

```bash
sudo systemctl status grimoire-bot
journalctl -u grimoire-bot -f
```

如果你不想手动安装 unit，也可以直接让第一次自动部署来完成；前提是 `DEPLOY_USER` 具备免密码 `sudo`，并且 `DEPLOY_RESTART_CMD` 可用。

首次自动部署成功后，再执行：

```bash
sudo systemctl start grimoire-bot
```

如果你使用 GitHub Actions 自动部署，推荐把 `DEPLOY_RESTART_CMD` 设为：

```bash
sudo systemctl restart grimoire-bot
```

执行自动部署的 SSH 用户需要具备该命令的免密码 `sudo` 权限。
