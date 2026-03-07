# Grimoire v2

Grimoire v2 是一个纯内存的 Telegram 绘图机器人：

- 接收 Telegram 文本消息
- 调用 OpenAI 兼容接口翻译提示词
- 调用 Xianyun NovelAI 中继生成图片
- 直接把图片发送回 Telegram

运行时不使用 SQLite，不做任务恢复。任务和 `/img` 偏好都只保存在内存里，进程重启后会清空。

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
