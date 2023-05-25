package logging

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func (s *splunkPodServer) checkLogs(query string, quiet bool) bool {
	e2e.Logf("find logs using query string: %s", query)
	searchResult, err := s.doQuery(query)
	if searchResult == nil {
		if !quiet {
			e2e.Logf("%v", err)
		}
		e2e.Logf("can't find logs for the query : %s", query)
		return false
	}
	if len(searchResult.Results) == 0 {
		e2e.Logf("can't find logs for the query: %s : Records.size() = 0", query)
		return false
	}
	e2e.Logf("Found records for the query: %s", query)
	return true
}

func (s *splunkPodServer) anyLogFound() bool {
	for _, logType := range []string{"infrastructure", "application", "audit"} {
		if s.checkLogs("log_type="+logType+"|head 1", true) {
			return true
		}
	}
	return false
}

func (s *splunkPodServer) allQueryFound(queries []string) bool {
	if len(queries) == 0 {
		queries = []string{
			"log_type=application|head 1",
			"log_type=\"infrastructure\" _SYSTEMD_INVOCATION_ID |head 1",
			"log_type=\"infrastructure\" container_image|head 1",
			"log_type=\"audit\" .linux-audit.log|head 1",
			"log_type=\"audit\" .ovn-audit.log|head 1",
			"log_type=\"audit\" .k8s-audit.log|head 1",
			"log_type=\"audit\" .openshift-audit.log|head 1",
		}
	}
	//return false if any query fail
	foundAll := true
	for _, query := range queries {
		if s.checkLogs(query, false) == false {
			foundAll = false
		}
	}
	return foundAll
}

func (s *splunkPodServer) allTypeLogsFound() bool {
	queries := []string{
		"log_type=\"infrastructure\" _SYSTEMD_INVOCATION_ID |head 1",
		"log_type=\"infrastructure\" container_image|head 1",
		"log_type=application|head 1",
		"log_type=audit|head 1",
	}
	return s.allQueryFound(queries)
}

func (s *splunkPodServer) doQuery(query string) (*splunkSearchResult, error) {
	searchID, err := s.requestSearchTask(query)
	if searchID == "" {
		return nil, err
	}
	return s.extractSearchResponse(searchID)
}

func (s *splunkPodServer) requestSearchTask(query string) (string, error) {
	h := make(http.Header)
	h.Add("Content-Type", "application/json")
	h.Add(
		"Authorization",
		"Basic "+base64.StdEncoding.EncodeToString([]byte(s.adminUser+":"+s.adminPassword)),
	)
	params := url.Values{}
	params.Set("search", "search "+query)

	resp, err := doHTTPRequest(h, "https://"+s.splunkdRoute, "/services/search/jobs", "", "POST", false, 2, strings.NewReader(params.Encode()), 201)

	if err != nil {
		return "", err
	}

	resmap := splunkSearchResp{}
	err = xml.Unmarshal(resp, &resmap)
	if err != nil {
		return "", err
	}
	return resmap.Sid, err
}

