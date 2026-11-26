// Package mtop: 商机挖掘用的闲鱼商品搜索（直调 PC 搜索接口）。
package mtop

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SearchAPI 是闲鱼 PC 站关键词搜索接口。
const SearchAPI = "https://h5api.m.goofish.com/h5/mtop.taobao.idlemtopsearch.pc.search/1.0/"

// searchAPIName 用于错误信息与日志。
const searchAPIName = "mtop.taobao.idlemtopsearch.pc.search"

// SearchItem 是一条搜索结果商品。
type SearchItem struct {
	// ItemID 是平台商品 ID。
	ItemID string
	// Title 是商品标题。
	Title string
	// Price 是商品价格文本（如 "160" 或 "99.5"）。
	Price string
	// ImageURL 是封面图地址（已补协议头）。
	ImageURL string
	// ItemURL 是 PC 端商品页地址。
	ItemURL string
	// Seller 是卖家昵称。
	Seller string
	// Area 是发货地文本。
	Area string
}

// SearchResult 是一次搜索的返回。
type SearchResult struct {
	// Items 是本页商品列表。
	Items []SearchItem
	// Page 是返回的页码（与请求一致）。
	Page int
	// PageSize 是本页请求条数。
	PageSize int
	// Total 是平台报告的总命中数（可能为 0 表示未知）。
	Total int
	// HasMore 表示是否还有下一页。
	HasMore bool
	// UpdatedCookies 是本次请求吸收响应 Cookie 后的最新 Cookie。
	UpdatedCookies string
}

