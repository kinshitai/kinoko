package serverclient

import (
	"github.com/kinoko-dev/kinoko/internal/decay"
	"github.com/kinoko-dev/kinoko/internal/injection"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// Compile-time interface assertions.
var (
	_ model.Embedder                 = (*HTTPEmbedder)(nil)
	_ model.SkillQuerier             = (*HTTPQuerier)(nil)
	_ decay.SkillReader              = (*HTTPDecayClient)(nil)
	_ decay.SkillWriter              = (*HTTPDecayClient)(nil)
	_ injection.InjectionEventWriter = (*HTTPInjectionEventWriter)(nil)
	_ model.SkillCommitter           = (*GitPushCommitter)(nil)
)
