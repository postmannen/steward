package steward

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	helloNodes            map[node]struct{}
	HelloNodes            prometheus.Gauge
	TotalRunningProcesses prometheus.Gauge
}

func newMetrics() *metrics {
	m := metrics{
		helloNodes: make(map[node]struct{}),
		TotalRunningProcesses: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "total_running_processes",
			Help: "The current number of total running processes",
		}),
		HelloNodes: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "hello_nodes",
			Help: "The current number of total nodes who have said hello",
		}),
	}

	return &m
}

func (s *server) startMetrics() {

	go func() {
		for {
			s.metrics.TotalRunningProcesses.Set(float64(len(s.processes)))
			s.metrics.HelloNodes.Set(float64(len(s.metrics.helloNodes)))
			time.Sleep(2 * time.Second)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
