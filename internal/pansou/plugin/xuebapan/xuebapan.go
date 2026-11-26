package xuebapan

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"xianyu-go/internal/pansou/model"
	"xianyu-go/internal/pansou/plugin"
)

const (
	pluginName        = "xuebapan"
	defaultBaseURL    = "https://www.xuebapan.com"
	searchPageCount   = 5
	resultLimit       = 100
	detailLimit       = 8
	detailConcurrency = 6
	contentFileLimit  = 12
	requestTimeout    = 12 * time.Second
	userAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
)

// baseURL 允许测试替换为本机 HTTP 服务，生产环境固定指向学霸盘官网。
var baseURL = defaultBaseURL

var (
	detailHashRegex = regexp.MustCompile(`/info/([0-9a-f]{32})\.html`)
	passwordRegex   = regexp.MustCompile(`^[A-Za-z0-9]{3,8}$`)
	spaceRegex      = regexp.MustCompile(`\s+`)
	promoKeywords   = []string{"扫码", "自助下单", "售后群", "加微信", "公众号", "QQ群"}
	imageExtensions = []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}
)

// searchItem 是搜索卡片中可稳定解析的字段。
type searchItem struct {
	hash     string
	title    string
	detail   string
	content  string
	datetime time.Time
	files    []string
}

// XuebapanPlugin 按学霸盘前端的 URL 规则检索服务端渲染页面。
type XuebapanPlugin struct {
	*plugin.BaseAsyncPlugin
}

// NewXuebapanPlugin 创建学霸盘异步搜索插件。
func NewXuebapanPlugin() *XuebapanPlugin {
	return &XuebapanPlugin{
		BaseAsyncPlugin: plugin.NewBaseAsyncPlugin(pluginName, 1),
	}
}

func init() {
	plugin.RegisterGlobalPlugin(NewXuebapanPlugin())
}

// Search 执行搜索并返回结果。
func (p *XuebapanPlugin) Search(keyword string, ext map[string]interface{}) ([]model.SearchResult, error) {
	result, err := p.SearchWithResult(keyword, ext)
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

// SearchWithResult 返回带最终结果标记的结果，供异步补齐链路使用。
func (p *XuebapanPlugin) SearchWithResult(keyword string, ext map[string]interface{}) (model.PluginSearchResult, error) {
	return p.AsyncSearchWithResult(keyword, p.doSearch, p.MainCacheKey, ext)
}

// doSearch 抓取搜索卡片并并发补充提取码。
func (p *XuebapanPlugin) doSearch(client *http.Client, keyword string, _ map[string]interface{}) ([]model.SearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}

	type pageResult struct {
		items []searchItem
		err   error
	}
	pageCh := make(chan pageResult, searchPageCount)
	for page := 1; page <= searchPageCount; page++ {
		go func(page int) {
			items, err := fetchSearchPage(client, keyword, page)
			pageCh <- pageResult{items: items, err: err}
		}(page)
	}

	items := make([]searchItem, 0, resultLimit)
	seen := make(map[string]bool)
	for page := 0; page < searchPageCount; page++ {
		result := <-pageCh
		if result.err != nil {
			continue
		}
		for _, item := range result.items {
			if seen[item.hash] || isPromotional(item) {
				continue
			}
			seen[item.hash] = true
			items = append(items, item)
			if len(items) >= resultLimit {
				break
			}
		}
		if len(items) >= resultLimit {
			break
		}
	}
	if len(items) == 0 {
		return nil, nil
	}

	passwords := make([]string, len(items))
	semaphore := make(chan struct{}, detailConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < len(items) && i < detailLimit; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			passwords[index] = fetchPassword(client, items[index].detail)
		}(i)
	}
	wg.Wait()

	results := make([]model.SearchResult, 0, len(items))
	for i, item := range items {
		results = append(results, model.SearchResult{
			UniqueID: fmt.Sprintf("%s-%s", pluginName, item.hash),
			Title:    item.title,
			Content:  item.content,
			Datetime: item.datetime,
			Links: []model.Link{{
				Type:     "baidu",
				URL:      strings.TrimRight(baseURL, "/") + "/goto/" + item.hash + ".html",
				Password: passwords[i],
				Datetime: item.datetime,
			}},
		})
	}
	return plugin.FilterResultsByKeyword(results, keyword), nil
}

