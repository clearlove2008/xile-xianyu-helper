package netdisk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 百度网盘网页 API。
const (
	baiduReferer = "https://pan.baidu.com/"
	baiduAppID   = "250528"
)

var (
	reShareUK   = regexp.MustCompile(`share_uk["']?\s*:\s*["']?(\d+)`)
	reShareID   = regexp.MustCompile(`shareid["']?\s*:\s*["']?(\d+)`)
	reBaiduSurl = regexp.MustCompile(`/s/([a-zA-Z0-9_-]+)`)
	reBaiduPwd  = regexp.MustCompile(`[?&]pwd=([a-zA-Z0-9]{4,6})`)
)

// BaiduClient 实现百度网盘的转存能力。
type BaiduClient struct {
	Doer Doer
}

// baiduResp 是百度 API 的响应壳。errno=0 表示成功。
type baiduResp struct {
	Errno   int    `json:"errno"`
	ErrMsg  string `json:"err_msg"`
	ShowMsg string `json:"show_msg"`
}

// errText 返回可读错误。
func (r baiduResp) errText() string {
	if r.ErrMsg != "" {
		return decodeUnicode(r.ErrMsg)
	}
	if r.ShowMsg != "" {
		return decodeUnicode(r.ShowMsg)
	}
	return "未知错误"
}

// baiduCommonParams 返回百度请求通用参数。
func baiduCommonParams(bdstoken string) url.Values {
	v := url.Values{}
	v.Set("channel", "chunlei")
	v.Set("web", "1")
	v.Set("clienttype", "0")
	v.Set("app_id", baiduAppID)
	if bdstoken != "" {
		v.Set("bdstoken", bdstoken)
	}
	return v
}

// baiduGetBdstoken 获取 CSRF token。
func (c *BaiduClient) baiduGetBdstoken(ctx context.Context, cookie string) (string, error) {
	if c == nil || c.Doer == nil {
		return "", nil
	}
	target := "https://pan.baidu.com/api/gettemplatevariable?" + url.Values{
		"clienttype": {"0"}, "app_id": {baiduAppID}, "fields": {`["bdstoken"]`},
	}.Encode()
	var raw struct {
		Errno  int `json:"errno"`
		Result struct {
			Bdstoken string `json:"bdstoken"`
		} `json:"result"`
	}
	if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, baiduReferer), &raw); err != nil {
		return "", err
	}
	return raw.Result.Bdstoken, nil
}

// baiduShareInfo 是从分享页解析出的凭据。
type baiduShareInfo struct {
	ShareID string
	UK      string
	SURL    string
	CleanS  string
	SeKey   string
}

// baiduVerify 验证提取码并解析分享凭据。
func (c *BaiduClient) baiduVerify(ctx context.Context, cookie, shareURL, passcode string) (baiduShareInfo, error) {
	info := baiduShareInfo{}
	match := reBaiduSurl.FindStringSubmatch(shareURL)
	if len(match) < 2 {
		return info, fmt.Errorf("无法从链接提取百度分享 ID: %s", shareURL)
	}
	info.SURL = match[1]
	info.CleanS = strings.TrimLeft(info.SURL, "1")
	if info.CleanS == "" {
		info.CleanS = info.SURL
	}

	// 若链接自带 pwd 参数，优先使用。
	if m := reBaiduPwd.FindStringSubmatch(shareURL); len(m) >= 2 {
		passcode = m[1]
	}

	// 抓分享页 HTML，解析 share_uk / shareid。
	pageURL := "https://pan.baidu.com/s/" + info.SURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return info, err
	}
	req.Header = jsonHeader(cookie, baiduReferer)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.Doer.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	raw := readAllText(resp.Body)
	if m := reShareUK.FindStringSubmatch(raw); len(m) >= 2 {
		info.UK = m[1]
	}
	if m := reShareID.FindStringSubmatch(raw); len(m) >= 2 {
		info.ShareID = m[1]
	}

	// 有提取码时调用 verify，拿 BDCLND（访问分享所需的 sekey）。
	if strings.TrimSpace(passcode) != "" {
		form := url.Values{"pwd": {passcode}, "vcode": {""}, "vcode_str": {""}}
		params := url.Values{
			"surl": {info.CleanS}, "t": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
			"channel": {"chunlei"}, "web": {"1"}, "clienttype": {"0"},
		}
		target := "https://pan.baidu.com/share/verify?" + params.Encode()
		var verify struct {
			Errno  int    `json:"errno"`
			ErrMsg string `json:"err_msg"`
			Randsk string `json:"randsk"`
		}
		if err := doForm(ctx, c.Doer, target, formHeader(cookie, baiduReferer), form, &verify); err != nil {
			return info, err
		}
		if verify.Errno != 0 {
			return info, fmt.Errorf("百度提取码验证失败: %s (errno=%d)", decodeUnicode(verify.ErrMsg), verify.Errno)
		}
		info.SeKey = verify.Randsk
		if info.SeKey == "" {
			info.SeKey = extractCookieValue(resp.Header, "BDCLND")
		}
	}
	return info, nil
}

