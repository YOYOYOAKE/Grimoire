package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	runnerapp "grimoire/internal/app/runner"
)

type telegramMessenger interface {
	SendText(ctx context.Context, chatID int64, replyToMessageID int64, text string) (int64, error)
	EditText(ctx context.Context, chatID int64, messageID int64, text string) error
	SendPhotoMessage(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) (int64, error)
	DeleteMessage(ctx context.Context, chatID int64, messageID int64) error
}

type bootstrapRunnerNotifier struct {
	bot     telegramMessenger
	rootDir string
}

func newBootstrapRunnerNotifier(bot telegramMessenger, rootDir string) *bootstrapRunnerNotifier {
	return &bootstrapRunnerNotifier{
		bot:     bot,
		rootDir: strings.TrimSpace(rootDir),
	}
}

func (n *bootstrapRunnerNotifier) SendText(ctx context.Context, userID string, text string, options runnerapp.MessageOptions) (string, error) {
	chatID, err := parseTelegramID("user id", userID)
	if err != nil {
		return "", err
	}
	replyTo, err := parseOptionalTelegramID("reply message id", options.ReplyToMessageID)
	if err != nil {
		return "", err
	}

	messageID, err := n.bot.SendText(ctx, chatID, replyTo, text)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(messageID, 10), nil
}

func (n *bootstrapRunnerNotifier) EditText(ctx context.Context, userID string, messageID string, text string, options runnerapp.MessageOptions) error {
	chatID, err := parseTelegramID("user id", userID)
	if err != nil {
		return err
	}
	telegramMessageID, err := parseTelegramID("message id", messageID)
	if err != nil {
		return err
	}
	return n.bot.EditText(ctx, chatID, telegramMessageID, text)
}

func (n *bootstrapRunnerNotifier) SendImage(ctx context.Context, userID string, path string, caption string, options runnerapp.MessageOptions) (string, error) {
	chatID, err := parseTelegramID("user id", userID)
	if err != nil {
		return "", err
	}
	replyTo, err := parseOptionalTelegramID("reply message id", options.ReplyToMessageID)
	if err != nil {
		return "", err
	}

	absolutePath := strings.TrimSpace(path)
	if absolutePath == "" {
		return "", fmt.Errorf("image path is required")
	}
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(n.rootDir, absolutePath)
	}
	content, err := os.ReadFile(absolutePath)
	if err != nil {
		return "", fmt.Errorf("read image %s: %w", absolutePath, err)
	}
	messageID, err := n.bot.SendPhotoMessage(ctx, chatID, replyTo, filepath.Base(absolutePath), caption, content)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(messageID, 10), nil
}

func (n *bootstrapRunnerNotifier) DeleteMessage(ctx context.Context, userID string, messageID string) error {
	chatID, err := parseTelegramID("user id", userID)
	if err != nil {
		return err
	}
	telegramMessageID, err := parseTelegramID("message id", messageID)
	if err != nil {
		return err
	}
	return n.bot.DeleteMessage(ctx, chatID, telegramMessageID)
}

func parseTelegramID(name string, raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s %q: %w", name, raw, err)
	}
	return value, nil
}

func parseOptionalTelegramID(name string, raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return parseTelegramID(name, raw)
}
