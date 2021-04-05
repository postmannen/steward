package steward

import (
	"log"
	"net"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics are generally used to hold the structure around metrics
// handling
type metrics struct {
	// The channel to pass metrics that should be processed
	metricsCh chan metricType
	// host and port where prometheus metrics will be exported
	hostAndPort string
}

// newMetrics will prepare and return a *metrics
func newMetrics(hostAndPort string) *metrics {
	m := metrics{
		metricsCh:   make(chan metricType),
		hostAndPort: hostAndPort,
	}

	return &m
}

type Metricer interface {
	Set(float64)
}

type metricType struct {
	metric prometheus.Collector
	value  float64
}

func (s *server) startMetrics() {

	// Receive and process all metrics
	go func() {
		for {
			for f := range s.metrics.metricsCh {
				// // Try to register the metric of the interface type prometheus.Collector
				// prometheus.Register(f.metric)

				// Check the real type of the interface type
				switch ff := f.metric.(type) {
				case prometheus.Gauge:

					// Try to register. If it is already registered we need to check the error
					// to get the previously registered collector, and update it's value. If it
					// is not registered we register the new collector, and sets it's value.
					err := prometheus.Register(ff)
					if err != nil {
						are, ok := err.(prometheus.AlreadyRegisteredError)
						if ok {
							// already registered, use the one we have and put it into ff
							ff = are.ExistingCollector.(prometheus.Gauge)

						}
					}

					ff.Set(f.value)
				}

			}

		}
	}()

	//http.Handle("/metrics", promhttp.Handler())
	//http.ListenAndServe(":2112", nil)
	n, err := net.Listen("tcp", s.metrics.hostAndPort)
	if err != nil {
		log.Printf("error: failed to open prometheus listen port: %v\n", err)
		os.Exit(1)
	}
	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.Handler())
	http.Serve(n, m)
}
