module github.com/qiangli/coreutils/external/otel

go 1.26.5

require (
	github.com/VictoriaMetrics/VictoriaLogs v1.121.1-0.20260714022219-19a73b567390
	github.com/VictoriaMetrics/VictoriaMetrics v1.146.1-0.20260630165203-c82127b6d4d1
	github.com/VictoriaMetrics/VictoriaTraces v0.9.4
	github.com/spf13/cobra v1.10.2
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/VictoriaMetrics/easyproto v1.2.0 // indirect
	github.com/VictoriaMetrics/fastcache v1.13.3 // indirect
	github.com/VictoriaMetrics/metrics v1.44.0 // indirect
	github.com/VictoriaMetrics/metricsql v0.87.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/valyala/gozstd v1.24.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	github.com/valyala/quicktemplate v1.8.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/jaegertracing/jaeger => github.com/qiangli/jaeger v0.0.0-20260426223533-5aaa7eb1f040

replace github.com/perses/perses => github.com/qiangli/perses v0.0.0-20260426190059-de437951b5e6
