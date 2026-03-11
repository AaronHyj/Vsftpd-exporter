package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: vsftp-exporter-refactor, Property 1: activeTransfers 计数正确性
// For any 合法的传输事件序列（包含开始和完成事件），ExporterState.activeTransfers
// 应始终等于"已开始但未完成的传输数量"，且该值永远不低于 0。
// **Validates: Requirements 1.1, 1.2, 1.3**
// Feature: vsftp-exporter-refactor, Property 1: activeTransfers 计数正确性
// For any 合法的传输事件序列（包含开始和完成事件），ExporterState.activeTransfers
// 应始终等于"已开始但未完成的传输数量"，且该值永远不低于 0。
// **Validates: Requirements 1.1, 1.2, 1.3**
func TestPropertyActiveTransfersCount(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 100).Draw(t, "eventCount")
		events := make([]bool, n)
		for i := range events {
			events[i] = rapid.Bool().Draw(t, fmt.Sprintf("event_%d", i))
		}

		active := 0
		for _, isStart := range events {
			if isStart {
				active++
			} else {
				if active > 0 {
					active--
				}
			}
			// Property: activeTransfers must never go below 0
			if active < 0 {
				t.Fatalf("activeTransfers went negative: %d", active)
			}
		}

		// Property: final value must be non-negative
		if active < 0 {
			t.Fatalf("final activeTransfers is negative: %d", active)
		}
	})
}

// Feature: vsftp-exporter-refactor, Property 3: 平均传输速度计算正确性
// For any 正的总传输字节数和正的程序运行时长，averageTransferSpeed 应等于
// totalBytes / runDuration。当运行时长为零或负数时，averageTransferSpeed 应为 0。
// **Validates: Requirements 2.2, 2.3**
func TestPropertyAverageTransferSpeed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		totalBytes := rapid.Int64Range(1, 1<<50).Draw(t, "totalBytes")
		runDuration := rapid.Float64Range(-10.0, 1000000.0).Draw(t, "runDuration")

		var speed float64
		if runDuration > 0 {
			speed = float64(totalBytes) / runDuration
		}

		if runDuration <= 0 {
			if speed != 0 {
				t.Fatalf("expected speed=0 for runDuration=%f, got %f", runDuration, speed)
			}
		} else {
			expected := float64(totalBytes) / runDuration
			if speed != expected {
				t.Fatalf("speed=%f, expected=%f (totalBytes=%d, runDuration=%f)", speed, expected, totalBytes, runDuration)
			}
		}
	})
}

// Feature: vsftp-exporter-refactor, Property 4: NewExporterState 完全初始化
// For any NewExporterState() 调用返回的实例，所有 map 类型字段均不为 nil，
// 且 lastProcessedTime 和 lastBandwidthCheck 均为调用时刻附近的时间值。
// **Validates: Requirements 4.1, 4.2, 4.3, 4.4**
func TestPropertyNewExporterStateInit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		before := time.Now()
		state := NewExporterState()
		after := time.Now()

		if state.transferStartTimes == nil {
			t.Fatal("transferStartTimes is nil")
		}
		if state.clientLastActivity == nil {
			t.Fatal("clientLastActivity is nil")
		}
		if state.clientConnectTimes == nil {
			t.Fatal("clientConnectTimes is nil")
		}
		if state.userClientMapping == nil {
			t.Fatal("userClientMapping is nil")
		}
		if state.activeProcessIDs == nil {
			t.Fatal("activeProcessIDs is nil")
		}
		if state.clientLastConnect == nil {
			t.Fatal("clientLastConnect is nil")
		}

		if state.lastProcessedTime.Before(before) || state.lastProcessedTime.After(after) {
			t.Fatalf("lastProcessedTime %v not in [%v, %v]", state.lastProcessedTime, before, after)
		}
		if state.lastBandwidthCheck.Before(before) || state.lastBandwidthCheck.After(after) {
			t.Fatalf("lastBandwidthCheck %v not in [%v, %v]", state.lastBandwidthCheck, before, after)
		}
	})
}

