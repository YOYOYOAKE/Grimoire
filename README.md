# Grimoire

Grimoire（魔导书）是一个利用大语言模型将自然语言翻译为 Novel AI 提示词，并自动生成图片的 Telegram 机器人。

## 功能

- Telegram 机器人交互
- 通过 OpenAI 兼容 API 实现自然语言到 Novel AI 提示词的翻译
- 通过 Xianyun Novel AI 中继 API 进行图像生成
- 消息与任务的持久化存储。

![Introduction](docs/introduction.png)

## 构建

```bash
go build -o bin/grimoire-bot ./cmd/grimoire-bot
```

## 运行

先设置环境变量（Telegram 凭证）：

```bash
export GRIMOIRE_TELEGRAM_BOT_TOKEN="123456:ABCDEF..."
export GRIMOIRE_TELEGRAM_ADMIN_USER_ID="123456789"
# 可选：Telegram 代理
# export GRIMOIRE_TELEGRAM_PROXY_URL="http://127.0.0.1:7890"
```

然后直接启动：

```bash
./bin/grimoire-bot
```

程序会使用以下固定路径：

- SQLite：`./data/grimoire.db`
- 图片目录：`./data/images`

首次启动后，请在 Telegram 中完成配置：

1. `/llm` 设置 `llm.base_url`、`llm.api_key`、`llm.model`
2. `/nai` 设置 `nai.api_key`、`nai.model`
3. `/img` 可设置默认图像尺寸和画师串

如果上述绘图必需项缺失，机器人会提示缺失键并引导去 `/llm` 和 `/nai` 配置。

## 配置文件说明

`configs/config.yaml` 与 `configs/config.yaml.example` 目前仅保留为历史参考，运行时不再读取。

## License

MIT License
