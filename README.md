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

先准备配置文件：

```bash
cp configs/config.yaml.example configs/config.yaml
# 编辑 configs/config.yaml，填写 Telegram / LLM / NAI 配置
```

然后直接启动：

```bash
./bin/grimoire-bot
```

程序会使用以下固定路径：

- 配置文件：`./configs/config.yaml`
- SQLite：`./data/grimoire.db`
- 图片目录：`./data/images`

配置说明：

- LLM/NAI/Telegram 配置仅从 `configs/config.yaml` 读取。
- `llm.openai_custom.enable` 与 `llm.openrouter.enable` 必须且仅能开启一个。
- `/llm` 和 `/nai` 命令已移除；需修改配置文件后重启生效。
- `/img` 仍可修改默认图像尺寸与画师串，这两项会保存到 SQLite。

如果绘图必需项缺失，机器人会提示缺失键并提示修改 `configs/config.yaml`。

## License

MIT License
