package startup

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
)

const ContextKeyCorrelation = "correlation_id"

type correlationIDHook struct{}

func (h correlationIDHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	ctx := e.GetCtx()
	if ctx == nil {
		return
	}

	if cid, ok := ctx.Value(ContextKeyCorrelation).(string); ok && cid != "" {
		e.Str("correlationID", cid)
	}
}

// Identify the current instance e.g. when running in a cluster the name of the pod
func getInstanceID() string {
	if v := os.Getenv("POD_UID"); v != "" {
		return v // K8s
	}
	if h, _ := os.Hostname(); h != "" {
		return h // otherwise
	}
	return uuid.NewString() //-- and if nothing else is available
}

// Initialize OpenTelemetry tracing and metrics
//
//	traceCollectorEndpoint: host:port of the OTLP trace collector
//	metricsCollectorEndpoint: host:port of the OTLP metrics collector
func initOpenTelemetry(buildVersion string, configuration *CommonConfiguration) (tp *trace.TracerProvider, mp *metric.MeterProvider) {
	ctx := context.Background()

	//-- create resource describing this service
	r, err := resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceName(configuration.ApplicationName),
			semconv.ServiceInstanceID(getInstanceID()),
			semconv.ServiceVersion(buildVersion),
		),
	)
	if err != nil {
		log.Warn().Err(err).Msg("initialize resource for OpenTelemetry failed: continuing without OpenTelemetry...")
		return nil, nil
	}

	//-- initialize Tracing
	if configuration.OpenTelemetry.TraceCollectorEndpoint != "" {
		traceCollectorEndpoint := configuration.OpenTelemetry.TraceCollectorEndpoint
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithInsecure(), // without tls only works for local/secure networks like inside k8s cluster
			otlptracegrpc.WithEndpoint(traceCollectorEndpoint),
			otlptracegrpc.WithTimeout(time.Duration(configuration.OpenTelemetry.TraceCollectorTimeoutSeconds)*time.Second),
		)
		if err != nil {
			log.Warn().Err(err).Msg("initialize OpenTelemetry traces failed. Continuing with OpenTelemetry traces...")
		} else {
			tp = trace.NewTracerProvider(
				trace.WithBatcher(exp,
					trace.WithMaxQueueSize(2048),
					trace.WithMaxExportBatchSize(512),
				),
				trace.WithResource(r),
			)
			//-- set global propagator to trace context is required so that the webservice can use it to link the traces to the incoming request
			// this is in the api: e.Use(otelecho.Middleware(serviceName, otelecho.WithTracerProvider(otel.GetTracerProvider()))
			otel.SetTextMapPropagator(propagation.TraceContext{})

			//-- set global tracer. with this tracing already works
			otel.SetTracerProvider(tp)
			log.Info().Msg("OpenTelemetry is enabled, sending traces to " + traceCollectorEndpoint)
		}
	}

	//-- initialize Metrics
	if configuration.OpenTelemetry.MetricsCollectorEndpoint != "" {
		metricsCollectorEndpoint := configuration.OpenTelemetry.MetricsCollectorEndpoint
		expm, err := otlpmetricgrpc.New(
			ctx,
			otlpmetricgrpc.WithEndpoint(metricsCollectorEndpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			log.Warn().Err(err).Msg("initialize OpenTelemetry metrics failed. Continuing with OpenTelemetry metrics...")
		} else {
			mp = metric.NewMeterProvider(
				metric.WithResource(r),
				metric.WithReader(
					metric.NewPeriodicReader(expm,
						metric.WithInterval(time.Duration(configuration.OpenTelemetry.MetricsReaderIntervalSeconds)*time.Second),
						metric.WithTimeout(time.Duration(configuration.OpenTelemetry.MetricsReaderTimeoutSeconds)*time.Second),
					),
				),
				metric.WithView(
					metric.NewView(
						metric.Instrument{
							Name: "http.server.request.duration",
						},
						metric.Stream{
							Aggregation: metric.AggregationExplicitBucketHistogram{
								Boundaries: []float64{0.05, 0.1, 0.2, 0.3, 0.5, 0.75, 1, 1.5, 2, 3, 5},
							},
						},
					),
				),
				//-- process metrics like CPU, Memory, FD, Threads
				metric.WithView(
					metric.NewView(
						metric.Instrument{
							Name: "process.*",
						},
						metric.Stream{},
					),
				),

				//-- runtime metrics (GC, Goroutines, Heap, Stack)
				metric.WithView(
					metric.NewView(
						metric.Instrument{
							Name: "process.runtime.go.*",
						},
						metric.Stream{},
					),
				),
			)

			otel.SetMeterProvider(mp)
			log.Info().Msg("OpenTelemetry is enabled, sending metrics to " + metricsCollectorEndpoint)
		}

		//-- RAM, gc, goroutines
		if err = runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Duration(configuration.OpenTelemetry.ReadMemStatsIntervalSeconds) * time.Second)); err != nil {
			log.Warn().Err(err).Msg("starting OpenTelemetry MemStats runtime failed")
		}

		//-- Process metrics like CPU, Memory, FD, Threads
		//-- without this process start thing no process metrics are collected
		if err = host.Start(); err != nil {
			log.Warn().Err(err).Msg("starting OpenTelemetry host failed")
		}
	}

	return tp, mp
}
