//go:build !linux

package collector

import "context"

// collectSelfNative 非 linux 平台无原生路径，直接走脚本（darwin 本机脚本）。
func collectSelfNative(ctx context.Context) (*RawSample, error) {
	return nil, errNativeUnsupported
}

// localScriptEnv 本机脚本路径的路径覆盖：非 linux 无 /proc 口径问题（darwin 脚本自带）。
func localScriptEnv() []string { return nil }
