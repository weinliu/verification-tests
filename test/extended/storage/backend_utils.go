package storage

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/tidwall/gjson"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Get the credential from cluster
func getCredentialFromCluster(oc *exutil.CLI) {
	switch cloudProvider {
	case "aws":
		credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", "json").Output()
		// Disconnected and STS type test clusters
		if strings.Contains(interfaceToString(err), "not found") {
			credential, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/ebs-cloud-credentials", "-n", "openshift-cluster-csi-drivers", "-o", "json").Output()
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterRegion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		os.Setenv("AWS_REGION", clusterRegion)
		// C2S type test clusters
		if gjson.Get(credential, `data.credentials`).Exists() && gjson.Get(credential, `data.role`).Exists() {
			c2sConfigPrefix := "/tmp/storage-c2sconfig-" + getRandomString() + "-"
			debugLogf("C2S config prefix is: %s", c2sConfigPrefix)
			extraCA, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap/kube-cloud-config", "-n", "openshift-cluster-csi-drivers", "-o", "json").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ioutil.WriteFile(c2sConfigPrefix+"ca.pem", []byte(gjson.Get(extraCA, `data.ca-bundle\.pem`).String()), 0644)).NotTo(o.HaveOccurred())
			os.Setenv("AWS_CA_BUNDLE", c2sConfigPrefix+"ca.pem")
		}
		// STS type test clusters
		if gjson.Get(credential, `data.credentials`).Exists() && !gjson.Get(credential, `data.aws_access_key_id`).Exists() {
			stsConfigPrefix := "/tmp/storage-stsconfig-" + getRandomString() + "-"
			debugLogf("STS config prefix is: %s", stsConfigPrefix)
			stsConfigBase64 := gjson.Get(credential, `data.credentials`).String()
			stsConfig, err := base64.StdEncoding.DecodeString(stsConfigBase64)
			o.Expect(err).NotTo(o.HaveOccurred())
			var tokenPath, roleArn string
			dataList := strings.Split(string(stsConfig), ` `)
			for _, subStr := range dataList {
				if strings.Contains(subStr, `/token`) {
					tokenPath = subStr
				}
				if strings.Contains(subStr, `arn:`) {
					roleArn = strings.Split(string(subStr), "\n")[0]
				}
			}
			cfgStr := strings.Replace(string(stsConfig), tokenPath, stsConfigPrefix+"token", -1)
			tempToken, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-cluster-csi-drivers", "deployment/aws-ebs-csi-driver-controller", "-c", "csi-driver", "--", "cat", tokenPath).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(ioutil.WriteFile(stsConfigPrefix+"config", []byte(cfgStr), 0644)).NotTo(o.HaveOccurred())
			o.Expect(ioutil.WriteFile(stsConfigPrefix+"token", []byte(tempToken), 0644)).NotTo(o.HaveOccurred())
			os.Setenv("AWS_ROLE_ARN", roleArn)
			os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", stsConfigPrefix+"token")
			os.Setenv("AWS_CONFIG_FILE", stsConfigPrefix+"config")
			os.Setenv("AWS_PROFILE", "storageAutotest"+getRandomString())
		} else {
			accessKeyIdBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
			accessKeyId, err := base64.StdEncoding.DecodeString(accessKeyIdBase64)
			o.Expect(err).NotTo(o.HaveOccurred())
			secureKey, err := base64.StdEncoding.DecodeString(secureKeyBase64)
			o.Expect(err).NotTo(o.HaveOccurred())
			os.Setenv("AWS_ACCESS_KEY_ID", string(accessKeyId))
			os.Setenv("AWS_SECRET_ACCESS_KEY", string(secureKey))
		}
	case "vsphere":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "gcp":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "azure":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "openstack":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	default:
		e2e.Logf("unknown cloud provider")
	}
}

// Get the volume detail info by persistent volume id
func getAwsVolumeInfoByVolumeId(volumeId string) (string, error) {
	mySession := session.Must(session.NewSession())
	svc := ec2.New(mySession, aws.NewConfig())
	input := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("volume-id"),
				Values: []*string{
					aws.String(volumeId),
				},
			},
		},
	}
	volumeInfo, err := svc.DescribeVolumes(input)
	return interfaceToString(volumeInfo), err
}

// Get the volume status "in use" or "avaiable" by persistent volume id
func getAwsVolumeStatusByVolumeId(volumeId string) (string, error) {
	volumeInfo, err := getAwsVolumeInfoByVolumeId(volumeId)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeStatus := gjson.Get(volumeInfo, `Volumes.0.State`).Str
	e2e.Logf("The volume %s status is %q on aws backend", volumeId, volumeStatus)
	return volumeStatus, err
}

