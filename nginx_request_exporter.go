// Copyright 2020 Volodymyr Polishchuk
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"gopkg.in/mcuadros/go-syslog.v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

const (
	namespace       = "nginx_request"
	applicationName = "Nginx Request Exporter"
)

func main() {
	parameters := readParameters()
	floatBuckets := parseMetricBuckets(parameters["metricBuckets"])
	channel, server, err := setupSyslogServer(parameters["syslogAddress"])
	syslogMessages, syslogParseFailures := setupSyslogSystemMetrics()

	processMetricsFromSyslog(channel, syslogMessages, syslogParseFailures, floatBuckets)
	startWebServer(parameters["metricsPath"], parameters["listenAddress"])
	waitForShutdown(server, err)
}

func GetEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func processMetricsFromSyslog(channel syslog.LogPartsChannel, syslogMessages prometheus.Counter, syslogParseFailures prometheus.Counter, floatBuckets []float64) {
	msgs := 0
	go func() {
		for part := range channel {
			syslogMessages.Inc()
			msgs++
			tag, _ := part["tag"].(string)
			if tag != "nginx" {
				log.Warn("Ignoring syslog message with wrong tag")
				syslogParseFailures.Inc()
				continue
			}
			server, _ := part["hostname"].(string)
			if server == "" {
				log.Warn("Hostname missing in syslog message")
				syslogParseFailures.Inc()
				continue
			}

			content, _ := part["content"].(string)
			if content == "" {
				log.Warn("Ignoring empty syslog message")
				syslogParseFailures.Inc()
				continue
			}

			metrics, labels, err := parseMessage(content)
			if err != nil {
				log.Error(err)
				continue
			}
			for _, metric := range metrics {
				var collector prometheus.Collector
				collector = prometheus.NewHistogramVec(prometheus.HistogramOpts{
					Namespace: namespace,
					Name:      metric.Name,
					Help:      fmt.Sprintf("Nginx request log value for %s", metric.Name),
					Buckets:   floatBuckets,
				}, labels.Names)
				if err := prometheus.Register(collector); err != nil {
					if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
						collector = are.ExistingCollector.(*prometheus.HistogramVec)
					} else {
						log.Error(err)
						continue
					}
				}
				collector.(*prometheus.HistogramVec).WithLabelValues(labels.Values...).Observe(metric.Value)
			}
		}
	}()
}

// Parse the buckets
func parseMetricBuckets(metricBuckets string) []float64 {
	var floatBuckets []float64
	for _, str := range strings.Split(metricBuckets, ",") {
		bucket, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
		if err != nil {
			log.Fatal(err)
		}
		floatBuckets = append(floatBuckets, bucket)
	}
	return floatBuckets
}

// Setup metrics
func setupSyslogSystemMetrics() (prometheus.Counter, prometheus.Counter) {
	syslogMessages := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_syslog_messages",
		Help:      "Current total syslog messages received.",
	})

	if err := prometheus.Register(syslogMessages); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	syslogParseFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_syslog_parse_failure",
		Help:      "Number of errors while parsing syslog messages.",
	})

	if err := prometheus.Register(syslogParseFailures); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	return syslogMessages, syslogParseFailures
}

// Set up syslog server
func setupSyslogServer(syslogAddress string) (syslog.LogPartsChannel, *syslog.Server, error) {
	channel := make(syslog.LogPartsChannel, 20000)
	handler := syslog.NewChannelHandler(channel)
	server := syslog.NewServer()
	server.SetFormat(syslog.RFC3164)
	server.SetHandler(handler)

	var err error
	if strings.HasPrefix(syslogAddress, "unix:") {
		err = server.ListenUnixgram(strings.TrimPrefix(syslogAddress, "unix:"))
	} else {
		err = server.ListenUDP(syslogAddress)
	}
	if err != nil {
		log.Fatal(err)
	}
	err = server.Boot()
	if err != nil {
		log.Fatal(err)
	}
	return channel, server, err
}

// Setup HTTP server
func startWebServer(metricsPath string, listenAddress string) {
	http.Handle(metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head>
		<title>` + applicationName + `</title>
		</head><body>
		<h1>` + applicationName + `</h1>
		<p><a href='` + metricsPath + `'>Metrics</a></p>
		</body></html>`))
	})

	go func() {
		log.Infof("Starting Server: %s", listenAddress)
		log.Fatal(http.ListenAndServe(listenAddress, nil))
	}()
}

func readParameters() map[string]string {
	var (
		listenAddress = GetEnv(
			"NRE_WEB_LISTEN_ADDRESS",
			*flag.String("web.listen-address", ":9147", "Address to listen on for web interface and telemetry."))
		metricsPath = GetEnv(
			"NRE_WEB_TELEMETRY_PATH",
			*flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics."))
		syslogAddress = GetEnv(
			"NRE_NGINX_SYSLOG_LISTENER",
			*flag.String("nginx.syslog-address", "0.0.0.0:9514", "Syslog listen address/socket for Nginx."))
		metricBuckets = GetEnv(
			"NRE_HISTOGRAM_BUCKETS",
			*flag.String("histogram.buckets", ".005,.01,.025,.05,.1,.25,.5,1,2.5,5,10", "Buckets for the Prometheus histogram."))
	)
	flag.Parse()
	return map[string]string{
		"listenAddress": listenAddress,
		"metricsPath":   metricsPath,
		"syslogAddress": syslogAddress,
		"metricBuckets": metricBuckets,
	}
}

// Listen to signals
func waitForShutdown(server *syslog.Server, err error) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	s := <-sigchan
	log.Infof("Received %v, terminating", s)
	err = server.Kill()
	if err != nil {
		log.Error(err)
	}
	os.Exit(0)
}
