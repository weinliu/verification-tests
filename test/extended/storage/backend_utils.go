package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/vim25"

	g "github.com/onsi/ginkgo/v2"
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
		// Disconnected, STS and CCO mode mint type test clusters get credential from storage csi driver
		if gjson.Get(credential, `metadata.annotations.cloudcredential\.openshift\.io/mode`).String() == "mint" || strings.Contains(interfaceToString(err), "not found") {
			e2e.Logf("Get credential from storage csi driver")
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
					tokenPath = strings.Split(string(subStr), "\n")[0]
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
			accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
			accessKeyID, err := base64.StdEncoding.DecodeString(accessKeyIDBase64)
			o.Expect(err).NotTo(o.HaveOccurred())
			secureKey, err := base64.StdEncoding.DecodeString(secureKeyBase64)
			o.Expect(err).NotTo(o.HaveOccurred())
			os.Setenv("AWS_ACCESS_KEY_ID", string(accessKeyID))
			os.Setenv("AWS_SECRET_ACCESS_KEY", string(secureKey))
		}
	case "vsphere":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "gcp":
		// Currently on gcp platform only completed get the gcp filestore credentials,
		// so it's not suitable for the test clusters without installed the gcp filestore csi driver operator
		if checkCSIDriverInstalled(oc, []string{"filestore.csi.storage.gke.io"}) {
			dirname := "/tmp/" + oc.Namespace() + "-creds"
			err := os.MkdirAll(dirname, 0777)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/gcp-filestore-cloud-credentials", "-n", "openshift-cluster-csi-drivers", "--to="+dirname, "--confirm").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", dirname+"/service_account.json")
		} else {
			e2e.Logf("GCP filestore csi driver operator is not installed")
		}
	case "azure":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	case "openstack":
		e2e.Logf("Get %s backend credential is under development", cloudProvider)
	default:
		e2e.Logf("unknown cloud provider")
	}
}

// GetAwsCredentialFromSpecifiedSecret gets the aws credential from specified secret in the test cluster, doesn't support sts,c2s,sc2s clusters
func getAwsCredentialFromSpecifiedSecret(oc *exutil.CLI, secretNamespace string, secretName string) {
	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/"+secretName, "-n", secretNamespace, "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	clusterRegion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	os.Setenv("AWS_REGION", clusterRegion)
	accessKeyIDBase64, secureKeyBase64 := gjson.Get(credential, `data.aws_access_key_id`).String(), gjson.Get(credential, `data.aws_secret_access_key`).String()
	accessKeyID, err := base64.StdEncoding.DecodeString(accessKeyIDBase64)
	o.Expect(err).NotTo(o.HaveOccurred())
	secureKey, err := base64.StdEncoding.DecodeString(secureKeyBase64)
	o.Expect(err).NotTo(o.HaveOccurred())
	os.Setenv("AWS_ACCESS_KEY_ID", string(accessKeyID))
	os.Setenv("AWS_SECRET_ACCESS_KEY", string(secureKey))
}

// getRootSecretNameByCloudProvider gets the root secret name by cloudProvider
func getRootSecretNameByCloudProvider() (rootSecret string) {
	switch cloudProvider {
	case "aws":
		return "aws-creds"
	case "gcp":
		return "gcp-credentials"
	case "azure":
		return "azure-credentials"
	case "vsphere":
		return "vsphere-creds"
	case "openstack":
		return "openstack-credentials"
	case "ovirt":
		return "ovirt-credentials"
	default:
		e2e.Logf("Unsupported platform: %s", cloudProvider)
		return rootSecret
	}
}

// isRootSecretExist returns bool, judges whether the root secret exist in the kube-system namespace
func isRootSecretExist(oc *exutil.CLI) bool {
	rootSecret := getRootSecretNameByCloudProvider()
	if rootSecret == "" {
		return false
	}
	return isSpecifiedResourceExist(oc, "secret/"+getRootSecretNameByCloudProvider(), "kube-system")
}

// Get the volume detail info by persistent volume id
func getAwsVolumeInfoByVolumeID(volumeID string) (string, error) {
	mySession := session.Must(session.NewSession())
	svc := ec2.New(mySession, aws.NewConfig())
	input := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("volume-id"),
				Values: []*string{
					aws.String(volumeID),
				},
			},
		},
	}
	volumeInfo, err := svc.DescribeVolumes(input)
	return interfaceToString(volumeInfo), err
}

// Get the volume status "in use" or "available" by persistent volume id
func getAwsVolumeStatusByVolumeID(volumeID string) (string, error) {
	volumeInfo, err := getAwsVolumeInfoByVolumeID(volumeID)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeStatus := gjson.Get(volumeInfo, `Volumes.0.State`).Str
	e2e.Logf("The volume %s status is %q on aws backend", volumeID, volumeStatus)
	return volumeStatus, err
}

