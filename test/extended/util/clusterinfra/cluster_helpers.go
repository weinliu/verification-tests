package clusterinfra

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
)

const (
	AWS          = "aws"
	GCP          = "gcp"
	AZURE        = "azure"
	VSPHERE      = "vsphere"
	NUTANIX      = "nutanix"
	OPENSTACK    = "openstack"
	IBMCLOUD     = "ibmcloud"
	ALIBABACLOUD = "alibabacloud"
	NONE         = "none"
	BAREMETAL    = "baremetal"
	OVIRT        = "ovirt"
)

const (
	//VsphereServer vSphere server hostname
	VsphereServer = "vcenter.sddc-44-236-21-251.vmwarevmc.com"
)

// SkipTestIfNotSupportedPlatform skip the test if current platform matches one of the provided not supported platforms
func SkipTestIfNotSupportedPlatform(oc *exutil.CLI, notsupported ...string) {
	p := exutil.CheckPlatform(oc)
	for _, nsp := range notsupported {
		if strings.EqualFold(nsp, p) {
			g.Skip("Skip this test scenario because it is not supported on the " + p + " platform")
		}
	}
}

// SkipTestIfSupportedPlatformNotMatched skip the test if supported platforms are not matched
func SkipTestIfSupportedPlatformNotMatched(oc *exutil.CLI, supported ...string) {
	var match bool
	p := exutil.CheckPlatform(oc)
	for _, sp := range supported {
		if strings.EqualFold(sp, p) {
			match = true
			break
		}
	}

	if !match {
		g.Skip("Skip this test scenario because it is not supported on the " + p + " platform")
	}
}

// GetAwsVolumeInfoAttachedToInstanceID get detail info of the volume attached to the instance id
func GetAwsVolumeInfoAttachedToInstanceID(instanceID string) (string, error) {
	mySession := session.Must(session.NewSession())
	svc := ec2.New(mySession, aws.NewConfig())
	input := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("attachment.instance-id"),
				Values: []*string{
					aws.String(instanceID),
				},
			},
		},
	}
	volumeInfo, err := svc.DescribeVolumes(input)
	newValue, _ := json.Marshal(volumeInfo)
	return string(newValue), err
}

// GetAwsCredentialFromCluster get aws credential from cluster
func GetAwsCredentialFromCluster(oc *exutil.CLI) {
	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", "json").Output()
	// Skip for sts and c2s clusters.
	if err != nil {
		g.Skip("Did not get credential to access aws, skip the testing.")

	}
	o.Expect(err).NotTo(o.HaveOccurred())
	accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
	accessKeyID, err1 := base64.StdEncoding.DecodeString(accessKeyIDBase64)
	o.Expect(err1).NotTo(o.HaveOccurred())
	secureKey, err2 := base64.StdEncoding.DecodeString(secureKeyBase64)
	o.Expect(err2).NotTo(o.HaveOccurred())
	clusterRegion, err3 := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
	o.Expect(err3).NotTo(o.HaveOccurred())
	os.Setenv("AWS_ACCESS_KEY_ID", string(accessKeyID))
	os.Setenv("AWS_SECRET_ACCESS_KEY", string(secureKey))
	os.Setenv("AWS_REGION", clusterRegion)
}

// GetVsphereCredentialFromCluster retrieves vSphere credentials as env variables
func GetVsphereCredentialFromCluster(oc *exutil.CLI) {

	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/vsphere-creds", "-n", "kube-system", "-o", "json").Output()
	// Skip for sts and c2s clusters.
	if err != nil {
		g.Skip("Did not get credential to access vSphere, skip the testing.")

	}

	// Scape the dots in the vsphere server hostname to access the json value
	scapedVsphereName := strings.ReplaceAll(VsphereServer, ".", "\\.")
	usernameBase64, passwordBase64 := gjson.Get(credential, `data.`+scapedVsphereName+`\.username`).String(), gjson.Get(credential, `data.`+scapedVsphereName+`\.password`).String()

	username, err := base64.StdEncoding.DecodeString(usernameBase64)
	o.Expect(err).NotTo(o.HaveOccurred())
	password, err := base64.StdEncoding.DecodeString(passwordBase64)
	o.Expect(err).NotTo(o.HaveOccurred())

	os.Setenv("VSPHERE_USER", string(username))
	os.Setenv("VSPHERE_PASSWORD", string(password))
	os.Setenv("VSPHERE_SERVER", VsphereServer)

}

// GetGcpCredentialFromCluster retrieves vSphere credentials as env variables
func GetGcpCredentialFromCluster(oc *exutil.CLI) {

	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/gcp-credentials", "-n", "kube-system", "-o", "json").Output()
	// Skip for sts and c2s clusters.
	if err != nil {
		g.Skip("Did not get credential to access GCP, skip the testing.")

	}

	serviceAccountBase64 := gjson.Get(credential, `data.service_account\.json`).String()

	serviceAccount, err := base64.StdEncoding.DecodeString(serviceAccountBase64)
	o.Expect(err).NotTo(o.HaveOccurred())

	os.Setenv("GOOGLE_CREDENTIALS", string(serviceAccount))

}

// IsAwsOutpostCluster judges whether the aws test cluster has outpost workers
func IsAwsOutpostCluster(oc *exutil.CLI) bool {
	if exutil.CheckPlatform(oc) != AWS {
		return false
	}
	workersLabel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "node-role.kubernetes.io/worker", "--show-labels").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Contains(workersLabel, `topology.ebs.csi.aws.com/outpost-id`)
}

// SkipForAwsOutpostCluster skip for Aws Outpost cluster
func SkipForAwsOutpostCluster(oc *exutil.CLI) {
	if IsAwsOutpostCluster(oc) {
		g.Skip("Skip for Aws Outpost cluster.")
	}
}

// IsAwsOutpostMixedCluster check whether the cluster is aws outpost mixed workers cluster
func IsAwsOutpostMixedCluster(oc *exutil.CLI) bool {
	return IsAwsOutpostCluster(oc) && len(ListNonOutpostWorkerNodes(oc)) > 0
}

// SkipForNotAwsOutpostMixedCluster skip for not Aws Outpost Mixed cluster
func SkipForNotAwsOutpostMixedCluster(oc *exutil.CLI) {
	if !IsAwsOutpostMixedCluster(oc) {
		g.Skip("Skip for not Aws Outpost Mixed cluster.")
	}
}
