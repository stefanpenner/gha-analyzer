package otlpfile

import (
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// StatusFromCode converts an OTel status code string to a Status.
func StatusFromCode(code string, description string) sdktrace.Status {
	st := sdktrace.Status{Description: description}
	switch code {
	case "OK", "STATUS_CODE_OK":
		st.Code = codes.Ok
	case "ERROR", "STATUS_CODE_ERROR":
		st.Code = codes.Error
	default:
		st.Code = codes.Unset
	}
	return st
}
