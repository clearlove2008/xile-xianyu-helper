package netdisk

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 夸克 clouddrive 公开网页 API。所有端点无需逆向，与浏览器请求一致。
const (
	quarkAPIBase       = "https://drive-pc.quark.cn/1/clouddrive"
	quarkSaveAPIBase   = "https://drive.quark.cn/1/clouddrive"
	quarkShareReferer  = "https://pan.quark.cn/"
	quarkRequiredParam = "pr=ucpro&fr=pc&uc_param_str="
)

// QuarkClient 实现夸克网盘的转存能力。Doer 可注入用于测试。
type QuarkClient struct {
	Doer Doer
}

// quarkResp 是夸克 API 的统一响应壳。
type quarkResp struct {
	Status  int    `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// quarkData 把 data 字段二次解码到目标结构。
func quarkData(raw any, out any) error {
	b, err := jsonBytes(raw)
	if err != nil {
		return err
	}
	return jsonUnmarshal(b, out)
}

// quarkParams 拼装夸克请求必需参数。
func quarkParams(extra map[string]string) string {
	q := quarkRequiredParam
	for k, v := range extra {
		q += "&" + k + "=" + url.QueryEscape(v)
	}
	q += fmt.Sprintf("&__t=%d", time.Now().UnixMilli())
	return q
}

// QuarkLoginCheck 校验 cookie 是否有效。
func (c *QuarkClient) QuarkLoginCheck(ctx context.Context, cookie string) (bool, error) {
	var resp quarkResp
	target := quarkAPIBase + "/config?" + quarkParams(nil)
	if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, quarkShareReferer), &resp); err != nil {
		return false, err
	}
	if resp.Status != 200 {
		return false, nil
	}
	return true, nil
}

// quarkStoken 换取分享访问令牌。需要提取码时 sourcePasscode 必填。
func (c *QuarkClient) quarkStoken(ctx context.Context, cookie, pwdID, passcode string) (string, error) {
	body, err := jsonBody(map[string]any{"pwd_id": pwdID, "passcode": passcode})
	if err != nil {
		return "", err
	}
	target := quarkAPIBase + "/share/sharepage/token?" + quarkParams(nil)
	var raw struct {
		Status  int    `json:"status"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Stoken string `json:"stoken"`
		} `json:"data"`
	}
	if err := doJSON(ctx, c.Doer, "POST", target, jsonHeader(cookie, quarkShareReferer), body, &raw); err != nil {
		return "", err
	}
	if raw.Status != 200 || raw.Data.Stoken == "" {
		return "", fmt.Errorf("获取 stoken 失败: %s（code=%d，可能需要正确提取码）", decodeUnicode(raw.Message), raw.Code)
	}
	return raw.Data.Stoken, nil
}

// quarkShareFile 是分享内的单个文件。
type quarkShareFile struct {
	FID           string `json:"fid"`
	ShareFIDToken string `json:"share_fid_token"`
	FileName      string `json:"file_name"`
}

