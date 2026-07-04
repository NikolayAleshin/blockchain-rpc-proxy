// Package observability wires opt-in telemetry: OpenTelemetry traces (OTLP),
// Prometheus metrics, trace-correlated logs, and Sentry error tracking. The
// golden rule: with no OTLP endpoint (or OTEL_SDK_DISABLED) the tracer is a no-op
// — the proxy runs identically with zero telemetry infrastructure. See
// docs/OBSERVABILITY.md and docs/ADR/0006-observability-otel.md.
package observability

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"blockchain-rpc-proxy/internal/config"
)

// Telemetry holds the telemetry lifecycle. Shutdown flushes and closes exporters.
type Telemetry struct {
	TracingEnabled bool
	shutdowns      []func(context.Context) error
}

// SetupTracing configures W3C propagation (always) and, when an OTLP endpoint is
// set and the SDK is not disabled, an OTLP/gRPC trace exporter with a batching
// processor and parent-based ratio sampling. Otherwise tracing is a no-op.
func SetupTracing(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Telemetry, error) {
	// Propagation is cheap and harmless even with a no-op tracer.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	t := &Telemetry{}
	if cfg.OTelSDKDisabled || cfg.OTLPEndpoint == "" {
		logger.Info("tracing export disabled (no OTLP endpoint); tracer is no-op")
		return t, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(), // OTEL_RESOURCE_ATTRIBUTES, OTEL_SERVICE_NAME
		resource.WithAttributes(attribute.String("service.name", cfg.OTelServiceName)),
	)
	if err != nil {
		return nil, err
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(normalizeEndpoint(cfg.OTLPEndpoint)))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp), // batched, bounded queue, drop-on-full
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.OTelSamplerArg))),
	)
	otel.SetTracerProvider(tp)

	t.TracingEnabled = true
	t.shutdowns = append(t.shutdowns, tp.Shutdown)
	logger.Info("tracing export enabled", slog.String("endpoint", cfg.OTLPEndpoint))
	return t, nil
}

// Shutdown flushes exporters; safe to call when tracing is disabled.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var firstErr error
	for _, fn := range t.shutdowns {
		if err := fn(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// normalizeEndpoint ensures the OTLP endpoint has a scheme so WithEndpointURL can
// parse it; a bare host:port is treated as insecure gRPC.
func normalizeEndpoint(ep string) string {
	if !strings.Contains(ep, "://") {
		return "http://" + ep
	}
	return ep
}
