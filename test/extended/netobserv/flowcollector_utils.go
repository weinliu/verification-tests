package netobserv

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// returns ture/false if flowcollector API exists.
func isFlowCollectorAPIExists(oc *exutil.CLI) (bool, error) {
	stdout, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("crd", "-o", "jsonpath='{.items[*].spec.names.kind}'").Output()

	if err != nil {
		return false, err
	}
	return strings.Contains(stdout, "FlowCollector"), nil
}

// Verify flow records from logs
func verifyFlowRecordFromLogs(podLog string) {
	re := regexp.MustCompile("{\"AgentIP\":.*")
	flowRecords := re.FindAllString(podLog, -3)
	for _, flow := range flowRecords {
		o.Expect(flow).Should(o.And(
			o.MatchRegexp("Bytes.:[0-9]+"),
			o.MatchRegexp("TimeFlowEndMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeFlowStartMs.:[1-9][0-9]+"),
			o.MatchRegexp("TimeReceived.:[1-9][0-9]+")), flow)
	}
}

// Get flow recrods from loki
func getFlowRecords(lokiValues [][]string) ([]FlowRecord, error) {
	flowRecords := []FlowRecord{}
	for _, values := range lokiValues {
		timestamp, _ := strconv.ParseInt(values[0], 10, 64)
		var flowlog Flowlog
		err := json.Unmarshal([]byte(values[1]), &flowlog)
		if err != nil {
			return []FlowRecord{}, err
		}
		flowRecord := FlowRecord{
			Timestamp: timestamp,
			Flowlog:   flowlog,
		}
		flowRecords = append(flowRecords, flowRecord)
	}
	return flowRecords, nil
}

// Verify some key and deterministic flow recrods fields and their values
func (flowlog *Flowlog) verifyFlowRecord() {
	flow := fmt.Sprintf("Flow log is: %+v\n", flowlog)
	o.Expect(flowlog.AgentIP).To(o.Equal(flowlog.DstK8S_HostIP), flow)
	o.Expect(flowlog.Bytes).Should(o.BeNumerically(">", 0), flow)
	now := time.Now()
	compareTime := now.Add(time.Duration(-2) * time.Hour)
	compareTimeMs := compareTime.UnixMilli()
	o.Expect(flowlog.TimeFlowEndMs).Should(o.BeNumerically(">", compareTimeMs), flow)
	o.Expect(flowlog.TimeFlowStartMs).Should(o.BeNumerically(">", compareTimeMs), flow)
	o.Expect(flowlog.TimeReceived).Should(o.BeNumerically(">", compareTime.Unix()), flow)
}

func (lokilabels Lokilabels) getLokiQueryLabels() string {
	label := reflect.ValueOf(&lokilabels).Elem()
	var lokiQuery = "{"
	for i := 0; i < label.NumField(); i++ {
		if label.Field(i).Interface() != "" {
			switch labelName := label.Type().Field(i).Name; labelName {
			case "App":
				lokiQuery += fmt.Sprintf("%s=\"%s\", ", strings.ToLower(label.Type().Field(i).Name), label.Field(i).Interface())
			case "RecordType":
				lokiQuery += fmt.Sprintf("_%s=\"%s\", ", label.Type().Field(i).Name, label.Field(i).Interface())
			case "FlowDirection":
				if label.Field(i).Interface() == "0" || label.Field(i).Interface() == "1" || label.Field(i).Interface() == "2" {
					lokiQuery += fmt.Sprintf("%s=\"%s\", ", label.Type().Field(i).Name, label.Field(i).Interface())
				}
			default:
				lokiQuery += fmt.Sprintf("%s=\"%s\", ", label.Type().Field(i).Name, label.Field(i).Interface())
			}
		}
	}
	lokiQuery = strings.TrimSuffix(lokiQuery, ", ")
	lokiQuery += "}"

	return lokiQuery
}

func (lokilabels Lokilabels) getLokiJSONfilterQuery(parameters ...string) string {
	lokiQuery := lokilabels.getLokiQueryLabels()
	if len(parameters) != 0 {
		lokiQuery += " | json"
		for _, p := range parameters {
			lokiQuery += fmt.Sprintf(" | %s", p)
		}
	}
	e2e.Logf("Loki query is %s", lokiQuery)
	return lokiQuery
}

func (lokilabels Lokilabels) getLokiRegexFilterQuery(parameters ...string) string {
	lokiQuery := lokilabels.getLokiQueryLabels()
	if len(parameters) != 0 {
		for _, p := range parameters {
			lokiQuery += fmt.Sprintf(" |~ %s", p)
		}
	}
	e2e.Logf("Loki query is %s", lokiQuery)
	return lokiQuery
}

func (lokilabels Lokilabels) getLokiQuery(filterType string, parameters ...string) string {
	var lokiQuery string
	switch filterType {
	case "JSON":
		lokiQuery = lokilabels.getLokiJSONfilterQuery(parameters...)
	case "REGEX":
		lokiQuery = lokilabels.getLokiRegexFilterQuery(parameters...)
	default:
		panic("loki filter is not supported yet")
	}
	return lokiQuery
}