// quarkListFiles 列出分享中的文件（第一页，最多 200 条）。
func (c *QuarkClient) quarkListFiles(ctx context.Context, cookie, pwdID, stoken string) ([]quarkShareFile, error) {
	target := quarkAPIBase + "/share/sharepage/detail?" + quarkParams(map[string]string{
		"pwd_id": pwdID, "stoken": stoken, "pdir_fid": "0", "force": "0",
		"_page": "1", "_size": "200", "_fetch_banner": "0", "_fetch_share": "0", "_sort": "file_name:asc",
	})
	var raw struct {
		Status int `json:"status"`
		Data   struct {
			List []struct {
				FID           string `json:"fid"`
				ShareFIDToken string `json:"share_fid_token"`
				FileName      string `json:"file_name"`
			} `json:"list"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, quarkShareReferer), &raw); err != nil {
		return nil, err
	}
	if raw.Status != 200 {
		return nil, fmt.Errorf("列分享文件失败: %s", decodeUnicode(raw.Message))
	}
	out := make([]quarkShareFile, 0, len(raw.Data.List))
	for _, f := range raw.Data.List {
		if f.FID == "" {
			continue
		}
		out = append(out, quarkShareFile{FID: f.FID, ShareFIDToken: f.ShareFIDToken, FileName: f.FileName})
	}
	return out, nil
}

// quarkSaveFiles 把分享文件转存到目标账号根目录，返回转存任务 ID。
func (c *QuarkClient) quarkSaveFiles(ctx context.Context, cookie, pwdID, stoken string, files []quarkShareFile) (string, error) {
	fids := make([]string, 0, len(files))
	tokens := make([]string, 0, len(files))
	for _, f := range files {
		fids = append(fids, f.FID)
		tokens = append(tokens, f.ShareFIDToken)
	}
	body, err := jsonBody(map[string]any{
		"fid_list":       fids,
		"fid_token_list": tokens,
		"to_pdir_fid":    "0",
		"pwd_id":         pwdID,
		"stoken":         stoken,
		"pdir_fid":       "0",
		"scene":          "link",
	})
	if err != nil {
		return "", err
	}
	target := quarkSaveAPIBase + "/share/sharepage/save?" + quarkParams(nil)
	var raw struct {
		Status  int    `json:"status"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := doJSON(ctx, c.Doer, "POST", target, jsonHeader(cookie, quarkShareReferer), body, &raw); err != nil {
		return "", err
	}
	if raw.Status != 200 {
		return "", fmt.Errorf("转存失败: %s（code=%d）", decodeUnicode(raw.Message), raw.Code)
	}
	// 部分响应无 task_id，转存已同步完成，用空串表示无需等待。
	return raw.Data.TaskID, nil
}

// quarkWaitTask 等待异步任务完成（最多约 30 秒）。
func (c *QuarkClient) quarkWaitTask(ctx context.Context, cookie, taskID string) error {
	if taskID == "" {
		return nil
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		target := quarkAPIBase + "/task?" + quarkParams(map[string]string{"task_id": taskID})
		var raw struct {
			Status int `json:"status"`
			Data   struct {
				Status int `json:"status"`
			} `json:"data"`
			Message string `json:"message"`
		}
		if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, quarkShareReferer), &raw); err != nil {
			return err
		}
		// status=2 完成，status=3 失败。
		if raw.Data.Status == 2 {
			return nil
		}
		if raw.Data.Status == 3 {
			return fmt.Errorf("转存任务失败: %s", decodeUnicode(raw.Message))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(800 * time.Millisecond):
		}
	}
	return fmt.Errorf("等待转存任务超时")
}

// quarkRootFiles 列出目标账号根目录文件，用于取回新转存文件的 fid。
func (c *QuarkClient) quarkRootFiles(ctx context.Context, cookie string) ([]quarkShareFile, error) {
	target := quarkAPIBase + "/file/sort?" + quarkParams(map[string]string{
		"pdir_fid": "0", "_page": "1", "_size": "200", "_fetch_banner": "0", "_fetch_share": "0", "_sort": "file_name:asc",
	})
	var raw struct {
		Status int `json:"status"`
		Data   struct {
			List []struct {
				FID      string `json:"fid"`
				FileName string `json:"file_name"`
			} `json:"list"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, quarkShareReferer), &raw); err != nil {
		return nil, err
	}
	if raw.Status != 200 {
		return nil, fmt.Errorf("列根目录失败: %s", decodeUnicode(raw.Message))
	}
	out := make([]quarkShareFile, 0, len(raw.Data.List))
	for _, f := range raw.Data.List {
		if f.FID != "" {
			out = append(out, quarkShareFile{FID: f.FID, FileName: f.FileName})
		}
	}
	return out, nil
}

// quarkCreateShare 用指定 fid 创建新分享，返回 share_id 与提取码。
func (c *QuarkClient) quarkCreateShare(ctx context.Context, cookie string, fids []string, title, passcode string, periodDays int) (string, string, error) {
	expiredType := 1
	switch periodDays {
	case 0:
		expiredType = 0
	case 1:
		expiredType = 1
	case 7:
		expiredType = 2
	case 30:
		expiredType = 3
	}
	if passcode == "" {
		passcode = randomPasscode(4)
	}
	body, err := jsonBody(map[string]any{
		"fid_list":     fids,
		"title":        title,
		"expired_type": expiredType,
		"passcode":     passcode,
		"use_passcode": true,
	})
	if err != nil {
		return "", "", err
	}
	target := quarkAPIBase + "/share?" + quarkParams(nil)
	var raw struct {
		Status  int    `json:"status"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ShareID string `json:"share_id"`
		} `json:"data"`
	}
	if err := doJSON(ctx, c.Doer, "POST", target, jsonHeader(cookie, quarkShareReferer), body, &raw); err != nil {
		return "", "", err
	}
	if raw.Status != 200 || raw.Data.ShareID == "" {
		return "", "", fmt.Errorf("创建分享失败: %s（code=%d）", decodeUnicode(raw.Message), raw.Code)
	}
	return raw.Data.ShareID, passcode, nil
}

