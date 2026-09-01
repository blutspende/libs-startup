package startup

import (
	"context"
	"fmt"
	"time"

	db "github.com/blutspende/libs-db"
	"github.com/grafana/pyroscope-go"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

func buildPostgres(commonConfig *CommonConfiguration, extendedConfig bool) (postgres db.Postgres, dbConn db.DbConnection) {
	dbConfig := db.PgConfig{
		ApplicationName: commonConfig.ApplicationName,
		Host:            commonConfig.PostgresDB.Host,
		Port:            commonConfig.PostgresDB.Port,
		User:            commonConfig.PostgresDB.User,
		Pass:            commonConfig.PostgresDB.Pass,
		Database:        commonConfig.PostgresDB.Database,
		SSLMode:         commonConfig.PostgresDB.SSLMode,
	}
	if extendedConfig {
		dbConfig.MaxOpenConnections = new(commonConfig.PostgresDB.MaxOpenConnections)
		dbConfig.MaxIdleConnections = new(commonConfig.PostgresDB.MaxIdleConnections)
		dbConfig.ConnectionMaxLifetimeSeconds = new(commonConfig.PostgresDB.ConnectionMaxLifetimeSeconds)
		dbConfig.ConnectionMaxIdleTimeSeconds = new(commonConfig.PostgresDB.ConnectionMaxIdleTimeSeconds)
		dbConfig.UseOpenTelemetry = commonConfig.OpenTelemetry.Enable
	}

	postgres = db.NewPostgres(dbConfig)

	sqlConn, err := postgres.Connect(context.Background())
	if err != nil {
		log.Fatal().
			Err(err).
			Str("url", fmt.Sprintf("%s:%d", commonConfig.PostgresDB.Host, commonConfig.PostgresDB.Port)).
			Str("user", commonConfig.PostgresDB.User).
			Msg("unable to connect to postgres")
	}

	dbConn = db.NewDbConnection(sqlConn)

	if commonConfig.PostgresDB.EnableQueryLogging {
		dbConn.EnableQueryLogging()
	}

	return
}

func buildRedis(commonConfig *CommonConfiguration) (redisClient *redis.Client) {
	if commonConfig.Redis.Enable {
		redisClient = redis.NewClient(&redis.Options{
			Addr:               commonConfig.Redis.Address,
			Protocol:           2,
			Password:           commonConfig.Redis.Password,
			MaxRetries:         commonConfig.Redis.MaxRetries,
			DialerRetries:      commonConfig.Redis.DialerRetries,
			DialerRetryTimeout: time.Duration(commonConfig.Redis.DialerRetryTimeoutMs),
		})
	} else {
		log.Warn().Msg("Redis is disabled")
	}
	return redisClient
}

func buildOtel(commonConfig *CommonConfiguration, buildVersion string, redisClient *redis.Client) (tracer *trace.TracerProvider, metrics *metric.MeterProvider) {
	if commonConfig.OpenTelemetry.Enable {
		tracer, metrics = initOpenTelemetry(buildVersion, commonConfig)
		if redisClient != nil {
			if err := redisotel.InstrumentTracing(redisClient); err != nil {
				log.Warn().Err(err).Msg("enable redis opentelemetry tracing failed")
			}
			if err := redisotel.InstrumentMetrics(redisClient); err != nil {
				log.Warn().Err(err).Msg("enable redis opentelemetry metrics failed")
			}
		}
	} else {
		log.Warn().Msg("OpenTelemetry is disabled - this is not recommended for production systems")
	}
	return tracer, metrics
}

func buildPyroscope(commonConfig *CommonConfiguration, buildVersion string) (profiler *pyroscope.Profiler) {
	if commonConfig.Pyroscope.Enable {
		var err error
		profiler, err = pyroscope.Start(pyroscope.Config{
			ApplicationName: commonConfig.ApplicationName,
			ServerAddress:   commonConfig.Pyroscope.Server,
			Tags: map[string]string{
				"service":  commonConfig.ApplicationName,
				"instance": getInstanceID(),
				"version":  buildVersion,
			},
			ProfileTypes: []pyroscope.ProfileType{
				pyroscope.ProfileCPU,
				pyroscope.ProfileAllocObjects,
				pyroscope.ProfileAllocSpace,
				pyroscope.ProfileInuseObjects,
				pyroscope.ProfileInuseSpace,
			},
		})
		if err != nil {
			log.Warn().Err(err).Msg("starting Pyroscope profiler failed - continuing without Pyroscope profiling")
		}
	}
	return profiler
}
