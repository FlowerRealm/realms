// Package obs 提供最小的可观测能力：结构化日志与必要字段，默认不记录敏感信息。
package obs

import (
	"log/slog"
	"os"
)

func NewLogger(env string) *slog.Logger {
	level := slog.LevelInfo
	if env == "dev" {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}
