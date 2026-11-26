// Package mtop: 重新上架域 — 读商品编辑详情并提交 mtop.idle.pc.idleitem.edit。
//
// 闲鱼没有独立的「重新上架」接口：PC 编辑页打开发布页时先调 editDetail 取回可编辑数据，
// 再原样回填一次 edit 即完成重发布。售罄下架的商品同样走这条链路。
package mtop

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	// ItemEditDetailAPI 是 PC 编辑页读取商品可编辑数据的端点。
	ItemEditDetailAPI = "https://h5api.m.goofish.com/h5/mtop.idle.pc.idleitem.editDetail/1.0/"
	// ItemEditAPI 是 PC 编辑页提交商品修改的端点；带 itemId 时即为重发布。
	ItemEditAPI = "https://h5api.m.goofish.com/h5/mtop.idle.pc.idleitem.edit/1.0/"
)

// 编辑详情里的响应专用字段：只用于页面渲染，回填时不能提交给 edit 接口。
var itemEditDetailResponseOnlyKeys = []string{
	"redirectUrl", "jumpUrl", "asyncSecurityInfo", "itemStatus", "bizcode", "bucketId", "scene",
}

// ItemEditDetail 是商品编辑页的可编辑数据快照。
type ItemEditDetail struct {
	// ItemID 是平台商品标识。
	ItemID string
	// ItemStatus 是平台返回的商品状态原文，仅用于诊断，不回填。
	ItemStatus string
	// Title 是商品标题，便于日志与审计展示。
	Title string
	// PriceCents 是商品售价（分）。
	PriceCents int64
	// Quantity 是平台当前库存；平台未下发时为 0。
	Quantity int
	// Raw 是平台原始数据，用于原样回填构造 edit 请求。
	Raw map[string]any
}

// ReshelfResult 是一次重新上架的结果。
type ReshelfResult struct {
	// ItemID 是被重新上架的商品标识。
	ItemID string
	// Quantity 是本次提交的库存数量。
	Quantity int
	// Ret 是平台返回的业务标记。
	Ret []string
}

// FetchItemEditDetail 读取商品的编辑详情。
func (c *ClientImpl) FetchItemEditDetail(ctx context.Context, cookiesStr, itemID string) (*ItemEditDetail, error) {
	// itemID 为空时无法定位商品，提前返回避免无意义请求。
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, errors.New("商品 ID 不能为空")
	}
	decoded, _, err := c.callMTop(ctx, cookiesStr, ItemEditDetailAPI, "mtop.idle.pc.idleitem.editDetail", "1.0",
		"a21ybx.publish.0.0", "a21ybx.home.sidebar.1.46413da6EPl7v5", "46413da6EPl7v5",
		map[string]any{"itemId": itemID})
	if err != nil {
		return nil, err
	}
	// ret 是平台业务标记；非 SUCCESS 时不能把空数据当成有效详情。
	if ret := retFromDecoded(decoded); !hasMTopSuccess(ret) {
		return nil, c.mtopResponseFailure("mtop.idle.pc.idleitem.editDetail", 200, ret, "平台 ret 未包含 SUCCESS")
	}
	// data 是编辑详情主体。
	data := mapFromAny(decoded["data"])
	if len(data) == 0 {
		return nil, errors.New("商品编辑详情为空，可能商品已删除或无权访问")
	}
	// textDTO 保存标题与描述。
	textDTO := mapFromAny(data["itemTextDTO"])
	// priceDTO 保存价格（分）。
	priceDTO := mapFromAny(data["itemPriceDTO"])
	return &ItemEditDetail{
		ItemID:     firstNonEmpty(mtopString(data["itemId"]), itemID),
		ItemStatus: mtopString(data["itemStatus"]),
		Title:      mtopString(textDTO["title"]),
		PriceCents: mtopInt64(priceDTO["priceInCent"]),
		Quantity:   mtopInt(data["quantity"]),
		Raw:        data,
	}, nil
}

