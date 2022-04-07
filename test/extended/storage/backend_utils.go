package storage

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
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
	VolumeId         string
	attachedNode     string
	State            string // Valid Values: creating | available | in-use | deleting | deleted | error
	DeviceById       string
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

//  Create a new customized pod object
func newEbsVolume(opts ...volOption) ebsVolume {
	defaultVol := ebsVolume{
		AvailabilityZone: "",
		Encrypted:        false,
		Size:             getRandomNum(5, 15),
		VolumeType:       "gp3",
		Device:           getVaildDeviceForEbsVol(),
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
	volumeInput := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(vol.AvailabilityZone),
		Encrypted:        aws.Bool(vol.Encrypted),
		Size:             aws.Int64(vol.Size),
		VolumeType:       aws.String(vol.VolumeType),
	}
	volInfo, err := ac.CreateVolume(volumeInput)
	o.Expect(err).NotTo(o.HaveOccurred())
	volumeId := gjson.Get(interfaceToString(volInfo), `VolumeId`).String()
	o.Expect(volumeId).NotTo(o.Equal(""))
	vol.VolumeId = volumeId
	return volumeId
}

// Create ebs volume on aws backend and waiting for state value to "avaiable"
func (vol *ebsVolume) createAndReadyToUse(ac *ec2.EC2) {
	vol.create(ac)
	vol.waitStateAsExpected(ac, "available")
	vol.State = "available"
	e2e.Logf("The ebs volume : \"%s\" [regin:\"%s\",az:\"%s\",size:\"%dGi\"] is ReadyToUse",
		vol.VolumeId, os.Getenv("AWS_REGION"), vol.AvailabilityZone, vol.Size)
}

// Get the ebs volume detail info
func (vol *ebsVolume) getInfo(ac *ec2.EC2) (string, error) {
	inputVol := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("volume-id"),
				Values: []*string{
					aws.String(vol.VolumeId),
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
		InstanceId: aws.String(instance.instanceId),
		VolumeId:   aws.String(vol.VolumeId),
	}
	req, resp := ac.AttachVolumeRequest(volumeInput)
	err := req.Send()
	if strings.Contains(fmt.Sprint(err), "is already in use") {
		e2e.Logf("Attached to \"%s\" failed of \"%+v\" try another Device", instance.instanceId, err)
		devMaps[strings.Split(vol.Device, "")[len(vol.Device)-1]] = true
		vol.Device = getVaildDeviceForEbsVol()
		volumeInput.Device = aws.String(vol.Device)
		req, resp = ac.AttachVolumeRequest(volumeInput)
		err = req.Send()
		debugLogf("Req:\"%+v\", Resp:\"%+v\"", req, resp)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	vol.attachedNode = instance.instanceId
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
			e2e.Logf("The ebs volume : \"%s\" state is as expected \"%s\"", vol.VolumeId, expectedState)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for ebs volume : \"%s\" state to  \"%s\" time out", vol.VolumeId, expectedState))
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
			e2e.Logf("The ebs volume : \"%s\" attached to instance \"%s\" succeed", vol.VolumeId, vol.attachedNode)
			vol.State = "in-use"
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Waiting for the ebs volume \"%s\" attach to node %s timeout", vol.VolumeId, vol.attachedNode))
}

// Attach the ebs volume to specified instance and wait for attach succeed
func (vol *ebsVolume) attachToInstanceSucceed(ac *ec2.EC2, oc *exutil.CLI, instance node) {
	vol.attachToInstance(ac, instance)
	vol.waitAttachSucceed(ac)
	vol.attachedNode = instance.instanceId
	// RHEL type deviceid generate basic rule
	if instance.osId == "rhel" {
		deviceInfo, err := execCommandInSpecificNode(oc, instance.name, "lsblk -J")
		o.Expect(err).NotTo(o.HaveOccurred())
		sameSizeDevices := gjson.Get(deviceInfo, `blockdevices.#(size=`+strconv.FormatInt(vol.Size, 10)+`G)#.name`).Array()
		sameTypeDevices := gjson.Get(deviceInfo, `blockdevices.#(type="disk")#.name`).Array()
		devices := sliceIntersect(strings.Split(strings.Trim(strings.Trim(fmt.Sprint(sameSizeDevices), "["), "]"), " "),
			strings.Split(strings.Trim(strings.Trim(fmt.Sprint(sameTypeDevices), "["), "]"), " "))
		o.Expect(devices).NotTo(o.BeEmpty())
		for _, device := range devices {
			if strings.Split(device, "")[len(device)-1] == strings.Split(vol.Device, "")[len(vol.Device)-1] {
				vol.DeviceById = "/dev/" + device
				break
			}
		}
	} else {
		// RHCOS type deviceid generate basic rule
		vol.DeviceById = "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol" + strings.TrimLeft(vol.VolumeId, "vol-")
	}
	e2e.Logf("Volume : \"%s\" attach to instance \"%s\" [Device:\"%s\", ById:\"%s\"]", vol.VolumeId, vol.attachedNode, vol.Device, vol.DeviceById)
}

// Request detach the ebs volume from instance
func (vol *ebsVolume) detach(ac *ec2.EC2) error {
	volumeInput := &ec2.DetachVolumeInput{
		Device:     aws.String(vol.Device),
		InstanceId: aws.String(vol.attachedNode),
		Force:      aws.Bool(false),
		VolumeId:   aws.String(vol.VolumeId),
	}
	if vol.attachedNode == "" {
		e2e.Logf("The ebs volume \"%s\" is not attached to any node,no need to detach", vol.VolumeId)
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
		VolumeId: aws.String(vol.VolumeId),
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
