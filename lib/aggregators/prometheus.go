package aggregators

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/docker/api/types/events"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
)

type PrometheusConfig struct {
	Path   string
	Port   int
	Labels []string
}

type Prometheus struct {
	labels []string
	port   int
	path   string
	logger *log.Entry

	containerActions *prometheus.CounterVec
	imageActions     *prometheus.CounterVec
	networkActions   *prometheus.CounterVec
	pluginActions    *prometheus.CounterVec
	volumeActions    *prometheus.CounterVec

	// not on stable yet
	// serviceActions   *prometheus.CounterVec
	// nodeActions      *prometheus.CounterVec
}

func NewPrometheus(cfg PrometheusConfig) (agg Prometheus, err error) {
	agg.logger = log.WithField("aggregator", "prometheus")
	agg.port = cfg.Port
	agg.path = cfg.Path
	agg.labels = cfg.Labels

	var containerActionLabels = []string{"action"}
	for _, label := range agg.labels {
		containerActionLabels = append(
			containerActionLabels,
			strings.Replace(label, ".", "_", -1))
	}

	agg.containerActions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:      "container_action",
		Help:      "Docker container actions performed",
		Subsystem: "devents",
	}, containerActionLabels)

	agg.imageActions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:      "image_action",
		Help:      "Docker image actions performed",
		Subsystem: "devents",
	}, []string{"action"})

	agg.networkActions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:      "network_action",
		Help:      "Docker network actions performed",
		Subsystem: "devents",
	}, []string{"action", "name", "type"})

	agg.pluginActions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:      "plugin_action",
		Help:      "Docker plugin actions performed",
		Subsystem: "devents",
	}, []string{"action", "name"})

	agg.volumeActions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:      "volume_action",
		Help:      "Docker volume actions performed",
		Subsystem: "devents",
	}, []string{"action", "driver"})

	prometheus.MustRegister(agg.containerActions)
	prometheus.MustRegister(agg.imageActions)
	prometheus.MustRegister(agg.networkActions)
	prometheus.MustRegister(agg.pluginActions)
	prometheus.MustRegister(agg.volumeActions)

	agg.logger.Info("aggregator initialized")
	return
}

func (p Prometheus) Run(evs <-chan events.Message, errs <-chan error) {
	var handlerErrChan = make(chan error)

	go func() {
		http.Handle(p.path, promhttp.Handler())
		err := http.ListenAndServe(fmt.Sprintf(":%d", p.port), nil)
		if err != nil {
			handlerErrChan <- err
		}
	}()

	p.logger.Info("listening to events")
	for {
		select {
		case err := <-handlerErrChan:
			p.logger.
				WithError(err).
				Error("metrics HTTP handler failed")
		case err := <-errs:
			p.logger.
				WithError(err).
				Error("events retrieval failed")
		case ev := <-evs:
			switch ev.Type {
			case events.ContainerEventType:
				labelValues := []string{
					ev.Action,
				}

				attrs := ev.Actor.Attributes
				for _, label := range p.labels {
					v, _ := attrs[label]
					labelValues = append(labelValues, v)
				}
				p.containerActions.
					WithLabelValues(labelValues...).
					Inc()
			case events.ImageEventType:
				p.imageActions.WithLabelValues(ev.Action).Inc()
			case events.NetworkEventType:
				netName, _ := ev.Actor.Attributes["name"]
				netType, _ := ev.Actor.Attributes["type"]

				p.networkActions.
					WithLabelValues(ev.Action, netName, netType).
					Inc()
			case events.PluginEventType:
				pluginName, _ := ev.Actor.Attributes["name"]

				p.pluginActions.
					WithLabelValues(ev.Action, pluginName).
					Inc()
			case events.VolumeEventType:
				volDriver, _ := ev.Actor.Attributes["driver"]
				p.volumeActions.
					WithLabelValues(ev.Action, volDriver).
					Inc()
			}

		}
	}
}
