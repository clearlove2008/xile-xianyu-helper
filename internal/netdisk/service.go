package netdisk

import (
	"context"
	"fmt"
)

// Service 是网盘转存能力的统一入口，供 HTTP 层调用。
type Service struct {
	Quark *QuarkClient
	Baidu *BaiduClient
	QR    *QRLoginManager
}

// NewService 创建默认网盘服务。
func NewService() *Service {
	return &Service{
		Quark: &QuarkClient{},
		Baidu: &BaiduClient{},
		QR:    NewQRLoginManager(),
	}
}

// Transfer 按平台执行转存。platform 必须是 quark/baidu。
func (s *Service) Transfer(ctx context.Context, platform string, in ShareInput) (ShareResult, error) {
	switch platform {
	case "quark":
		return s.Quark.Transfer(ctx, in)
	case "baidu":
		return s.Baidu.Transfer(ctx, in)
	default:
		return ShareResult{}, fmt.Errorf("不支持的网盘平台: %s", platform)
	}
}

// BeginQRLogin 发起扫码登录。
func (s *Service) BeginQRLogin(ctx context.Context, platform string) (QRLoginStatus, error) {
	return s.QR.Begin(ctx, platform)
}

// QRLoginStatus 查询扫码状态。
func (s *Service) QRLoginStatus(sessionID string) (QRLoginStatus, error) {
	return s.QR.Status(sessionID)
}

// CancelQRLogin 取消扫码会话。
func (s *Service) CancelQRLogin(sessionID string) error {
	return s.QR.Cancel(sessionID)
}

// CheckLogin 校验某平台的 cookie 是否有效。
func (s *Service) CheckLogin(ctx context.Context, platform, cookie string) (bool, error) {
	switch platform {
	case "quark":
		return s.Quark.QuarkLoginCheck(ctx, cookie)
	case "baidu":
		// 百度通过 bdstoken 接口间接校验。
		token, err := s.Baidu.baiduGetBdstoken(ctx, cookie)
		if err != nil {
			return false, err
		}
		return token != "", nil
	default:
		return false, fmt.Errorf("不支持的网盘平台: %s", platform)
	}
}
