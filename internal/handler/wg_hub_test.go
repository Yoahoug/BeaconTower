package handler

import "testing"

func TestWgZipSafeName(t *testing.T) {
	cases := []struct {
		id   int64
		name string
		tag  string
		want string
	}{
		{4, "Mac", "A", "Mac-A.conf"},
		{3, "Win PC", "B", "Win_PC-B.conf"},
		{2, "本机备用", "B", "peer2-B.conf"},
		// 回归：纯中文名被逐字替换成 "_-_"，早期只 Trim("_") 会剩下 "-"，
		// 于是文件名成了没有名字的 "--A.conf"（多个中文名还会互相覆盖）
		{5, "验证-临时", "A", "peer5-A.conf"},
		{6, "-_.", "B", "peer6-B.conf"},
		{7, "../../etc/passwd", "A", "etc_passwd-A.conf"},
		{8, "  A B  ", "B", "A_B-B.conf"},
	}
	for _, c := range cases {
		if got := wgZipSafeName(c.id, c.name, c.tag); got != c.want {
			t.Errorf("wgZipSafeName(%q) = %q，期望 %q", c.name, got, c.want)
		}
	}
}
