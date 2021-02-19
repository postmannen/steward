package steward

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics are generally used to hold the structure around metrics
// handling
type metrics struct {
	// sayHelloNodes are the register where the register where nodes
	// who have sent an sayHello are stored. Since the sayHello
	// subscriber is a handler that will be just be called when a
	// hello message is received we need to store the metrics somewhere
	// else, that is why we store it here....at least for now.
	sayHelloNodes map[node]struct{}
	// The channel to pass metrics that should be processed
	metricsCh chan metricType
}

// HERE:
func newMetrics() *metrics {
	m := metrics{
		sayHelloNodes: make(map[node]struct{}),
	}
	m.metricsCh = make(chan metricType)

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
	// go func(ch chan metricType) {
	// 	for {
	// 		s.metrics.metricsCh <- metricType{
	// 			metric: prometheus.NewGauge(prometheus.GaugeOpts{
	// 				Name: "total_running_processes",
	// 				Help: "The current number of total running processes",
	// 			}),
	// 			value: float64(len(s.processes)),
	// 		}
	// 		time.Sleep(time.Second * 2)
	// 	}
	// }(s.metrics.metricsCh)

	// go func(ch chan metricType) {
	// 	for {
	// 		s.metrics.metricsCh <- metricType{
	// 			metric: prometheus.NewGauge(prometheus.GaugeOpts{
	// 				Name: "hello_nodes",
	// 				Help: "The current number of total nodes who have said hello",
	// 			}),
	// 			value: float64(len(s.metrics.sayHelloNodes)),
	// 		}
	// 		time.Sleep(time.Second * 2)
	// 	}
	// }(s.metrics.metricsCh)

	// Receive and process all metrics
	go func() {
		for {
			for f := range s.metrics.metricsCh {
				fmt.Printf("********** RANGED A METRIC = %v, %v\n", f.metric, f.value)
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

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
