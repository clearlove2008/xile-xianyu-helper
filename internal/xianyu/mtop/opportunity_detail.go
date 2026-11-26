package mtop

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// opportunityMaxImages 与发布接口单商品图片上限保持一致。
const opportunityMaxImages = 9

// OpportunityDetail 保存详情接口原文与全部商品图，不使用搜索卡片标题或摘要作为描述兜底。
type OpportunityDetail struct {
	ItemID      string `json:"itemId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// ImageURLs 是详情 imageInfos 中的完整图片地址（主图在前，已补协议头并去重）。
	ImageURLs []string `json:"imageUrls"`
}

// FetchOpportunityDetail 只读一次公开商品详情，复用签名和节流；风控或详情缺失时阻止发布。
func (c *ClientImpl) FetchOpportunityDetail(ctx context.Context, cookies, itemID string) (*OpportunityDetail, error) {
	if !regexp.MustCompile(`^[0-9]{1,24}$`).MatchString(itemID) {
		return nil, fmt.Errorf("商品 ID 无效")
	}
	// decoded、status、err 为公开详情请求结果，不向日志暴露原始响应和凭证。
	decoded, _, status, err := c.callMTopOnce(ctx, cookies, "https://h5api.m.goofish.com/h5/mtop.taobao.idle.pc.detail/1.0/", "mtop.taobao.idle.pc.detail", "1.0", "a21ybx.item.0.0", "a21ybx.personal.0.0", strconv.FormatInt(time.Now().UnixMilli(), 10), map[string]any{"itemId": itemID})
	if err != nil {
		return nil, err
	}
	if !hasMTopSuccess(retFromDecoded(decoded)) {
		return nil, c.mtopResponseFailure("商品详情", status, retFromDecoded(decoded), "无法读取完整商品描述")
	}
	return parseOpportunityDetail(decoded, itemID)
}

// parseOpportunityDetail 原样保留 desc 中的换行、空行、空格、标点和列表，不 trim、不省略或概括。
func parseOpportunityDetail(decoded map[string]any, itemID string) (*OpportunityDetail, error) {
	// data、item 是详情响应的正式商品对象。
	data, _ := decoded["data"].(map[string]any)
	item, _ := data["itemDO"].(map[string]any)
	// description 必须来自明确的全文字段，缺失时不可退回摘要。
	description, _ := item["desc"].(string)
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("未获取到商品完整描述，已停止上架；请检查原商品是否可访问")
	}
	// actual 用于拒绝返回了其他商品详情的异常响应。
	actual := mtopString(item["itemId"])
	if actual != "" && actual != itemID {
		return nil, fmt.Errorf("商品详情 ID 不匹配，已停止上架")
	}
	return &OpportunityDetail{
		ItemID: itemID, Title: mtopString(item["title"]), Description: description,
		ImageURLs: parseOpportunityImageURLs(item["imageInfos"]),
	}, nil
}

// parseOpportunityImageURLs 读取详情 imageInfos[].url，保持平台原顺序（主图在前）。
// 搜索卡片只提供一张 picUrl，完整图片列表只能来自详情接口。
func parseOpportunityImageURLs(raw any) []string {
	images := make([]string, 0)
	seen := make(map[string]struct{})
	infos, _ := raw.([]any)
	for _, rawInfo := range infos {
		info, ok := rawInfo.(map[string]any)
		if !ok {
			continue
		}
		url := normalizeImageURL(searchString(info["url"]))
		if !strings.HasPrefix(url, "https://") {
			continue
		}
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}
		images = append(images, url)
		if len(images) >= opportunityMaxImages {
			break
		}
	}
	return images
}
