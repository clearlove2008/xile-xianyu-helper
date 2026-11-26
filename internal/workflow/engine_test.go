package workflow

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"xianyu-go/internal/application/items"
	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/mtop"
)

// fakeSearcher 记录搜索调用并返回预设结果。
type fakeSearcher struct {
	keyword string
	items   []mtop.SearchItem
	err     error
}

func (f *fakeSearcher) SearchItems(_ context.Context, _ string, keyword string, _, _ int) (*mtop.SearchResult, error) {
	f.keyword = keyword
	if f.err != nil {
		return nil, f.err
	}
	return &mtop.SearchResult{Items: f.items, Page: 1, PageSize: len(f.items)}, nil
}

// fakePublisher 记录发布调用并返回固定商品 ID。
type fakePublisher struct {
	calls    []string
	failNext bool
}

func (f *fakePublisher) PublishSingle(_ context.Context, input items.PublishInput) (items.PublishOutcome, error) {
	if f.failNext {
		f.failNext = false
		return items.PublishOutcome{}, errors.New("平台拒绝")
	}
	f.calls = append(f.calls, input.CookieID+"|"+input.Title+"|"+strconv.FormatInt(input.PriceCents, 10))
	return items.PublishOutcome{Result: &items.PublishResult{ItemID: "9001"}}, nil
}

// newTestEngine 构造一个使用临时库的引擎，并写入两个账号凭证。
func newTestEngine(t *testing.T, searcher Searcher, publisher Publisher) *Engine {
	t.Helper()
	database, dialect, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	store := db.NewStore(database, dialect)
	if _, err := database.Exec(`INSERT INTO users (id, username, email, password_hash) VALUES (1,'u','u@e.com','x')`); err != nil {
		t.Fatalf("写入用户失败: %v", err)
	}
	if err := store.Cookies.Save(context.Background(), "acc-search", "unb=1; _m_h5_tk=token_1", 1); err != nil {
		t.Fatalf("写入搜索凭证失败: %v", err)
	}
	if err := store.Cookies.Save(context.Background(), "acc-pub", "unb=1; _m_h5_tk=token_2", 1); err != nil {
		t.Fatalf("写入发布凭证失败: %v", err)
	}
	return New(store, searcher, publisher, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// 任务 CRUD 往返：创建、读取、更新、启停、软删。
func TestTaskCRUDRoundtrip(t *testing.T) {
	engine := newTestEngine(t, &fakeSearcher{}, &fakePublisher{})
	ctx := context.Background()

	id, err := engine.CreateTask(ctx, TaskInput{
		Keyword: "露营车", SearchCookieID: "acc-search", PublishCookieIDs: []string{"acc-pub"},
		TargetCount: 2, IntervalMinutes: 30, PublishGapSeconds: 10, Quantity: 1,
	})
	if err != nil || id <= 0 {
		t.Fatalf("创建任务失败: %v", err)
	}
	task, err := engine.GetTask(ctx, id)
	if err != nil || task == nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if task.Keyword != "露营车" || task.IntervalMinutes != 30 || len(task.PublishCookieIDs) != 1 {
		t.Fatalf("任务内容不符: %+v", task)
	}
	// 未启用的任务 next_run_at 为空。
	if task.NextRunAt != nil {
		t.Fatalf("未启用任务不应有执行时间")
	}

	if err := engine.UpdateTask(ctx, id, TaskInput{
		Keyword: "折叠小推车", SearchCookieID: "acc-search", PublishCookieIDs: []string{"acc-pub"},
		TargetCount: 3, IntervalMinutes: 60, PublishGapSeconds: 15, Quantity: 1,
	}); err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}
	task, _ = engine.GetTask(ctx, id)
	if task.Keyword != "折叠小推车" || task.TargetCount != 3 {
		t.Fatalf("更新未生效: %+v", task)
	}

	if err := engine.ToggleTask(ctx, id, true); err != nil {
		t.Fatalf("启用任务失败: %v", err)
	}
	task, _ = engine.GetTask(ctx, id)
	if !task.Enabled || task.NextRunAt == nil {
		t.Fatalf("启用后应安排执行时间: %+v", task)
	}

	if err := engine.DeleteTask(ctx, id); err != nil {
		t.Fatalf("删除任务失败: %v", err)
	}
	task, _ = engine.GetTask(ctx, id)
	if task != nil {
		t.Fatalf("软删后任务不应再可见")
	}
}

// 非法输入校验：缺关键词、缺账号、数量越界。
func TestTaskInputValidation(t *testing.T) {
	engine := newTestEngine(t, &fakeSearcher{}, &fakePublisher{})
	ctx := context.Background()
	if _, err := engine.CreateTask(ctx, TaskInput{Keyword: "", SearchCookieID: "a", PublishCookieIDs: []string{"b"}}); err == nil {
		t.Fatal("空关键词应被拒绝")
	}
	if _, err := engine.CreateTask(ctx, TaskInput{Keyword: "k", SearchCookieID: "", PublishCookieIDs: []string{"b"}}); err == nil {
		t.Fatal("缺搜索账号应被拒绝")
	}
	if _, err := engine.CreateTask(ctx, TaskInput{Keyword: "k", SearchCookieID: "a", PublishCookieIDs: nil}); err == nil {
		t.Fatal("缺发布账号应被拒绝")
	}
	if _, err := engine.CreateTask(ctx, TaskInput{Keyword: "k", SearchCookieID: "a", PublishCookieIDs: []string{"b"}, TargetCount: 99}); err == nil {
		t.Fatal("数量越界应被拒绝")
	}
}

// RunOnce 全链路：搜索 → 价格过滤 → 发布，并写去重与日志；重复执行不再发布。
func TestRunOncePublishesAndDeduplicates(t *testing.T) {
	// coverServer 提供源商品封面图，避免测试发起真实外网请求。
	coverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-image-bytes"))
	}))
	t.Cleanup(coverServer.Close)
	searcher := &fakeSearcher{items: []mtop.SearchItem{
		{ItemID: "1001", Title: "露营车 九成新", Price: "160", ImageURL: coverServer.URL + "/a.jpg"},
		{ItemID: "1002", Title: "贵的露营车", Price: "1600"}, // 超出价格上限，过滤
		{ItemID: "1003", Title: "便宜的挂件", Price: "9"},    // 低于价格下限，过滤
		{ItemID: "1004", Title: "无价格商品", Price: ""},     // 无价格，过滤
	}}
	publisher := &fakePublisher{}
	engine := newTestEngine(t, searcher, publisher)
	ctx := context.Background()

	id, err := engine.CreateTask(ctx, TaskInput{
		Keyword: "露营车", SearchCookieID: "acc-search", PublishCookieIDs: []string{"acc-pub"},
		PriceMinCents: 1000, PriceMaxCents: 20000, TargetCount: 5,
		IntervalMinutes: 30, PublishGapSeconds: 5, UseSourceImage: true, Quantity: 1,
	})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if err := engine.RunOnce(ctx, id); err != nil {
		t.Fatalf("执行任务失败: %v", err)
	}
	if len(publisher.calls) != 1 || !strings.Contains(publisher.calls[0], "acc-pub") {
		t.Fatalf("应只发布 1 件符合价格区间的商品，实际: %v", publisher.calls)
	}
	if !strings.Contains(publisher.calls[0], "160") {
		t.Fatalf("未配置固定价时应使用源价: %v", publisher.calls)
	}
	// 任务固定价优先。
	if _, err := engine.store.DB.ExecContext(ctx,
		`UPDATE workflow_tasks SET price_cents = 9900 WHERE id = ?`, id); err != nil {
		t.Fatalf("更新固定价失败: %v", err)
	}

	// 去重表已有记录后重复执行不再发布同一源商品。
	if err := engine.RunOnce(ctx, id); err != nil {
		t.Fatalf("重复执行失败: %v", err)
	}
	if len(publisher.calls) != 1 {
		t.Fatalf("重复执行不应再发布同一源商品，实际: %v", publisher.calls)
	}

	// 运行日志应有记录。
	logs, err := engine.ListLogs(ctx, id, 20)
	if err != nil || len(logs) == 0 {
		t.Fatalf("应写入运行日志: %v %v", logs, err)
	}
}