// Delete backend volume
func deleteBackendVolumeByVolumeID(oc *exutil.CLI, volumeID string) (string, error) {
	switch cloudProvider {
	case "aws":
		if strings.Contains(volumeID, "::") {
			volumeID = strings.Split(volumeID, "::")[1]
			mySession := session.Must(session.NewSession())
			svc := efs.New(mySession)
			deleteAccessPointID := &efs.DeleteAccessPointInput{
				AccessPointId: aws.String(volumeID),
			}
			req, resp := svc.DeleteAccessPointRequest(deleteAccessPointID)
			return interfaceToString(resp), req.Send()
		}
		mySession := session.Must(session.NewSession())
		svc := ec2.New(mySession, aws.NewConfig())
		deleteVolumeID := &ec2.DeleteVolumeInput{
			VolumeId: aws.String(volumeID),
		}
		req, resp := svc.DeleteVolumeRequest(deleteVolumeID)
		return interfaceToString(resp), req.Send()
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

// Check the volume status becomes available, status is "available"
func checkVolumeAvailableOnBackend(volumeID string) (bool, error) {
	volumeStatus, err := getAwsVolumeStatusByVolumeID(volumeID)
	availableStatus := []string{"available"}
	return contains(availableStatus, volumeStatus), err
}

// Check the volume is deleted
func checkVolumeDeletedOnBackend(volumeID string) (bool, error) {
	volumeStatus, err := getAwsVolumeStatusByVolumeID(volumeID)
	deletedStatus := []string{""}
	return contains(deletedStatus, volumeStatus), err
}

// Waiting the volume become available
func waitVolumeAvailableOnBackend(oc *exutil.CLI, volumeID string) {
	switch cloudProvider {
	case "aws":
		if strings.Contains(volumeID, "::") {
			e2e.Logf("Get EFS volume: \"%s\" status is under development", volumeID)
		} else {
			err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
				volumeStatus, errinfo := checkVolumeAvailableOnBackend(volumeID)
				if errinfo != nil {
					e2e.Logf("the err:%v, wait for volume %v to become available.", errinfo, volumeID)
					return volumeStatus, errinfo
				}
				if !volumeStatus {
					return volumeStatus, nil
				}
				return volumeStatus, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The volume:%v, is not available.", volumeID))
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

// Waiting the volume become deleted
func waitVolumeDeletedOnBackend(oc *exutil.CLI, volumeID string) {
	switch cloudProvider {
	case "aws":
		if strings.Contains(volumeID, "::") {
			e2e.Logf("Get EFS volume: \"%s\" status is under development", volumeID)
		} else {
			err := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
				volumeStatus, errinfo := checkVolumeDeletedOnBackend(volumeID)
				if errinfo != nil {
					e2e.Logf("the err:%v, wait for volume %v to be deleted.", errinfo, volumeID)
					return volumeStatus, errinfo
				}
				if !volumeStatus {
					return volumeStatus, nil
				}
				return volumeStatus, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The volume:%v, still exist.", volumeID))
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
func getAwsVolumeTypeByVolumeID(volumeID string) string {
	volumeInfo, err := getAwsVolumeInfoByVolumeID(volumeID)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeType := gjson.Get(volumeInfo, `Volumes.0.VolumeType`).Str
	e2e.Logf("The volume %s type is %q on aws backend", volumeID, volumeType)
	return volumeType
}

// Get the volume iops by volume id
func getAwsVolumeIopsByVolumeID(volumeID string) int64 {
	volumeInfo, err := getAwsVolumeInfoByVolumeID(volumeID)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeIops := gjson.Get(volumeInfo, `Volumes.0.Iops`).Int()
	e2e.Logf("The volume %s Iops is %d on aws backend", volumeID, volumeIops)
	return volumeIops
}

// Init the aws session
func newAwsClient() *ec2.EC2 {
	mySession := session.Must(session.NewSession())
	svc := ec2.New(mySession, aws.NewConfig())
	return svc
}

type ebsVolume struct {
	AvailabilityZone string
	Encrypted        bool
	Size             int64  // The size of the volume, in GiBs
	VolumeType       string // Valid Values: standard | io1 | io2 | gp2 | sc1 | st1 | gp3
	Device           string
	ActualDevice     string
	volumeID         string
	attachedNode     string
	State            string // Valid Values: creating | available | in-use | deleting | deleted | error
	DeviceByID       string
	ExpandSize       int64
	clusterIDTagKey  string
}

// function option mode to change the default values of ebs volume attribute
type volOption func(*ebsVolume)

// Replace the default value of ebs volume AvailabilityZone
func setVolAz(az string) volOption {
	return func(vol *ebsVolume) {
		vol.AvailabilityZone = az
	}
}

// Replace the default value of ebs volume Encrypted
func setVolEncrypted(encryptedBool bool) volOption {
	return func(vol *ebsVolume) {
		vol.Encrypted = encryptedBool
	}
}

// Replace the default value of ebs volume Size
func setVolSize(size int64) volOption {
	return func(vol *ebsVolume) {
		vol.Size = size
	}
}

// Replace the default value of ebs volume VolumeType
func setVolType(volType string) volOption {
	return func(vol *ebsVolume) {
		vol.VolumeType = volType
	}
}

// Replace the default value of ebs volume Device
func setVolDevice(device string) volOption {
	return func(vol *ebsVolume) {
		vol.Device = device
	}
}

// Replace the default value of ebs volume clusterID tag key
func setVolClusterIDTagKey(clusterIDTagKey string) volOption {
	return func(vol *ebsVolume) {
		vol.clusterIDTagKey = clusterIDTagKey
	}
}

// Create a new customized pod object
func newEbsVolume(opts ...volOption) ebsVolume {
	defaultVol := ebsVolume{
		AvailabilityZone: "",
		Encrypted:        false,
		Size:             getRandomNum(5, 15),
		VolumeType:       "gp3",
		Device:           getValidDeviceForEbsVol(),
	}
	for _, o := range opts {
		o(&defaultVol)
	}
	return defaultVol
}

// Request create ebs volume on aws backend
// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateVolume.html
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
func (vol *ebsVolume) create(ac *ec2.EC2) string {
	// Adapt for aws local zones which only support "gp2" volume
	// https://aws.amazon.com/about-aws/global-infrastructure/localzones/locations/
	if len(strings.Split(vol.AvailabilityZone, "-")) > 3 {
		vol.VolumeType = "gp2"
	}
	volumeInput := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(vol.AvailabilityZone),
		Encrypted:        aws.Bool(vol.Encrypted),
		Size:             aws.Int64(vol.Size),
		VolumeType:       aws.String(vol.VolumeType),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String("volume"),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("Name"),
						Value: aws.String("storage-lso-test-" + getRandomString()),
					},
					{
						Key:   aws.String(vol.clusterIDTagKey),
						Value: aws.String("owned"),
					},
				},
			},
		},
	}
	volInfo, err := ac.CreateVolume(volumeInput)
	debugLogf("EBS Volume info:\n%+v", volInfo)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeID := gjson.Get(interfaceToString(volInfo), `VolumeId`).String()
	o.Expect(volumeID).NotTo(o.Equal(""))
	vol.volumeID = volumeID
	return volumeID
}

// Create ebs volume on aws backend and waiting for state value to "available"
func (vol *ebsVolume) createAndReadyToUse(ac *ec2.EC2) {
	vol.create(ac)
	vol.waitStateAsExpected(ac, "available")
	vol.State = "available"
	e2e.Logf("The ebs volume : \"%s\" [regin:\"%s\",az:\"%s\",size:\"%dGi\"] is ReadyToUse",
		vol.volumeID, os.Getenv("AWS_REGION"), vol.AvailabilityZone, vol.Size)
}

// Get the ebs volume detail info
func (vol *ebsVolume) getInfo(ac *ec2.EC2) (string, error) {
	inputVol := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("volume-id"),
				Values: []*string{
					aws.String(vol.volumeID),
				},
			},
		},
	}
	volumeInfo, err := ac.DescribeVolumes(inputVol)
	return interfaceToString(volumeInfo), err
}