// baiduListFiles 列出分享中的文件。
func (c *BaiduClient) baiduListFiles(ctx context.Context, cookie string, info baiduShareInfo) ([]baiduShareFile, error) {
	params := baiduCommonParams("")
	params.Set("shorturl", info.SURL)
	params.Set("root", "1")
	target := "https://pan.baidu.com/share/list?" + params.Encode()
	var raw struct {
		Errno int `json:"errno"`
		List  []struct {
			FSID  int64  `json:"fs_id"`
			Path  string `json:"path"`
			Name  string `json:"file_name"`
			IsDir int    `json:"isdir"`
		} `json:"list"`
		ErrMsg string `json:"err_msg"`
	}
	header := jsonHeader(cookie, baiduReferer)
	if info.SeKey != "" {
		header.Set("Cookie", strings.TrimSpace(cookie)+"; BDCLND="+info.SeKey)
	}
	if err := doGet(ctx, c.Doer, target, header, &raw); err != nil {
		return nil, err
	}
	if raw.Errno != 0 {
		return nil, fmt.Errorf("列百度分享文件失败: %s (errno=%d)", decodeUnicode(raw.ErrMsg), raw.Errno)
	}
	out := make([]baiduShareFile, 0, len(raw.List))
	for _, f := range raw.List {
		out = append(out, baiduShareFile{FSID: f.FSID, Path: f.Path, Name: f.Name, IsDir: f.IsDir == 1})
	}
	return out, nil
}

type baiduShareFile struct {
	FSID  int64
	Path  string
	Name  string
	IsDir bool
}

// baiduTransfer 转存分享文件到目标账号，返回任务 ID。
func (c *BaiduClient) baiduTransfer(ctx context.Context, cookie string, info baiduShareInfo, files []baiduShareFile, bdstoken string) (string, error) {
	fsids := make([]string, 0, len(files))
	paths := make([]string, 0, len(files))
	for _, f := range files {
		fsids = append(fsids, strconv.FormatInt(f.FSID, 10))
		paths = append(paths, f.Path)
	}
	params := baiduCommonParams(bdstoken)
	form := url.Values{}
	form.Set("fsidlist", "["+strings.Join(fsids, ",")+"]")
	form.Set("path", "/")
	form.Set("shareid", info.ShareID)
	form.Set("from", info.UK)
	form.Set("sekey", info.SeKey)
	_ = paths

	target := "https://pan.baidu.com/share/transfer?" + params.Encode()
	var raw struct {
		baiduResp
		TaskID int64 `json:"taskid"`
	}
	if err := doForm(ctx, c.Doer, target, formHeader(cookie, baiduReferer), form, &raw); err != nil {
		return "", err
	}
	if raw.Errno != 0 {
		return "", fmt.Errorf("百度转存失败: %s (errno=%d)", raw.errText(), raw.Errno)
	}
	if raw.TaskID == 0 {
		return "", nil // 同步完成
	}
	return strconv.FormatInt(raw.TaskID, 10), nil
}

// baiduWaitTask 等待转存任务完成。
func (c *BaiduClient) baiduWaitTask(ctx context.Context, cookie, taskID, bdstoken string) error {
	if taskID == "" {
		return nil
	}
	params := baiduCommonParams(bdstoken)
	params.Set("taskid", taskID)
	target := "https://pan.baidu.com/share/taskquery?" + params.Encode()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var raw struct {
			baiduResp
			Status int `json:"status"`
		}
		if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, baiduReferer), &raw); err != nil {
			return err
		}
		if raw.Errno != 0 {
			return fmt.Errorf("查询转存任务失败: %s", raw.errText())
		}
		if raw.Status == 4 { // 4=完成
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(800 * time.Millisecond):
		}
	}
	return fmt.Errorf("等待百度转存任务超时")
}

// baiduRootFiles 列出目标账号根目录文件，用于取回新转存文件的 fs_id。
func (c *BaiduClient) baiduRootFiles(ctx context.Context, cookie, bdstoken string) ([]baiduShareFile, error) {
	params := baiduCommonParams(bdstoken)
	params.Set("dir", "/")
	target := "https://pan.baidu.com/api/list?" + params.Encode()
	var raw struct {
		Errno int `json:"errno"`
		List  []struct {
			FSID int64  `json:"fs_id"`
			Path string `json:"path"`
			Name string `json:"filename"`
		} `json:"list"`
		ErrMsg string `json:"err_msg"`
	}
	if err := doGet(ctx, c.Doer, target, jsonHeader(cookie, baiduReferer), &raw); err != nil {
		return nil, err
	}
	if raw.Errno != 0 {
		return nil, fmt.Errorf("列百度根目录失败: %s (errno=%d)", decodeUnicode(raw.ErrMsg), raw.Errno)
	}
	out := make([]baiduShareFile, 0, len(raw.List))
	for _, f := range raw.List {
		out = append(out, baiduShareFile{FSID: f.FSID, Path: f.Path, Name: f.Name})
	}
	return out, nil
}

