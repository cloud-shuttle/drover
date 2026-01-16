// Package telemetry provides OpenTelemetry observability for Drover
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const (
	// DefaultServiceName is the default service name for telemetry
	DefaultServiceName = "drover"

	// DefaultServiceVersion is populated at build time
	DefaultServiceVersion = "dev"

	// DefaultOTLPEndpoint is the default OTLP collector endpoint
	DefaultOTLPEndpoint = "localhost:4317"

	// EnvOTLPEndpoint is the environment variable for custom OTLP endpoint
	EnvOTLPEndpoint = "DROVER_OTEL_ENDPOINT"

	// EnvOTelEnabled is the environment variable to enable/disable telemetry
	EnvOTelEnabled = "DROVER_OTEL_ENABLED"
)

// Config holds telemetry configuration
type Config struct {
	// ServiceName is the name of the service
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment
	Environment string

	// OTLPEndpoint is the OTLP collector endpoint (host:port)
	OTLPEndpoint string

	// Enabled determines if telemetry is active
	Enabled bool

	// SampleRate is the trace sampling rate (0.0 to 1.0)
	SampleRate float64
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	cfg := &Config{
		ServiceName:    DefaultServiceName,
		ServiceVersion: DefaultServiceVersion,
		Environment:    getEnvironment(),
		OTLPEndpoint:   getOTLPEndpoint(),
		Enabled:        isEnabled(),
		SampleRate:     1.0, // Sample all traces in development
	}

	// Lower sample rate in production
	if cfg.Environment == "production" {
		cfg.SampleRate = 0.1
	}

	return cfg
}

// getEnvironment determines the deployment environment
func getEnvironment() string {
	if env := os.Getenv("DROVER_ENV"); env != "" {
		return env
	}
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		return env
	}
	return "development"
}

// getOTLPEndpoint returns the OTLP endpoint from env or default
func getOTLPEndpoint() string {
	if endpoint := os.Getenv(EnvOTLPEndpoint); endpoint != "" {
		return endpoint
	}
	return DefaultOTLPEndpoint
}

// isEnabled checks if telemetry is enabled via environment variable
func isEnabled() bool {
	if enabled := os.Getenv(EnvOTelEnabled); enabled != "" {
		return enabled == "true" || enabled == "1"
	}
	return false // Disabled by default
}

// Init initializes OpenTelemetry with the given configuration.
// Returns a shutdown function that should be called when the application exits.
func Init(ctx context.Context, cfg *Config) (shutdown func(context.Context) error, err error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithOSType(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize tracing
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)

	// Initialize metrics
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	// Set global providers
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize metric instruments
	if err := initMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	// Return shutdown function
	return func(ctx context.Context) error {
		var errs []error

		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}

		if len(errs) > 0 {
			return fmt.Errorf("telemetry shutdown errors: %v", errs)
		}
		return nil
	}, nil
}

// MustInit initializes telemetry or panics on error.
// Useful for main package initialization.
func MustInit(ctx context.Context, cfg *Config) func(context.Context) error {
	shutdown, err := Init(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize telemetry: %v", err))
	}
	return shutdown
}
