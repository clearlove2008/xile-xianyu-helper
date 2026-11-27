package service

import (
	"strings"
	"testing"
	"time"

	"xianyu-go/internal/search/model"
)

// 复现用户反馈：搜「八仙」时 t.me 按字模糊命中「八个/神仙/八段锦」，
// 把整条搬运合集里的无关资源（1-9 年级随堂练习）也带出来了。
func TestMergeResultsByTypeFiltersTGDumpByLinkTitle(t *testing.T) {
	result := model.SearchResult{
		Channel:  "quanziyuanshe",
		UniqueID: "quanziyuanshe_1234",
		Title:    "资源合集",
		Content: "全+C2:D55国最新1-9年级随堂练习及答案1\n" +
			"https://pan.quark.cn/s/c5e9c9f3b70f\n" +
			"逃学威龙 1-3部（1991-1993）2160p 60fps\n" +
			"https://pan.quark.cn/s/309734840bf3\n" +
			"八仙饭店之人肉叉烧包 国粤语配音\n" +
			"https://pan.quark.cn/s/baxianfandian\n" +
			"沙丘2（2024）4K HDR\n" +
			"https://pan.quark.cn/s/shqiu2\n",
		Links: []model.Link{
			{Type: "quark", URL: "https://pan.quark.cn/s/c5e9c9f3b70f"},
			{Type: "quark", URL: "https://pan.quark.cn/s/309734840bf3"},
			{Type: "quark", URL: "https://pan.quark.cn/s/baxianfandian"},
			{Type: "quark", URL: "https://pan.quark.cn/s/shqiu2"},
		},
		Datetime: time.Now(),
	}

	merged := mergeResultsByType([]model.SearchResult{result}, "八仙", nil)

	links := merged["quark"]
	if len(links) != 1 {
		t.Fatalf("合集消息应只保留标题含关键词的链接，实际 %d 条: %+v", len(links), links)
	}
	if links[0].URL != "https://pan.quark.cn/s/baxianfandian" {
		t.Fatalf("保留的链接不对: %s", links[0].URL)
	}
}

// 单资源消息：标题不含关键词但描述里完整出现关键词时不能误杀。
func TestMergeResultsByTypeKeepsTGSingleWhenContentMatches(t *testing.T) {
	result := model.SearchResult{
		Channel:  "quanziyuanshe",
		UniqueID: "quanziyuanshe_5678",
		Title:    "经典限制级电影合集124部",
		Content: "最新收集整理124部经典限制级电影，包含八仙饭店之人肉叉烧包、西西里的美丽传说\n" +
			"https://pan.xunlei.com/s/VNj0j3abc\n",
		Links: []model.Link{
			{Type: "xunlei", URL: "https://pan.xunlei.com/s/VNj0j3abc"},
		},
		Datetime: time.Now(),
	}

	merged := mergeResultsByType([]model.SearchResult{result}, "八仙", nil)

	if len(merged["xunlei"]) != 1 {
		t.Fatalf("描述含关键词的单资源消息应保留，实际 %d 条", len(merged["xunlei"]))
	}
}

// 单资源消息：只沾了「神仙」里的「仙」，整词不匹配就该丢掉。
func TestMergeResultsByTypeDropsTGCharLevelNoise(t *testing.T) {
	result := model.SearchResult{
		Channel:  "quanziyuanshe",
		UniqueID: "quanziyuanshe_9012",
		Title:    "中国奇谭（更新至06）",
		Content: "名称：中国奇谭（更新至06）\n" +
			"描述：短片集由八个植根于中国传统文化的独立故事组成，其中一段叫《乡村巴士带走了王孩儿和神仙》。\n" +
			"https://pan.quark.cn/s/qitan\n",
		Links: []model.Link{
			{Type: "quark", URL: "https://pan.quark.cn/s/qitan"},
		},
		Datetime: time.Now(),
	}

	merged := mergeResultsByType([]model.SearchResult{result}, "八仙", nil)

	if len(merged["quark"]) != 0 {
		t.Fatalf("仅逐字命中（八/仙 分开出现）的结果应被过滤，实际 %d 条", len(merged["quark"]))
	}
}

func TestContainsAllTokens(t *testing.T) {
	cases := []struct {
		text    string
		keyword string
		want    bool
	}{
		{"八仙饭店之人肉叉烧包", "八仙", true},
		{"八段锦完整版", "八仙", false},
		{"乡村巴士带走了王孩儿和神仙", "八仙", false},
		{"沙丘2 4K HDR 国英音轨", "沙丘 4k", true},
		{"沙丘2 1080p", "沙丘 4k", false},
		{"Avatar The Way of Water", "avatar", true},
		{"随便什么内容", "  ", true},
	}

	for _, c := range cases {
		if got := containsAllTokens(c.text, c.keyword); got != c.want {
			t.Errorf("containsAllTokens(%q, %q) = %v, want %v", c.text, c.keyword, got, c.want)
		}
	}
}

// 搬运合集常见形态：整条消息只有一行，「标题  URL标题  URL」首尾相连。
// 这时每条链接的标题必须取它前面紧邻的文字，而不是消息首行标题。
func TestMergeResultsByTypeSplitsSingleLineDumpTitles(t *testing.T) {
	content := "会计师课程资源合集  https://pan.quark.cn/s/a16147596ecf" +
		"6月19日 付费+精选短剧  https://pan.quark.cn/s/62dce7ba84ab" +
		"🔥三体 NETFLIX版 4K高码  https://pan.quark.cn/s/a5b03f9734f4" +
		"功夫熊猫4  https://pan.quark.cn/s/d3bebe85f588"

	result := model.SearchResult{
		Channel:  "quanziyuanshe",
		UniqueID: "quanziyuanshe_2799",
		Title:    "会计师课程资源合集",
		Content:  content,
		Links: []model.Link{
			// 上游把整条消息的首行标题当 WorkTitle 填给了每条链接
			{Type: "quark", URL: "https://pan.quark.cn/s/a16147596ecf", WorkTitle: "会计师课程资源合集"},
			{Type: "quark", URL: "https://pan.quark.cn/s/62dce7ba84ab", WorkTitle: "会计师课程资源合集"},
			{Type: "quark", URL: "https://pan.quark.cn/s/a5b03f9734f4", WorkTitle: "会计师课程资源合集"},
			{Type: "quark", URL: "https://pan.quark.cn/s/d3bebe85f588", WorkTitle: "会计师课程资源合集"},
		},
		Datetime: time.Now(),
	}

	merged := mergeResultsByType([]model.SearchResult{result}, "三体", nil)

	links := merged["quark"]
	if len(links) != 1 {
		t.Fatalf("只应保留三体那条链接，实际 %d 条: %+v", len(links), links)
	}
	if !strings.Contains(links[0].Note, "三体") {
		t.Fatalf("保留链接的标题应取自正文，实际 note=%q", links[0].Note)
	}
}
