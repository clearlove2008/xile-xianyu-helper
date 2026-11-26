// Package mtop: 换主域 — 上传新图并替换已上架商品的指定图片，走 idleitem.edit 提交。
//
// 用于「旧店名图片批量换新」这类一次性运维：保留商品标题/价格/描述/库存等全部字段，
// 仅把 imageInfoDOList 中指定下标的图片 url/width/height 换成新上传的图片。
package mtop

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// ReplaceImageResult 是一次换图的结果。
type ReplaceImageResult struct {
	ItemID   string
	Index    int
	NewURL   string
	Ret      []string
	Applied  bool
}

// UploadedImageInfo 是上传图片后可用于回填 imageInfoDOList 的最小信息。
type UploadedImageInfo struct {
	URL    string
	Width  int
	Height int
}

// UploadItemImage 把本地图片上传到闲鱼 CDN,返回可直接回填的 url 与尺寸。
func (c *ClientImpl) UploadItemImage(ctx context.Context, cookiesStr string, img PublishImage) (*UploadedImageInfo, string, error) {
	if len(img.Data) == 0 {
		return nil, cookiesStr, errors.New("图片数据为空")
	}
	if img.Filename == "" {
		img.Filename = "image.png"
	}
	if img.ContentType == "" {
		img.ContentType = "image/png"
	}
	uploaded, updated, err := c.uploadPublishImage(ctx, cookiesStr, img)
	if err != nil {
		return nil, updated, err
	}
	if uploaded.URL == "" {
		return nil, updated, errors.New("图片上传成功但缺少 url")
	}
	return &UploadedImageInfo{URL: uploaded.URL, Width: uploaded.Width, Height: uploaded.Height}, updated, nil
}

// BuildReplaceImagePayload 构造换图提交体：复制编辑详情，只替换 imageInfoDOList[index] 的图片信息。
// apply=false 时返回构造好的 payload 供 dry-run 检查，不做任何写操作。
func (c *ClientImpl) BuildReplaceImagePayload(detail *ItemEditDetail, index int, img *UploadedImageInfo, quantity int) (map[string]any, error) {
	if detail == nil {
		return nil, errors.New("编辑详情为空")
	}
	if img == nil || img.URL == "" {
		return nil, errors.New("新图片信息为空")
	}
	payload := buildReshelfPayload(detail, quantity)
	list, ok := payload["imageInfoDOList"].([]any)
	if !ok {
		return nil, fmt.Errorf("imageInfoDOList 类型异常: %T", payload["imageInfoDOList"])
	}
	if index < 0 || index >= len(list) {
		return nil, fmt.Errorf("图片下标越界: index=%d len=%d", index, len(list))
	}
	entry, ok := list[index].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("图片 %d 结构异常: %T", index, list[index])
	}
	// 只替换 url 与尺寸，保留 major/type/status/extraInfo 等平台要求的原字段。
	entry["url"] = img.URL
	if img.Width > 0 {
		entry["widthSize"] = strconv.Itoa(img.Width)
	}
	if img.Height > 0 {
		entry["heightSize"] = strconv.Itoa(img.Height)
	}
	newList := make([]any, len(list))
	copy(newList, list)
	newList[index] = entry
	payload["imageInfoDOList"] = newList
	return payload, nil
}

// DropPayloadFields 从提交体中剔除指定字段，用于绕过新版编辑器字段导致的 PC edit 拒绝。
func (c *ClientImpl) DropPayloadFields(payload map[string]any, keys []string) {
	for _, key := range keys {
		delete(payload, key)
	}
}

// SubmitEditPayload 提交一个已构造好的 edit payload。仅由换图工具显式调用。
func (c *ClientImpl) SubmitEditPayload(ctx context.Context, cookiesStr, itemID string, payload map[string]any) ([]string, error) {
	if itemID == "" {
		return nil, errors.New("商品 ID 不能为空")
	}
	decoded, _, err := c.callMTop(ctx, cookiesStr, ItemEditAPI, "mtop.idle.pc.idleitem.edit", "1.0",
		"a21ybx.publish.0.0", "a21ybx.home.sidebar.1.46413da6EPl7v5", "46413da6EPl7v5", payload)
	if err != nil {
		return nil, err
	}
	ret := retFromDecoded(decoded)
	if !hasMTopSuccess(ret) {
		return nil, c.mtopResponseFailure("mtop.idle.pc.idleitem.edit", 200, ret, "平台 ret 未包含 SUCCESS")
	}
	return ret, nil
}
