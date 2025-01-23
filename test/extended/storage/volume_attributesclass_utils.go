package storage

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

type volumeAttributesClass struct {
	name       string
	volumeType string
	template   string
	iops       string
	throughput string
	driverName string
}

// function option mode to change the default values of VolumeAttributesClass parameters, e.g. name, volumeType, iops, throughput etc.
type volumeAttributesClassOption func(*volumeAttributesClass)

// Replace the default value of VolumeAttributesClass name parameter
func setVolumeAttributesClassName(name string) volumeAttributesClassOption {
	return func(this *volumeAttributesClass) {
		this.name = name
	}
}

// Replace the default value of VolumeAttributesClass volumeType parameter
func setVolumeAttributesClassType(volumeType string) volumeAttributesClassOption {
	return func(this *volumeAttributesClass) {
		this.volumeType = volumeType
	}
}

// Replace the default value of VolumeAttributesClass template parameter
func setVolumeAttributesClassTemplate(template string) volumeAttributesClassOption {
	return func(this *volumeAttributesClass) {
		this.template = template
	}
}

// Replace the default value of VolumeAttributesClass iops parameter
func setVolumeAttributesClassIops(iops string) volumeAttributesClassOption {
	return func(this *volumeAttributesClass) {
		this.iops = iops
	}
}

// Replace the default value of VolumeAttributesClass throughput parameter
func setVolumeAttributesClassThroughput(throughput string) volumeAttributesClassOption {
	return func(this *volumeAttributesClass) {
		this.throughput = throughput
	}
}

// Replace the default value of VolumeAttributesClass driverName parameter
func setVolumeAttributesClassDriverName(driverName string) volumeAttributesClassOption {
	return func(this *volumeAttributesClass) {
		this.driverName = driverName
	}
}

// Create a new customized VolumeAttributesClass object
func newVolumeAttributesClass(opts ...volumeAttributesClassOption) volumeAttributesClass {
	defaultVolumeAttributesClass := volumeAttributesClass{
		name:       "my-vac-" + getRandomString(),
		template:   "volumeattributesclass-template.yaml",
		volumeType: "gp3",
		iops:       "3000",
		throughput: "125",
		driverName: "ebs.csi.aws.com",
	}

	for _, o := range opts {
		o(&defaultVolumeAttributesClass)
	}

	return defaultVolumeAttributesClass
}

// Create new VolumeAttributesClass with customized parameters
func (vac *volumeAttributesClass) create(oc *exutil.CLI) {
	err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", vac.template, "-p", "VACNAME="+vac.name, "VOLUMETYPE="+vac.volumeType,
		"IOPS="+vac.iops, "THROUGHPUT="+vac.throughput, "DRIVERNAME="+vac.driverName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create a new customized VolumeAttributesClass with extra parameters
func (vac *volumeAttributesClass) createWithExtraParameters(oc *exutil.CLI, vacParameters map[string]string) {
	extraParameters := map[string]interface{}{
		"jsonPath":   `items.0.`,
		"parameters": vacParameters,
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", vac.template, "-p",
		"VACNAME="+vac.name, "DRIVERNAME="+vac.driverName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Delete the VolumeAttributesClass resource
func (vac *volumeAttributesClass) deleteAsAdmin(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("volumeattributesclass", vac.name, "--ignore-not-found").Execute()
}
