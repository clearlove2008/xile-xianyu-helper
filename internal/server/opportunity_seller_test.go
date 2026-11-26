package server

import "testing"

// TestOpportunitySellerID 验证数字 ID、主页参数与短链接/其他主机/商品链接的边界。
func TestOpportunitySellerID(t *testing.T) {
	// inputs 包含合法主页和不能作为卖家主页处理的地址。
	inputs := map[string]string{"123": "123", "https://www.goofish.com/personal?userId=123": "123", "https://pages.goofish.com/sharexy?userid=123": "123", "": ""}
	for input, expected := range inputs {
		if actual, err := opportunitySellerID(input); err != nil || actual != expected {
			t.Errorf("主页 ID 解析不匹配")
		}
	}
	for _, input := range []string{"https://evil.example/?userId=123", "https://www.goofish.com/item?id=123", "https://m.tb.cn/h.abc", "卖家昵称"} {
		if _, err := opportunitySellerID(input); err == nil {
			t.Errorf("无效卖家输入被接受")
		}
	}
}
