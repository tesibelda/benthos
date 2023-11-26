package pure

import (
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/benthosdev/benthos/v4/internal/bundle"
	"github.com/benthosdev/benthos/v4/internal/component/tracer"
	"github.com/benthosdev/benthos/v4/internal/docs"
)

func init() {
	_ = bundle.AllTracers.Add(func(c tracer.Config, nm bundle.NewManagement) (trace.TracerProvider, error) {
		return noop.NewTracerProvider(), nil
	}, docs.ComponentSpec{
		Name:    "none",
		Type:    docs.TypeTracer,
		Status:  docs.StatusStable,
		Summary: `Do not send tracing events anywhere.`,
		Config:  docs.FieldObject("", ""),
	})
}