// quarkShareURL 用 share_id 换取可访问链接。
func (c *QuarkClient) quarkShareURL(ctx context.Context, cookie, shareID string) (string, error) {
	body, err := jsonBody(map[string]any{"share_id": shareID})
	if err != nil {
		return "", err
	}
	target := quarkAPIBase + "/share/password?" + quarkParams(nil)
	var raw struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ShareURL string `json:"share_url"`
		} `json:"data"`
	}
	if err := doJSON(ctx, c.Doer, "POST", target, jsonHeader(cookie, quarkShareReferer), body, &raw); err != nil {
		return "", err
	}
	if raw.Status != 200 || raw.Data.ShareURL == "" {
		return "", fmt.Errorf("获取分享链接失败: %s", decodeUnicode(raw.Message))
	}
	return raw.Data.ShareURL, nil
}

// Transfer 执行夸克一键转存：取 stoken → 列文件 → 转存 → 建分享 → 取链接。
func (c *QuarkClient) Transfer(ctx context.Context, in ShareInput) (ShareResult, error) {
	result := ShareResult{Platform: "quark"}
	cookie := strings.TrimSpace(in.Cookie)
	if cookie == "" {
		return result, fmt.Errorf("夸克账号 cookie 为空")
	}
	pwdID, err := extractQuarkPwdID(in.ShareURL)
	if err != nil {
		return result, err
	}
	stoken, err := c.quarkStoken(ctx, cookie, pwdID, strings.TrimSpace(in.Passcode))
	if err != nil {
		return result, err
	}
	files, err := c.quarkListFiles(ctx, cookie, pwdID, stoken)
	if err != nil {
		return result, err
	}
	if len(files) == 0 {
		return result, fmt.Errorf("分享中没有可转存的文件")
	}
	taskID, err := c.quarkSaveFiles(ctx, cookie, pwdID, stoken, files)
	if err != nil {
		return result, err
	}
	if err := c.quarkWaitTask(ctx, cookie, taskID); err != nil {
		return result, err
	}

	// 根目录取回新文件 fid：按文件名匹配，取交集。
	rootFiles, err := c.quarkRootFiles(ctx, cookie)
	if err != nil {
		return result, err
	}
	names := make(map[string]bool, len(files))
	for _, f := range files {
		base := strings.TrimSuffix(f.FileName, ".url")
		names[base] = true
	}
	newFIDs := make([]string, 0, len(files))
	for _, rf := range rootFiles {
		if names[rf.FileName] || names[strings.TrimSuffix(rf.FileName, ".url")] {
			newFIDs = append(newFIDs, rf.FID)
		}
	}
	if len(newFIDs) == 0 {
		// 匹配不到时退化为用源 fid（部分账号转存后 fid 不变）。
		for _, f := range files {
			newFIDs = append(newFIDs, f.FID)
		}
	}

	title := files[0].FileName
	if len(files) > 1 {
		title = files[0].FileName + " 等 " + strconv.Itoa(len(files)) + " 个文件"
	}
	shareID, passcode, err := c.quarkCreateShare(ctx, cookie, newFIDs, title, strings.TrimSpace(in.NewPasscode), in.PeriodDays)
	if err != nil {
		return result, err
	}
	shareURL, err := c.quarkShareURL(ctx, cookie, shareID)
	if err != nil {
		return result, err
	}
	return ShareResult{
		ShareURL:  shareURL,
		Passcode:  passcode,
		Title:     title,
		FileCount: len(files),
		Platform:  "quark",
	}, nil
}
