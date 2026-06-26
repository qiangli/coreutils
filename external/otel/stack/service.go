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

	jaegerOTLPPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("jaeger otlp port: %w", err)
	}
	jaegerQueryPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("jaeger query port: %w", err)
	}
	jaegerQueryGRPCPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("jaeger query grpc port: %w", err)
	}
	mgr.AddComponent(NewJaegerComponentWithQueryGRPC(jaegerOTLPPort, jaegerQueryPort, jaegerQueryGRPCPort, filepath.Join(dataDir, "jaeger")))

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
		JaegerOTLPPort:         jaegerOTLPPort,
	}
	coll := NewEmbeddedCollector(collCfg, filepath.Join(dataDir, "collector"))
	mgr.AddComponent(coll)

	mgr.AddComponent(NewPrometheusComponent(
		filepath.Join(dataDir, "prometheus"),
		fmt.Sprintf("127.0.0.1:%d", collCfg.PrometheusPort),
	))
	mgr.AddComponent(NewAlertmanagerComponent())

	persesPort, err := allocate()
	if err != nil {
		return nil, fmt.Errorf("perses port: %w", err)
	}
	mgr.AddComponent(NewPersesComponent(
		persesPort,
		fmt.Sprintf("http://127.0.0.1:%d/prometheus", cfg.proxyPort()),
		filepath.Join(dataDir, "perses"),
	))

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
