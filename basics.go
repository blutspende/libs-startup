package startup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	db "github.com/blutspende/libs-db"
	"github.com/grafana/pyroscope-go"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

var ErrFailedToLoadDotEnvFile = errors.New("failed to load .env file")

func loadDotEnvFile() error {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("%w: %w", ErrFailedToLoadDotEnvFile, err)
		}
	}
	return nil
}

func configureLogger(configuration *CommonConfiguration, utc bool, hook zerolog.Hook) {
	logLevel := configuration.ZeroLogLevel
	zerolog.SetGlobalLevel(logLevel)

	if utc {
		zerolog.TimeFieldFormat = "2006-01-02T15:04:05.999Z"
		zerolog.TimestampFunc = func() time.Time {
			return time.Now().UTC()
		}
	} else {
		zerolog.TimeFieldFormat = "2006-01-02T15:04:05.999Z07:00"
		zerolog.TimestampFunc = func() time.Time {
			return time.Now()
		}
	}

	consoleWriter := zerolog.NewConsoleWriter()
	consoleLogger := zerolog.New(consoleWriter)
	if hook != nil {
		consoleLogger = consoleLogger.Hook(hook)
	}

	consoleLoggerContext := consoleLogger.With()
	if logLevel <= zerolog.DebugLevel {
		consoleLoggerContext = consoleLoggerContext.Caller()
	}
	consoleLoggerContext = consoleLoggerContext.Caller().Stack().Timestamp()

	log.Logger = consoleLoggerContext.Logger()
	zerolog.DefaultContextLogger = &log.Logger
}

func initGracefulShutdown(postgres db.Postgres, redisClient *redis.Client, tracer *trace.TracerProvider, metrics *metric.MeterProvider, profiler *pyroscope.Profiler, extensionFunc func()) context.Context {
	// Init cancelable context for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM) //nolint

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := <-sigChan
		log.Info().Msgf("received termination signal: %+v", sig)
		log.Info().Msg("graceful shutdown initiated")

		log.Info().Msg("canceling context")
		cancel()

		if postgres != nil {
			log.Info().Msg("closing DB connection")
			err := postgres.Close()
			if err != nil {
				log.Error().Err(err).Msg("failed to close DB connection")
			}
		}

		if redisClient != nil {
			log.Info().Msg("closing Redis connection")
			if err := redisClient.Close(); err != nil {
				log.Error().Err(err).Msg("failed to close Redis client")
			}
		}

		if profiler != nil {
			log.Info().Msg("stopping Pyroscope profiler")
			err := profiler.Stop()
			if err != nil {
				log.Error().Err(err).Msg("failed to shutdown Pyroscope profiler")
			}
		}

		if tracer != nil {
			log.Info().Msg("shutting down OpenTelemetry tracer")
			err := tracer.Shutdown(context.Background())
			if err != nil {
				log.Error().Err(err).Msg("failed to shutdown OpenTelemetry tracer")
			}
		}
		if metrics != nil {
			log.Info().Msg("shutting down OpenTelemetry meter")
			err := metrics.Shutdown(context.Background())
			if err != nil {
				log.Error().Err(err).Msg("failed to shutdown OpenTelemetry meter")
			}
		}

		if extensionFunc != nil {
			extensionFunc()
		}

		log.Info().Msg("shutting down")
		os.Exit(0)
	}()

	return ctx
}
