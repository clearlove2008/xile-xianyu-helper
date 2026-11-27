// Command shoprobe 只读探测闲鱼店铺商品分组与已售出商品，用于确认重新上架的可行性。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"xianyu-go/internal/db"
	"xianyu-go/internal/relister"
	"xianyu-go/internal/xianyu/mtop"
	"xianyu-go/internal/xianyu/protocol"
	"xianyu-go/internal/xianyu"
)

const (
	dataKeyPath = `C:\ProgramData\XileXianyuHelper\data-key`
	dbPath      = `C:\ProgramData\XileXianyuHelper\data\xianyu_data.db`
)

func main() {
	key, err := os.ReadFile(dataKeyPath)
	if err != nil {
		fmt.Println("读取 data-key 失败:", err)
		return
	}
	os.Setenv("XIANYU_DATA_KEY", strings.TrimSpace(string(key)))

	raw, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Println("打开数据库失败:", err)
		return
	}
	defer raw.Close()

	store := db.NewStore(raw, db.DialectSQLite)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rows, err := raw.QueryContext(ctx, `SELECT id, COALESCE(nickname,'') FROM cookies ORDER BY id`)
	if err != nil {
		fmt.Println("查询账号失败:", err)
		return
	}
	type account struct{ id, nick string }
	var accounts []account
	for rows.Next() {
		var a account
		if err := rows.Scan(&a.id, &a.nick); err != nil {
			continue
		}
		accounts = append(accounts, a)
	}
	rows.Close()

	client := mtop.NewClient()
	// SEARCH_ITEM 用第一个有凭证的账号直调一次商机搜索接口，验证移植的搜索链路。
	if keyword := os.Getenv("SEARCH_ITEM"); keyword != "" {
		for _, a := range accounts {
			cookie, cerr := store.Cookies.GetValue(ctx, a.id)
			if cerr != nil || strings.TrimSpace(cookie) == "" {
				continue
			}
			fmt.Printf("== 账号 %s (%s) 搜索「%s」\n", a.id, a.nick, keyword)
			result, serr := client.SearchItems(ctx, cookie, keyword, 1, 10)
			if serr != nil {
				fmt.Printf("搜索失败: %v\n", serr)
				return
			}
			fmt.Printf("结果 %d 条:\n", len(result.Items))
			for i, item := range result.Items {
				fmt.Printf("  %d. [%s] %s ¥%s 卖家:%s 地区:%s\n", i+1, item.ItemID, truncate(item.Title, 30), item.Price, truncate(item.Seller, 12), item.Area)
				if i >= 4 {
					break
				}
			}
			return
		}
		fmt.Println("没有可用账号凭证")
		return
	}
	// SCAN=1 时用真实平台跑一轮巡检（只读分组与商品；无售罄商品时不会上架任何东西）。
	if os.Getenv("SCAN") == "1" {
		scanner := relister.New(store, client, slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
		outcomes := scanner.ScanOnce(ctx)
		fmt.Printf("巡检结果: %d 条\n", len(outcomes))
		for _, o := range outcomes {
			fmt.Printf("   %s %s [%s] %s %s\n", o.AccountID, o.ItemID, o.GroupName, o.Action, o.Detail)
		}
		return
	}
	for _, a := range accounts {
		cookie, err := store.Cookies.GetValue(ctx, a.id)
		if err != nil || strings.TrimSpace(cookie) == "" {
			fmt.Printf("== %s(%s) 取凭证失败: %v\n", a.id, a.nick, err)
			continue
		}
		fmt.Printf("== 账号 %s (%s)\n", a.id, a.nick)
		// DUMP_ITEM_LIST 指定的账号复刻一次带分组信息的 item.list 请求，打印平台原始响应，
		// 用于排查「部分账号拿不到 itemGroupList」的分组解析问题。
		if dumpAccount := os.Getenv("DUMP_ITEM_LIST"); dumpAccount == a.id {
			raw, derr := dumpItemList(ctx, cookie)
			if derr != nil {
				fmt.Printf("   item.list 原始请求失败: %v\n", derr)
			} else {
				fmt.Printf("   item.list 原始响应: %s\n", truncate(raw, 2<<20))
			}
		}
		groups, err := client.FetchItemGroups(ctx, cookie)
		if err != nil {
			fmt.Printf("   分组拉取失败: %v\n", err)
			continue
		}
		for _, g := range groups {
			fmt.Printf("   分组 %-6s id=%-10s type=%d 数量=%d default=%v\n", g.GroupName, g.GroupID, g.GroupType, g.ItemNumber, g.Default)
		}
		// PUBLISH_ACCOUNT 指定的账号执行一次带默认发货地的真实发布，用于验证
		// INVALID_MAP_LOCATION 修复；标题与价格都按测试商品约定，验证后需人工删除。
		if publishAccount := os.Getenv("PUBLISH_ACCOUNT"); publishAccount == a.id && os.Getenv("PUBLISH_LOC_TEST") == "1" {
			// imageData 是测试主图；类目预测与上传由 PublishItem 内部完成。
			imageData, _ := os.ReadFile("C:/Users/Administrator/WorkBuddy/2026-11-16-10-58-52/test-cover.png")
			loc := &mtop.PublishLocation{
				Province: "云南", City: "昆明", Area: "官渡区", DivisionID: "530111",
				Latitude: 24.950285, Longitude: 102.748888,
			}
			res, perr := client.PublishItem(ctx, cookie, mtop.PublishItemRequest{
				Title: "系统自动化测试占用请勿购买-发货地验证", Description: "系统自动化测试占用请勿购买-发货地验证",
				PriceCents: 99900, Quantity: 1, PostageMode: "free", Virtual: true,
				Location: loc,
				Images: []mtop.PublishImage{{Filename: "cover.png", ContentType: "image/png", Data: imageData}},
			})
			if perr != nil {
				fmt.Printf("   测试发布失败: %v\n", perr)
			} else {
				fmt.Printf("   测试发布成功: itemId=%s price=%s quantity=%d\n", res.ItemID, res.PriceText, res.Quantity)
			}
		}
		// 取样一个在售商品的编辑详情，确认重发布所需字段是否齐全。
		if probeItem := os.Getenv("PROBE_ITEM"); probeItem != "" {
			detail, derr := client.FetchItemEditDetail(ctx, cookie, probeItem)
			if derr != nil {
				fmt.Printf("   editDetail 失败: %v\n", derr)
			} else {
				fmt.Printf("   editDetail 成功: itemId=%s status=%q title=%q price=%d分 quantity=%d 字段数=%d\n",
					detail.ItemID, detail.ItemStatus, truncate(detail.Title, 20), detail.PriceCents, detail.Quantity, len(detail.Raw))
				keys := make([]string, 0, len(detail.Raw))
				for k := range detail.Raw {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				fmt.Printf("   字段: %s\n", strings.Join(keys, ","))
				if addr, ok := detail.Raw["itemAddrDTO"].(map[string]any); ok {
					b, _ := json.Marshal(addr)
					fmt.Printf("   itemAddrDTO=%s\n", string(b))
				}
				for _, lk := range []string{"divisionId", "area", "city", "prov", "latitude", "longitude", "poiId", "poiName", "location", "address"} {
					if v, ok := detail.Raw[lk]; ok {
						fmt.Printf("   loc %s=%v\n", lk, v)
					}
				}
			}
		}
		// 列出在售商品，便于挑选测试用商品。
		if probeItem := os.Getenv("LIST_ON_SALE"); probeItem != "" {
			for _, g := range groups {
				if g.GroupName != "在售" {
					continue
				}
				res, lerr := client.FetchItemsInGroup(ctx, cookie, g.GroupID, g.GroupName, 1, 20)
				if lerr != nil {
					fmt.Printf("   在售列表失败: %v\n", lerr)
					break
				}
				for _, it := range res.Items {
					fmt.Printf("   在售 id=%s status=%d price=%s title=%s\n", it.ID, it.ItemStatus, it.Price, truncate(it.Title, 26))
				}
			}
		}
		// 重新上架探测：只在显式给出商品 ID 时执行，属于写操作。
		if reshelfItem := os.Getenv("RESHELF_ITEM"); reshelfItem != "" && os.Getenv("RESHELF_ACCOUNT") == a.id {
			qty, _ := strconv.Atoi(os.Getenv("RESHELF_QTY"))
			detail, derr := client.FetchItemEditDetail(ctx, cookie, reshelfItem)
			if derr != nil {
				fmt.Printf("   重上架前置 editDetail 失败: %v\n", derr)
			} else {
				payload := mtop.DebugReshelfPayload(detail, qty)
				fmt.Printf("   将提交 %d 个字段，quantity=%v\n", len(payload), payload["quantity"])
				result, rerr := client.ReshelfItem(ctx, cookie, reshelfItem, qty)
				if rerr != nil {
					fmt.Printf("   重新上架失败: %v\n", rerr)
				} else {
					fmt.Printf("   重新上架成功: itemId=%s quantity=%d ret=%v\n", result.ItemID, result.Quantity, result.Ret)
				}
			}
		}
		// 逐个非在售分组取样商品，观察 itemStatus 与标题。
		for _, g := range groups {
			if g.GroupName == "在售" || g.GroupName == "综合" || g.ItemNumber == 0 {
				continue
			}
			res, err := client.FetchItemsInGroup(ctx, cookie, g.GroupID, g.GroupName, 1, 20)
			if err != nil {
				fmt.Printf("   [%s] 商品拉取失败: %v\n", g.GroupName, err)
				continue
			}
			fmt.Printf("   [%s] 取到 %d 件（平台总量 %d）\n", g.GroupName, len(res.Items), res.TotalCount)
			for i, it := range res.Items {
				if i >= 6 {
					break
				}
				fmt.Printf("      id=%s status=%d price=%s title=%s\n", it.ID, it.ItemStatus, it.Price, truncate(it.Title, 28))
			}
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// dumpItemList 复刻 mtop 包的 item.list 请求（needGroupInfo=true），返回平台原始 JSON。
// 参数与 fetchItemsPageOnce 保持一致，便于逐字节对照排查。
func dumpItemList(ctx context.Context, cookie string) (string, error) {
	cookies := protocol.TransCookies(cookie)
	userID := cookies["unb"]
	if userID == "" {
		return "", fmt.Errorf("cookie 缺少 unb 字段")
	}
	groupID := os.Getenv("DUMP_GROUP_ID")
	if groupID == "" {
		groupID = "58877261"
	}
	groupName := os.Getenv("DUMP_GROUP_NAME")
	if groupName == "" {
		groupName = "在售"
	}
	data := map[string]any{
		"needGroupInfo": true,
		"pageNumber":    1,
		"pageSize":      20,
		"groupName":     groupName,
		"groupId":       groupID,
		"defaultGroup":  true,
		"userId":        userID,
	}
	rawData, _ := json.Marshal(data)
	dataVal := string(rawData)
	t := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sign := protocol.GenerateSign(t, protocol.SignToken(cookie), dataVal)
	query := "jsv=2.7.2&appKey=" + protocol.SignAppKey + "&t=" + t + "&sign=" + url.QueryEscape(sign) +
		"&v=1.0&type=originaljson&accountSite=xianyu&dataType=json&timeout=20000" +
		"&api=mtop.idle.web.xyh.item.list&sessionOption=AutoLoginOnly&spm_cnt=a21ybx.im.0.0&spm_pre=a21ybx.collection.menu.1.272b5141NafCNK"
	body := "data=" + url.QueryEscape(dataVal)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mtop.ItemListAPI+"?"+query, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	h := req.Header
	h.Set("accept", "application/json")
	h.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8")
	h.Set("cache-control", "no-cache")
	h.Set("content-type", "application/x-www-form-urlencoded")
	h.Set("pragma", "no-cache")
	h.Set("priority", "u=1, i")
	xianyu.ApplyBrowserFingerprint(h)
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-site")
	h.Set("referer", "https://www.goofish.com/")
	h.Set("origin", "https://www.goofish.com")
	h.Set("cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
