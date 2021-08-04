package steward

import (
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics are generally used to hold the structure around metrics
// handling
type metrics struct {
	// The channel to pass metrics that should be processed
	promRegistry *prometheus.Registry
	// host and port where prometheus metrics will be exported
	hostAndPort string
}

// newMetrics will prepare and return a *metrics
func newMetrics(hostAndPort string) *metrics {
	m := metrics{
		promRegistry: prometheus.NewRegistry(),
		hostAndPort:  hostAndPort,
	}

	return &m
}

func (m *metrics) start() error {

	//http.Handle("/metrics", promhttp.Handler())
	//http.ListenAndServe(":2112", nil)
	n, err := net.Listen("tcp", m.hostAndPort)
	if err != nil {
		return fmt.Errorf("error: startMetrics: failed to open prometheus listen port: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	err = http.Serve(n, mux)
	if err != nil {
		return fmt.Errorf("error: startMetrics: failed to start http.Serve: %v", err)
	}

	return nil
}