// Request attach the ebs volume to specified instance
func (vol *ebsVolume) attachToInstance(ac *ec2.EC2, instance node) *ec2.VolumeAttachment {
	volumeInput := &ec2.AttachVolumeInput{
		Device:     aws.String(vol.Device),
		InstanceId: aws.String(instance.instanceID),
		VolumeId:   aws.String(vol.volumeID),
	}
	req, resp := ac.AttachVolumeRequest(volumeInput)
	err := req.Send()
	// Enhancement: When the node already attached several volumes retry new device
	// to avoid device name conflict cause the attach action failed
	if strings.Contains(fmt.Sprint(err), "is already in use") {
		for i := 1; i <= 8; i++ {
			devMaps[strings.Split(vol.Device, "")[len(vol.Device)-1]] = true
			vol.Device = getValidDeviceForEbsVol()
			volumeInput.Device = aws.String(vol.Device)
			req, resp = ac.AttachVolumeRequest(volumeInput)
			e2e.Logf("Attached to \"%s\" failed of \"%+v\" try next*%d* Device \"%s\"",
				instance.instanceID, err, i, vol.Device)
			err = req.Send()
			debugLogf("Req:\"%+v\", Resp:\"%+v\"", req, resp)
			if err == nil {
				break
			}
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	vol.attachedNode = instance.instanceID
	return resp
}

// Waiting for the ebs volume state to expected value
func (vol *ebsVolume) waitStateAsExpected(ac *ec2.EC2, expectedState string) {
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		volInfo, errinfo := vol.getInfo(ac)
		if errinfo != nil {
			e2e.Logf("Get ebs volume failed :%v, wait for next round get.", errinfo)
			return false, errinfo
		}
		if gjson.Get(volInfo, `Volumes.0.State`).String() == expectedState {
			e2e.Logf("The ebs volume : \"%s\" [regin:\"%s\",az:\"%s\",size:\"%dGi\"] is as expected \"%s\"",
				vol.volumeID, os.Getenv("AWS_REGION"), vol.AvailabilityZone, vol.Size, expectedState)
			vol.State = expectedState
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for ebs volume : \"%s\" state to  \"%s\" time out", vol.volumeID, expectedState))
}

// Waiting for the ebs volume size to expected value
func (vol *ebsVolume) waitSizeAsExpected(ac *ec2.EC2, expectedSize int64) {
	var expectedSizeString = strconv.FormatInt(expectedSize, 10)
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		volInfo, errinfo := vol.getInfo(ac)
		if errinfo != nil {
			e2e.Logf("Get ebs volume failed :%v, wait for next round get.", errinfo)
			return false, errinfo
		}
		if gjson.Get(volInfo, `Volumes.0.Size`).String() == expectedSizeString {
			e2e.Logf("The ebs volume : \"%s\" [regin:\"%s\",az:\"%s\",size:\"%dGi\"] is expand to \"%sGi\"",
				vol.volumeID, os.Getenv("AWS_REGION"), vol.AvailabilityZone, vol.Size, expectedSizeString)
			vol.Size = expectedSize
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for ebs volume : \"%s\" expand to \"%sGi\" time out", vol.volumeID, expectedSizeString))
}

