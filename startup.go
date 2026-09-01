package startup

import (
	"context"
	"fmt"

	db "github.com/blutspende/libs-db"
	"github.com/grafana/pyroscope-go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	Configuration         Configuration
	BuildVersion          string
	UsePostgres           bool
	UseExtendedPgConfig   bool
	UseRedis              bool
	UseOtel               bool
	UsePyroscope          bool
	UtcLogging            bool
	StartupExtensionFunc  func(Configuration) error
	ShutdownExtensionFunc func()
}

func Startup(cfg Config) (ctx context.Context, dbConn db.DbConnection,
	redisClient *redis.Client) {
	// .env
	err := loadDotEnvFile()
	if err != nil {
		startupPanic(err)
	}

	// Configuration
	err = ReadConfiguration(cfg.Configuration)
	if err != nil {
		startupPanic(err)
	}
	commonConfig := cfg.Configuration.GetCommonConfig()

	// Logger
	var hook zerolog.Hook
	if cfg.UseOtel {
		hook = correlationIDHook{}
	}
	configureLogger(commonConfig, cfg.UtcLogging, hook)

	// Log startup
	log.Info().Msgf("%s - starting...", commonConfig.ApplicationName)

	// Postgres
	var postgres db.Postgres
	if cfg.UsePostgres {
		postgres, dbConn = buildPostgres(commonConfig, cfg.UseExtendedPgConfig)
	}

	// Redis
	if cfg.UseRedis {
		redisClient = buildRedis(commonConfig)
	}

	// OpenTelemetry
	var tracer *trace.TracerProvider
	var metrics *metric.MeterProvider
	if cfg.UseOtel {
		tracer, metrics = buildOtel(commonConfig, cfg.BuildVersion, redisClient)
	}

	// Pyroscope
	var profiler *pyroscope.Profiler
	if cfg.UsePyroscope {
		profiler = buildPyroscope(commonConfig, cfg.BuildVersion)
	}

	// Extension function
	if cfg.StartupExtensionFunc != nil {
		err = cfg.StartupExtensionFunc(cfg.Configuration)
		if err != nil {
			startupPanic(err)
		}
	}

	// Graceful shutdown
	ctx = initGracefulShutdown(postgres, redisClient, tracer, metrics, profiler, cfg.ShutdownExtensionFunc)

	// Return everything
	return
}

func startupPanic(err error) {
	panic(fmt.Errorf("startup failed: %w", err))
}
