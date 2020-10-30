# Nginx Request Exporter for Prometheus

This is a [Prometheus](https://prometheus.io/) exporter for [Nginx](http://nginx.org/) requests. 

In contrast to existing exporters nginx-request-exporter does *not* scrape the [stub status module](http://nginx.org/en/docs/http/ngx_http_stub_status_module.html) for server status but records statistics for HTTP requests.

By default nginx-request-exporter listens on port 9147 for HTTP requests.

## Installation

### Using `go get`

```shell script
go get github.com/volodymyr-polishchuk/nginx-request-exporter
```

## Docker

Build image:
```shell script
docker build -t nginx-request-exporter .
```

Run:
```
docker run --rm -p 9514:9514 -p 9514:9514 nginx-request-exporter
```

## Configuration

nginx-request-exporter consumes access log records using the syslog protocol. Nginx needs to be configured to log to nginx-request-exporter's syslog port. To enable syslog logging add a `access_log` statement to your Nginx configuration:

```
access_log syslog:server=127.0.0.1:9514 prometheus;
```

## Log format

nginx-request-exporter uses a custom log format that needs to be defined in the `http` context.

The format has to only include key/value pairs:

* A key/value pair delimited by a colon denotes a metric name&value
* A key/value pair delimited by an equal sign denotes a label name&value that is added to all metrics.

Example:

```
log_format prometheus 'time:$request_time status=$status host="$host" method="$request_method" upstream="$upstream_addr"';
```

Key `hostname` is mandatory.

Multiple metrics can be recorded and all [variables](http://nginx.org/en/docs/varindex.html) available in Nginx can be used. 
Currently, nginx-request-exporter has to be restarted when the log format is changed.

## Environment variables

| Env                         | Default                                  | Description                                          |
|-----------------------------|------------------------------------------|------------------------------------------------------|
| `NRE_WEB_LISTEN_ADDRESS`    | `:9147`                                  | Address to listen on for web interface and telemetry |
| `NRE_WEB_TELEMETRY_PATH`    | `/metrics`                               | Path under which to expose metrics                   |
| `NRE_NGINX_SYSLOG_LISTENER` | `0.0.0.0:9514`                           | Syslog listen address/socket for Nginx               |
| `NRE_HISTOGRAM_BUCKETS`     | `.005,.01,.025,.05,.1,.25,.5,1,2.5,5,10` | Buckets for the Prometheus histogram                 |

## Metrics

```text
# HELP nginx_request_exporter_syslog_messages Current total syslog messages received.
# TYPE nginx_request_exporter_syslog_messages counter
# HELP nginx_request_exporter_syslog_parse_failure Number of errors while parsing syslog messages.
# TYPE nginx_request_exporter_syslog_parse_failure counter
# HELP nginx_request_time Nginx request log value for time
# TYPE nginx_request_time histogram
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
# HELP process_max_fds Maximum number of open file descriptors.
# TYPE process_max_fds gauge
# HELP process_open_fds Number of open file descriptors.
# TYPE process_open_fds gauge
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.
# TYPE process_start_time_seconds gauge
# HELP process_virtual_memory_bytes Virtual memory size in bytes.
# TYPE process_virtual_memory_bytes gauge
# HELP promhttp_metric_handler_requests_in_flight Current number of scrapes being served.
# TYPE promhttp_metric_handler_requests_in_flight gauge
# HELP promhttp_metric_handler_requests_total Total number of scrapes by HTTP status code.
# TYPE promhttp_metric_handler_requests_total counter
```

How to get all metrics:
```shell script
curl localhost:9147/metrics | grep -E "# HELP|# TYPE" | grep -v " go_"
```