// Waiting for the ebs volume attach to node succeed
func (vol *ebsVolume) waitAttachSucceed(ac *ec2.EC2) {
	err := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		volInfo, errinfo := vol.getInfo(ac)
		if errinfo != nil {
			e2e.Logf("Get ebs volume failed :%v, wait for next round get.", errinfo)
			return false, errinfo
		}
		if gjson.Get(volInfo, `Volumes.0.Attachments.0.State`).String() == "attached" {
			e2e.Logf("The ebs volume : \"%s\" attached to instance \"%s\" succeed", vol.volumeID, vol.attachedNode)
			vol.State = "in-use"
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for the ebs volume \"%s\" attach to node %s timeout", vol.volumeID, vol.attachedNode))
}

// Attach the ebs volume to specified instance and wait for attach succeed
func (vol *ebsVolume) attachToInstanceSucceed(ac *ec2.EC2, oc *exutil.CLI, instance node) {
	vol.attachToInstance(ac, instance)
	vol.waitAttachSucceed(ac)
	vol.attachedNode = instance.instanceID
	deviceByID := "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol" + strings.TrimPrefix(vol.volumeID, "vol-")
	deviceInfo, err := execCommandInSpecificNode(oc, instance.name, "lsblk -J")
	o.Expect(err).NotTo(o.HaveOccurred())
	sameSizeDevices := gjson.Get(deviceInfo, `blockdevices.#(size=`+strconv.FormatInt(vol.Size, 10)+`G)#.name`).Array()
	sameTypeDevices := gjson.Get(deviceInfo, `blockdevices.#(type="disk")#.name`).Array()
	devices := sliceIntersect(strings.Split(strings.Trim(strings.Trim(fmt.Sprint(sameSizeDevices), "["), "]"), " "),
		strings.Split(strings.Trim(strings.Trim(fmt.Sprint(sameTypeDevices), "["), "]"), " "))
	o.Expect(devices).NotTo(o.BeEmpty())
	e2e.Logf(`The same type and size filtered Devices are: "%v"`, devices)
	if len(devices) == 1 {
		vol.ActualDevice = "/dev/" + devices[0]
	} else {
		for _, device := range devices {
			if strings.Split(device, "")[len(device)-1] == strings.Split(vol.Device, "")[len(vol.Device)-1] {
				vol.ActualDevice = "/dev/" + device
				break
			}
		}
	}
	if statInfo, _ := execCommandInSpecificNode(oc, instance.name, "stat "+deviceByID); strings.Contains(statInfo, "symbolic link") {
		vol.DeviceByID = deviceByID
	} else {
		vol.DeviceByID = vol.ActualDevice
	}
	e2e.Logf("Volume : \"%s\" attach to instance \"%s\" [Device:\"%s\", ById:\"%s\", ActualDevice:\"%s\"]", vol.volumeID, vol.attachedNode, vol.Device, vol.DeviceByID, vol.ActualDevice)
}

// Request detach the ebs volume from instance
func (vol *ebsVolume) detach(ac *ec2.EC2) error {
	volumeInput := &ec2.DetachVolumeInput{
		Device:     aws.String(vol.Device),
		InstanceId: aws.String(vol.attachedNode),
		Force:      aws.Bool(false),
		VolumeId:   aws.String(vol.volumeID),
	}
	if vol.attachedNode == "" {
		e2e.Logf("The ebs volume \"%s\" is not attached to any node,no need to detach", vol.volumeID)
		return nil
	}
	req, resp := ac.DetachVolumeRequest(volumeInput)
	err := req.Send()
	debugLogf("Resp:\"%+v\", Err:\"%+v\"", resp, err)
	return err
}

// Detach the ebs volume from instance and wait detach action succeed
func (vol *ebsVolume) detachSucceed(ac *ec2.EC2) {
	err := vol.detach(ac)
	o.Expect(err).NotTo(o.HaveOccurred())
	vol.waitStateAsExpected(ac, "available")
	vol.State = "available"
}

// Delete the ebs volume
func (vol *ebsVolume) delete(ac *ec2.EC2) error {
	deleteVolumeID := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(vol.volumeID),
	}
	req, resp := ac.DeleteVolumeRequest(deleteVolumeID)
	err := req.Send()
	debugLogf("Resp:\"%+v\", Err:\"%+v\"", resp, err)
	return err
}