// 搜索失败时任务返回错误但不崩溃，且推进 next_run_at。
func TestRunOnceSearchFailure(t *testing.T) {
	searcher := &fakeSearcher{err: errors.New("触发风控")}
	publisher := &fakePublisher{}
	engine := newTestEngine(t, searcher, publisher)
	ctx := context.Background()
	id, _ := engine.CreateTask(ctx, TaskInput{
		Keyword: "k", SearchCookieID: "acc-search", PublishCookieIDs: []string{"acc-pub"},
		TargetCount: 1, IntervalMinutes: 30, PublishGapSeconds: 5, UseSourceImage: true,
	})
	if err := engine.RunOnce(ctx, id); err == nil {
		t.Fatal("搜索失败应返回错误")
	}
	// next_run_at 已推进，避免失败任务高频重试。
	task, _ := engine.GetTask(ctx, id)
	if task.NextRunAt == nil || !task.NextRunAt.After(time.Now()) {
		t.Fatalf("失败后应推迟下次执行: %+v", task.NextRunAt)
	}
}

// 价格解析与 data URL 解码的边界。
func TestParsePriceCents(t *testing.T) {
	cases := map[string]int64{"160": 16000, "160.5": 16050, "0.99": 99, "": 0, "abc": 0, "-5": 0}
	for in, want := range cases {
		if got := parsePriceCents(in); got != want {
			t.Fatalf("parsePriceCents(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDecodeDataURL(t *testing.T) {
	data, contentType, err := decodeDataURL("data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("img")))
	if err != nil || string(data) != "img" || contentType != "image/png" {
		t.Fatalf("解码失败: %v %s %v", data, contentType, err)
	}
	if _, _, err := decodeDataURL("https://example.com/a.png"); err == nil {
		t.Fatal("非 data URL 应报错")
	}
}

// filterCandidates 价格区间过滤。
func TestFilterCandidatesPriceRange(t *testing.T) {
	engine := newTestEngine(t, &fakeSearcher{}, &fakePublisher{})
	task := Task{PriceMinCents: 1500, PriceMaxCents: 5000, PublishCookieIDs: []string{"acc-pub"}}
	items := []mtop.SearchItem{
		{ItemID: "1", Title: "a", Price: "10"}, {ItemID: "2", Title: "b", Price: "30"},
		{ItemID: "3", Title: "c", Price: "60"}, {ItemID: "4", Title: "", Price: "30"}, {ItemID: "", Title: "d", Price: "30"},
	}
	got := engine.filterCandidates(context.Background(), task, items)
	if len(got) != 1 || got[0].ItemID != "2" {
		t.Fatalf("价格区间过滤结果不符: %+v", got)
	}
}
