package stack

import (
	"fmt"
	"path/filepath"
)

type Service struct {
	Manager       *StackManager
	CollectorAddr string
	HTTPAddr      string
}

func NewService(cfg *Config, dataDir string) (*Service, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	mgr := NewStackManager(cfg, dataDir)

	allocate := AllocatePort
	vlogsPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("victorialogs port: %w", err)
	}
	mgr.AddComponent(NewVictoriaLogsComponent(vlogsPort, filepath.Join(dataDir, "vlogs")))

	// JAEGER IS GONE. VictoriaTraces stores the traces and serves its own vmui.
	//
	//	jaeger        2,240 transitive dependencies
	//	victoria      ~113  (the same logstorage engine as VictoriaLogs)
	//
	// Twenty times the weight, for the same job.
	vtracesPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("victoria-traces port: %w", err)
	}
	mgr.AddComponent(NewVictoriaTracesComponent(vtracesPort, filepath.Join(dataDir, "vtraces")))

	collGRPCPort, err := resolveOTLPPort(cfg.OTLPGRPCPort, 4317, "OTLP gRPC", allocate)
	if err != nil {
		return nil, err
	}
	collHTTPPort, err := resolveOTLPPort(cfg.OTLPHTTPPort, 4318, "OTLP HTTP", allocate)
	if err != nil {
		return nil, err
	}
	collPromPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("collector prometheus port: %w", err)
	}
	collCfg := CollectorConfig{
		GRPCPort:               collGRPCPort,
		HTTPPort:               collHTTPPort,
		PrometheusPort:         collPromPort,
		VictoriaLogsPort:       vlogsPort,
		VictoriaLogsPathPrefix: "/logs",
		VictoriaTracesPort:     vtracesPort,
	}
	coll := NewEmbeddedCollector(collCfg, filepath.Join(dataDir, "collector"))
	mgr.AddComponent(coll)

	mgr.AddComponent(NewPrometheusComponent(
		filepath.Join(dataDir, "prometheus"),
		fmt.Sprintf("127.0.0.1:%d", collCfg.PrometheusPort),
	))
	// PERSES AND ALERTMANAGER ARE GONE.
	//
	// Perses cost 1,478 transitive dependencies to render dashboards — for a stack whose
	// storage layer (VictoriaLogs) does its entire job in 113. Twenty times the weight of
	// the thing it was decorating. And Victoria ships vmui, a query UI that is already in
	// the binary, so the dashboards were a second UI over the same data.
	//
	// Alertmanager routes alerts to pagers and Slack. This is a LOCAL DEBUGGING STACK on
	// a developer's laptop. Nobody is being paged.
	//
	// Measured, not asserted: see the dep table in the commit message.

	return &Service{
		Manager:       mgr,
		CollectorAddr: coll.GRPCAddr(),
		HTTPAddr:      fmt.Sprintf("%s:%d", cfg.proxyBindAddr(), cfg.proxyPort()),
	}, nil
}

func resolveOTLPPort(configured, defaultPort int, label string, allocate func() (int, error)) (int, error) {
	if configured < 0 {
		return allocate()
	}
	port := defaultPort
	if configured > 0 {
		port = configured
	}
	if !IsPortAvailable(port) {
		return 0, fmt.Errorf("%s port %d already in use; configure a different port or set a negative value for ephemeral allocation", label, port)
	}
	return port, nil
}
