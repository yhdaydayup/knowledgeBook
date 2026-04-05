package feishu

import (
	"strconv"
	"strings"
)

type ParsedCommand struct {
	Namespace string
	Name      string
	Args      []string
}

func ParseCommand(text string) ParsedCommand {
	text = strings.TrimSpace(text)
	if text == "" {
		return ParsedCommand{}
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ParsedCommand{}
	}
	first := strings.TrimPrefix(parts[0], "/")
	cmd := ParsedCommand{Namespace: first, Name: first, Args: parts[1:]}
	if first == "kb" && len(parts) > 1 {
		cmd.Name = strings.ToLower(strings.TrimSpace(parts[1]))
		cmd.Args = parts[2:]
	}
	return cmd
}

func ParseInt64(v string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(v), 10, 64) }
