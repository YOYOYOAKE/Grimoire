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

准备配置文件：

```bash
cp configs/config.yaml.example configs/config.yaml
```

并在 `config.yaml` 中配置：

- Telegram Bot Token
- 管理员 Telegram ID
- LLM API 端点和密钥
- Xianyun Novel AI 中继 API Key

然后启动项目：

```bash
./bin/grimoire-bot -config /home/YOAKE/dev/Grimoire/configs/config.yaml
```

## License

MIT License
