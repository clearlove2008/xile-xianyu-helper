// Command imgreplace 把一批已上架商品的指定主图换成新上传的图片。
//
// 用法（在 xianyu-src 目录）：
//   IMGREPLACE_PLAN=plan.json go run ./cmd/imgreplace
//
// plan.json 格式：
//   {"dry_run":true, "account_id":"2222518039340", "items":[
//     {"item_id":"1102878729872","image_index":0,"file":"C:/.../modified/xxx.jpg"}]}
//
// dry_run=true 时只上传图片并构造 payload,不提交 edit；确认无误后改为 false 真实执行。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/mtop"
)

const (
	dataKeyPath = `C:\ProgramData\YdisksXianyuHelper\data-key`
	dbPath      = `C:\ProgramData\YdisksXianyuHelper\data\xianyu_data.db`
)

type planItem struct {
	ItemID     string `json:"item_id"`
	ImageIndex int    `json:"image_index"`
	File       string `json:"file"`
}

type plan struct {
	DryRun        bool       `json:"dry_run"`
	AccountID     string     `json:"account_id"`
	ForceQuantity int        `json:"force_quantity"`
	DropFields    []string   `json:"drop_fields"`
	Items         []planItem `json:"items"`
}

type result struct {
	ItemID   string `json:"item_id"`
	Status   string `json:"status"`
	NewURL   string `json:"new_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

func main() {
	planPath := os.Getenv("IMGREPLACE_PLAN")
	if planPath == "" {
		fmt.Println("请设置 IMGREPLACE_PLAN 指向计划 JSON")
		return
	}
	rawPlan, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Println("读取计划失败:", err)
		return
	}
	var p plan
	if err := json.Unmarshal(rawPlan, &p); err != nil {
		fmt.Println("解析计划失败:", err)
		return
	}
	key, err := os.ReadFile(dataKeyPath)
	if err != nil {
		fmt.Println("读取 data-key 失败:", err)
		return
	}
	os.Setenv("XIANYU_DATA_KEY", strings.TrimSpace(string(key)))

	rawDB, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Println("打开数据库失败:", err)
		return
	}
	defer rawDB.Close()
	store := db.NewStore(rawDB, db.DialectSQLite)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cookie, err := store.Cookies.GetValue(ctx, p.AccountID)
	if err != nil || strings.TrimSpace(cookie) == "" {
		fmt.Printf("取账号 %s 凭证失败: %v\n", p.AccountID, err)
		return
	}
	client := mtop.NewClient()
	results := make([]result, 0, len(p.Items))

	for _, it := range p.Items {
		r := result{ItemID: it.ItemID}
		data, rerr := os.ReadFile(it.File)
		if rerr != nil {
			r.Status, r.Detail = "failed", "读图失败: "+rerr.Error()
			results = append(results, r)
			continue
		}
		ext := strings.ToLower(filepath.Ext(it.File))
		ctype := "image/jpeg"
		if ext == ".png" {
			ctype = "image/png"
		}
		// 1) 读取编辑详情
		detail, derr := client.FetchItemEditDetail(ctx, cookie, it.ItemID)
		if derr != nil {
			r.Status, r.Detail = "failed", "editDetail 失败: "+derr.Error()
			results = append(results, r)
			continue
		}
		// 2) 上传新图
		uploaded, updated, uerr := client.UploadItemImage(ctx, cookie, mtop.PublishImage{
			Filename: "replacement" + ext, ContentType: ctype, Data: data,
		})
		if updated != "" {
			cookie = updated
		}
		if uerr != nil {
			r.Status, r.Detail = "failed", "上传图片失败: "+uerr.Error()
			results = append(results, r)
			continue
		}
		r.NewURL = uploaded.URL
		// 3) 构造换图 payload
		payload, berr := client.BuildReplaceImagePayload(detail, it.ImageIndex, uploaded, p.ForceQuantity)
		if berr != nil {
			r.Status, r.Detail = "failed", "构造 payload 失败: "+berr.Error()
			results = append(results, r)
			continue
		}
		if len(p.DropFields) > 0 {
			client.DropPayloadFields(payload, p.DropFields)
		}
		if p.DryRun {
			r.Status = "dry_run_ok"
			b, _ := json.Marshal(payload["imageInfoDOList"])
			r.Detail = fmt.Sprintf("图片列表=%s", string(b))
			results = append(results, r)
			continue
		}
		// 4) 提交 edit（真实写操作）
		ret, serr := client.SubmitEditPayload(ctx, cookie, it.ItemID, payload)
		if serr != nil {
			r.Status, r.Detail = "failed", "提交 edit 失败: "+serr.Error()
			results = append(results, r)
			continue
		}
		r.Status = "ok"
		r.Detail = fmt.Sprintf("ret=%v", ret)
		results = append(results, r)
		fmt.Printf("[%s] 换图成功 url=%s\n", it.ItemID, uploaded.URL)
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
	_ = os.WriteFile(os.Getenv("IMGREPLACE_RESULT"), out, 0644)
}