// Delete the ebs volume and wait for delete succeed
func (vol *ebsVolume) deleteSucceed(ac *ec2.EC2) {
	err := vol.delete(ac)
	o.Expect(err).NotTo(o.HaveOccurred())
	vol.waitStateAsExpected(ac, "")
}

// Detach and delete the ebs volume and wait for all actions succeed
func (vol *ebsVolume) detachAndDeleteSucceed(ac *ec2.EC2) {
	vol.detachSucceed(ac)
	vol.deleteSucceed(ac)
}

// Send expand the EBS volume capacity request
func (vol *ebsVolume) expand(ac *ec2.EC2, expandCapacity int64) error {
	expandVolumeInput := &ec2.ModifyVolumeInput{
		Size:     aws.Int64(expandCapacity),
		VolumeId: aws.String(vol.volumeID),
	}
	req, resp := ac.ModifyVolumeRequest(expandVolumeInput)
	err := req.Send()
	debugLogf("Resp:\"%+v\", Err:\"%+v\"", resp, err)
	vol.ExpandSize = expandCapacity
	return err
}

// Expand the EBS volume capacity and wait for the expanding succeed
func (vol *ebsVolume) expandSucceed(ac *ec2.EC2, expandCapacity int64) {
	err := wait.Poll(10*time.Second, 90*time.Second, func() (bool, error) {
		expandErr := vol.expand(ac, expandCapacity)
		// On AWS some regions the volume resize service usually has the service busy 503 error, after retry it could be successful
		if expandErr != nil {
			e2e.Logf(`Sending the volume:%s expand request failed of "%s", try again`, vol.volumeID, expandErr.Error())
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for sending the volume:%s expand request successfully timeout", vol.volumeID))
	vol.waitSizeAsExpected(ac, expandCapacity)
}

// Send reboot instance request
func rebootInstance(ac *ec2.EC2, instanceID string) error {
	return dryRunRebootInstance(ac, instanceID, false)
}

// dryRunRebootInstance sends reboot instance request with DryRun parameter
func dryRunRebootInstance(ac *ec2.EC2, instanceID string, dryRun bool) error {
	rebootInstancesInput := &ec2.RebootInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
		DryRun:      aws.Bool(dryRun),
	}
	req, resp := ac.RebootInstancesRequest(rebootInstancesInput)
	err := req.Send()
	debugLogf("Resp:\"%+v\", Err:\"%+v\"", resp, err)
	return err
}

// Reboot specified instance and wait for rebooting succeed
func rebootInstanceAndWaitSucceed(ac *ec2.EC2, instanceID string) {
	err := rebootInstance(ac, instanceID)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Send reboot Instance:\"%+s\" request Succeed", instanceID)
	instancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}
	err = ac.WaitUntilInstanceRunning(instancesInput)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Reboot Instance:\"%+s\" Succeed", instanceID)
}

// start AWS instance
func startInstance(ac *exutil.AwsClient, instanceID string) {
	stateErr := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		state, err := ac.GetAwsInstanceState(instanceID)
		if err != nil {
			e2e.Logf("%v", err)
			return false, nil
		}
		if state == "running" {
			e2e.Logf("The instance is already running")
			return true, nil
		}
		if state == "stopped" {
			err = ac.StartInstance(instanceID)
			if err != nil {
				return false, err
			}
			return true, nil
		}
		e2e.Logf("The instance is in %s, please check.", state)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(stateErr, fmt.Sprintf("Start instance %q failed.", instanceID))
}

func stopInstance(ac *exutil.AwsClient, instanceID string) {
	stateErr := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
		state, err := ac.GetAwsInstanceState(instanceID)
		if err != nil {
			e2e.Logf("%v", err)
			return false, nil
		}
		if state == "stopped" {
			e2e.Logf("The instance is already stopped.")
			return true, nil
		}
		if state == "running" {
			err = ac.StopInstance(instanceID)
			if err != nil {
				return false, err
			}
			return true, nil
		}
		e2e.Logf("The instance is in %s, please check.", state)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(stateErr, fmt.Sprintf("Stop instance %q failed.", instanceID))
}

// Create aws backend session connection
func newAwsSession(oc *exutil.CLI) *session.Session {
	getCredentialFromCluster(oc)
	return session.Must(session.NewSession())
}

// Init the aws kms svc client
func newAwsKmsClient(awsSession *session.Session) *kms.KMS {
	return kms.New(awsSession, aws.NewConfig().WithRegion(os.Getenv("AWS_REGION")))
}

// Creates aws customized customer managed kms key
func createAwsCustomizedKmsKey(kmsSvc *kms.KMS, keyEncryptionAlgorithm string, keyUsage string) (kmsKeyResult *kms.CreateKeyOutput, err error) {
	kmsKeyResult, err = kmsSvc.CreateKey(&kms.CreateKeyInput{
		KeySpec:  aws.String(keyEncryptionAlgorithm),
		KeyUsage: aws.String(keyUsage),
		Tags: []*kms.Tag{
			{
				TagKey:   aws.String("Purpose"),
				TagValue: aws.String("ocp-storage-qe-ci-test"),
			},
		},
	})
	e2e.Logf("Create kms key result info:\n%v", kmsKeyResult)
	if err != nil {
		e2e.Logf("Failed to create kms key: %v", err)
		return kmsKeyResult, err
	}
	e2e.Logf("Create kms key:\"%+s\" Succeed", *kmsKeyResult.KeyMetadata.KeyId)
	return kmsKeyResult, nil
}

