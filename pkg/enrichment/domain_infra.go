package enrichment

// FaaSDomain returns the OTel FaaS semantic convention enricher.
// Covers: faas.trigger, faas.name, faas.invoked_name
// Semconv version: 1.20+
func FaaSDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "faas",
		Version:  "1.20.0",
		Priority: 50,
		DetectKeys: []string{
			"faas.trigger",
			"faas.invoked_name",
		},
		EnrichFunc: enrichFaaS,
	}
}

func enrichFaaS(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	trigger := attrs["faas.trigger"]
	if trigger == "" && attrs["faas.invoked_name"] == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "faas",
		Icon:     "λ ",
		BarChar:  "█",
		Color:    "blue",
	}

	if trigger != "" {
		h.Detail = trigger
	}
	if fname := attrs["faas.name"]; fname != "" {
		if trigger != "" {
			h.Detail = fname + " (" + trigger + ")"
		} else {
			h.Detail = fname
		}
	}

	if svc := attrs["service.name"]; svc != "" {
		h.ServiceName = svc
	}

	return h
}

// K8sDomain returns the OTel Kubernetes semantic convention enricher.
// Covers: k8s.pod.*, k8s.deployment.*, k8s.namespace.*, k8s.node.*
// Semconv version: 1.20+ (resource attributes)
func K8sDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "k8s",
		Version:  "1.20.0",
		Priority: 60,
		DetectKeys: []string{
			"k8s.deployment.name",
			"k8s.pod.name",
			"k8s.namespace.name",
			"k8s.container.name",
		},
		EnrichFunc: enrichK8s,
	}
}

func enrichK8s(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	// K8s attributes are typically resource-level, not span-level.
	// We enrich if a k8s span exists (e.g., from K8s API tracing).
	deploy := attrs["k8s.deployment.name"]
	pod := attrs["k8s.pod.name"]
	ns := attrs["k8s.namespace.name"]

	if deploy == "" && pod == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "k8s",
		Icon:     "☸ ",
		BarChar:  "█",
		Color:    "blue",
	}

	if deploy != "" {
		h.Detail = "deploy/" + deploy
		if ns != "" {
			h.Detail = ns + "/" + h.Detail
		}
	} else if pod != "" {
		h.Detail = "pod/" + pod
		if ns != "" {
			h.Detail = ns + "/" + h.Detail
		}
	}

	return h
}

// CloudDomain returns the OTel cloud semantic convention enricher.
// Covers: cloud.provider, cloud.region, cloud.platform
// Semconv version: 1.20+
func CloudDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "cloud",
		Version:  "1.20.0",
		Priority: 60,
		DetectKeys: []string{
			"cloud.provider",
			"cloud.platform",
		},
		EnrichFunc: enrichCloud,
	}
}

func enrichCloud(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	provider := attrs["cloud.provider"]
	if provider == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "cloud",
		Icon:     "☁ ",
		BarChar:  "█",
		Color:    "blue",
		Detail:   provider,
	}

	if region := attrs["cloud.region"]; region != "" {
		h.Detail += " (" + region + ")"
	}
	if platform := attrs["cloud.platform"]; platform != "" {
		h.Detail = platform
		if region := attrs["cloud.region"]; region != "" {
			h.Detail += " (" + region + ")"
		}
	}

	return h
}

// ContainerDomain returns the OTel container semantic convention enricher.
// Covers: container.id, container.name, container.image.name
// Semconv version: 1.20+
func ContainerDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "container",
		Version:  "1.20.0",
		Priority: 60,
		DetectKeys: []string{
			"container.id",
			"container.name",
			"container.image.name",
		},
		EnrichFunc: enrichContainer,
	}
}

func enrichContainer(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	containerName := attrs["container.name"]
	imageID := attrs["container.image.name"]
	if containerName == "" && imageID == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "container",
		Icon:     "📦 ",
		BarChar:  "█",
		Color:    "blue",
	}

	if containerName != "" {
		h.Detail = containerName
	} else if imageID != "" {
		h.Detail = imageID
	}

	return h
}

// ProcessDomain returns the OTel process semantic convention enricher.
// Covers: process.pid, process.executable.name, process.command
// Semconv version: 1.20+
func ProcessDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "process",
		Version:  "1.20.0",
		Priority: 70,
		DetectKeys: []string{
			"process.executable.name",
			"process.command",
		},
		EnrichFunc: enrichProcess,
	}
}

func enrichProcess(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	exe := attrs["process.executable.name"]
	cmd := attrs["process.command"]
	if exe == "" && cmd == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "process",
		Icon:     "⚙ ",
		BarChar:  "█",
		Color:    "blue",
	}

	if exe != "" {
		h.Detail = exe
	} else if cmd != "" {
		h.Detail = cmd
	}

	return h
}

// NetworkDomain returns the OTel network semantic convention enricher.
// Covers: network.protocol.name, network.transport, net.*
// Semconv version: 1.20+
func NetworkDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "network",
		Version:  "1.20.0",
		Priority: 70,
		DetectKeys: []string{
			"network.protocol.name",
			"network.transport",
		},
		EnrichFunc: enrichNetwork,
	}
}

func enrichNetwork(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
	proto := attrs["network.protocol.name"]
	transport := attrs["network.transport"]
	if proto == "" && transport == "" {
		return SpanHints{}
	}

	h := SpanHints{
		Category: "network",
		Icon:     "🔗 ",
		BarChar:  "█",
		Color:    "blue",
	}

	if proto != "" {
		h.Detail = proto
		if version := attrs["network.protocol.version"]; version != "" {
			h.Detail += "/" + version
		}
	} else if transport != "" {
		h.Detail = transport
	}

	return h
}

// GenericDomain returns the catch-all enricher for unrecognized spans.
// Priority 200: always last in the chain.
func GenericDomain() *DomainEnricher {
	return &DomainEnricher{
		Domain:   "generic",
		Version:  "1.0.0",
		Priority: 200,
		EnrichFunc: func(name string, attrs map[string]string, isZeroDuration bool) SpanHints {
			h := SpanHints{
				Category: "operation",
				Icon:     "● ",
				BarChar:  "█",
				Color:    "blue",
			}

			if isZeroDuration {
				h.IsMarker = true
				h.Category = "marker"
				h.SortPriority = -1
				h.Icon = "▲ "
				h.BarChar = "▲"
			}

			// OTel status code
			switch attrs["otel.status_code"] {
			case "OK":
				h.Outcome = "success"
				h.Color = "green"
			case "ERROR":
				h.Outcome = "failure"
				h.Color = "red"
			}

			// Span kind icons
			if h.Icon == "● " {
				switch attrs["otel.span_kind"] {
				case "SERVER":
					h.Icon = "⇣ "
				case "CLIENT":
					h.Icon = "⇢ "
				case "PRODUCER":
					h.Icon = "⇡ "
				case "CONSUMER":
					h.Icon = "⇠ "
				}
			}

			// Service context
			if svc := attrs["service.name"]; svc != "" {
				h.ServiceName = svc
			}
			if env := attrs["deployment.environment"]; env != "" {
				h.Environment = env
			}

			// Artifact grouping (GitHub-specific but no better place)
			if artifactName, ok := attrs["github.artifact_name"]; ok && artifactName != "" {
				h.GroupKey = "artifact"
			}

			return h
		},
	}
}
