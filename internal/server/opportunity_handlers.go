package server

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"xianyu-go/internal/httpapi"
	"xianyu-go/internal/xianyu/mtop"
)

// opportunitySearchResponse 是商机搜索的 HTTP 响应。
type opportunitySearchResponse struct {
	// Success 表示搜索是否成功。
	Success bool `json:"success"`
	// Items 是商品列表。
	Items []opportunitySearchItem `json:"items"`
	// Page 是当前页码。
	Page int `json:"page"`
	// PageSize 是本页条数。
	PageSize int `json:"pageSize"`
	// Total 是平台报告的总命中数。
	Total int `json:"total"`
	// HasMore 表示是否还有下一页。
	HasMore bool `json:"hasMore"`
}

// opportunitySearchItem 是一条搜索结果的 HTTP 模型。
type opportunitySearchItem struct {
	ItemID   string `json:"itemId"`
	Title    string `json:"title"`
	Price    string `json:"price"`
	ImageURL string `json:"imageUrl"`
	ItemURL  string `json:"itemUrl"`
	Seller   string `json:"seller"`
	Area     string `json:"area"`
}

// searchOpportunities 用指定账号按关键词搜索闲鱼在售商品（商机挖掘）。
// Cookie 明文只在本 handler 与平台调用链之间流转，不进入日志与响应。
func (s *Server) searchOpportunities(w http.ResponseWriter, r *http.Request) {
	// keyword 是搜索关键词。
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	// seller 接受数字 ID 或闲鱼主页链接，不请求用户提供的网址。
	seller, err := opportunitySellerID(r.URL.Query().Get("seller"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if keyword == "" && seller == "" {
		writeErr(w, http.StatusBadRequest, "搜索关键词不能为空")
		return
	}
	if seller == "" && len([]rune(keyword)) > 50 {
		writeErr(w, http.StatusBadRequest, "关键词过长（最多 50 字）")
		return
	}
	// cookieID 是执行搜索的账号凭证 ID。
	cookieID := strings.TrimSpace(r.URL.Query().Get("cookie_id"))
	if cookieID == "" {
		writeErr(w, http.StatusBadRequest, "请选择搜索账号")
		return
	}
	// cookies 已完成归属校验的平台 Cookie 明文。
	cookies, _, ok := s.cookieForCurrentUser(w, r, cookieID)
	if !ok {
		return
	}
	// page、pageSize 是分页参数。
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	// client 是无状态的 MTOP 客户端，按请求创建。
	client := mtop.NewClient()
	result, err := client.SearchOpportunityItems(r.Context(), cookies, keyword, page, pageSize, seller)
	if err != nil {
		s.writeOpportunitySearchError(w, err)
		return
	}
	// items 是转换后的 HTTP 模型列表。
	items := make([]opportunitySearchItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, opportunitySearchItem{
			ItemID:   item.ItemID,
			Title:    item.Title,
			Price:    item.Price,
			ImageURL: item.ImageURL,
			ItemURL:  item.ItemURL,
			Seller:   item.Seller,
			Area:     item.Area,
		})
	}
	writeJSON(w, http.StatusOK, opportunitySearchResponse{
		Success:  true,
		Items:    items,
		Page:     result.Page,
		PageSize: result.PageSize,
		Total:    result.Total,
		HasMore:  result.HasMore,
	})
}

// writeOpportunitySearchError 把搜索失败映射为稳定的 HTTP 状态与中文提示。
func (s *Server) writeOpportunitySearchError(w http.ResponseWriter, err error) {
	var mtopErr *mtop.MTopResponseError
	if errors.As(err, &mtopErr) {
		switch mtopErr.Kind {
		case mtop.MTopErrorRiskVerification:
			writeErrCode(w, http.StatusServiceUnavailable, httpapi.CodeServiceUnavailable, "搜索触发平台风控，请稍后再试或换一个账号搜索", "")
			return
		case mtop.MTopErrorSessionExpired:
			writeErrCode(w, http.StatusConflict, httpapi.CodeConflict, "账号登录已失效，请到账号管理重新登录", "")
			return
		case mtop.MTopErrorTokenExpired:
			writeErrCode(w, http.StatusConflict, httpapi.CodeConflict, "账号签名凭证已过期，请到账号管理重新登录后再搜索", "")
			return
		}
	}
	writeErrCode(w, http.StatusBadGateway, httpapi.CodeBadGateway, "搜索失败："+err.Error(), "")
}

// opportunitySellerID 只解析卖家 ID，不跟随短链接，避免向任意用户网址发送请求。
func opportunitySellerID(input string) (string, error) {
	// value 是用户输入的 ID 或主页地址。
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if len(value) > 2048 {
		return "", errors.New("卖家链接过长")
	}
	// digits 限定平台数字 ID，不接受昵称或商品 ID 链接。
	digits := regexp.MustCompile(`^[0-9]{1,24}$`)
	if digits.MatchString(value) {
		return value, nil
	}
	// parsed 只允许闲鱼主页及手机分享页，防止把其他网站的 userId 当作卖家。
	parsed, err := url.Parse(value)
	if err != nil {
		return "", errors.New("请输入卖家数字 ID 或完整闲鱼主页链接")
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || (parsed.Hostname() != "www.goofish.com" && parsed.Hostname() != "goofish.com" && parsed.Hostname() != "pages.goofish.com" && parsed.Hostname() != "h5.m.goofish.com") {
		return "", errors.New("请先打开分享短链接，再复制闲鱼主页地址或数字用户 ID")
	}
	// id 从主页查询参数提取；商品链接的 id 参数不会被接受。
	id := parsed.Query().Get("userId")
	if id == "" {
		id = parsed.Query().Get("userid")
	}
	if !digits.MatchString(id) {
		return "", errors.New("链接中没有有效的 userId，请复制卖家主页链接")
	}
	return id, nil
}