// Creates aws customer managed symmetric KMS key for encryption and decryption
func createAwsRsaKmsKey(kmsSvc *kms.KMS) *kms.CreateKeyOutput {
	kmsKeyResult, err := createAwsCustomizedKmsKey(kmsSvc, "RSA_4096", "ENCRYPT_DECRYPT")
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return kmsKeyResult
}

// Creates aws customer managed asymmetric RSA KMS key for encryption and decryption
func createAwsSymmetricKmsKey(kmsSvc *kms.KMS) *kms.CreateKeyOutput {
	kmsKeyResult, err := createAwsCustomizedKmsKey(kmsSvc, "SYMMETRIC_DEFAULT", "ENCRYPT_DECRYPT")
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return kmsKeyResult
}

// Describes aws customer managed kms key info
func describeAwsKmsKeyByID(kmsSvc *kms.KMS, kmsKeyID string) (describeResult *kms.DescribeKeyOutput, err error) {
	describeResult, err = kmsSvc.DescribeKey(&kms.DescribeKeyInput{
		KeyId: aws.String(kmsKeyID),
	})
	e2e.Logf("Describe kms key result is:\n%v", describeResult)
	if err != nil {
		e2e.Logf(`Failed to describe kms key "%s":\n%v`, kmsKeyID, err)
	}
	return describeResult, err
}

// Schedules aws customer managed kms key deletion by keyID
func scheduleAwsKmsKeyDeletionByID(kmsSvc *kms.KMS, kmsKeyID string) error {
	scheduleKeyDeletionResult, err := kmsSvc.ScheduleKeyDeletion(&kms.ScheduleKeyDeletionInput{
		KeyId: aws.String(kmsKeyID),
		// "PendingWindowInDays" value is optional. If you include a value, it must be between 7 and 30, inclusive.
		// If you do not include a value, it defaults to 30.
		PendingWindowInDays: aws.Int64(7),
	})
	e2e.Logf("Schedule kms key deletion result info:\n%v", scheduleKeyDeletionResult)
	if err != nil {
		e2e.Logf(`Failed to schedule kms key "%s" deletion: %v`, kmsKeyID, err)
	}
	return err
}

// Cancels aws customer managed kms key deletion by keyID
func cancelAwsKmsKeyDeletionByID(kmsSvc *kms.KMS, kmsKeyID string) error {
	scheduleKeyDeletionResult, err := kmsSvc.CancelKeyDeletion(&kms.CancelKeyDeletionInput{
		KeyId: aws.String(kmsKeyID),
	})
	e2e.Logf("Cancels kms key deletion result info:\n%v", scheduleKeyDeletionResult)
	if err != nil {
		e2e.Logf(`Failed to cancel kms key "%s" deletion: %v`, kmsKeyID, err)
	}
	return err
}

// Enables aws customer managed kms key
func enableAwsKmsKey(kmsSvc *kms.KMS, kmsKeyID string) error {
	enableKeyResult, err := kmsSvc.EnableKey(&kms.EnableKeyInput{
		KeyId: aws.String(kmsKeyID),
	})
	e2e.Logf("Enables kms key result info:\n%v", enableKeyResult)
	if err != nil {
		e2e.Logf(`Failed to enable kms key "%s": %v`, kmsKeyID, err)
		describeAwsKmsKeyByID(kmsSvc, kmsKeyID)
	}
	return err
}

// Gets aws customer managed kms key policy
func getAwsKmsKeyPolicy(kmsSvc *kms.KMS, kmsKeyID string) (getKeyPolicyResult *kms.GetKeyPolicyOutput, err error) {
	getKeyPolicyResult, err = kmsSvc.GetKeyPolicy(&kms.GetKeyPolicyInput{
		KeyId:      aws.String(kmsKeyID),
		PolicyName: aws.String("default"),
	})
	e2e.Logf("Gets kms key default policy result info:\n%v", getKeyPolicyResult)
	if err != nil {
		e2e.Logf(`Failed to get kms key "%s" policy: %v`, kmsKeyID, err)
	}
	return getKeyPolicyResult, err
}

// Updates aws customer managed kms key policy
func updateAwsKmsKeyPolicy(kmsSvc *kms.KMS, kmsKeyID string, policyContentStr string) error {
	updateKeyPolicyResult, err := kmsSvc.PutKeyPolicy(&kms.PutKeyPolicyInput{
		KeyId:      aws.String(kmsKeyID),
		Policy:     aws.String(policyContentStr),
		PolicyName: aws.String("default"),
	})
	e2e.Logf("Updates kms key policy result info:\n%v", updateKeyPolicyResult)
	if err != nil {
		e2e.Logf(`Failed to update kms key "%s" policy: %v`, kmsKeyID, err)
		return err
	}
	return nil
}