// searchURL 复现学霸盘前端 pan.js 的搜索跳转规则：/s/<关键词>-<页码>.html。
func searchURL(keyword string, page int) string {
	return fmt.Sprintf("%s/s/%s-%d.html", strings.TrimRight(baseURL, "/"), url.PathEscape(keyword), page)
}

// fetchSearchPage 解析一页搜索结果。学霸盘前端把搜索词拼成 /s/<词>-<页>.html。
func fetchSearchPage(client *http.Client, keyword string, page int) ([]searchItem, error) {
	target := searchURL(keyword, page)
	doc, err := fetchDocument(client, target)
	if err != nil {
		return nil, err
	}

	items := make([]searchItem, 0, 20)
	doc.Find(".resource-item-wrap.valid").Each(func(_ int, selection *goquery.Selection) {
		anchor := selection.Find("h3.resource-title a[href]").First()
		href, exists := anchor.Attr("href")
		if !exists {
			return
		}
		matches := detailHashRegex.FindStringSubmatch(href)
		if len(matches) < 2 {
			return
		}
		title := normalize(anchor.Text())
		if title == "" {
			return
		}

		files := make([]string, 0, contentFileLimit)
		selection.Find(".detail-wrap .detail-item-title").Each(func(_ int, file *goquery.Selection) {
			name := normalize(file.Text())
			if name != "" && len(files) < contentFileLimit {
				files = append(files, name)
			}
		})
		if len(files) == 0 {
			return
		}

		size := normalize(selection.Find(".resource-meta .em").First().Text())
		content := "文件：" + strings.Join(files, "；")
		if size != "" {
			content += "\n大小：" + size
		}
		dateText := normalize(selection.Find(".other-info .time").First().Text())
		var datetime time.Time
		if dateText != "" {
			if parsed, err := time.Parse("2006-01-02", dateText); err == nil {
				datetime = parsed
			}
		}

		items = append(items, searchItem{
			hash:     matches[1],
			title:    title,
			detail:   href,
			content:  content,
			datetime: datetime,
			files:    files,
		})
	})
	return items, nil
}

// fetchPassword 从详情页读取提取码。详情页拿不到时返回空串，搜索结果仍然可用。
func fetchPassword(client *http.Client, detailPath string) string {
	detailURL := absoluteURL(detailPath)
	doc, err := fetchDocument(client, detailURL)
	if err != nil {
		return ""
	}
	password := normalize(doc.Find("#pwd").First().Text())
	if !passwordRegex.MatchString(password) {
		return ""
	}
	return password
}

// fetchDocument 请求页面并解析为 DOM。
func fetchDocument(client *http.Client, target string) (*goquery.Document, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	ctx, cancel := context.WithTimeout(req.Context(), requestTimeout)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求 %s 返回 HTTP %d", target, resp.StatusCode)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

// absoluteURL 把详情页相对路径补全为绝对地址。
func absoluteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	root, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return root.ResolveReference(ref).String()
}

// isPromotional 过滤伪装成资源的扫码引流卡片，保留正常文件列表。
func isPromotional(item searchItem) bool {
	joined := item.title + " " + strings.Join(item.files, " ")
	hasPromo := false
	for _, keyword := range promoKeywords {
		if strings.Contains(joined, keyword) {
			hasPromo = true
			break
		}
	}
	if !hasPromo {
		return false
	}
	for _, file := range item.files {
		lower := strings.ToLower(file)
		isImage := false
		for _, ext := range imageExtensions {
			if strings.HasSuffix(lower, ext) {
				isImage = true
				break
			}
		}
		if !isImage && !strings.Contains(file, "扫码") && !strings.Contains(file, "下单") {
			return false
		}
	}
	return true
}

// normalize 折叠 HTML 文本中的多余空白，保留单个换行供内容分段。
func normalize(value string) string {
	return strings.TrimSpace(spaceRegex.ReplaceAllString(value, " "))
}