// baiduCreateShare 创建新分享，返回链接与提取码。
func (c *BaiduClient) baiduCreateShare(ctx context.Context, cookie string, fsids []int64, passcode string, periodDays int, bdstoken string) (string, string, error) {
	if passcode == "" {
		passcode = randomPasscode(4)
	}
	params := baiduCommonParams(bdstoken)
	form := url.Values{}
	form.Set("fid_list", "["+joinInt64(fsids)+"]")
	form.Set("schannel", "4")
	form.Set("channel_list", "[]")
	form.Set("period", strconv.Itoa(periodDays))
	form.Set("pwd", passcode)
	target := "https://pan.baidu.com/share/set?" + params.Encode()
	var raw struct {
		baiduResp
		Link     string `json:"link"`
		ShortURL string `json:"shorturl"`
		ShareID  string `json:"shareid"`
		Pwd      string `json:"pwd"`
	}
	if err := doForm(ctx, c.Doer, target, formHeader(cookie, baiduReferer), form, &raw); err != nil {
		return "", "", err
	}
	if raw.Errno != 0 {
		return "", "", fmt.Errorf("创建百度分享失败: %s (errno=%d)", raw.errText(), raw.Errno)
	}
	shareURL := raw.Link
	if shareURL == "" && raw.ShortURL != "" {
		shareURL = "https://pan.baidu.com/s/" + raw.ShortURL
	}
	if shareURL == "" {
		return "", "", fmt.Errorf("创建分享成功但未返回链接")
	}
	finalPwd := raw.Pwd
	if finalPwd == "" {
		finalPwd = passcode
	}
	return shareURL, finalPwd, nil
}

// Transfer 执行百度一键转存：verify → 列文件 → 转存 → 建分享 → 取链接。
func (c *BaiduClient) Transfer(ctx context.Context, in ShareInput) (ShareResult, error) {
	result := ShareResult{Platform: "baidu"}
	cookie := strings.TrimSpace(in.Cookie)
	if cookie == "" {
		return result, fmt.Errorf("百度账号 cookie 为空")
	}
	info, err := c.baiduVerify(ctx, cookie, in.ShareURL, in.Passcode)
	if err != nil {
		return result, err
	}
	bdstoken, err := c.baiduGetBdstoken(ctx, cookie)
	if err != nil {
		return result, err
	}
	files, err := c.baiduListFiles(ctx, cookie, info)
	if err != nil {
		return result, err
	}
	if len(files) == 0 {
		return result, fmt.Errorf("分享中没有可转存的文件")
	}
	taskID, err := c.baiduTransfer(ctx, cookie, info, files, bdstoken)
	if err != nil {
		return result, err
	}
	if err := c.baiduWaitTask(ctx, cookie, taskID, bdstoken); err != nil {
		return result, err
	}

	rootFiles, err := c.baiduRootFiles(ctx, cookie, bdstoken)
	if err != nil {
		return result, err
	}
	names := make(map[string]bool, len(files))
	for _, f := range files {
		names[f.Name] = true
	}
	fsids := make([]int64, 0, len(files))
	for _, rf := range rootFiles {
		if names[rf.Name] {
			fsids = append(fsids, rf.FSID)
		}
	}
	if len(fsids) == 0 {
		for _, f := range files {
			fsids = append(fsids, f.FSID)
		}
	}

	title := files[0].Name
	if len(files) > 1 {
		title = files[0].Name + " 等 " + strconv.Itoa(len(files)) + " 个文件"
	}
	shareURL, passcode, err := c.baiduCreateShare(ctx, cookie, fsids, strings.TrimSpace(in.NewPasscode), in.PeriodDays, bdstoken)
	if err != nil {
		return result, err
	}
	return ShareResult{
		ShareURL:  shareURL,
		Passcode:  passcode,
		Title:     title,
		FileCount: len(files),
		Platform:  "baidu",
	}, nil
}

// joinInt64 把 int64 列表拼成 JSON 数组字符串。
func joinInt64(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.FormatInt(v, 10))
	}
	return strings.Join(parts, ",")
}

// readAllText 读取响应文本。
func readAllText(r io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return ""
	}
	return string(raw)
}

// extractCookieValue 从响应头中读取指定 cookie 值（用于 BDCLND）。
func extractCookieValue(header http.Header, name string) string {
	for _, raw := range header.Values("Set-Cookie") {
		parts := strings.Split(raw, ";")
		if len(parts) > 0 && strings.HasPrefix(strings.TrimSpace(parts[0]), name+"=") {
			return strings.TrimPrefix(strings.TrimSpace(parts[0]), name+"=")
		}
	}
	return ""
}