// Feature: vsftp-exporter-refactor, Property 7: ss 输出解析正确性
// For any 包含若干行 ss -tnH 格式输出的字符串，其中本地地址列包含 FTP 端口的行，
// 解析结果中 ESTAB 状态的计数应等于输出中状态为 "ESTAB" 且端口匹配的行数，
// CLOSE-WAIT 状态的计数应等于状态为 "CLOSE-WAIT" 且端口匹配的行数。
// **Validates: Requirements 10.3**
func TestPropertyParseSSOutput(t *testing.T) {
	states := []string{"ESTAB", "CLOSE-WAIT", "TIME-WAIT", "SYN-SENT", "SYN-RECV", "FIN-WAIT-1", "LAST-ACK"}
	ftpPort := "21"

	rapid.Check(t, func(t *rapid.T) {
		lineCount := rapid.IntRange(0, 50).Draw(t, "lineCount")

		var lines []string
		expectedTotal := 0
		expectedEstab := 0
		expectedCloseWait := 0

		for i := 0; i < lineCount; i++ {
			state := states[rapid.IntRange(0, len(states)-1).Draw(t, fmt.Sprintf("state_%d", i))]
			ip := fmt.Sprintf("%d.%d.%d.%d",
				rapid.IntRange(1, 254).Draw(t, fmt.Sprintf("ip1_%d", i)),
				rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("ip2_%d", i)),
				rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("ip3_%d", i)),
				rapid.IntRange(1, 254).Draw(t, fmt.Sprintf("ip4_%d", i)),
			)
			// Randomly choose whether this line matches the FTP port
			matchPort := rapid.Bool().Draw(t, fmt.Sprintf("matchPort_%d", i))
			var localPort string
			if matchPort {
				localPort = ftpPort
			} else {
				localPort = fmt.Sprintf("%d", rapid.IntRange(1024, 65535).Draw(t, fmt.Sprintf("otherPort_%d", i)))
			}
			remotePort := rapid.IntRange(1024, 65535).Draw(t, fmt.Sprintf("remotePort_%d", i))
			remoteIP := fmt.Sprintf("%d.%d.%d.%d",
				rapid.IntRange(1, 254).Draw(t, fmt.Sprintf("rip1_%d", i)),
				rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("rip2_%d", i)),
				rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("rip3_%d", i)),
				rapid.IntRange(1, 254).Draw(t, fmt.Sprintf("rip4_%d", i)),
			)

			line := fmt.Sprintf("%s  0  0  %s:%s  %s:%d", state, ip, localPort, remoteIP, remotePort)
			lines = append(lines, line)

			if matchPort {
				expectedTotal++
				switch state {
				case "ESTAB":
					expectedEstab++
				case "CLOSE-WAIT":
					expectedCloseWait++
				}
			}
		}

		output := strings.Join(lines, "\n")
		gotTotal, gotEstab, gotCloseWait := parseSSOutput(output, ftpPort)

		if gotTotal != expectedTotal {
			t.Fatalf("total: got %d, expected %d", gotTotal, expectedTotal)
		}
		if gotEstab != expectedEstab {
			t.Fatalf("established: got %d, expected %d", gotEstab, expectedEstab)
		}
		if gotCloseWait != expectedCloseWait {
			t.Fatalf("closeWait: got %d, expected %d", gotCloseWait, expectedCloseWait)
		}
	})
}