func (lokilabels Lokilabels) GetMonolithicLokiFlowLogs(lokiRoute string, startTime time.Time, parameters ...string) ([]FlowRecord, error) {
	lc := newLokiClient(lokiRoute, startTime).retry(5)
	lc.quiet = false
	lc.localhost = true
	lokiQuery := lokilabels.getLokiQuery("REGEX", parameters...)
	flowRecords := []FlowRecord{}
	var res *lokiQueryResponse
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(context.Context) (done bool, err error) {
		var qErr error
		res, qErr = lc.searchLogsInLoki("", lokiQuery)
		if qErr != nil {
			e2e.Logf("\ngot error %v when getting %s logs for query: %s\n", qErr, lokiQuery)
			return false, qErr
		}

		// return results if no error and result is empty
		// caller should add assertions to ensure len([]FlowRecord) is as they expected for given loki query
		return len(res.Data.Result) >= 0, nil
	})

	if err != nil {
		return flowRecords, err
	}

	for _, result := range res.Data.Result {
		flowRecords, err = getFlowRecords(result.Values)
		if err != nil {
			return []FlowRecord{}, err
		}
	}

	return flowRecords, err
}

// TODO: add argument for condition to be matched.
// Get flows from Loki logs
func (lokilabels Lokilabels) getLokiFlowLogs(token, lokiRoute string, startTime time.Time, parameters ...string) ([]FlowRecord, error) {
	lc := newLokiClient(lokiRoute, startTime).withToken(token).retry(5)
	tenantID := "network"
	lokiQuery := lokilabels.getLokiQuery("JSON", parameters...)
	flowRecords := []FlowRecord{}
	var res *lokiQueryResponse
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 300*time.Second, false, func(context.Context) (done bool, err error) {
		var qErr error
		res, qErr = lc.searchLogsInLoki(tenantID, lokiQuery)
		if qErr != nil {
			e2e.Logf("\ngot error %v when getting %s logs for query: %s\n", qErr, tenantID, lokiQuery)
			return false, qErr
		}

		// return results if no error and result is empty
		// caller should add assertions to ensure len([]FlowRecord) is as they expected for given loki query
		return len(res.Data.Result) >= 0, nil
	})

	if err != nil {
		return flowRecords, err
	}

	for _, result := range res.Data.Result {
		flowRecords, err = getFlowRecords(result.Values)
		if err != nil {
			return []FlowRecord{}, err
		}
	}

	return flowRecords, err
}

// Verify loki flow records and if it was written in the last 5 minutes
func verifyLokilogsTime(token, lokiRoute string, startTime time.Time) error {
	lc := newLokiClient(lokiRoute, startTime).withToken(token).retry(5)
	res, err := lc.searchLogsInLoki("network", "{app=\"netobserv-flowcollector\", FlowDirection=\"0\"}")

	if err != nil {
		return err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(res.Data.Result) == 0 {
		exutil.AssertWaitPollNoErr(err, "network logs are not found")
	}
	flowRecords := []FlowRecord{}

	for _, result := range res.Data.Result {
		flowRecords, err = getFlowRecords(result.Values)
		if err != nil {
			return err
		}
	}

	for _, r := range flowRecords {
		r.Flowlog.verifyFlowRecord()
	}
	return nil
}

// Verify some key and deterministic conversation record fields and their values
func (flowlog *Flowlog) verifyConversationRecord() {
	conversationRecord := fmt.Sprintf("Conversation record in error: %+v\n", flowlog)
	o.Expect(flowlog.Bytes).Should(o.BeNumerically(">", 0), conversationRecord)
	now := time.Now()
	compareTime := now.Add(time.Duration(-2) * time.Hour)
	compareTimeMs := compareTime.UnixMilli()
	o.Expect(flowlog.TimeFlowEndMs).Should(o.BeNumerically(">", compareTimeMs), conversationRecord)
	o.Expect(flowlog.TimeFlowStartMs).Should(o.BeNumerically(">", compareTimeMs), conversationRecord)
	o.Expect(flowlog.HashId).NotTo(o.BeEmpty(), conversationRecord)
	o.Expect(flowlog.NumFlowLogs).Should(o.BeNumerically(">", 0), conversationRecord)
}

// Verify loki conversation records and if it was written in the last 5 minutes
func verifyConversationRecordTime(record []FlowRecord) {
	for _, r := range record {
		r.Flowlog.verifyConversationRecord()
	}
}

// Verify flow correctness based on number of bytes
func verifyFlowCorrectness(objectSize string, flowRecords []FlowRecord) {
	var multiplier int = 0
	switch unit := objectSize[len(objectSize)-1:]; unit {
	case "K":
		multiplier = 1024
	case "M":
		multiplier = 1024 * 1024
	case "G":
		multiplier = 1024 * 1024 * 1024
	default:
		panic("invalid object size unit")
	}
	nObject, _ := strconv.Atoi(objectSize[0 : len(objectSize)-1])
	// minBytes is the size of the object fetched
	minBytes := nObject * multiplier
	// maxBytes is the minBytes +2% tolerance
	maxBytes := int(float64(minBytes) + (float64(minBytes) * 0.02))
	var errFlows float64 = 0
	nflows := float64(len(flowRecords))

	for _, r := range flowRecords {
		// occurs very rarely but sometimes >= comparison can be flaky
		// when eBPF-agent evicts packets sooner,
		// currently it configured to be 15seconds.
		if r.Flowlog.Bytes <= minBytes {
			errFlows += 1
		}
		if r.Flowlog.Bytes >= maxBytes {
			errFlows += 1
		}
		r.Flowlog.verifyFlowRecord()
	}
	// allow only 10% of flows to have Bytes violating minBytes and maxBytes.
	tolerance := math.Ceil(nflows * 0.10)
	o.Expect(errFlows).Should(o.BeNumerically("<=", tolerance))
}

func removeSAFromAdmin(oc *exutil.CLI, saName string, namespace string) error {
	return oc.WithoutNamespace().AsAdmin().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-admin", "-z", saName, "-n", namespace).Execute()

}

func addSAToAdmin(oc *exutil.CLI, saName string, namespace string) error {
	return oc.WithoutNamespace().AsAdmin().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", "-z", saName, "-n", namespace).Execute()
}
