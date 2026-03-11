package main

import "github.com/prometheus/client_golang/prometheus"

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

	ftpLoginTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_last_login_time",
		Help: "Timestamp of last successful FTP login.",
	})

	ftpLoginTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_login_total",
		Help: "Total number of FTP logins.",
	})

	ftpUploadTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_upload_total",
		Help: "Total number of FTP uploads.",
	})

	ftpDownloadTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_download_total",
		Help: "Total number of FTP downloads.",
	})

	uploadBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_upload_bytes_total",
		Help: "Total number of bytes uploaded.",
	})

	downloadBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_download_bytes_total",
		Help: "Total number of bytes downloaded.",
	})

	transferDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "vsftp_transfer_duration_seconds",
		Help:    "Duration of file transfers in seconds.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	})

	averageTransferSpeed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_average_transfer_speed_bytes_per_second",
		Help: "Average transfer speed in bytes per second.",
	})

	failedLoginsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_failed_logins_total",
		Help: "Total number of failed login attempts.",
	})

	transferErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_transfer_errors_total",
		Help: "Total number of transfer errors by type.",
	}, []string{"type"})

	connectionTimeoutsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_connection_timeouts_total",
		Help: "Total number of connection timeouts.",
	})

	authenticationErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_authentication_errors_total",
		Help: "Total number of authentication errors.",
	})

	maxConnectionsReachedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_max_connections_reached_total",
		Help: "Total number of times max connections limit was reached.",
	})

	bandwidthUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_bandwidth_usage_bytes_per_second",
		Help: "Current bandwidth usage in bytes per second.",
	})

	clientConnectionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_client_connections_total",
		Help: "Total number of connections by client IP address.",
	}, []string{"client_ip"})

	uniqueClients = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_unique_clients",
		Help: "Number of unique client IP addresses with recent activity.",
	})

	userLoginsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_user_logins_total",
		Help: "Total number of successful logins by username.",
	}, []string{"username"})

	userConnectionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_user_connections_total",
		Help: "Total number of connections by username.",
	}, []string{"username"})

	connectionLoginDelaySeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "vsftp_connection_login_delay_seconds",
		Help:    "Time delay between connection and successful login in seconds.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
	})

	rapidReconnectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vsftp_rapid_reconnections_total",
		Help: "Total number of rapid reconnections (same IP within 30 seconds).",
	})

	activeProcesses = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vsftp_active_processes",
		Help: "Number of active vsftpd processes based on log entries.",
	})

	clientFilesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsftp_client_files_total",
		Help: "Total number of files transferred by client IP address and direction.",
	}, []string{"client_ip", "direction"})
)

func init() {
	prometheus.MustRegister(
		ftpLoginSuccess,
		ftpConnections,
		establishedConnections,
		closeWaitConnections,
		ftpLoginTime,
		ftpLoginTotal,
		ftpUploadTotal,
		ftpDownloadTotal,
		uploadBytesTotal,
		downloadBytesTotal,
		transferDurationSeconds,
		averageTransferSpeed,
		failedLoginsTotal,
		transferErrorsTotal,
		connectionTimeoutsTotal,
		authenticationErrorsTotal,
		maxConnectionsReachedTotal,
		bandwidthUsage,
		clientConnectionsTotal,
		uniqueClients,
		userLoginsTotal,
		userConnectionsTotal,
		connectionLoginDelaySeconds,
		rapidReconnectionsTotal,
		activeProcesses,
		clientFilesTotal,
	)
}
