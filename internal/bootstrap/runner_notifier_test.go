package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	runnerapp "grimoire/internal/app/runner"
)

type telegramMessengerStub struct {
	sendTextChatID    int64
	sendTextReplyTo   int64
	sendTextText      string
	sendTextMessageID int64

	sendPhotoChatID    int64
	sendPhotoReplyTo   int64
	sendPhotoName      string
	sendPhotoCaption   string
	sendPhotoContent   []byte
	sendPhotoMessageID int64

	editChatID    int64
	editMessageID int64
	editText      string

	deleteChatID    int64
	deleteMessageID int64
}

func (s *telegramMessengerStub) SendText(_ context.Context, chatID int64, replyToMessageID int64, text string) (int64, error) {
	s.sendTextChatID = chatID
	s.sendTextReplyTo = replyToMessageID
	s.sendTextText = text
	if s.sendTextMessageID == 0 {
		return 42, nil
	}
	return s.sendTextMessageID, nil
}

func (s *telegramMessengerStub) EditText(_ context.Context, chatID int64, messageID int64, text string) error {
	s.editChatID = chatID
	s.editMessageID = messageID
	s.editText = text
	return nil
}

func (s *telegramMessengerStub) SendPhotoMessage(_ context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) (int64, error) {
	s.sendPhotoChatID = chatID
	s.sendPhotoReplyTo = replyToMessageID
	s.sendPhotoName = filename
	s.sendPhotoCaption = caption
	s.sendPhotoContent = append([]byte(nil), content...)
	if s.sendPhotoMessageID == 0 {
		return 84, nil
	}
	return s.sendPhotoMessageID, nil
}

func (s *telegramMessengerStub) DeleteMessage(_ context.Context, chatID int64, messageID int64) error {
	s.deleteChatID = chatID
	s.deleteMessageID = messageID
	return nil
}

func TestBootstrapRunnerNotifierSendTextConvertsIDs(t *testing.T) {
	messenger := &telegramMessengerStub{}
	notifier := newBootstrapRunnerNotifier(messenger, "/tmp/grimoire")

	messageID, err := notifier.SendText(context.Background(), "123", "hello", runnerapp.MessageOptions{ReplyToMessageID: "7"})
	if err != nil {
		t.Fatalf("send text: %v", err)
	}
	if messageID != "42" {
		t.Fatalf("unexpected message id: %q", messageID)
	}
	if messenger.sendTextChatID != 123 || messenger.sendTextReplyTo != 7 || messenger.sendTextText != "hello" {
		t.Fatalf("unexpected send text call: %#v", messenger)
	}
}

func TestBootstrapRunnerNotifierSendImageReadsRelativePath(t *testing.T) {
	rootDir := t.TempDir()
	imagePath := filepath.Join(rootDir, "data", "images", "task-1.jpg")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("mkdir image dir: %v", err)
	}
	if err := os.WriteFile(imagePath, []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	messenger := &telegramMessengerStub{}
	notifier := newBootstrapRunnerNotifier(messenger, rootDir)

	messageID, err := notifier.SendImage(
		context.Background(),
		"123",
		"data/images/task-1.jpg",
		"caption",
		runnerapp.MessageOptions{ReplyToMessageID: "8"},
	)
	if err != nil {
		t.Fatalf("send image: %v", err)
	}
	if messageID != "84" {
		t.Fatalf("unexpected image message id: %q", messageID)
	}
	if messenger.sendPhotoChatID != 123 || messenger.sendPhotoReplyTo != 8 {
		t.Fatalf("unexpected send photo ids: %#v", messenger)
	}
	if messenger.sendPhotoName != "task-1.jpg" || messenger.sendPhotoCaption != "caption" || string(messenger.sendPhotoContent) != "jpg" {
		t.Fatalf("unexpected send photo payload: %#v", messenger)
	}
}

func TestBootstrapRunnerNotifierDeleteMessageConvertsIDs(t *testing.T) {
	messenger := &telegramMessengerStub{}
	notifier := newBootstrapRunnerNotifier(messenger, "/tmp/grimoire")

	if err := notifier.DeleteMessage(context.Background(), "123", "9"); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	if messenger.deleteChatID != 123 || messenger.deleteMessageID != 9 {
		t.Fatalf("unexpected delete message call: %#v", messenger)
	}
}