// SearchOpportunityItems 按关键词或显式卖家 ID 查询公开在售商品。
// 注意：搜索场景不允许主动刷新签名 Token（会触发平台风控），Token 失效直接报错由调用方提示重新登录。
func (c *ClientImpl) SearchOpportunityItems(ctx context.Context, cookiesStr, keyword string, page, pageSize int, sellerID ...string) (*SearchResult, error) {
	// sellerID 非空时使用卖家公开商品列表，原关键词调用保持兼容。
	if len(sellerID) > 0 && sellerID[0] != "" {
		return c.searchSellerItems(ctx, cookiesStr, sellerID[0], page)
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("搜索关键词不能为空")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	// data 与闲鱼 PC 搜索页发出的请求保持一致。
	data := map[string]any{
		"keyword":           keyword,
		"pageNumber":        page,
		"rowsPerPage":       pageSize,
		"fromFilter":        false,
		"sortValue":         "",
		"sortField":         "",
		"searchReqFromPage": "pcSearch",
		"customDistance":    "",
		"gps":               "",
		"customGps":         "",
		"propValueStr":      map[string]any{},
		"extraFilterValue":  "{}",
		"userPositionJson":  "{}",
	}
	// 单次调用不重试：搜索接口主动刷新 Token 会触发 Baxia 风控。
	decoded, updated, status, err := c.callMTopOnce(ctx, cookiesStr, SearchAPI, searchAPIName, "1.0",
		"a21ybx.search.0.0", "a21ybx.home.sidebar.1.search", strconv.FormatInt(time.Now().UnixMilli(), 10), data)
	if err != nil {
		return nil, err
	}
	ret := retFromDecoded(decoded)
	if !hasMTopSuccess(ret) {
		return nil, c.mtopResponseFailure(searchAPIName, status, ret, "搜索接口未返回成功")
	}
	result := parseSearchResponse(decoded)
	result.Page = page
	result.PageSize = pageSize
	result.UpdatedCookies = updated
	return result, nil
}

// parseSearchResponse 从成功的搜索响应中提取商品列表与分页信息。
// 响应结构为 data.resultList[].data.item.main.exContent，各层都做防御性断言。
func parseSearchResponse(decoded map[string]any) *SearchResult {
	result := &SearchResult{}
	data, _ := decoded["data"].(map[string]any)
	if data == nil {
		return result
	}
	// resultInfo 里带分页信息。
	if info, ok := data["resultInfo"].(map[string]any); ok {
		if hasNext, ok := info["hasNextPage"].(bool); ok {
			result.HasMore = hasNext
		}
		if numFound, ok := info["numFound"].(float64); ok {
			result.Total = int(numFound)
		}
	}
	// resultList 是主结果容器。
	rawList, _ := data["resultList"].([]any)
	for _, rawEntry := range rawList {
		entry, _ := rawEntry.(map[string]any)
		if entry == nil {
			continue
		}
		item := searchItemFromResultEntry(entry)
		if item.ItemID != "" || item.Title != "" {
			result.Items = append(result.Items, item)
		}
	}
	// 平台有时把结果放在 items / itemList，按相同结构尝试。
	if len(result.Items) == 0 {
		for _, key := range []string{"items", "itemList"} {
			rawList, _ := data[key].([]any)
			for _, rawEntry := range rawList {
				entry, _ := rawEntry.(map[string]any)
				if entry == nil {
					continue
				}
				item := searchItemFromResultEntry(entry)
				if item.ItemID != "" || item.Title != "" {
					result.Items = append(result.Items, item)
				}
			}
			if len(result.Items) > 0 {
				break
			}
		}
	}
	return result
}

// searchItemFromResultEntry 解析单条结果：兼容 resultList[].data.item.main 与直接卡片两种形态。
func searchItemFromResultEntry(entry map[string]any) SearchItem {
	item := SearchItem{}
	// main 是主数据节点：resultList[].data.item.main。
	main := entry
	if inner, ok := entry["data"].(map[string]any); ok {
		if itemNode, ok := inner["item"].(map[string]any); ok {
			if mainNode, ok := itemNode["main"].(map[string]any); ok {
				main = mainNode
			}
		}
	}
	exContent, _ := main["exContent"].(map[string]any)
	clickParam, _ := main["clickParam"].(map[string]any)

	if exContent != nil {
		item.ItemID = searchString(exContent["itemId"])
		item.Title = searchString(exContent["title"])
		item.ImageURL = normalizeImageURL(searchString(exContent["picUrl"]))
		item.Seller = searchString(exContent["userNickName"])
		item.Area = searchString(exContent["area"])
		item.Price = searchPriceFromExContent(exContent["price"])
	}
	// clickParam.args 里的价格与 itemId 是兜底来源。
	if clickParam != nil {
		if args, ok := clickParam["args"].(map[string]any); ok {
			if item.ItemID == "" {
				item.ItemID = searchString(args["itemId"])
			}
			if item.Price == "" {
				item.Price = searchString(args["price"])
			}
		}
	}
	if item.ItemID != "" {
		item.ItemURL = "https://www.goofish.com/item?id=" + item.ItemID
	}
	return item
}

// searchPriceFromExContent 从 exContent.price 数组中取整数价格文本。
// price 形如 [{"type":"integer","text":"160"},{"type":"decimal","text":".5"}]。
func searchPriceFromExContent(raw any) string {
	priceList, _ := raw.([]any)
	var integerPart, decimalPart string
	for _, rawPrice := range priceList {
		price, _ := rawPrice.(map[string]any)
		if price == nil {
			continue
		}
		switch searchString(price["type"]) {
		case "integer":
			integerPart = searchString(price["text"])
		case "decimal":
			decimalPart = strings.TrimPrefix(searchString(price["text"]), ".")
		}
	}
	if integerPart != "" && decimalPart != "" {
		return integerPart + "." + decimalPart
	}
	return integerPart
}

// searchString 把 JSON 字段安全转成字符串（数字也接受）。
func searchString(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// normalizeImageURL 补全协议头：平台常返回 //img.alicdn.com/... 形式。
func normalizeImageURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	if strings.HasPrefix(u, "http://") {
		return "https://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

// SearchItems 保留商品工作流的既有接口，关键词搜索不受卖家模式扩展影响。
func (c *ClientImpl) SearchItems(ctx context.Context, cookiesStr, keyword string, page, pageSize int) (*SearchResult, error) {
	return c.SearchOpportunityItems(ctx, cookiesStr, keyword, page, pageSize)
}