// Delete backend volume
func deleteBackendVolumeByVolumeId(oc *exutil.CLI, volumeId string) (string, error) {
	switch cloudProvider {
	case "aws":
		if strings.Contains(volumeId, "::") {
			e2e.Logf("Delete EFS volume: \"%s\" access_points is under development", volumeId)
			return "under development now", nil
		} else {
			mySession := session.Must(session.NewSession())
			svc := ec2.New(mySession, aws.NewConfig())
			deleteVolumeID := &ec2.DeleteVolumeInput{
				VolumeId: &volumeId,
			}
			req, resp := svc.DeleteVolumeRequest(deleteVolumeID)
			return interfaceToString(resp), req.Send()
		}
	case "vsphere":
		e2e.Logf("Delete %s backend volume is under development", cloudProvider)
		return "under development now", nil
	case "gcp":
		e2e.Logf("Delete %s backend volume is under development", cloudProvider)
		return "under development now", nil
	case "azure":
		e2e.Logf("Delete %s backend volume is under development", cloudProvider)
		return "under development now", nil
	case "openstack":
		e2e.Logf("Delete %s backend volume is under development", cloudProvider)
		return "under development now", nil
	default:
		e2e.Logf("unknown cloud provider")
		return "under development now", nil
	}
}

//  Check the volume status becomes avaiable, status is "avaiable"
func checkVolumeAvaiableOnBackend(volumeId string) (bool, error) {
	volumeStatus, err := getAwsVolumeStatusByVolumeId(volumeId)
	avaiableStatus := []string{"available"}
	return contains(avaiableStatus, volumeStatus), err
}

//  Check the volume is deleted
func checkVolumeDeletedOnBackend(volumeId string) (bool, error) {
	volumeStatus, err := getAwsVolumeStatusByVolumeId(volumeId)
	deletedStatus := []string{""}
	return contains(deletedStatus, volumeStatus), err
}

//  Waiting the volume become avaiable
func waitVolumeAvaiableOnBackend(oc *exutil.CLI, volumeId string) {
	switch cloudProvider {
	case "aws":
		if strings.Contains(volumeId, "::") {
			e2e.Logf("Get EFS volume: \"%s\" status is under development", volumeId)
		} else {
			err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
				volumeStatus, errinfo := checkVolumeAvaiableOnBackend(volumeId)
				if errinfo != nil {
					e2e.Logf("the err:%v, wait for volume %v to become avaiable.", errinfo, volumeId)
					return volumeStatus, errinfo
				}
				if !volumeStatus {
					return volumeStatus, nil
				}
				return volumeStatus, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The volume:%v, is not avaiable.", volumeId))
		}
	case "vsphere":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	case "gcp":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	case "azure":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	case "openstack":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	default:
		e2e.Logf("unknown cloud provider")
	}
}

//  Waiting the volume become deleted
func waitVolumeDeletedOnBackend(oc *exutil.CLI, volumeId string) {
	switch cloudProvider {
	case "aws":
		if strings.Contains(volumeId, "::") {
			e2e.Logf("Get EFS volume: \"%s\" status is under development", volumeId)
		} else {
			err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
				volumeStatus, errinfo := checkVolumeDeletedOnBackend(volumeId)
				if errinfo != nil {
					e2e.Logf("the err:%v, wait for volume %v to be deleted.", errinfo, volumeId)
					return volumeStatus, errinfo
				}
				if !volumeStatus {
					return volumeStatus, nil
				}
				return volumeStatus, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The volume:%v, still exist.", volumeId))
		}
	case "vsphere":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	case "gcp":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	case "azure":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	case "openstack":
		e2e.Logf("Get %s backend volume status is under development", cloudProvider)
	default:
		e2e.Logf("unknown cloud provider")
	}
}

// Get the volume type by volume id
func getAwsVolumeTypeByVolumeId(volumeId string) string {
	volumeInfo, err := getAwsVolumeInfoByVolumeId(volumeId)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeType := gjson.Get(volumeInfo, `Volumes.0.VolumeType`).Str
	e2e.Logf("The volume %s type is %q on aws backend", volumeId, volumeType)
	return volumeType
}

// Get the volume iops by volume id
func getAwsVolumeIopsByVolumeId(volumeId string) int64 {
	volumeInfo, err := getAwsVolumeInfoByVolumeId(volumeId)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeIops := gjson.Get(volumeInfo, `Volumes.0.Iops`).Int()
	e2e.Logf("The volume %s Iops is %d on aws backend", volumeId, volumeIops)
	return volumeIops
}
