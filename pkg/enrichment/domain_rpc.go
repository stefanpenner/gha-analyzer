package enrichment

// RPCDomain returns the OTel RPC semantic convention enricher.
// Covers: rpc.system, rpc.service, rpc.method, rpc.grpc.*
// Semconv version: 1.20+ (experimental)
func RPCDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "rpc",
		Version:  "1.20.0",
		Priority: 50,
		DetectKeys: []string{
			"rpc.system",
		},
		EnrichFunc: enrichRPC,
	}
}

func enrichRPC(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	rpcSystem := attrs["rpc.system"]
	if rpcSystem == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "rpc",
		Icon:     "⇌ ",
		BarChar:  "█",
		Color:    "blue",
		Detail:   rpcSystem,
	}

	if svc := attrs["rpc.service"]; svc != "" {
		if method := attrs["rpc.method"]; method != "" {
			h.Detail = rpcSystem + " " + svc + "/" + method
		} else {
			h.Detail = rpcSystem + " " + svc
		}
	}

	if svc := attrs["service.name"]; svc != "" {
		h.ServiceName = svc
	}

	return h
}
