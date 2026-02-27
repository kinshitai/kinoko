package apiclient

import (
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Compile-time interface assertions.
var (
	_ model.Embedder       = (*HTTPEmbedder)(nil)
	_ model.SkillQuerier   = (*HTTPQuerier)(nil)
	_ model.SkillCommitter = (*GitPushCommitter)(nil)
)
