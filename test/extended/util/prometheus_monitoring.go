package util

import (
	"fmt"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	prometheusUrl          = "https://prometheus-k8s.openshift-monitoring.svc:9091"
	prometheusInstantQuery = "/api/v1/query"
	prometheusRangeQuery   = "/api/v1/query_range"
	prometheusAlerts       = "/api/v1/alerts"
	prometheusRules        = "/api/v1/rules"
	prometheusNamespace    = "openshift-monitoring"
	prometheusK8s          = "prometheus-k8s"
)

// query parameters:
//  query=<string>: Prometheus expression query string.
//  time=<rfc3339 | unix_timestamp>: Evaluation timestamp. Optional.
//  timeout=<duration>: Evaluation timeout. Optional. Defaults to and is capped by the value of the -query.timeout flag.
type PrometheusInstantQueryParams struct {
	Query   string
	Time    string
	Timeout string
}

// query range parameters
//  query=<string>: Prometheus expression query string.
//  start=<rfc3339 | unix_timestamp>: Start timestamp, inclusive.
//  end=<rfc3339 | unix_timestamp>: End timestamp, inclusive.
//  step=<duration | float>: Query resolution step width in duration format or float number of seconds.
//  timeout=<duration>: Evaluation timeout. Optional. Defaults to and is capped by the value of the -query.timeout flag.
type PrometheusRangeQueryParams struct {
	Query   string
	Start   string
	End     string
	Step    string
	Timeout string
}

//  Define a monitor object
type Monitor struct {
	token    string
	ocClient *CLI
}

// Init a monitor
func NewMonitor(oc *CLI) (*Monitor, error) {
	var mo Monitor
	var err error
	mo.ocClient = oc
	mo.token, err = getSAToken(oc)
	return &mo, err
}

// Query executes a query in prometheus. .../query?query=$query_to_execute
func (mo *Monitor) SimpleQuery(query string) (string, error) {
	queryParams := PrometheusInstantQueryParams{Query: query}
	return mo.InstantQuery(queryParams)
}

// Query executes a query in prometheus with time and timeout.
//   Example:  curl 'http://host:port/api/v1/query?query=up&time=2015-07-01T20:10:51.781Z'
func (mo *Monitor) InstantQuery(queryParams PrometheusInstantQueryParams) (string, error) {
	queryString := ""
	if queryParams.Query != "" {
		queryString = queryString + " --data-urlencode query=" + queryParams.Query
	}
	if queryParams.Time != "" {
		queryString = queryString + " --data-urlencode time=" + queryParams.Time
	}
	if queryParams.Timeout != "" {
		queryString = queryString + " --data-urlencode timeout=" + queryParams.Timeout
	}

	getCmd := "curl -k -s -H \"" + fmt.Sprintf("Authorization: Bearer %v", mo.token) + "\" " + queryString + " " + prometheusUrl + prometheusInstantQuery
	return RemoteShPod(mo.ocClient, prometheusNamespace, "statefulsets/"+prometheusK8s, "sh", "-c", getCmd)
}

// QueryRange executes a query range in prometheus with start, end, step and timeout
//   Example: curl 'http://host:port/api/v1/query_range?query=metricname&start=2015-07-01T20:10:30.781Z&end=2015-07-01T20:11:00.781Z&step=15s'
func (mo *Monitor) RangeQuery(queryParams PrometheusRangeQueryParams) (string, error) {
	queryString := ""
	if queryParams.Query != "" {
		queryString = queryString + " --data-urlencode query=" + queryParams.Query
	}
	if queryParams.Start != "" {
		queryString = queryString + " --data-urlencode start=" + queryParams.Start
	}
	if queryParams.End != "" {
		queryString = queryString + " --data-urlencode end=" + queryParams.End
	}
	if queryParams.Step != "" {
		queryString = queryString + " --data-urlencode step=" + queryParams.Step
	}
	if queryParams.Timeout != "" {
		queryString = queryString + " --data-urlencode timeout=" + queryParams.Timeout
	}

	getCmd := "curl -k -s -H \"" + fmt.Sprintf("Authorization: Bearer %v", mo.token) + "\" " + queryString + " " + prometheusUrl + prometheusRangeQuery
	return RemoteShPod(mo.ocClient, prometheusNamespace, "statefulsets/"+prometheusK8s, "sh", "-c", getCmd)
}

// GetAlerts returns all alerts. It doesn't use the alermanager, and it returns alerts in 'pending' state too
func (mo *Monitor) GetAlerts() (string, error) {
	getCmd := "curl -k -s -H \"" + fmt.Sprintf("Authorization: Bearer %v", mo.token) + "\" " + prometheusUrl + prometheusAlerts
	return RemoteShPod(mo.ocClient, prometheusNamespace, "statefulsets/"+prometheusK8s, "sh", "-c", getCmd)
}

func (mo *Monitor) queryRules(query string) (string, error) {
	query_string := ""
	if query != "" {
		query_string = "?" + query
	}
	getCmd := "curl -k -s -H \"" + fmt.Sprintf("Authorization: Bearer %v", mo.token) + "\" " + prometheusUrl + prometheusRules + query_string
	return RemoteShPod(mo.ocClient, prometheusNamespace, "statefulsets/"+prometheusK8s, "sh", "-c", getCmd)
}

// GetAllRules returns all rules
func (mo *Monitor) GetAllRules() (string, error) {
	return mo.queryRules("")
}

// GetAlertRules returns all alerting rules
func (mo *Monitor) GetAlertRules() (string, error) {
	return mo.queryRules("type=alert")
}

// GetRecordRules returns all recording rules
func (mo *Monitor) GetRecordRules() (string, error) {
	return mo.queryRules("type=record")
}

// GetSAToken get a token assigned to prometheus-k8s from openshift-monitoring namespace
func getSAToken(oc *CLI) (string, error) {
	e2e.Logf("Getting a token assgined to prometheus-k8s from %s namespace...", prometheusNamespace)
	return oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", prometheusK8s, "-n", prometheusNamespace).Output()
}
