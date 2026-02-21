//go:build !embedding

package main

import (
	"log/slog"

	"github.com/kinoko-dev/kinoko/internal/serve/embedding"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

// initEmbedEngine returns nil when built without the "embedding" tag.
// The /api/v1/embed endpoint will return 503 (no engine available).
func initEmbedEngine(_ *config.Config, _ *slog.Logger) (embedding.Engine, error) {
	return nil, nil
}
