package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 配置结构体
type Config struct {
	FTPServer   string `json:"ftp_server"`
	FTPPort     string `json:"ftp_port"`
	FTPUser     string `json:"ftp_username"`
	FTPPass     string `json:"ftp_password"`
	LogFilePath string `json:"log_file_path"`
}

// 定义指标
var (
	ftpLoginSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_login_success",
		Help: "Indicates if the login to the FTP server is successful (1 for success, 0 for failure).",
	})

	ftpConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_connections",
		Help: "Current number of FTP connections.",
	})

	establishedConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_established_connections",
		Help: "Number of ESTABLISHED FTP connections.",
	})

	closeWaitConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_close_wait_connections",
		Help: "Number of CLOSE_WAIT FTP connections.",
	})

	filesDownloaded = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_files_received_total",
		Help: "Total number of files received (downloaded) from the FTP server.",
	})

	filesUploaded = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_files_sent_total",
		Help: "Total number of files sent (uploaded) to the FTP server.",
	})
)

func init() {
	// 注册指标
	prometheus.MustRegister(ftpLoginSuccess)
	prometheus.MustRegister(ftpConnections)
	prometheus.MustRegister(establishedConnections)
	prometheus.MustRegister(closeWaitConnections)
	prometheus.MustRegister(filesDownloaded)
	prometheus.MustRegister(filesUploaded)
}

func main() {
	config := loadConfig("config.json")

	go func() {
		var lastProcessedTime time.Time
		for {
			// 检查 FTP 服务登录状态
			checkFTPLogin(config)
			// 获取当前连接数
			checkConnections(config.FTPPort)
			// 解析 FTP 日志文件，统计新增的文件传输事件
			lastProcessedTime = parseFTPLog(config.LogFilePath, lastProcessedTime)
			time.Sleep(10 * time.Second) // 每10秒检查一次
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Exporter listening on port 9100")
	http.ListenAndServe(":9100", nil)
}

// 加载配置文件
func loadConfig(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		fmt.Println("Error opening config file:", err)
		os.Exit(1)
	}
	defer configFile.Close()

	byteValue, err := ioutil.ReadAll(configFile)
	if err != nil {
		fmt.Println("Error reading config file:", err)
		os.Exit(1)
	}

	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		fmt.Println("Error parsing config file:", err)
		os.Exit(1)
	}

	return config
}

// 检查 FTP 服务登录状态
func checkFTPLogin(config Config) {
	address := fmt.Sprintf("%s:%s", config.FTPServer, config.FTPPort)
	client, err := ftp.Dial(address, ftp.DialWithTimeout(5*time.Second))
	if err != nil {
		ftpLoginSuccess.Set(0)
		fmt.Println("Failed to connect to FTP server:", err)
		return
	}
	defer client.Quit()

	err = client.Login(config.FTPUser, config.FTPPass)
	if err != nil {
		ftpLoginSuccess.Set(0)
		fmt.Println("Failed to login to FTP server:", err)
		return
	}

	// 登录成功
	ftpLoginSuccess.Set(1)
}

// 获取当前连接数和各状态的连接数
func checkConnections(port string) {
	cmd := exec.Command("netstat", "-an")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Error executing netstat:", err)
		return
	}

	var totalConnections, established, closeWait int
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ":"+port) {
			totalConnections++
			if strings.Contains(line, "ESTABLISHED") {
				established++
			} else if strings.Contains(line, "CLOSE_WAIT") {
				closeWait++
			}
		}
	}

	ftpConnections.Set(float64(totalConnections))
	establishedConnections.Set(float64(established))
	closeWaitConnections.Set(float64(closeWait))
}

// 解析 FTP 日志文件，统计新增的文件传输事件
func parseFTPLog(logFilePath string, lastProcessedTime time.Time) time.Time {
	file, err := os.Open(logFilePath)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		return lastProcessedTime
	}
	defer file.Close()

	var latestProcessedTime time.Time
	reader := bufio.NewReader(file)

	currentYear := time.Now().Year()

	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Error reading log file:", err)
			return lastProcessedTime
		}

		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		// 解析时间
		timestampStr := strings.Join(fields[0:4], " ") // 不含年份的时间字符串
		timestampStr = fmt.Sprintf("%s %d", timestampStr, currentYear)
		timestamp, err := time.Parse("Mon Jan 2 15:04:05 2006", timestampStr)
		if err != nil {
			fmt.Println("Error parsing time:", err)
			continue
		}

		// 忽略已经处理的日志行
		if !timestamp.After(lastProcessedTime) {
			continue
		}

		// 更新最新处理的时间
		if timestamp.After(latestProcessedTime) {
			latestProcessedTime = timestamp
		}

		// 检查传输方向
		direction := fields[11] // 'i' for upload, 'o' for download
		if direction == "i" {
			filesUploaded.Inc()
		} else if direction == "o" {
			filesDownloaded.Inc()
		}
	}

	if latestProcessedTime.After(lastProcessedTime) {
		return latestProcessedTime
	}
	return lastProcessedTime
}