// Adds the aws customer managed kms key user
func addAwsKmsKeyUser(kmsSvc *kms.KMS, kmsKeyID string, userArn string) {
	userPermissionPolicyTemplateByte := []byte(
		`{
			"Version": "2012-10-17",
			"Id": "user-permission-key-policy-template",
			"Statement": [
				{
					"Sid": "Enable IAM User Permissions",
					"Effect": "Allow",
					"Principal": {
						"AWS": "arn:aws:iam::112233:root"
					},
					"Action": "kms:*",
					"Resource": "*"
				},
				{
					"Sid": "Allow use of the key",
					"Effect": "Allow",
					"Principal": {
						"AWS": "arn:aws:iam::112233:user/myuser"
					},
					"Action": [
						"kms:Encrypt",
						"kms:Decrypt",
						"kms:ReEncrypt*",
						"kms:GenerateDataKey*",
						"kms:DescribeKey"
					],
					"Resource": "*"
				},
				{
					"Sid": "Allow attachment of persistent resources",
					"Effect": "Allow",
					"Principal": {
						"AWS": "arn:aws:iam::112233:user/myuser"
					},
					"Action": [
						"kms:CreateGrant",
						"kms:ListGrants",
						"kms:RevokeGrant"
					],
					"Resource": "*",
					"Condition": {
						"Bool": {
							"kms:GrantIsForAWSResource": "true"
						}
					}
				}
			]
		}`)
	arnInfo, parseErr := arn.Parse(userArn)
	o.Expect(parseErr).ShouldNot(o.HaveOccurred())
	userPermissionPolicy, err := sjson.Set(string(userPermissionPolicyTemplateByte), `Statement.0.Principal.AWS`, "arn:aws:iam::"+arnInfo.AccountID+":root")
	o.Expect(err).ShouldNot(o.HaveOccurred())
	userPermissionPolicy, err = sjson.Set(userPermissionPolicy, `Statement.1.Principal.AWS`, userArn)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	userPermissionPolicy, err = sjson.Set(userPermissionPolicy, `Statement.2.Principal.AWS`, userArn)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	err = updateAwsKmsKeyPolicy(kmsSvc, kmsKeyID, userPermissionPolicy)
	o.Expect(err).ShouldNot(o.HaveOccurred())
}

// Cancels aws customer managed kms key deletion and enables the kms key by keyID
func cancelDeletionAndEnableAwsKmsKey(kmsSvc *kms.KMS, kmsKeyID string) {
	o.Expect(cancelAwsKmsKeyDeletionByID(kmsSvc, kmsKeyID)).ShouldNot(o.HaveOccurred())
	o.Expect(enableAwsKmsKey(kmsSvc, kmsKeyID)).ShouldNot(o.HaveOccurred())
}

// Init the aws resourcegroupstaggingapi client
func newAwsResourceGroupsTaggingAPIClient(awsSession *session.Session) *resourcegroupstaggingapi.ResourceGroupsTaggingAPI {
	return resourcegroupstaggingapi.New(awsSession, aws.NewConfig().WithRegion(os.Getenv("AWS_REGION")))
}

// Gets aws resources list by tags
func getAwsResourcesListByTags(rgtAPI *resourcegroupstaggingapi.ResourceGroupsTaggingAPI, resourceTypes []string, tagFilters map[string][]string) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	var finalTagFilters []*resourcegroupstaggingapi.TagFilter
	for tagKey, tagValues := range tagFilters {
		finalTagFilters = append(finalTagFilters, &resourcegroupstaggingapi.TagFilter{
			Key:    aws.String(tagKey),
			Values: aws.StringSlice(tagValues),
		})
	}
	filtersResult, err := rgtAPI.GetResources(&resourcegroupstaggingapi.GetResourcesInput{
		ResourcesPerPage:    aws.Int64(50),
		ResourceTypeFilters: aws.StringSlice(resourceTypes),
		TagFilters:          finalTagFilters,
	})
	e2e.Logf("FiltersResult result is:\n%v", filtersResult)
	if err != nil {
		e2e.Logf(`Failed to get resources by tagFilters "%s":\n%v`, tagFilters, err)
		return filtersResult, err
	}
	return filtersResult, nil
}

// Gets aws user arn by name
func getAwsUserArnByName(iamAPI *iam.IAM, userName string) (userArn string, err error) {
	userInfo, err := iamAPI.GetUser(&iam.GetUserInput{
		UserName: aws.String(userName),
	})
	e2e.Logf("Get user arn result is:\n%v", userInfo)
	if err != nil {
		e2e.Logf(`Failed to get user: "%s" arn by name\n%v`, userName, err)
	}
	return *userInfo.User.Arn, err
}

// Gets ebs-csi-driver user arn
func getAwsEbsCsiDriverUserArn(oc *exutil.CLI, iamAPI *iam.IAM) (driverUserArn string) {
	ebsCsiDriverUserName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cloud-credential-operator", "credentialsrequest/aws-ebs-csi-driver-operator", "-o=jsonpath={.status.providerStatus.user}").Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	driverUserArn, err = getAwsUserArnByName(iamAPI, ebsCsiDriverUserName)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return driverUserArn
}

