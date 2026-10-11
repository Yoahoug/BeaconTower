package geoip

import "testing"

// 解析不出来必须返回 nil：旧实现返回 Result{Region:"未定位"} 占位对象且永不返回
// nil，调用方无法区分「解析失败」与「解析成功」——占位文案被当值落库正是
// v2.7.2「未定位」线上事故的根因（doc/17 §1.2）。
func TestLookupUndeterminedReturnsNil(t *testing.T) {
	old := onlineLookupFn
	onlineLookupFn = func(string) *Result { return nil }
	t.Cleanup(func() { onlineLookupFn = old })

	if r := Lookup("8.8.8.8"); r != nil {
		t.Fatalf("解析链全败应返回 nil，得到 %+v", r)
	}
	if r := Lookup(""); r != nil {
		t.Fatalf("空 IP 应返回 nil，得到 %+v", r)
	}
	if r := Lookup("not-an-ip"); r != nil {
		t.Fatalf("非法 IP 应返回 nil，得到 %+v", r)
	}
}

func TestLookupOnlineResultPassThrough(t *testing.T) {
	old := onlineLookupFn
	onlineLookupFn = func(string) *Result {
		return &Result{Country: "US", City: "Los Angeles", Region: "美国"}
	}
	t.Cleanup(func() { onlineLookupFn = old })

	r := Lookup("107.173.35.9")
	if r == nil || r.Country != "US" || r.City != "Los Angeles" || RegionText(r) != "美国" {
		t.Fatalf("在线结果应透传: %+v", r)
	}
}

func TestLookupPrivateAndStatic(t *testing.T) {
	old := onlineLookupFn
	onlineLookupFn = func(string) *Result { return nil }
	t.Cleanup(func() { onlineLookupFn = old })

	if r := Lookup("192.168.1.10"); r == nil || r.Country != "LAN" || RegionText(r) != "内网" {
		t.Fatalf("内网地址应直返内网: %+v", r)
	}
	// 文档网段静态映射（联调离线兜底）：在线源不可用时也必须能定位
	if r := Lookup("203.0.113.7"); r == nil || r.Country != "HK" || RegionText(r) != "香港" {
		t.Fatalf("静态映射应生效: %+v", r)
	}
}

// RegionText 不再输出占位文案：存储层据此把空串当「未知」，展示层各自回退。
func TestRegionTextAndPlaceholder(t *testing.T) {
	if RegionText(nil) != "" {
		t.Fatal("未解析应返回空串（不得再输出「未定位」）")
	}
	if RegionText(&Result{}) != "" {
		t.Fatal("字段全空应返回空串")
	}
	if RegionText(&Result{Country: "US"}) != "US" {
		t.Fatal("无中文名时应回退国家码")
	}
	if !IsPlaceholder("未定位") || !IsPlaceholder(" 未定位 ") {
		t.Fatal("占位文案（含空白）应被识别")
	}
	if IsPlaceholder("美国") || IsPlaceholder("") || IsPlaceholder("内网") {
		t.Fatal("正常文本不应被判为占位")
	}
}
