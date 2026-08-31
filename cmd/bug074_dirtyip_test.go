package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// BUG-074:vsftpd 并发写日志偶发两行粘连,CONNECT 行 Client "::ffff: 后混入
// 下一行日志文本,解析器把整个脏串当 IP 产生垃圾 label 序列。
func TestDirtyClientIPDropped(t *testing.T) {
	clientConnectionsTotal.Reset()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "vsftpd.log")
	// 脏 CONNECT 行 + 正常 CONNECT 行
	log := "Sun Aug 30 12:46:55 2026 [pid 3] CONNECT: Client \"::ffff:Sun Aug 30 12:47:23 2026 [pid 1] [ftpupload] OK LOGIN: Client\"\n" +
		"Sun Aug 30 12:47:24 2026 [pid 4] CONNECT: Client \"::ffff:172.25.7.70\"\n"
	if err := os.WriteFile(path, []byte(log), 0644); err != nil {
		t.Fatal(err)
	}
	state := NewExporterState()
	if err := parseVsftpdLog(&Config{}, path, state, nil); err != nil {
		t.Fatal(err)
	}
	dirty := testutil.ToFloat64(clientConnectionsTotal.WithLabelValues("::ffff:Sun Aug 30 12:47:23 2026 [pid 1] [ftpupload] OK LOGIN: Client"))
	if dirty != 0 {
		t.Errorf("脏 IP 产生了 label 序列,计数=%v,期望 0", dirty)
	}
	normal := testutil.ToFloat64(clientConnectionsTotal.WithLabelValues("::ffff:172.25.7.70"))
	if normal != 1 {
		t.Errorf("正常 IPv4-mapped label 计数=%v,期望 1", normal)
	}
}

// 合法 IP 形态(纯 v4 / v6 / ::ffff: 映射)均不应被误拦
func TestValidClientIPAccepted(t *testing.T) {
	cases := map[string]bool{
		"172.25.7.70":          true,
		"::ffff:172.25.7.70":   true,
		"2001:db8::1":          true,
		"::ffff:Sun Aug 30...": false, // 粘连脏串
		"":                     false,
		"not-an-ip":            false,
		"172.25.7":             false, // 缺一段
		"172.25.7.70.99":       false, // 多一段
	}
	for ip, want := range cases {
		if got := isValidClientIP(ip); got != want {
			t.Errorf("isValidClientIP(%q)=%v,期望 %v", ip, got, want)
		}
	}
}
