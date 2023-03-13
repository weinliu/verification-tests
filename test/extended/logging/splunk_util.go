package logging

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func (s *splunkPodServer) checkLogs(query string, quiet bool) bool {
	var searchResult splunkSearchResult
	if err := s.doQuery(query, &searchResult); err != nil {
		if !quiet {
			e2e.Logf("%v", err)
		}
		e2e.Logf("can't find logs with query string: %s", query)
		return false
	}
	if len(searchResult.Results) == 0 {
		e2e.Logf("can't find logs with query string: %s : Records.size() = 0", query)
		return false
	}
	e2e.Logf("find logs with query string: %s", query)
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

func (s *splunkPodServer) allLogsFound(queries []string) bool {
	if len(queries) == 0 {
		queries = []string{
			"log_type=infrastructure base-search _SYSTEMD_INVOCATION_ID |head 1",
			"log_type=infrastructure base-search container_image|head 1",
			"log_type=application|head 1",
			"log_type=audit base-search /var/log/audit/audit.log|head 1",
			"log_type=audit base-search /var/log/ovn/acl-audit-log.log|head 1",
			"log_type=audit base-search /var/log/kube-apiserver/audit.log|head 1",
			"log_type=audit base-search /var/log/oauth-server/audit.log|head 1",
		}
	}
	for _, query := range queries {
		if !s.checkLogs(query, false) {
			return false
		}
	}
	return true
}

func (s *splunkPodServer) doQuery(query string, out interface{}) error {
	searchID, err := s.requestSearchTask(query)
	if searchID == "" {
		return err
	}
	return s.extractSearchResponse(searchID, out)
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

	resp, err := doHTTPRequest(h, "https://"+s.splunkdRoute, "/services/search/jobs", "", "POST", false, 2, strings.NewReader(params.Encode()))

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

func (s *splunkPodServer) extractSearchResponse(searchID string, out interface{}) error {
	h := make(http.Header)
	h.Add("Content-Type", "application/json")
	h.Add(
		"Authorization",
		"Basic "+base64.StdEncoding.EncodeToString([]byte(s.adminUser+":"+s.adminPassword)),
	)
	params := url.Values{}
	params.Add("output_mode", "json")
	resp, err := doHTTPRequest(h, "https://"+s.splunkdRoute, "/services/search/jobs/"+searchID+"/results", params.Encode(), "GET", true, 3, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(resp, out)
}

func createToSplunkSecret(oc *exutil.CLI, secretNamespace string, secretName string, hecToken string, caFile string, keyFile string, certFile string, passphrase string) {
	// create secret to Splunk server
	secretArgs := []string{"secret", "generic", secretName, "-n", secretNamespace, "--from-literal=hecToken=" + hecToken}
	if caFile != "" {
		secretArgs = append(secretArgs, "--from-file=ca-bundle.crt="+caFile)
	}
	if keyFile != "" {
		secretArgs = append(secretArgs, "--from-file=tls.key="+keyFile)
	}
	if certFile != "" {
		secretArgs = append(secretArgs, "--from-file=tls.crt="+certFile)
	}
	if passphrase != "" {
		secretArgs = append(secretArgs, "--from-literal=passphrase="+passphrase)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args(secretArgs...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Set the default values to the splunkPodServer Object
func (s *splunkPodServer) init() {
	s.adminUser = "admin"
	s.adminPassword = getRandomString()
	s.hecToken = uuid.New().String()
	//https://idelta.co.uk/generate-hec-tokens-with-python/,https://docs.splunk.com/Documentation/SplunkCloud/9.0.2209/Security/Passwordbestpracticesforadministrators
	s.serviceName = s.name + "-service"
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
	if s.authType == "tls_mutual" || s.authType == "tls_serveronly" {
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
	s.splunkdRoute = s.name + "-splunkd." + s.namespace + "." + appDomain
	//splunkd hec URL
	s.hecRoute = s.name + "-hec." + s.namespace + "." + appDomain

	err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "nonroot", "-z", "default", "-n", s.namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	// Create secret used by splunk
	switch s.authType {
	case "http":
		s.deployHTTPSplunk(oc)
	case "tls_mutual":
		s.deployCustomCertClientForceSplunk(oc)
	case "tls_serveronly":
		s.deployCustomCertSplunk(oc)
	default:
		s.deployHTTPSplunk(oc)
	}
	waitForStatefulsetReady(oc, s.namespace, s.name)
}

func (s *splunkPodServer) deployHTTPSplunk(oc *exutil.CLI) {
	filePath := exutil.FixturePath("testdata", "logging", "external-log-stores", "splunk")
	//Create secret for splunk Statefulset
	secretTemplate := filepath.Join(filePath, "secret_splunk_template.yaml")
	secret := resource{"secret", s.name, s.namespace}
	err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword, "-p", "HEC_SSL=False", "-p", "HTTP_SSL=0")
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
	dat1, err := os.ReadFile(s.caFile)
	if err != nil {
		return err
	}
	dat2, err := os.ReadFile(s.keyFile)
	if err != nil {
		return err
	}
	dat3, err := os.ReadFile(s.certFile)
	if err != nil {
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
	secretTemplate := filepath.Join(filePath, "secret_splunk_template.yaml")
	secret := resource{"secret", s.name, s.namespace}
	err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword, "-p HEC_SSL=True -p HTTP_SSL=1")
	o.Expect(err).NotTo(o.HaveOccurred())

	hecPemFile := "/tmp/" + getRandomString() + "hecAllKeys.crt"
	defer os.Remove(hecPemFile)
	err = s.genHecPemFile(hecPemFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	args := []string{"data", "secret/" + secret.name, "-n", secret.namespace}
	args = append(args, "--from-file=hec.pem="+hecPemFile)
	args = append(args, "--from-file=ca.pem="+s.caFile)
	args = append(args, "--from-file=key.pem="+s.keyFile)
	args = append(args, "--from-file=crt.pem="+s.certFile)
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
	secretTemplate := filepath.Join(filePath, "secret_splunk_template.yaml")
	secret := resource{"secret", s.name, s.namespace}
	err := secret.applyFromTemplate(oc, "-f", secretTemplate, "-n", s.namespace, "-p", "NAME="+secret.name, "-p", "HEC_TOKEN="+s.hecToken, "-p", "PASSWORD="+s.adminPassword, "-p HEC_SSL=True -p HTTP_SSL=1 -p HEC_CLIENTAUTH=True")
	o.Expect(err).NotTo(o.HaveOccurred())

	hecPemFile := "/tmp/" + getRandomString() + "hecAllKeys.crt"
	defer os.Remove(hecPemFile)
	err = s.genHecPemFile(hecPemFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	secretArgs := []string{"data", "secret/" + secret.name, "-n", secret.namespace}
	secretArgs = append(secretArgs, "--from-file=hec.pem="+hecPemFile)
	secretArgs = append(secretArgs, "--from-file=ca.pem="+s.caFile)
	secretArgs = append(secretArgs, "--from-file=key.pem="+s.keyFile)
	secretArgs = append(secretArgs, "--from-file=crt.pem="+s.certFile)
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
