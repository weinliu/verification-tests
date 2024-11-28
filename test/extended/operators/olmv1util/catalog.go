package olmv1util

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"strings"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

const (
	v1ApiPath = "api/v1"
	v1ApiData = "all"
)

type ClusterCatalogDescription struct {
	Name                string
	PullSecret          string
	TypeName            string
	Imageref            string
	ContentURL          string
	Status              string
	PollIntervalMinutes string
	LabelKey            string // default is olmv1-test
	LabelValue          string // suggest to use case id
	Template            string
}

func (clustercatalog *ClusterCatalogDescription) Create(oc *exutil.CLI) {
	e2e.Logf("=========Create clustercatalog %v=========", clustercatalog.Name)
	err := clustercatalog.CreateWithoutCheck(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	clustercatalog.WaitCatalogStatus(oc, "true", "Serving", 0)
	clustercatalog.GetcontentURL(oc)
}

func (clustercatalog *ClusterCatalogDescription) CreateWithoutCheck(oc *exutil.CLI) error {
	paremeters := []string{"-n", "default", "--ignore-unknown-parameters=true", "-f", clustercatalog.Template, "-p"}
	if len(clustercatalog.Name) > 0 {
		paremeters = append(paremeters, "NAME="+clustercatalog.Name)
	}
	if len(clustercatalog.PullSecret) > 0 {
		paremeters = append(paremeters, "SECRET="+clustercatalog.PullSecret)
	}
	if len(clustercatalog.TypeName) > 0 {
		paremeters = append(paremeters, "TYPE="+clustercatalog.TypeName)
	}
	if len(clustercatalog.Imageref) > 0 {
		paremeters = append(paremeters, "IMAGE="+clustercatalog.Imageref)
	}
	if len(clustercatalog.PollIntervalMinutes) > 0 {
		paremeters = append(paremeters, "POLLINTERVALMINUTES="+clustercatalog.PollIntervalMinutes)
	}
	if len(clustercatalog.LabelKey) > 0 {
		paremeters = append(paremeters, "LABELKEY="+clustercatalog.LabelKey)
	}
	if len(clustercatalog.LabelValue) > 0 {
		paremeters = append(paremeters, "LABELVALUE="+clustercatalog.LabelValue)
	}
	err := exutil.ApplyClusterResourceFromTemplateWithError(oc, paremeters...)
	return err
}

func (clustercatalog *ClusterCatalogDescription) WaitCatalogStatus(oc *exutil.CLI, status string, conditionType string, consistentTime int) {
	e2e.Logf("========= check clustercatalog %v status is %s =========", clustercatalog.Name, status)

	jsonpath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, conditionType)
	errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", jsonpath)
		if err != nil {
			e2e.Logf("output is %v, error is %v, and try next", output, err)
			return false, nil
		}
		if !strings.Contains(strings.ToLower(output), strings.ToLower(status)) {
			e2e.Logf("status is %v, not %v, and try next", output, status)
			clustercatalog.Status = output
			return false, nil
		}
		return true, nil
	})
	if errWait != nil {
		GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o=jsonpath-as-json={.status}")
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("clustercatalog status is not %s", status))
	}
	if consistentTime != 0 {
		e2e.Logf("make sure clustercatalog %s status is %s consistently for %ds", clustercatalog.Name, status, consistentTime)
		o.Consistently(func() string {
			output, _ := GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", jsonpath)
			return strings.ToLower(output)
		}, time.Duration(consistentTime)*time.Second, 5*time.Second).Should(o.ContainSubstring(strings.ToLower(status)),
			"clustercatalog %s status is not %s", clustercatalog.Name, status)
	}
}

