package mtop

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// searchSellerItems 参考 pyxianyu 的公开主页协议，使用现有签名与账号节流链路；不刷新 Token 或自动重试。
func (c *ClientImpl) searchSellerItems(ctx context.Context, cookies, seller string, page int) (*SearchResult, error) {
	if !regexp.MustCompile(`^[0-9]{1,24}$`).MatchString(seller) {
		return nil, fmt.Errorf("卖家用户 ID 无效")
	}
	if page < 1 {
		page = 1
	}
	if page > 50 {
		return nil, fmt.Errorf("单次卖家浏览最多 50 页，请缩小采集范围")
	}
	// payload 使用公开主页的分页参数，不复用当前登录账号的固定分组 ID。
	payload := map[string]any{"needGroupInfo": false, "pageNumber": page, "userId": seller, "pageSize": 20}
	// decoded、updated、status、err 是一次有节流的读取结果，凭证只留在调用链内。
	decoded, updated, status, err := c.callMTopOnce(ctx, cookies, ItemListAPI, "mtop.idle.web.xyh.item.list", "1.0", "a21ybx.personal.0.0", "a21ybx.home.0.0", strconv.FormatInt(time.Now().UnixMilli(), 10), payload)
	if err != nil {
		return nil, err
	}
	if !hasMTopSuccess(retFromDecoded(decoded)) {
		return nil, c.mtopResponseFailure("卖家商品列表", status, retFromDecoded(decoded), "卖家商品列表未返回成功")
	}
	// result 仅返回明确在售且属于该卖家的卡片，空列表是有效响应。
	result := parseSellerSearchResponse(decoded, seller)
	result.Page, result.PageSize, result.UpdatedCookies = page, 20, updated
	return result, nil
}

// parseSellerSearchResponse 过滤已售出、非商品卡片与重复 ID，并保留平台分页状态。
func parseSellerSearchResponse(decoded map[string]any, seller string) *SearchResult {
	// result 初始化为空数组，保证 HTTP 响应不会生成 null 商品列表。
	result := &SearchResult{Items: []SearchItem{}}
	// data、cards 是平台公开商品卡片容器，缺失时按空页处理。
	data, _ := decoded["data"].(map[string]any)
	cards, _ := data["cardList"].([]any)
	result.HasMore, _ = data["nextPage"].(bool)
	result.Total = mtopInt(data["totalCount"])
	// seen 对单页置顶重复卡片去重。
	seen := map[string]bool{}
	// raw 是待校验的商品卡片。
	for _, raw := range cards {
		// card、item 将未知类型安全地转为字段容器。
		card, _ := raw.(map[string]any)
		item, _ := card["cardData"].(map[string]any)
		if mtopInt(card["cardType"]) != 1003 || item["itemStatus"] == nil || mtopInt(item["itemStatus"]) != 0 {
			continue
		}
		// owner 在平台提供归属字段时防止误收推荐商品。
		owner := mtopString(item["userId"])
		if owner != "" && owner != seller {
			continue
		}
		// parsed 复用现有图片、价格和商品 ID 解析器。
		parsed := parseItemList(map[string]any{"cardList": []any{raw}})
		if len(parsed) != 1 || seen[parsed[0].ID] {
			continue
		}
		// entry 是已校验的唯一在售商品。
		entry := parsed[0]
		seen[entry.ID] = true
		if strings.HasPrefix(entry.PicURL, "//") {
			entry.PicURL = "https:" + entry.PicURL
		}
		result.Items = append(result.Items, SearchItem{ItemID: entry.ID, Title: entry.Title, Price: entry.Price, ImageURL: entry.PicURL, ItemURL: entry.WebURL, Seller: seller})
	}
	return result
}
