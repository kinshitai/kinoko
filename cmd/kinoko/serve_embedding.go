//go:build embedding

package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/embedding"
)

// initEmbedEngine initializes the real ONNX embedding engine.
// The model directory is expected at <dataDir>/models/bge-small-en-v1.5,
// containing model.onnx, tokenizer.json, and libonnxruntime.so.
func initEmbedEngine(cfg *config.Config, logger *slog.Logger) (embedding.Engine, error) {
	modelDir := filepath.Join(cfg.Server.DataDir, "models", "bge-small-en-v1.5")
	engine, err := embedding.NewONNXEngine(modelDir)
	if err != nil {
		return nil, fmt.Errorf("init ONNX embedding engine: %w", err)
	}
	logger.Info("ONNX embedding engine loaded", "model_dir", modelDir, "dims", engine.Dims(), "model", engine.ModelID())
	return engine, nil
}