func (clustercatalog *ClusterCatalogDescription) GetcontentURL(oc *exutil.CLI) {
	e2e.Logf("=========Get clustercatalog %v contentURL =========", clustercatalog.Name)
	route, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "catalogd-service", "-n", "openshift-catalogd", "-o=jsonpath={.spec.host}").Output()
	if err != nil && !strings.Contains(route, "NotFound") {
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if route == "" || err != nil {
		output, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("route", "reencrypt", "--service=catalogd-service", "-n", "openshift-catalogd").Output()
		e2e.Logf("output is %v, error is %v", output, err)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, 10*time.Second, false, func(ctx context.Context) (bool, error) {
			route, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "catalogd-service", "-n", "openshift-catalogd", "-o=jsonpath={.spec.host}").Output()
			if err != nil {
				e2e.Logf("output is %v, error is %v, and try next", route, err)
				return false, nil
			}
			if route == "" {
				e2e.Logf("route is empty")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "get route catalogd-service failed")
	}
	o.Expect(route).To(o.ContainSubstring("catalogd-service-openshift-catalogd"))
	contentURL, err := GetNoEmpty(oc, "clustercatalog", clustercatalog.Name, "-o", "jsonpath={.status.urls.base}")
	o.Expect(err).NotTo(o.HaveOccurred())
	contentURL = contentURL + "/" + v1ApiPath + "/" + v1ApiData
	clustercatalog.ContentURL = strings.Replace(contentURL, "catalogd-service.openshift-catalogd.svc", route, 1)
	e2e.Logf("clustercatalog contentURL is %s", clustercatalog.ContentURL)
}

func (clustercatalog *ClusterCatalogDescription) DeleteWithoutCheck(oc *exutil.CLI) {
	e2e.Logf("=========DeleteWithoutCheck clustercatalog %v=========", clustercatalog.Name)
	exutil.CleanupResource(oc, 4*time.Second, 160*time.Second, exutil.AsAdmin, exutil.WithoutNamespace, "clustercatalog", clustercatalog.Name)
}

func (clustercatalog *ClusterCatalogDescription) Delete(oc *exutil.CLI) {
	e2e.Logf("=========Delete clustercatalog %v=========", clustercatalog.Name)
	clustercatalog.DeleteWithoutCheck(oc)
	//add check later
}

func (clustercatalog *ClusterCatalogDescription) Patch(oc *exutil.CLI, patch string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("clustercatalog", clustercatalog.Name, "--type", "merge", "-p", patch).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Get clustercatalog info content
func (clustercatalog *ClusterCatalogDescription) GetContent(oc *exutil.CLI) []byte {
	if clustercatalog.ContentURL == "" {
		clustercatalog.GetcontentURL(oc)
	}

	var proxy string
	if os.Getenv("http_proxy") != "" {
		proxy = os.Getenv("http_proxy")
	} else if os.Getenv("https_proxy") != "" {
		proxy = os.Getenv("https_proxy")
	} else if os.Getenv("HTTP_PROXY") != "" {
		proxy = os.Getenv("HTTP_PROXY")
	} else if os.Getenv("HTTPS_PROXY") != "" {
		proxy = os.Getenv("HTTPS_PROXY")
	}

	var tr *http.Transport
	if len(proxy) > 0 {
		e2e.Logf("take proxy to access cluster")
		proxyURL, err := url.Parse(proxy)
		o.Expect(err).NotTo(o.HaveOccurred())
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Proxy: http.ProxyURL(proxyURL),
		}
	} else {
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Get(clustercatalog.ContentURL)
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			e2e.Logf("Error closing body: %v", err)
		}
	}()
	o.Expect(err).NotTo(o.HaveOccurred())
	curlOutput, err := io.ReadAll(resp.Body)
	o.Expect(err).NotTo(o.HaveOccurred())
	return curlOutput
}

// RelatedImagesInfo returns the relatedImages info
type RelatedImagesInfo struct {
	Image string `json:"image"`
	Name  string `json:"name"`
}

// BundleData returns the bundle info
type BundleData struct {
	Image         string              `json:"image"`
	Name          string              `json:"name"`
	Package       string              `json:"package"`
	RelatedImages []RelatedImagesInfo `json:"relatedImages"`
	Schema        string              `json:"schema"`
	Properties    json.RawMessage     `json:"properties"` // properties data are complex and will be output as strings
}

func GetBundlesName(bundlesDataOut []BundleData) []string {

	var bundlesName []string
	var singleBundleData BundleData

	for _, singleBundleData = range bundlesDataOut {
		bundlesName = append(bundlesName, singleBundleData.Name)
	}
	return bundlesName
}

