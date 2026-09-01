package startup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
)

var ErrFailedToReadConfiguration = errors.New("failed to read configuration")
var ErrFailedToParseLogLevel = errors.New("failed to parse log level")
var ErrInvalidLogLevel = errors.New("invalid log level")

type CommonConfiguration struct {
	ApplicationName string `envconfig:"APPLICATION_NAME" required:"true"`

	LogLevel     string `envconfig:"LOG_LEVEL" default:"INFO"`
	ZeroLogLevel zerolog.Level

	PostgresDB struct {
		Host               string `envconfig:"DB_SERVER" required:"true"`
		Port               uint32 `envconfig:"DB_PORT" required:"true"`
		User               string `envconfig:"DB_USER" required:"true"`
		Pass               string `envconfig:"DB_PASS" required:"true"`
		Database           string `envconfig:"DB_DATABASE" required:"true"`
		SSLMode            string `envconfig:"DB_SSL_MODE" required:"true"`
		EnableQueryLogging bool   `envconfig:"DB_QUERY_LOGGING" default:"false"`
		// Extended settings
		MaxOpenConnections           int `envconfig:"DB_MAX_OPEN_CONNECTIONS" default:"8"`
		MaxIdleConnections           int `envconfig:"DB_MAX_IDLE_CONNECTIONS" default:"8"`
		ConnectionMaxLifetimeSeconds int `envconfig:"DB_CONNECTION_MAX_LIFETIME_SECONDS" default:"180"`
		ConnectionMaxIdleTimeSeconds int `envconfig:"DB_CONNECTION_MAX_IDLE_TIME_SECONDS" default:"30"`
	}

	OIDC struct {
		BaseURL      string `envconfig:"OIDC_BASE_URL" required:"false"`
		ClientID     string `envconfig:"OIDC_CLIENT_ID" required:"false"`
		ClientSecret string `envconfig:"OIDC_CLIENT_SECRET" required:"false"`
	}

	Redis struct {
		Enable                   bool   `envconfig:"REDIS_ENABLE" default:"false"`
		Address                  string `envconfig:"REDIS_ADDRESS" default:"redis:6379"`
		Password                 string `envconfig:"REDIS_PASSWORD" default:""`
		DefaultTTLMinutes        int    `envconfig:"REDIS_DEFAULT_TTL_MINUTES" default:"1440"`
		RefreshRetryAttempts     int    `envconfig:"REDIS_REFRESH_RETRY_ATTEMPTS" default:"5"`
		RefreshRetryWaitStartMs  int    `envconfig:"REDIS_REFRESH_RETRY_WAIT_START_MS" default:"500"`
		RefreshRetryWaitExponent int    `envconfig:"REDIS_REFRESH_RETRY_WAIT_EXPONENT" default:"5"`
		MaxRetries               int    `envconfig:"REDIS_MAX_RETRIES" default:"-1"`
		DialerRetries            int    `envconfig:"REDIS_DIALER_RETRIES" default:"1"`
		DialerRetryTimeoutMs     int    `envconfig:"REDIS_DIALER_RETRY_TIMEOUT_MS" default:"50"`
	}

	OpenTelemetry struct {
		Enable                       bool   `envconfig:"OTEL_ENABLE" default:"false"`
		TraceCollectorEndpoint       string `envconfig:"OTEL_TRACE_COLLECTOR_ENDPOINT" default:"otel:4317"`
		TraceCollectorTimeoutSeconds int    `envconfig:"OTEL_TRACE_COLLECTOR_TIMEOUT_SECONDS" default:"5"`
		MetricsCollectorEndpoint     string `envconfig:"OTEL_METRICS_COLLECTOR_ENDPOINT" required:"false" default:"otel:4317"`
		MetricsReaderTimeoutSeconds  int    `envconfig:"OTEL_METRICS_READER_TIMEOUT_SECONDS" default:"5"`
		MetricsReaderIntervalSeconds int    `envconfig:"OTEL_METRICS_READER_INTERVAL_SECONDS" default:"15"`
		ReadMemStatsIntervalSeconds  int    `envconfig:"OTEL_READ_MEMSTATS_INTERVAL_SECONDS" default:"30"`
	}

	Pyroscope struct {
		Enable bool   `envconfig:"PYROSCOPE_ENABLE" default:"false"`
		Server string `envconfig:"PYROSCOPE_SERVER" default:"http://pyroscope:4040"`
	}
}

func (c *CommonConfiguration) GetCommonConfig() *CommonConfiguration {
	return c
}

type Configuration interface {
	GetCommonConfig() *CommonConfiguration
}

func ReadConfiguration(configuration Configuration) error {
	err := envconfig.Process("", configuration)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToReadConfiguration, err)
	}

	commonConfig := configuration.GetCommonConfig()

	zeroLogLevel, err := parseLogLevel(commonConfig.LogLevel)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToParseLogLevel, err)
	}
	commonConfig.ZeroLogLevel = zeroLogLevel

	return nil
}

func parseLogLevel(logLevel string) (zeroLogLevel zerolog.Level, err error) {
	switch strings.ToUpper(logLevel) {
	case "TRACE":
		zeroLogLevel = zerolog.TraceLevel
	case "DEBUG":
		zeroLogLevel = zerolog.DebugLevel
	case "INFO":
		zeroLogLevel = zerolog.InfoLevel
	case "WARN":
		zeroLogLevel = zerolog.WarnLevel
	case "ERROR":
		zeroLogLevel = zerolog.ErrorLevel
	case "FATAL":
		zeroLogLevel = zerolog.FatalLevel
	case "PANIC":
		zeroLogLevel = zerolog.PanicLevel
	case "":
		zeroLogLevel = zerolog.NoLevel
	case "DISABLED":
		zeroLogLevel = zerolog.Disabled
	default:
		return zeroLogLevel, fmt.Errorf("%w: %s", ErrInvalidLogLevel, logLevel)
	}
	return zeroLogLevel, nil
}