// ReshelfItem 重新上架指定商品。
// quantity > 0 时按该库存提交，否则沿用编辑详情里的库存；两者都没有时补 1 件。
func (c *ClientImpl) ReshelfItem(ctx context.Context, cookiesStr, itemID string, quantity int) (*ReshelfResult, error) {
	// detail 是回填用的编辑详情。
	detail, err := c.FetchItemEditDetail(ctx, cookiesStr, itemID)
	if err != nil {
		return nil, err
	}
	// payload 是按平台要求补齐标识字段后的提交体。
	payload := buildReshelfPayload(detail, quantity)
	decoded, _, err := c.callMTop(ctx, cookiesStr, ItemEditAPI, "mtop.idle.pc.idleitem.edit", "1.0",
		"a21ybx.publish.0.0", "a21ybx.home.sidebar.1.46413da6EPl7v5", "46413da6EPl7v5", payload)
	if err != nil {
		return nil, err
	}
	// ret 是提交结果标记。
	ret := retFromDecoded(decoded)
	if !hasMTopSuccess(ret) {
		return nil, c.mtopResponseFailure("mtop.idle.pc.idleitem.edit", 200, ret, "平台 ret 未包含 SUCCESS")
	}
	return &ReshelfResult{ItemID: detail.ItemID, Quantity: quantity, Ret: ret}, nil
}

// buildReshelfPayload 把编辑详情转换成 edit 提交体。
// 平台要求原样回填，因此只剔除纯响应字段，其余字段保持不变。
func buildReshelfPayload(detail *ItemEditDetail, quantity int) map[string]any {
	// payload 复制一份编辑详情，避免修改调用方持有的原始数据。
	payload := make(map[string]any, len(detail.Raw)+5)
	for key, value := range detail.Raw {
		payload[key] = value
	}
	for _, key := range itemEditDetailResponseOnlyKeys {
		delete(payload, key)
	}
	// itemID 是本次编辑的目标商品。
	itemID := firstNonEmpty(detail.ItemID, mtopString(payload["itemId"]))
	payload["itemId"] = itemID
	// 平台要求每次提交带唯一编码与来源，缺省时按 itemId 回填。
	payload["uniqueCode"] = strconv.FormatInt(time.Now().UnixMicro(), 10)
	payload["sourceId"] = itemID
	payload["bizcode"] = "pcMainPublish"
	payload["publishScene"] = "pcMainPublish"

	// 库存：显式传参优先，其次沿用详情，最后按 1 件兜底。
	finalQuantity := quantity
	if finalQuantity <= 0 {
		finalQuantity = detail.Quantity
	}
	if finalQuantity <= 0 {
		finalQuantity = 1
	}
	payload["quantity"] = strconv.Itoa(finalQuantity)
	return payload
}

// IsMultiInventoryRejected 判断错误是否为平台「账号不允许多库存商品」的确定性拒绝。
// 该限制对发布与编辑接口同时生效（2026-11-17 喜乐API 账号实测：edit 提交 quantity>1
// 同样返回 FAIL_BIZ_ITEM_ERROR_MULTI_INVENTORY_ITEM_CAN_NOT_PUBLISH），且因账号而异
// （同载荷在喜乐程序员账号可成功）。命中时调用方不应重试原数量，应按 1 件降级。
func IsMultiInventoryRejected(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "multi_inventory_item_can_not_publish") ||
		strings.Contains(text, "不能发布多库存宝贝")
}

// mtopInt64 把平台返回的字符串或数字金额转换成 int64；无法识别时返回 0。
func mtopInt64(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0
		}
		return n
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// SanitizeReshelfFailure 把重新上架失败原因压缩成一行可审计文本。
func SanitizeReshelfFailure(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return text
}

// DebugReshelfPayload 返回构造出的提交体，供本地探测程序查看字段；生产路径不依赖它。
func DebugReshelfPayload(detail *ItemEditDetail, quantity int) map[string]any {
	return buildReshelfPayload(detail, quantity)
}
