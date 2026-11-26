package studio

import (
	"fmt"
	"strings"
)

// DefaultSystemPrompt 是发给图片模型的默认角色约束。
// 四条规则对应实际踩过的坑：整图重绘会顺手改掉无关文字、替换内容会被复制到别的文字块、
// 中文容易出现错字或生造字、以及模型会自作主张加角标和水印。
const DefaultSystemPrompt = `You are a precise Chinese e-commerce cover image editor.
Rules you must always follow:
1. Only change the text blocks explicitly requested. Everything else — layout, grid alignment, photography, lighting, shadows, color grading and background — stays pixel-identical.
2. Each requested replacement maps to exactly ONE existing text block. Never copy a replacement value into another text block.
3. Chinese characters must stay Chinese and be rendered exactly as given, character by character. No misspelling, no garbled glyphs, no invented characters, no traditional/simplified substitution.
4. Do not add watermarks, logos, badges, stickers or any decorative element that is not in the original.`

// PromptFields 是文案侧的结构化字段；字段留空表示该文字块不需要改动。
type PromptFields struct {
	// SystemPrompt 是用户在设置页维护的角色约束。
	SystemPrompt string
	// Headline 是主标题。
	Headline string
	// Price 是价格数字，单位元单独渲染。
	Price string
	// Points 是卖点清单，逐行渲染。
	Points []string
	// Extra 是自由补充要求，例如"价格用红色""保留右下角角标"。
	Extra string
	// Copy 是完整商品文案，仅在模型缺少结构化字段时作为兜底参考。
	Copy string
	// Shipping 是一键发货文字，用于对齐卖点措辞。
	Shipping string
	// HasImage 表示本次是否带原图，决定改图还是文生图措辞。
	HasImage bool
}

// BuildPrompt 把结构化字段组装成一段可直接交给模型的指令。
// 有原图时强调"只改指定文字"，无原图时给出整张封面的排版要求。
func BuildPrompt(f PromptFields) string {
	// parts 按顺序累积最终指令，保证系统约束始终在最前面。
	parts := make([]string, 0, 16)
	system := strings.TrimSpace(f.SystemPrompt)
	if system == "" {
		system = DefaultSystemPrompt
	}
	parts = append(parts, system, "")

	if f.HasImage {
		parts = append(parts,
			"TASK: Edit the provided image. Keep the original design language and only touch the text blocks listed below.",
			"KEEP UNCHANGED: overall layout, grid alignment, typography style, photography, lighting, shadows, color grading, background, and every Chinese character that is not listed below.",
		)
	} else {
		parts = append(parts,
			"TASK: Create a square 1:1 Chinese e-commerce cover image for a second-hand marketplace listing.",
			"STYLE: clean editorial poster layout, generous whitespace, strong visual hierarchy, one hero headline, one large price, a short list of selling points, high contrast, sharp legible Chinese typography.",
		)
	}

	// lines 保存所有需要精确渲染的文字条目。
	lines := make([]string, 0, 8)
	if v := strings.TrimSpace(f.Headline); v != "" {
		lines = append(lines, fmt.Sprintf("- Main headline: %q", v))
	}
	if v := strings.TrimSpace(f.Price); v != "" {
		lines = append(lines, fmt.Sprintf("- Price: %q followed by a small Chinese character 元", v))
	}
	for i, p := range f.Points {
		if v := strings.TrimSpace(p); v != "" {
			lines = append(lines, fmt.Sprintf("- Selling point %d: %q", i+1, v))
		}
	}
	if len(lines) > 0 {
		parts = append(parts, "", "RENDER THESE TEXTS EXACTLY, character by character:")
		parts = append(parts, lines...)
	}

	if v := strings.TrimSpace(f.Extra); v != "" {
		parts = append(parts, "", "ADDITIONAL INSTRUCTION (highest priority after the rules above): "+v)
	}
	if len(lines) == 0 && strings.TrimSpace(f.Copy) != "" {
		// 没有结构化字段时，把文案前几行作为文字候选，避免模型凭空编造中文。
		parts = append(parts, "", "TEXT REFERENCE (pull the words from here, do not invent new Chinese):", firstLines(f.Copy, 4))
	}
	if v := strings.TrimSpace(f.Shipping); v != "" {
		parts = append(parts, "", "TONE REFERENCE (do not print this, only match the wording style):", firstLines(v, 2))
	}

	parts = append(parts, "", "OUTPUT: 1024x1024, sharp typography, no watermark, no extra logo, no decorative text.")
	return strings.Join(parts, "\n")
}

// firstLines 取前 n 个非空行，用于给模型提供文字来源而不用整篇文案。
func firstLines(text string, n int) string {
	out := make([]string, 0, n)
	for _, line := range strings.Split(text, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			out = append(out, v)
			if len(out) >= n {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}