func GetBundlesNameByPakcage(bundlesDataOut []BundleData, packageName string) []string {

	var bundlesName []string
	var singleBundleData BundleData

	for _, singleBundleData = range bundlesDataOut {
		if singleBundleData.Package == packageName {
			bundlesName = append(bundlesName, singleBundleData.Name)
		}
	}
	return bundlesName
}

func GetBundlesImageTag(bundlesDataOut []BundleData) []string {

	var bundlesName []string
	var singleBundleData BundleData

	for _, singleBundleData = range bundlesDataOut {
		bundlesName = append(bundlesName, singleBundleData.Image)
	}
	return bundlesName
}

func GetBundleInfoByName(bundlesDataOut []BundleData, packageName string, bundleName string) *BundleData {

	var singleBundleData BundleData

	for _, singleBundleData = range bundlesDataOut {
		if singleBundleData.Name == bundleName && singleBundleData.Package == packageName {
			return &singleBundleData
		}
	}
	return nil
}

// EntriesInfo returns the entries info
type EntriesInfo struct {
	Name     string   `json:"name"`
	Replaces string   `json:"replaces"`
	Skips    []string `json:"skips"`
}

// ChannelData returns the channel info
type ChannelData struct {
	Entries []EntriesInfo `json:"entries"`
	Name    string        `json:"name"`
	Package string        `json:"package"`
	Schema  string        `json:"schema"`
}

func GetChannelByPakcage(channelDataOut []ChannelData, packageName string) []ChannelData {

	var channelDataByPackage []ChannelData
	var singleChannelData ChannelData
	for _, singleChannelData = range channelDataOut {
		if singleChannelData.Package == packageName {
			channelDataByPackage = append(channelDataByPackage, singleChannelData)
		}
	}
	return channelDataByPackage
}

func GetChannelNameByPakcage(channelDataOut []ChannelData, packageName string) []string {

	var channelsName []string
	var singleChannelData ChannelData

	for _, singleChannelData = range channelDataOut {
		if singleChannelData.Package == packageName {
			channelsName = append(channelsName, singleChannelData.Name)
		}
	}
	return channelsName
}

// PackageData returns the package info
type PackageData struct {
	DefaultChannel string `json:"defaultChannel"`
	Name           string `json:"name"`
	Schema         string `json:"schema"`
}

func ListPackagesName(packageDataOut []PackageData) []string {

	var packagesName []string
	var singlePackageData PackageData

	for _, singlePackageData = range packageDataOut {
		packagesName = append(packagesName, singlePackageData.Name)
	}
	return packagesName
}

// ReferenceInfo returns the Reference info
type ReferenceInfo struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

// EntriesInfo returns the entries info
type DeprecatedEntriesInfo struct {
	Message   string        `json:"message"`
	Reference ReferenceInfo `json:"reference"`
}

// DeprecationData returns the deprecated info
type DeprecationData struct {
	Entries []DeprecatedEntriesInfo `json:"entries"`
	Package string                  `json:"package"`
	Schema  string                  `json:"schema"`
}

func GetDeprecatedChannelNameByPakcage(deprecationDataOut []DeprecationData, packageName string) []string {

	var channelsName []string
	var singleDeprecationData DeprecationData
	var deprecatedEntriesInfo DeprecatedEntriesInfo

	for _, singleDeprecationData = range deprecationDataOut {
		if singleDeprecationData.Package == packageName {
			for _, deprecatedEntriesInfo = range singleDeprecationData.Entries {
				if deprecatedEntriesInfo.Reference.Schema == "olm.channel" {
					channelsName = append(channelsName, deprecatedEntriesInfo.Reference.Name)
				}
			}
		}
	}
	return channelsName
}

func GetDeprecatedBundlesNameByPakcage(deprecationDataOut []DeprecationData, packageName string) []string {

	var bundlesName []string
	var singleDeprecationData DeprecationData
	var deprecatedEntriesInfo DeprecatedEntriesInfo

	for _, singleDeprecationData = range deprecationDataOut {
		if singleDeprecationData.Package == packageName {
			for _, deprecatedEntriesInfo = range singleDeprecationData.Entries {
				if deprecatedEntriesInfo.Reference.Schema == "olm.bundle" {
					bundlesName = append(bundlesName, deprecatedEntriesInfo.Reference.Name)
				}
			}
		}
	}
	return bundlesName
}

