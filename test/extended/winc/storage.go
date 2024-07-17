package winc

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-windows] Windows_Containers Storage", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLIWithoutNamespace("default")

	g.BeforeEach(func() {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		iaasPlatform = strings.ToLower(output)
		var err error
		privateKey, err = exutil.GetPrivateKey()
		o.Expect(err).NotTo(o.HaveOccurred())
		publicKey, err = exutil.GetPublicKey()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("Smokerun-Author:jfrancoa-NonPreRelease-Longduration-Critical-66352-Windows workloads support CSI persistent storage [Serial]", func() {

		// TODO: Add support for other providers. Only known vSphere and Azure driver installation steps
		if iaasPlatform != "vsphere" && iaasPlatform != "azure" {
			g.Skip(iaasPlatform + " is not implemented yet, skipping")
		}

		namespace := "winc-66352"
		pvcs := []string{"pvc1", "pvc2"}

		defer deleteProject(oc, namespace)
		createProject(oc, namespace)

		// Add the SCC to the service account after creating the project
		g.By("Adding privileged SCC to service account")
		_, err := oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", fmt.Sprintf("system:serviceaccount:%s:azure-file-csi-driver-controller-sa", namespace)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		driverName := "vmware-vsphere-csi-driver-node-windows"
		if iaasPlatform == "azure" {
			driverName = "azure-file-csi-driver-node-windows"
		}
		dsInfo, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset", driverName, "-n", "openshift-cluster-csi-drivers", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(dsInfo, driverName) {
			g.By("Installing CSI Driver for Windows nodes")
			defer uninstallWindowsCSIDriver(oc, iaasPlatform)
			err := installWindowsCSIDriver(oc, iaasPlatform)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Create storageclass")
		scName := "fast-66352"
		manifestFile, err := exutil.GenerateManifestFile(oc, "winc", iaasPlatform+"_storageclass.yaml", map[string]string{"<name>": scName})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(manifestFile)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("storageclass", scName).Output()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		windowsHosts := getWindowsHostNames(oc)
		winInternalIP := getWindowsInternalIPs(oc)
		bastionHost := getSSHBastionHost(oc, iaasPlatform)
		for idx, pvc := range pvcs {

			g.By("Create Persistent Volume Claims")

			accessMode := "ReadWriteMany"
			if iaasPlatform == "vsphere" {
				accessMode = "ReadWriteOnce"
			}
			manifestFile, err = exutil.GenerateManifestFile(oc, "winc", "pvc.yaml", map[string]string{"<name>": pvc, "<namespace>": namespace, "<sc-name>": scName, "<access-mode>": accessMode, "<size>": "500Mi"})
			o.Expect(err).NotTo(o.HaveOccurred())
			defer os.Remove(manifestFile)
			defer deleteResource(oc, "pvc", pvc, namespace)
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", manifestFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			winHost := windowsHosts[idx]
			if iaasPlatform == "vsphere" {
				// Label nodes for those PVCs which support ReadWriteOnce only
				g.By(fmt.Sprintf("Label windows node %v to ensure all workloads from win-weberver-%v land in that Windows node", winHost, pvc))
				defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", winHost, "nodepvc-").Output()
				oc.AsAdmin().WithoutNamespace().Run("label").Args("node", winHost, "nodepvc="+pvc).Output()
			}

			g.By(fmt.Sprintf("Create Windows deployment %v-%v with persistent volume %v", windowsWorkloads, pvc, pvc))
			defer deleteResource(oc, "deployment", windowsWorkloads+"-"+pvc, namespace)
			nodeSelector := ""
			// if vsphere we need to make sure that each deployment lands in a different node
			if iaasPlatform == "vsphere" {
				nodeSelector = "nodepvc: " + pvc
			}
			createWorkload(oc, namespace, "windows_web_server_pvc.yaml", map[string]string{"<id>": pvc, "<pvc-name>": pvc, "<windows_container_image>": getConfigMapData(oc, wincTestCM, "primary_windows_container_image", defaultNamespace), "<node-selector>": nodeSelector}, false, windowsWorkloads+"-"+pvc)
			// Wait for the workloads to be created. Not using the logic in createWindowsWokrload
			// because the deployment name is different than win-webserver, as well as the number
			// of replicas
			poolErr := wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
				return checkWorkloadCreated(oc, windowsWorkloads+"-"+pvc, namespace, 3), nil
			})
			if poolErr != nil {
				e2e.Failf("Windows workload %v-%v is not ready after waiting up to 15 minutes ...", windowsWorkloads, pvc)
			}

			// Obtain the driver Name from the PV by using the PVC name
			e2e.Logf("Obtaining PV name for PVC: %s", pvc)
			pvName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", "-n", namespace, pvc, "-o=jsonpath={.spec.volumeName}").Output()
			if err != nil {
				e2e.Logf("Failed to get PV name for PVC: %s, error: %v", pvc, err)
			}
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("PV name for PVC %s: %s", pvc, pvName)

			// Print more log
			e2e.Logf("Obtaining CSI driver name for PV: %s", pvName)
			csiDriverName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", pvName, "-o=jsonpath={.spec.csi.driver}").Output()
			if err != nil {
				e2e.Logf("Failed to get CSI driver name for PV: %s, error: %v", pvName, err)
			}

			cmd := "ls -r C:\\var\\lib\\kubelet\\plugins\\kubernetes.io\\csi\\" + csiDriverName + "\\*\\globalmount"
			msg, err := runPSCommand(bastionHost, winInternalIP[idx], cmd, privateKey, iaasPlatform)
			if err != nil {
				e2e.Logf("Failed to run command: %s, error: %v", cmd, err)
			}
			o.Expect(err).NotTo(o.HaveOccurred())
			// The volume created under the directory
			// C:\var\lib\kubelet\plugins\kubernetes.io\csi\<driver_name>\
			// should contain the index.html file, which is the one created in the
			// volume mount.
			e2e.Logf("Command executed: %s", cmd)
			e2e.Logf("Message received: %s", msg)
			e2e.Logf("Error (if any): %v", err)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("index.html"), "Failed to check CSI volume being mounted on Windows node %v", winHost)

		}

		// Perform the check in a new loop to ensure both PVCs are already created.
		for _, pvc := range pvcs {
			// CHECK THAT modifying C:\html\index.html is reflected in all pods which
			// bind the same vSphere volume.
			hostIPArray, err := getWorkloadsHostIP(oc, windowsWorkloads+"-"+pvc, namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			winPodNameArray, err := getWorkloadsNames(oc, windowsWorkloads+"-"+pvc, namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			winPodIPArray, err := getWorkloadsIP(oc, windowsWorkloads+"-"+pvc, namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("svc", windowsWorkloads+"-"+pvc, "-n", namespace, "-o=jsonpath={.spec.ports[0].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check that service is available and the Web Server is serving content")
			var msgCmd []byte
			if iaasPlatform == "vsphere" {
				msgCmd, _ = exec.Command("bash", "-c", "curl "+hostIPArray[0]+":"+nodePort).Output()
			} else {
				externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads+"-"+pvc, namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				msgCmd, _ = exec.Command("bash", "-c", "curl "+externalIP).Output()
			}
			o.Expect(string(msgCmd)).To(o.ContainSubstring("Windows Container Web Server"))

			// Modify webserver html file stored in the volume for that deployment
			modText := "Windows Container Web Server " + strings.ToUpper(pvc) + " Modified"
			g.By(fmt.Sprintf("Set web server service %v to %v", windowsWorkloads+"-"+pvc, modText))
			command := []string{"-n", namespace, winPodNameArray[0], "--", "pwsh.exe", "-Command", "echo \"<html><body><H1>" + modText + "</H1></body></html>\" > C:\\html\\index.html"}
			_, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args(command...).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Verify that the modification is available accessing the ClusterIP as well as other POD's IP")
			if iaasPlatform == "vsphere" {
				msgCmd, _ = exec.Command("bash", "-c", "curl "+hostIPArray[0]+":"+nodePort).Output()
			} else {
				externalIP, err := getExternalIP(iaasPlatform, oc, windowsWorkloads+"-"+pvc, namespace)
				o.Expect(err).NotTo(o.HaveOccurred())
				msgCmd, _ = exec.Command("bash", "-c", "curl "+externalIP).Output()
			}
			o.Expect(string(msgCmd)).To(o.ContainSubstring(modText))

			command = []string{"-n", namespace, winPodNameArray[0], "--", "curl", winPodIPArray[1]}
			outExec, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(command...).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(outExec).To(o.ContainSubstring(modText))
		}
	})

})
