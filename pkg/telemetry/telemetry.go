// Package telemetry gives bashy an OpenTelemetry voice.
//
// bashy could already RUN an observability stack (`bashy otel` — collector, Jaeger,
// VictoriaMetrics, Perses) and emitted NOTHING into it. A collector with no data. The
// umbrella's own trace-propagation contract lists service.name values for ycode,
// cloudbox-hub, outpost, loom and act_runner — and not for bashy. The foundation of the
// stack was the one tier that was invisible.
//
// # What this is FOR, which is not "instrument everything"
//
// Six hours of debugging produced five bugs, and every one of them was invisible in the
// same way:
//
//	"the agent can't converge"    -> our 25-turn cap, and truncation EXITED 0
//	"its delegates fail"          -> our cap discarded their work; the error carried nothing
//	"rate limits killed the run"  -> three 429s, all recovered; no per-provider signal existed
//	"the config didn't apply"     -> the field was never merged, and nothing said so
//	"the model did nothing"       -> a 4096-byte pty truncation nobody could see
//
// Not one was a mystery about WHAT the code did. Every one was a number that got used
// without saying where it came from, or a bound that got hit without saying so.
//
// So this package has exactly two jobs, and they are the two that would have caught all
// five:
//
//	PROVENANCE — a value recorded next to WHERE IT CAME FROM. A number without its
//	             provenance cannot be audited. (The one signal that did catch a bug today
//	             was a log line printing `from_provider=false` next to a token count.)
//
//	BOUNDS     — a limit records when it BINDS, always, as a first-class event. A bound
//	             you cannot see is not a bound, it is a trap. Truncation, caps, timeouts,
//	             rate limits: each is a fact about the harness, and each was being
//	             silently absorbed.
//
// # Discipline
//
// Configuration is pure standard OTEL env vars (OTEL_EXPORTER_OTLP_ENDPOINT,
// OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES, OTEL_TRACES_SAMPLER). When the endpoint
// is unset, everything here is a NO-OP with no allocation and no goroutine — bashy is a
// shell, and a shell that pays for telemetry it is not exporting is a shell nobody uses.
// The global propagator is still installed even in no-op mode, so the wire-format
// contract survives without a collector.
package telemetry

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ServiceName is bashy's identity on the OTel plane. It joins the umbrella's existing
// service.name set (ycode, cloudbox-hub, outpost, loom, act_runner) — which it was
// missing from.
const ServiceName = "bashy"

// instrumentationName identifies this instrumentation library in every span it emits.
const instrumentationName = "github.com/qiangli/coreutils/pkg/telemetry"

var (
	initOnce  sync.Once
	provider  *sdktrace.TracerProvider
	mprovider *sdkmetric.MeterProvider
	enabled   bool
)

// Enabled reports whether telemetry is actually exporting. Callers should not need this
// — every helper here is safe when it is false — but a `bashy doctor` line that says so
// out loud is worth more than a silent no-op.
func Enabled() bool { return enabled }

// Init wires the OTel plane from standard env vars, once.
//
// With OTEL_EXPORTER_OTLP_ENDPOINT unset it installs the global propagator (so the wire
// contract holds) and nothing else: no exporter, no batcher, no goroutine, no cost.
func Init(ctx context.Context) (shutdown func(context.Context) error) {
	shutdown = func(context.Context) error { return nil }

	initOnce.Do(func() {
		// The propagator goes in either way. A hop that drops traceparent orphans every
		// span downstream of it, and that is true whether or not THIS process exports.
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))

		endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
		if endpoint == "" {
			return // no-op mode. Deliberately, and completely.
		}

		// BOTH PROTOCOLS, chosen by the standard env var. Not one, hard-coded.
		//
		// The embedded stack no longer runs an OTel Collector — the only thing in it that
		// spoke gRPC. Every Victoria component ingests OTLP/HTTP natively and the proxy
		// fans /v1/{traces,logs,metrics} out to them, so the stack's own endpoint is HTTP.
		//
		// But hard-coding HTTP would have broken every gRPC collector in the world,
		// including the umbrella's own otlp-receiver — which is exactly how I found out,
		// because the first version silently sent to nothing and I nearly recorded another
		// process's spans as proof that it worked.
		//
		// OTEL_EXPORTER_OTLP_PROTOCOL is the standard knob: "grpc" or "http/protobuf".
		// The spec's default is http/protobuf, and so is ours.
		var exp sdktrace.SpanExporter
		var err error
		switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"))) {
		case "grpc":
			exp, err = otlptracegrpc.New(ctx)
		default:
			exp, err = otlptracehttp.New(ctx)
		}
		if err != nil {
			// A shell must not fail to start because a collector is down. Say so once,
			// on stderr, and carry on in no-op mode — SILENTLY degrading here would be
			// the very bug this package exists to make visible.
			os.Stderr.WriteString("bashy: telemetry disabled — OTLP exporter: " + err.Error() + "\n")
			return
		}

		svc := os.Getenv("OTEL_SERVICE_NAME")
		if svc == "" {
			svc = ServiceName
		}
		res, _ := resource.Merge(resource.Default(), resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(svc),
		))

		provider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(provider)
		var mexp sdkmetric.Exporter
		switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"))) {
		case "grpc":
			mexp, _ = otlpmetricgrpc.New(ctx)
		default:
			mexp, _ = otlpmetrichttp.New(ctx)
		}
		if mexp != nil {
			mprovider = sdkmetric.NewMeterProvider(
				sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mexp)),
				sdkmetric.WithResource(res),
			)
			otel.SetMeterProvider(mprovider)
		}

		enabled = true

		// SAY SO. A feature that is silently on is as hard to debug as one that is
		// silently off — and "is telemetry actually running?" is the first question
		// anyone asks when the collector is empty. One line, to stderr, once.
		if os.Getenv("BASHY_TELEMETRY_QUIET") == "" {
			os.Stderr.WriteString("bashy: telemetry on → " + endpoint + " (service=" + svc + ")\n")
		}

		shutdown = func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if mprovider != nil {
				_ = mprovider.Shutdown(ctx)
			}
			return provider.Shutdown(ctx)
		}
	})

	return shutdown
}

