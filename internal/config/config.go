package config

var (
	CertDirs                    *string
	DefaultWarningDays          *int
	DefaultCheckIntervalMinutes *int
	Debug                       *bool
)

const (
	DEBUG                          = "DEBUG"
	DEFAULT_WARNING_DAYS           = "DEFAULT_WARNING_DAYS"
	DEFAULT_CHECK_INTERVAL_MINUTES = "DEFAULT_CHECK_INTERVAL_MINUTES"
	CERT_DIRS                      = "CERT_DIRS"
	NODE_NAME                      = "NODE_NAME"
	VALID                          = "VALID"
	EXPIRING                       = "EXPIRING"
	EXPIRED                        = "EXPIRED"
)