func getgcloudClient(oc *exutil.CLI) *exutil.Gcloud {
	if exutil.CheckPlatform(oc) != "gcp" {
		g.Skip("it is not gcp platform!")
	}
	projectID, err := exutil.GetGcpProjectID(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	gcloud := exutil.Gcloud{ProjectID: projectID}
	return gcloud.Login()
}

// getPdVolumeInfoFromGCP returns pd volume detailed info from backend
func getPdVolumeInfoFromGCP(oc *exutil.CLI, pvID string, filterArgs ...string) map[string]interface{} {
	pdVolumeInfo, err := getgcloudClient(oc).GetPdVolumeInfo(pvID, filterArgs...)
	o.Expect(err).NotTo(o.HaveOccurred())
	debugLogf(`The volume:"%s" info is %s`, pvID, string(pdVolumeInfo))
	var pdVolumeInfoJSONMap map[string]interface{}
	json.Unmarshal([]byte(pdVolumeInfo), &pdVolumeInfoJSONMap)
	return pdVolumeInfoJSONMap
}

func getFilestoreInstanceFromGCP(oc *exutil.CLI, pvID string, filterArgs ...string) map[string]interface{} {
	filestoreInfo, err := getgcloudClient(oc).GetFilestoreInstanceInfo(pvID, filterArgs...)
	o.Expect(err).NotTo(o.HaveOccurred())
	debugLogf("Check filestore info %s", string(filestoreInfo))
	var filestoreJSONMap map[string]interface{}
	json.Unmarshal([]byte(filestoreInfo), &filestoreJSONMap)
	return filestoreJSONMap
}

// isAzureStackCluster judges whether the test cluster is on azure stack platform
func isAzureStackCluster(oc *exutil.CLI) bool {
	if cloudProvider != "azure" {
		return false
	}
	azureCloudType, errMsg := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
	o.Expect(errMsg).NotTo(o.HaveOccurred())
	return strings.EqualFold(azureCloudType, "AzureStackCloud")
}

// GetvSphereCredentials gets the vsphere credentials from cluster
func GetvSphereCredentials(oc *exutil.CLI) (host string, username string, password string) {
	credential, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "kube-system", "secret/"+getRootSecretNameByCloudProvider(), "-o", `jsonpath={.data}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get the vSphere cluster root credentials")
	for key, value := range gjson.Parse(credential).Value().(map[string]interface{}) {
		if strings.Contains(key, "username") {
			username = fmt.Sprint(value)
			host = strings.TrimSuffix(key, ".username")
		}
		if strings.Contains(key, "password") {
			password = fmt.Sprint(value)
		}
	}

	o.Expect(host).ShouldNot(o.BeEmpty(), "Failed to get the vSphere vcenter host")
	o.Expect(username).ShouldNot(o.BeEmpty(), "Failed to get the vSphere vcenter user")
	o.Expect(password).ShouldNot(o.BeEmpty(), "Failed to get the vSphere vcenter password")

	decodeByte, decodeErr := base64.StdEncoding.DecodeString(username)
	o.Expect(decodeErr).NotTo(o.HaveOccurred(), "Failed to decode the user")
	username = string(decodeByte)
	decodeByte, decodeErr = base64.StdEncoding.DecodeString(password)
	o.Expect(decodeErr).NotTo(o.HaveOccurred(), "Failed to decode the password")
	password = string(decodeByte)

	return host, username, password
}

// NewVim25Client init the vsphere vim25.Client
func NewVim25Client(ctx context.Context, oc *exutil.CLI) *vim25.Client {
	host, user, pwd := GetvSphereCredentials(oc)
	vsmURL := &url.URL{

		Scheme: "https",

		Host: host,

		Path: "/sdk",
	}
	vsmURL.User = url.UserPassword(user, pwd)
	govmomiClient, err := govmomi.NewClient(ctx, vsmURL, true)
	o.Expect(err).ShouldNot(o.HaveOccurred(), "Failed to init the vim25 client")
	return govmomiClient.Client
}

// Get the root directory path of EFS access point
func getEFSVolumeAccessPointRootDirectoryPath(oc *exutil.CLI, volumeID string) string {
	accessPointsInfo, err := describeEFSVolumeAccessPoints(oc, volumeID)
	o.Expect(err).NotTo(o.HaveOccurred())
	path := gjson.Get(accessPointsInfo, `AccessPoints.0.RootDirectory.Path`).String()
	e2e.Logf("RootDirectoryPath is %v", path)
	return path
}

// Describe the AccessPoints info from accesspointID
func describeEFSVolumeAccessPoints(oc *exutil.CLI, volumeID string) (string, error) {
	volumeID = strings.Split(volumeID, "::")[1]
	mySession := session.Must(session.NewSession())
	svc := efs.New(mySession)
	describeAccessPointID := &efs.DescribeAccessPointsInput{
		AccessPointId: aws.String(volumeID),
	}

	accessPointsInfo, err := svc.DescribeAccessPoints(describeAccessPointID)
	return interfaceToString(accessPointsInfo), err
}
