package studio

import "context"

// EngineClient 是后端无人值守模块（如商品工作流）复用 AI 能力的最小端口。
// 与 HTTP Handler 请求即传配置不同，这里的配置来自服务端设置存储，由调用方解析后传入。
type EngineClient interface {
	// Chat 用文字模型生成一段文本（文案改写等）。
	Chat(ctx context.Context, cfg ModelConfig, system, user string) (string, error)
	// Generate 用图片模型生成一张封面图，返回 data URL。
	Generate(ctx context.Context, cfg ModelConfig, in GenerateRequest) (GenerateResult, error)
}

// NewEngineClient 返回供工作流等后台模块使用的 AI 客户端。
func NewEngineClient() EngineClient {
	return newClient()
}
