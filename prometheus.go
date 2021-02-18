package steward

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	totalRunningProcesses prometheus.Gauge
}

func newMetrics() *metrics {
	m := metrics{
		totalRunningProcesses: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "total_running_processes",
			Help: "The current number of total running processes",
		}),
	}

	return &m
}

func (s *server) startMetrics() {

	go func() {
		for {
			s.metrics.totalRunningProcesses.Set(float64(len(s.processes)))
			time.Sleep(2 * time.Second)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