type ContentData struct {
	Packages     []PackageData
	Channels     []ChannelData
	Bundles      []BundleData
	Deprecations []DeprecationData
}

// Unmarshal Content
func (clustercatalog *ClusterCatalogDescription) UnmarshalContent(oc *exutil.CLI, schema string) (ContentData, error) {
	var (
		singlePackageData     PackageData
		singleChannelData     ChannelData
		singleBundleData      BundleData
		singleDeprecationData DeprecationData
		ContentData           ContentData
		targetData            interface{}
		err                   error
	)

	switch schema {
	case "all":
		return clustercatalog.UnmarshalAllContent(oc)
	case "bundle":
		targetData = &singleBundleData
	case "channel":
		targetData = &singleChannelData
	case "package":
		targetData = &singlePackageData
	case "deprecations":
		targetData = &singleDeprecationData
	default:
		return ContentData, fmt.Errorf("unsupported schema: %s", schema)
	}

	contents := clustercatalog.GetContent(oc)
	lines := strings.Split(string(contents), "\n")

	for _, line := range lines {
		if strings.Contains(line, "\"schema\":\"olm."+schema+"\"") {
			if err = json.Unmarshal([]byte(line), targetData); err != nil {
				return ContentData, err
			}

			switch schema {
			case "bundle":
				ContentData.Bundles = append(ContentData.Bundles, singleBundleData)
			case "channel":
				ContentData.Channels = append(ContentData.Channels, singleChannelData)
			case "package":
				ContentData.Packages = append(ContentData.Packages, singlePackageData)
			case "deprecations":
				ContentData.Deprecations = append(ContentData.Deprecations, singleDeprecationData)
			}
		}
	}

	err = nil

	switch schema {
	case "bundle":
		if len(ContentData.Bundles) == 0 {
			err = fmt.Errorf("can not get Bundles")
		}
	case "channel":
		if len(ContentData.Channels) == 0 {
			err = fmt.Errorf("can not get Channels")
		}
	case "package":
		if len(ContentData.Packages) == 0 {
			err = fmt.Errorf("can not get Packages")
		}
	case "deprecations":
		if len(ContentData.Deprecations) == 0 {
			err = fmt.Errorf("can not get Deprecations")
		}
	}
	return ContentData, err

}

func (clustercatalog *ClusterCatalogDescription) UnmarshalAllContent(oc *exutil.CLI) (ContentData, error) {
	var ContentData ContentData

	contents := clustercatalog.GetContent(oc)
	lines := strings.Split(string(contents), "\n")

	for _, line := range lines {
		if strings.Contains(line, "\"schema\":\"olm.bundle\"") || strings.Contains(line, "\"schema\":\"olm.channel\"") || strings.Contains(line, "\"schema\":\"olm.package\"") || strings.Contains(line, "\"schema\":\"olm.deprecations\"") {

			var targetData interface{}
			switch {
			case strings.Contains(line, "\"schema\":\"olm.bundle\""):
				targetData = new(BundleData)
			case strings.Contains(line, "\"schema\":\"olm.channel\""):
				targetData = new(ChannelData)
			case strings.Contains(line, "\"schema\":\"olm.package\""):
				targetData = new(PackageData)
			case strings.Contains(line, "\"schema\":\"olm.deprecations\""):
				targetData = new(DeprecationData)
			}

			if err := json.Unmarshal([]byte(line), targetData); err != nil {
				return ContentData, err
			}

			switch data := targetData.(type) {
			case *BundleData:
				ContentData.Bundles = append(ContentData.Bundles, *data)
			case *ChannelData:
				ContentData.Channels = append(ContentData.Channels, *data)
			case *PackageData:
				ContentData.Packages = append(ContentData.Packages, *data)
			case *DeprecationData:
				ContentData.Deprecations = append(ContentData.Deprecations, *data)
			}
		}
	}
	if len(ContentData.Bundles) == 0 && len(ContentData.Channels) == 0 && len(ContentData.Packages) == 0 && len(ContentData.Deprecations) == 0 {
		return ContentData, fmt.Errorf("no any bundle, channel or package are got")
	}
	return ContentData, nil
}
