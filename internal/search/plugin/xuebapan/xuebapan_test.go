package xuebapan

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xianyu-go/internal/search/plugin"
)

const (
	testHash   = "11111111111111111111111111111111"
	promoHash  = "22222222222222222222222222222222"
	noiseHash  = "33333333333333333333333333333333"
	testDetail = `<span id="pwd">2023</span>`
	testValid  = `<div class="resource-item-wrap valid"><div class="resource-item">
		<h3 class="resource-title"><a class="valid" href="/info/11111111111111111111111111111111.html"><mark>考研</mark>真题</a></h3>
		<div class="detail-wrap"><p class="detail-item-wrap"><span class="detail-item-title">政治真题.pdf</span><span> 1.0MB</span></p>
		<p class="detail-item-wrap"><span class="detail-item-title">英语真题.pdf</span><span> 1.0MB</span></p></div>
		<div class="resource-meta"><span class="meta-item"><span class="label">文件大小</span><span class="em">2.0MB</span></span></div>
		<div class="other-info"><p class="time">2022-12-18</p></div>
	</div></div>`
	testPromo = `<div class="resource-item-wrap valid"><div class="resource-item">
		<h3 class="resource-title"><a class="valid" href="/info/22222222222222222222222222222222.html">2027<mark>考研</mark>群获取</a></h3>
		<div class="detail-wrap"><p class="detail-item-wrap"><span class="detail-item-title">扫码下单.jpg</span></p></div>
	</div></div>`
	testAd    = `<div class="resource-item-wrap"><div class="resource-item"><h3 class="resource-title"><a href="/info/ad.html">广告</a></h3></div></div>`
	testNoise = `<div class="resource-item-wrap valid"><div class="resource-item">
		<h3 class="resource-title"><a class="valid" href="/info/33333333333333333333333333333333.html">无关内容</a></h3>
		<div class="detail-wrap"><p class="detail-item-wrap"><span class="detail-item-title">a.txt</span></p></div>
	</div></div>`
)

// TestXuebapanSearch 验证解析、去重、引流过滤与提取码补齐。
func TestXuebapanSearch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/s/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.HasSuffix(r.URL.Path, "-2.html") {
			fmt.Fprint(w, testValid+testNoise)
			return
		}
		fmt.Fprint(w, testValid+testPromo+testAd)
	})
	mux.HandleFunc("/info/"+testHash+".html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, testDetail)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	previous := baseURL
	baseURL = server.URL
	defer func() { baseURL = previous }()

	results, err := NewXuebapanPlugin().doSearch(server.Client(), "考研", nil)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d 条: %+v", len(results), results)
	}
	got := results[0]
	if got.UniqueID != "xuebapan-"+testHash {
		t.Errorf("UniqueID = %q", got.UniqueID)
	}
	if got.Title != "考研真题" {
		t.Errorf("Title = %q", got.Title)
	}
	if !strings.Contains(got.Content, "政治真题.pdf") || !strings.Contains(got.Content, "2.0MB") {
		t.Errorf("Content 缺少文件列表或大小: %q", got.Content)
	}
	if want := time.Date(2022, 12, 18, 0, 0, 0, 0, time.UTC); !got.Datetime.Equal(want) {
		t.Errorf("Datetime = %v，期望 %v", got.Datetime, want)
	}
	if len(got.Links) != 1 {
		t.Fatalf("Links 数量 = %d", len(got.Links))
	}
	link := got.Links[0]
	if link.Type != "baidu" || link.Password != "2023" {
		t.Errorf("链接类型或提取码错误: %+v", link)
	}
	if want := server.URL + "/goto/" + testHash + ".html"; link.URL != want {
		t.Errorf("链接地址 = %q，期望 %q", link.URL, want)
	}
}

// TestXuebapanSearchURL 验证搜索地址与前端 pan.js 的跳转规则一致。
func TestXuebapanSearchURL(t *testing.T) {
	previous := baseURL
	baseURL = "https://www.xuebapan.com"
	defer func() { baseURL = previous }()

	want := "https://www.xuebapan.com/s/%E8%80%83%E7%A0%94%20%E6%95%B0%E5%AD%A6-2.html"
	if got := searchURL("考研 数学", 2); got != want {
		t.Fatalf("searchURL = %q，期望 %q", got, want)
	}
}

// TestXuebapanPluginRegistered 验证插件自注册，能被 Search 默认配置发现。
func TestXuebapanPluginRegistered(t *testing.T) {
	registered, ok := plugin.GetPluginByName(pluginName)
	if !ok {
		t.Fatal("学霸盘插件未注册")
	}
	if registered.Name() != pluginName {
		t.Fatalf("插件名称 = %q", registered.Name())
	}
}
