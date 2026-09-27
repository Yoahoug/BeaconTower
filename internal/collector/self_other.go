//go:build !linux

package collector

import "context"

// collectSelfNative 非 linux 平台无原生路径，直接走脚本（darwin 本机脚本）。
func collectSelfNative(ctx context.Context) (*RawSample, error) {
	return nil, errNativeUnsupported
}