// Tracer returns bashy's tracer. Safe before Init: the global provider is a no-op until
// a host configures one.
func Tracer() trace.Tracer { return otel.Tracer(instrumentationName) }

// standalone starts a span for a fact that has no parent — a bound that bound, or a
// number whose provenance matters, recorded from a code path nobody thought to
// instrument.
//
// It reads the GLOBAL provider, so it is free (a no-op span, never recorded) in a process
// that configured no telemetry, and real in one that did.
func standalone(ctx context.Context, name string) (context.Context, func()) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx, name)
	return ctx, func() { span.End() }
}

// --- The two things this package exists for --------------------------------------

// Provenance records a VALUE together with WHERE IT CAME FROM.
//
// This is the whole point. A token count of 6482 tells you nothing; a token count of 6482
// that the PROVIDER REPORTED tells you the context gate is working, and a token count of
// 6482 that we ESTIMATED tells you it is not. Today, the only reason a dead
// usage-plumbing bug was caught at all is that one log line printed the second thing next
// to the first.
//
//	source: where the number came from — "provider", "estimate", "config", "default",
//	        "cache", "measured", "declared". Whatever it is, SAY it.
//
// A number without its provenance cannot be audited, and an unauditable number will
// eventually be wrong in a way nobody can see.
func Provenance(ctx context.Context, name string, value int64, source string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		// NO ACTIVE SPAN IS NOT A REASON TO SAY NOTHING.
		//
		// The first version returned here, and it silently emitted nothing at every call
		// site that happened not to sit inside an instrumented span — which was most of
		// them, including the truncation path this exists to record. A helper that
		// quietly does nothing when its precondition is unmet is the exact disease this
		// package was built to detect, committed inside the detector.
		//
		// A fact worth recording is worth its own span.
		var end func()
		ctx, end = standalone(ctx, "value."+name)
		defer end()
		span = trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return // genuinely no provider configured; now it really is free
		}
	}
	span.AddEvent("value", trace.WithAttributes(
		attribute.String("value.name", name),
		attribute.Int64("value.amount", value),
		attribute.String("value.source", source),
	))
}

// BoundHit records that a LIMIT BOUND — that something was cut short, capped, truncated,
// throttled or timed out.
//
// Every failure this package was built in response to was a bound that bound silently:
//
//	a 25-iteration cap that stopped an agent mid-investigation and EXITED 0
//	a 15-iteration subagent cap that discarded its findings and returned an error
//	a 4096-byte pty write that truncated a prompt with no indication
//	a rate limit absorbed by a retry that nothing counted
//
// A bound you cannot see is not a bound, it is a trap. If a limit changes what the system
// did, it says so here — always, even when the run recovers. ESPECIALLY when the run
// recovers, because a bound that binds and recovers is the one nobody investigates until
// it stops recovering.
func BoundHit(ctx context.Context, kind string, limit int64, actual int64, detail string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		// See Provenance: a bound that binds outside an instrumented span still bound.
		// It gets its own span rather than vanishing.
		var end func()
		ctx, end = standalone(ctx, "bound."+kind)
		defer end()
		span = trace.SpanFromContext(ctx)
		if !span.IsRecording() {
			return
		}
	}
	span.AddEvent("bound.hit", trace.WithAttributes(
		attribute.String("bound.kind", kind), // "iterations", "bytes", "rate_limit", "timeout", "context_window"
		attribute.Int64("bound.limit", limit),
		attribute.Int64("bound.actual", actual),
		attribute.String("bound.detail", detail),
	))
	span.SetAttributes(attribute.Bool("bound.was_hit", true))
}
