package storage

import (
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Define the NFS Server related functions
type nfsServer struct {
	deploy deployment
	svc    service
}

// function option mode to change the default values of nfsServer Object attributes
type nfsServerOption func(*nfsServer)

// Replace the default value of nfsServer deployment
func setNfsServerDeployment(deploy deployment) nfsServerOption {
	return func(nfs *nfsServer) {
		nfs.deploy = deploy
	}
}

// Replace the default value of nfsServer service
func setNfsServerSvc(svc service) nfsServerOption {
	return func(nfs *nfsServer) {
		nfs.svc = svc
	}
}

//  Create a new customized nfsServer object
func newNfsServer(opts ...nfsServerOption) nfsServer {
	serverName := "nfs-" + getRandomString()
	defaultNfsServer := nfsServer{
		deploy: newDeployment(setDeploymentName(serverName), setDeploymentApplabel(serverName), setDeploymentMountpath("/mnt/data")),
		svc:    newService(setServiceSelectorLable(serverName)),
	}
	for _, o := range opts {
		o(&defaultNfsServer)
	}
	return defaultNfsServer
}

// Install the specified NFS Server on cluster
func (nfs *nfsServer) install(oc *exutil.CLI) {
	nfs.deploy.create(oc)
	nfs.deploy.waitReady(oc)
	nfs.svc.name = "nfs-service"
	nfs.svc.create(oc)
	nfs.svc.getClusterIP(oc)
	e2e.Logf("Install NFS server successful, serverIP is %s", nfs.svc.clusterIP)
}

// Uninstall the specified NFS Server from cluster
func (nfs *nfsServer) uninstall(oc *exutil.CLI) {
	nfs.svc.deleteAsAdmin(oc)
	nfs.deploy.deleteAsAdmin(oc)
}

// Define the iSCSI Server related functions
type iscsiServer struct {
	deploy deployment
	svc    service
}

// function option mode to change the default values of iscsiServer Object attributes
type iscsiServerOption func(*iscsiServer)

// Replace the default value of iscsiServer deployment
func setIscsiServerDeployment(deploy deployment) iscsiServerOption {
	return func(iscsi *iscsiServer) {
		iscsi.deploy = deploy
	}
}

// Replace the default value of iscsiServer service
func setIscsiServerSvc(svc service) iscsiServerOption {
	return func(iscsi *iscsiServer) {
		iscsi.svc = svc
	}
}

//  Create a new customized iscsiServer object
func newIscsiServer(opts ...iscsiServerOption) iscsiServer {
	serverName := "iscsi-target" + getRandomString()
	serviceName := "iscsi-service" + getRandomString()
	defaultIscsiServer := iscsiServer{
		deploy: newDeployment(setDeploymentName(serverName), setDeploymentApplabel(serverName), setDeploymentMountpath("/lib/modules")),
		svc:    newService(setServiceName(serviceName), setServiceSelectorLable(serverName), setServiceNodePort("0"), setServicePort("3260"), setServiceTargetPort("3260"), setServiceProtocol("TCP")),
	}
	for _, o := range opts {
		o(&defaultIscsiServer)
	}
	return defaultIscsiServer
}

// Install the specified iSCSI Server on cluster
func (iscsi *iscsiServer) install(oc *exutil.CLI) {
	iscsi.deploy.create(oc)
	iscsi.deploy.waitReady(oc)
	iscsi.svc.create(oc)
	iscsi.svc.getClusterIP(oc)
	cmd := "targetcli /iscsi/iqn.2016-04.test.com:storage.target00/tpg1/portals create " + iscsi.svc.clusterIP //setup to work with service
	msg, _err := execCommandInSpecificPod(oc, iscsi.deploy.namespace, iscsi.deploy.getPodList(oc)[0], cmd)
	o.Expect(_err).NotTo(o.HaveOccurred())
	o.Expect(msg).To(o.ContainSubstring("Created network portal " + iscsi.svc.clusterIP + ":3260"))
	e2e.Logf("Install iSCSI server successful, serverIP is %s", iscsi.svc.clusterIP)
}

// Uninstall the specified iSCSI Server from cluster
func (iscsi *iscsiServer) uninstall(oc *exutil.CLI) {
	iscsi.svc.deleteAsAdmin(oc)
	iscsi.deploy.deleteAsAdmin(oc)
}