func (s *splunkPodServer) extractSearchResponse(searchID string) (*splunkSearchResult, error) {
	h := make(http.Header)
	h.Add("Content-Type", "application/json")
	h.Add(
		"Authorization",
		"Basic "+base64.StdEncoding.EncodeToString([]byte(s.adminUser+":"+s.adminPassword)),
	)
	params := url.Values{}
	params.Add("output_mode", "json")

	var searchResult *splunkSearchResult
	err := wait.Poll(15*time.Second, 120*time.Second, func() (bool, error) {
		resp, err1 := doHTTPRequest(h, "https://"+s.splunkdRoute, "/services/search/jobs/"+searchID+"/results", params.Encode(), "GET", true, 1, nil, 200)
		if err1 != nil {
			e2e.Logf("failed to get response: %v, try next round", err1)
			return false, nil
		}
		err2 := json.Unmarshal(resp, &searchResult)
		if err2 != nil {
			e2e.Logf("failed to Unmarshal splunk response: %v, try next round", err2)
			return false, nil
		}

		if len(searchResult.Results) == 0 {
			e2e.Logf("no records from splunk server, try next round")
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return searchResult, fmt.Errorf("can't get records from splunk server")
	}
	return searchResult, nil
}

// Set the default values to the splunkPodServer Object
func (s *splunkPodServer) init() {
	s.adminUser = "admin"
	s.adminPassword = getRandomString()
	s.hecToken = uuid.New().String()
	//https://idelta.co.uk/generate-hec-tokens-with-python/,https://docs.splunk.com/Documentation/SplunkCloud/9.0.2209/Security/Passwordbestpracticesforadministrators
	s.serviceName = s.name + "-0"
	s.serviceURL = s.serviceName + "." + s.namespace + ".svc"
	if s.name == "" {
		s.name = "splunk-default"
	}
	//authType must be one of "http|tls_serveronly|tls_mutual"
	//Note: when authType==http, you can still access splunk via https://${splunk_route}
	if s.authType == "" {
		s.authType = "http"
	}
	if s.version == "" {
		s.version = "9.0"
	}

	//Exit if anyone of caFile, keyFile,CertFile is null
	if s.authType == "tls_clientauth" || s.authType == "tls_serveronly" {
		o.Expect(s.caFile == "").To(o.BeFalse())
		o.Expect(s.keyFile == "").To(o.BeFalse())
		o.Expect(s.certFile == "").To(o.BeFalse())
	}
}

func (s *splunkPodServer) deploy(oc *exutil.CLI) {
	// Get route URL of splunk service
	appDomain, err := getAppDomain(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	//splunkd route URL
	s.splunkdRoute = s.name + "-splunkd-" + s.namespace + "." + appDomain
	//splunkd hec URL
	s.hecRoute = s.name + "-hec-" + s.namespace + "." + appDomain
	s.webRoute = s.name + "-web-" + s.namespace + "." + appDomain

	err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "nonroot", "-z", "default", "-n", s.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	// Create secret used by splunk
	switch s.authType {
	case "http":
		s.deployHTTPSplunk(oc)
	case "tls_clientauth":
		s.deployCustomCertClientForceSplunk(oc)
	case "tls_serveronly":
		s.deployCustomCertSplunk(oc)
	default:
		s.deployHTTPSplunk(oc)
	}
	//In general, it take 1 minutes to be started, here wait 30second before call  waitForStatefulsetReady
	time.Sleep(30 * time.Second)
	waitForStatefulsetReady(oc, s.namespace, s.name)
}

func (s *splunkPodServer) deployHTTPSplunk(oc *exutil.CLI) {
	filePath := exutil.FixturePath("testdata", "logging", "external-log-stores", "splunk")
	//Create secret for splunk Statefulset
	secretTemplate := filepath.Join(filePath, "secret_splunk_template.yaml")
	secret := resource{"secret", s.name, s.namespace}
	err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword)
	o.Expect(err).NotTo(o.HaveOccurred())

	//create splunk StatefulSet
	statefulsetTemplate := filepath.Join(filePath, "statefulset_splunk-"+s.version+"_template.yaml")
	splunkSfs := resource{"StatefulSet", s.name, s.namespace}
	err = splunkSfs.applyFromTemplate(oc, "-f", statefulsetTemplate, "-p", "NAME="+s.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//create route for splunk service
	routeHecTemplate := filepath.Join(filePath, "route-edge_splunk_template.yaml")
	routeHec := resource{"route", s.name + "-hec", s.namespace}
	err = routeHec.applyFromTemplate(oc, "-f", routeHecTemplate, "-p", "NAME="+routeHec.name, "-p", "SERVICE_NAME="+s.serviceName, "-p", "PORT_NAME=http-hec", "-p", "ROUTE_HOST="+s.hecRoute)
	o.Expect(err).NotTo(o.HaveOccurred())

	routeSplunkdTemplate := filepath.Join(filePath, "route-passthrough_splunk_template.yaml")
	routeSplunkd := resource{"route", s.name + "-splunkd", s.namespace}
	err = routeSplunkd.applyFromTemplate(oc, "-f", routeSplunkdTemplate, "-p", "NAME="+routeSplunkd.name, "-p", "SERVICE_NAME="+s.serviceName, "-p", "PORT_NAME=https-splunkd", "-p", "ROUTE_HOST="+s.splunkdRoute)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (s *splunkPodServer) genHecPemFile(hecFile string) error {
	dat1, err := os.ReadFile(s.certFile)
	if err != nil {
		e2e.Logf("Can not find the certFile %s", s.certFile)
		return err
	}
	dat2, err := os.ReadFile(s.keyFile)
	if err != nil {
		e2e.Logf("Can not find the keyFile %s", s.keyFile)
		return err
	}
	dat3, err := os.ReadFile(s.caFile)
	if err != nil {
		e2e.Logf("Can not find the caFile %s", s.caFile)
		return err
	}

	buf := []byte{}
	buf = append(buf, dat1...)
	buf = append(buf, dat2...)
	buf = append(buf, dat3...)
	err = os.WriteFile(hecFile, buf, 0644)
	return err
}

func (s *splunkPodServer) deployCustomCertSplunk(oc *exutil.CLI) {
	//Create basic secret content for splunk Statefulset
	filePath := exutil.FixturePath("testdata", "logging", "external-log-stores", "splunk")
	secretTemplate := filepath.Join(filePath, "secret_tls_splunk_template.yaml")
	if s.passphrase != "" {
		secretTemplate = filepath.Join(filePath, "secret_tls_passphase_splunk_template.yaml")
	}
	secret := resource{"secret", s.name, s.namespace}
	if s.passphrase != "" {
		err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword, "-p", "PASSPHASE="+s.passphrase)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword)
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	//HEC need all in one PEM file.
	hecPemFile := "/tmp/" + getRandomString() + "hecAllKeys.crt"
	defer os.Remove(hecPemFile)
	err := s.genHecPemFile(hecPemFile)
	o.Expect(err).NotTo(o.HaveOccurred())

	//The secret will be mounted into splunk pods and used in server.conf,inputs.conf
	args := []string{"data", "secret/" + secret.name, "-n", secret.namespace}
	args = append(args, "--from-file=hec.pem="+hecPemFile)
	args = append(args, "--from-file=ca.pem="+s.caFile)
	args = append(args, "--from-file=key.pem="+s.keyFile)
	args = append(args, "--from-file=cert.pem="+s.certFile)
	if s.passphrase != "" {
		args = append(args, "--from-literal=passphrase="+s.passphrase)
	}
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args(args...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	//create splunk StatefulSet
	statefulsetTemplate := filepath.Join(filePath, "statefulset_splunk-"+s.version+"_template.yaml")
	splunkSfs := resource{"StatefulSet", s.name, s.namespace}
	err = splunkSfs.applyFromTemplate(oc, "-f", statefulsetTemplate, "-p", "NAME="+splunkSfs.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//create route for splunk service
	routeHecTemplate := filepath.Join(filePath, "route-passthrough_splunk_template.yaml")
	routeHec := resource{"route", s.name + "-hec", s.namespace}
	err = routeHec.applyFromTemplate(oc, "-f", routeHecTemplate, "-p", "NAME="+routeHec.name, "-p", "SERVICE_NAME="+s.serviceName, "-p", "PORT_NAME=http-hec", "-p", "ROUTE_HOST="+s.hecRoute)
	o.Expect(err).NotTo(o.HaveOccurred())

	routeSplunkdTemplate := filepath.Join(filePath, "route-passthrough_splunk_template.yaml")
	routeSplunkd := resource{"route", s.name + "-splunkd", s.namespace}
	err = routeSplunkd.applyFromTemplate(oc, "-f", routeSplunkdTemplate, "-p", "NAME="+routeSplunkd.name, "-p", "SERVICE_NAME="+s.serviceName, "-p", "PORT_NAME=https-splunkd", "-p", "ROUTE_HOST="+s.splunkdRoute)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (s *splunkPodServer) deployCustomCertClientForceSplunk(oc *exutil.CLI) {
	//Create secret for splunk Statefulset
	filePath := exutil.FixturePath("testdata", "logging", "external-log-stores", "splunk")
	secretTemplate := filepath.Join(filePath, "secret_tls_splunk_template.yaml")
	if s.passphrase != "" {
		secretTemplate = filepath.Join(filePath, "secret_tls_passphase_splunk_template.yaml")
	}
	secret := resource{"secret", s.name, s.namespace}
	if s.passphrase != "" {
		err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword, "-p", "HEC_CLIENTAUTH=True", "-p", "PASSPHASE="+s.passphrase)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword, "-p", "HEC_CLIENTAUTH=True")
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	//HEC need all in one PEM file.
	hecPemFile := "/tmp/" + getRandomString() + "hecAllKeys.crt"
	defer os.Remove(hecPemFile)
	err := s.genHecPemFile(hecPemFile)
	o.Expect(err).NotTo(o.HaveOccurred())

	//The secret will be mounted into splunk pods and used in server.conf,inputs.conf
	secretArgs := []string{"data", "secret/" + secret.name, "-n", secret.namespace}
	secretArgs = append(secretArgs, "--from-file=hec.pem="+hecPemFile)
	secretArgs = append(secretArgs, "--from-file=ca.pem="+s.caFile)
	secretArgs = append(secretArgs, "--from-file=key.pem="+s.keyFile)
	secretArgs = append(secretArgs, "--from-file=cert.pem="+s.certFile)
	if s.passphrase != "" {
		secretArgs = append(secretArgs, "--from-literal=passphrase="+s.passphrase)
	}
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args(secretArgs...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	//create splunk StatefulSet
	statefulsetTemplate := filepath.Join(filePath, "statefulset_splunk-"+s.version+"_template.yaml")
	splunkSfs := resource{"StatefulSet", s.name, s.namespace}
	err = splunkSfs.applyFromTemplate(oc, "-f", statefulsetTemplate, "-p", "NAME="+splunkSfs.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//create route for splunk service
	routeHecTemplate := filepath.Join(filePath, "route-passthrough_splunk_template.yaml")
	routeHec := resource{"route", s.name + "-hec", s.namespace}
	err = routeHec.applyFromTemplate(oc, "-f", routeHecTemplate, "-p", "NAME="+routeHec.name, "-p", "SERVICE_NAME="+s.serviceName, "-p", "PORT_NAME=http-hec", "-p", "ROUTE_HOST="+s.hecRoute)
	o.Expect(err).NotTo(o.HaveOccurred())

	routeSplunkdTemplate := filepath.Join(filePath, "route-passthrough_splunk_template.yaml")
	routeSplunkd := resource{"route", s.name + "-splunkd", s.namespace}
	err = routeSplunkd.applyFromTemplate(oc, "-f", routeSplunkdTemplate, "-p", "NAME="+routeSplunkd.name, "-p", "SERVICE_NAME="+s.serviceName, "-p", "PORT_NAME=https-splunkd", "-p", "ROUTE_HOST="+s.splunkdRoute)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (s *splunkPodServer) destroy(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delelte").Args("route", s.name+"-hec", "-n", s.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delelte").Args("route", s.name+"-splunkd", "-n", s.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("statefulset", s.name, "-n", "-n", s.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", s.name, "-n", "-n", s.namespace).Execute()
	oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-scc-from-user", "nonroot", "-z", "default", "-n", s.namespace).Execute()
}

// Create the secret which is used in CLF
func (toSp *toSplunkSecret) create(oc *exutil.CLI) {
	secretArgs := []string{"secret", "generic", toSp.name, "-n", toSp.namespace}

	if toSp.hecToken != "" {
		secretArgs = append(secretArgs, "--from-literal=hecToken="+toSp.hecToken)
	}
	if toSp.caFile != "" {
		secretArgs = append(secretArgs, "--from-file=ca-bundle.crt="+toSp.caFile)
	}
	if toSp.keyFile != "" {
		secretArgs = append(secretArgs, "--from-file=tls.key="+toSp.keyFile)
	}
	if toSp.certFile != "" {
		secretArgs = append(secretArgs, "--from-file=tls.crt="+toSp.certFile)
	}
	if toSp.passphrase != "" {
		secretArgs = append(secretArgs, "--from-literal=passphrase="+toSp.passphrase)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args(secretArgs...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (toSp *toSplunkSecret) delete(oc *exutil.CLI) {
	s := resource{"secret", toSp.name, toSp.namespace}
	s.clear(oc)
}
