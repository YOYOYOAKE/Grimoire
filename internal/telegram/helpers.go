package telegram

import "strings"

func splitCommand(text string) (command string, rest string) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", ""
	}
	command = strings.ToLower(parts[0])
	if i := strings.Index(command, "@"); i >= 0 {
		command = command[:i]
	}
	if len(parts) > 1 {
		rest = strings.Join(parts[1:], " ")
	}
	return command, rest
}

func isAdminUser(adminUserID int64, userID int64) bool {
	return adminUserID > 0 && adminUserID == userID
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