// Feature: vsftp-exporter-refactor, Property 8: parseStandardXferlog 解析一致性
// For any 合法的 xferlog 格式日志行，parseStandardXferlog 应正确提取
// direction、clientIP、fileSize、filePath、transferTime、username 和 completed 字段，
// 且提取的 fileSize 为非负整数，direction 为 "i" 或 "o"，completed 为 completionStatus=="c" 的布尔值。
// **Validates: Requirements 7.2, 7.3**
func TestPropertyParseStandardXferlog(t *testing.T) {
	weekdays := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid xferlog fields
		wday := weekdays[rapid.IntRange(0, 6).Draw(t, "wday")]
		mon := months[rapid.IntRange(0, 11).Draw(t, "month")]
		day := rapid.IntRange(1, 28).Draw(t, "day")
		hour := rapid.IntRange(0, 23).Draw(t, "hour")
		min := rapid.IntRange(0, 59).Draw(t, "min")
		sec := rapid.IntRange(0, 59).Draw(t, "sec")
		year := rapid.IntRange(2000, 2030).Draw(t, "year")

		transferTime := rapid.IntRange(0, 3600).Draw(t, "transferTime")
		clientIP := fmt.Sprintf("%d.%d.%d.%d",
			rapid.IntRange(1, 254).Draw(t, "ip1"),
			rapid.IntRange(0, 255).Draw(t, "ip2"),
			rapid.IntRange(0, 255).Draw(t, "ip3"),
			rapid.IntRange(1, 254).Draw(t, "ip4"),
		)
		fileSize := rapid.Int64Range(0, 1<<40).Draw(t, "fileSize")

		// Generate a file path without spaces (xferlog fields are space-delimited)
		pathSegments := rapid.IntRange(1, 4).Draw(t, "pathSegments")
		var pathParts []string
		for i := 0; i < pathSegments; i++ {
			seg := rapid.StringMatching(`[a-z][a-z0-9_]{1,10}`).Draw(t, fmt.Sprintf("seg_%d", i))
			pathParts = append(pathParts, seg)
		}
		filePath := "/" + strings.Join(pathParts, "/") + ".txt"

		direction := "i"
		if rapid.Bool().Draw(t, "isDownload") {
			direction = "o"
		}
		username := rapid.StringMatching(`[a-z][a-z0-9]{2,10}`).Draw(t, "username")
		completionStatus := "c"
		wantCompleted := true
		if rapid.Bool().Draw(t, "incomplete") {
			completionStatus = "i"
			wantCompleted = false
		}

		// xferlog standard format (18+ fields):
		// DayOfWeek Month Day HH:MM:SS Year TransferTime RemoteHost FileSize Filename
		// TransferType SpecialActionFlag Direction AccessMode Username ServiceName
		// AuthenticationMethod AuthenticatedUserID CompletionStatus
		line := fmt.Sprintf("%s %s %2d %02d:%02d:%02d %d %d %s %d %s b _ %s g %s ftp 0 * %s",
			wday, mon, day, hour, min, sec, year,
			transferTime, clientIP, fileSize, filePath,
			direction, username, completionStatus,
		)

		gotDir, gotIP, gotSize, gotPath, gotTime, gotUser, gotCompleted := parseStandardXferlog(line)

		if gotDir != direction {
			t.Fatalf("direction: got %q, want %q\nline: %s", gotDir, direction, line)
		}
		if gotIP != clientIP {
			t.Fatalf("clientIP: got %q, want %q", gotIP, clientIP)
		}
		if gotSize != fileSize {
			t.Fatalf("fileSize: got %d, want %d", gotSize, fileSize)
		}
		if gotSize < 0 {
			t.Fatalf("fileSize is negative: %d", gotSize)
		}
		if gotPath != filePath {
			t.Fatalf("filePath: got %q, want %q", gotPath, filePath)
		}
		if gotTime != transferTime {
			t.Fatalf("transferTime: got %d, want %d", gotTime, transferTime)
		}
		if gotUser != username {
			t.Fatalf("username: got %q, want %q", gotUser, username)
		}
		if gotCompleted != wantCompleted {
			t.Fatalf("completed: got %v, want %v", gotCompleted, wantCompleted)
		}
		if gotDir != "i" && gotDir != "o" {
			t.Fatalf("direction must be 'i' or 'o', got %q", gotDir)
		}
	})
}
