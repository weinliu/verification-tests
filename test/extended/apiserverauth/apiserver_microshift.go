package apiserverauth

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-api-machinery] API_Server on Microshift", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")
	var tmpdir string

	g.JustBeforeEach(func() {
		tmpdir = "/tmp/-OCP-microshift-apiseerver-cases-" + exutil.GetRandomString() + "/"
		err := os.MkdirAll(tmpdir, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("The cluster should be healthy before running case.")
		errSanity := clusterSanityCheckMicroShift(oc)
		if errSanity != nil {
			e2e.Failf("Cluster health check failed before running case :: %s ", errSanity)
		}
	})

	g.JustAfterEach(func() {
		os.RemoveAll(tmpdir)
		logger.Infof("test dir %s is cleaned up", tmpdir)
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Longduration-NonPreRelease-Medium-63298-[Apiserver] manifest directory scanning [Disruptive][Slow]", func() {
		var (
			e2eTestNamespace = "microshift-ocp63298"
			etcConfigYaml    = "/etc/microshift/config.yaml"
			etcConfigYamlbak = "/etc/microshift/config.yaml.bak"
			tmpManifestPath  = "/etc/microshift/manifests.d/my-app/base /etc/microshift/manifests.d/my-app/dev /etc/microshift/manifests.d/my-app/dev/patches/"
			user             = "redhat"
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift vm")
		fqdnName := getMicroshiftHostname(oc)

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "hello-openshift-dev-app-ocp63298", "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "busybox-base-app-ocp63298", "--ignore-not-found").Execute()
		}()

		defer func() {
			etcConfigCMD := fmt.Sprintf(`'configfile=%v;
			configfilebak=%v;
			if [ -f $configfilebak ]; then
				cp $configfilebak $configfile; 
				rm -f $configfilebak;
			else
				rm -f $configfile;
			fi'`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		defer func() {
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo rm -rf "+tmpManifestPath)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("3. Take backup of config file")
		etcConfig := fmt.Sprintf(`'configfile=%v;
		configfilebak=%v;
		if [ -f $configfile ]; then 
			cp $configfile $configfilebak;
		fi'`, etcConfigYaml, etcConfigYamlbak)
		_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfig)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("4. Create tmp manifest path on node")
		_, dirErr := runSSHCommand(fqdnName, user, "sudo mkdir -p "+tmpManifestPath)
		o.Expect(dirErr).NotTo(o.HaveOccurred())

		//  Setting glob path values to multiple values should load manifests from all of them.
		exutil.By("4.1 Set glob path values to the manifest option in config")
		etcConfig = fmt.Sprintf(`
manifests:
  kustomizePaths:
  - /etc/microshift/manifests.d/my-app/*/
  - /etc/microshift/manifests.d/my-app/*/patches`)
		changeMicroshiftConfig(etcConfig, fqdnName, etcConfigYaml)

		newSrcFiles := map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				"/etc/microshift/manifests.d/my-app/base/",
				"NAMESPACEVAR",
				"base-app-ocp63298",
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				"/etc/microshift/manifests.d/my-app/base/",
				"NAMESPACEVAR",
				"base-app-ocp63298",
			},
			"hello-openshift.yaml": {
				"microshift-hello-openshift.yaml",
				"/etc/microshift/manifests.d/my-app/dev/patches/",
				"NAMESPACEVAR",
				"dev-app-ocp63298",
			},
			"kustomization": {
				"microshift-hello-openshift-kustomization.yaml",
				"/etc/microshift/manifests.d/my-app/dev/patches/",
				"NAMESPACEVAR",
				"dev-app-ocp63298",
			},
		}
		exutil.By("4.2 Create kustomization and deployemnt files")
		addKustomizationToMicroshift(fqdnName, newSrcFiles)
		restartErr := restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("4.3 Check pods after microshift restart")
		podsOutput := getPodsList(oc, "hello-openshift-dev-app-ocp63298")
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case :: Failed :: Pods are not created, manifests are not loaded from defined location")
		podsOutput = getPodsList(oc, "busybox-base-app-ocp63298")
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case :: Failed :: Pods are not created, manifests are not loaded from defined location")
		e2e.Logf("Test case :: Passed :: Pods are created, manifests are loaded from defined location :: %s", podsOutput[0])
	})

	// author: dpunia@redhat.com
	g.It("Author:dpunia-MicroShiftBoth-ConnectedOnly-ROSA-ARO-OSD_CCS-Medium-10969-Create clusterip service [Serial]", func() {
		var (
			caseID          = "10969"
			name            = "ocp-" + caseID + "-openshift"
			namespace       = "e2e-apiserver-" + caseID + "-" + exutil.GetRandomString()
			image           = "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
			serviceEndpoint string
			servicechkout   string
			servicechkerror error
			serviceIP       net.IP
		)

		exutil.By("1. Create one new namespace for the test scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(namespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(namespace)
		isNsPrivileged, _ := exutil.IsNamespacePrivileged(oc, namespace)
		if !isNsPrivileged {
			outputError := exutil.SetNamespacePrivileged(oc, namespace)
			o.Expect(outputError).NotTo(o.HaveOccurred())
		}

		exutil.By("2) Create new Hello OpenShift pod")
		appYamlFile := tmpdir + "ocp10969-hello-pod.yaml"
		appYaml := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  labels:
    app: %s
spec:
  containers:
  - name: %s
    image: %s
    ports:
    - containerPort: 8080
    imagePullPolicy: IfNotPresent
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
      privileged: false`, name, name, name, image)
		f, err := os.Create(appYamlFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", appYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().WithoutNamespace().Run("delete", "-f", appYamlFile, "-n", namespace).Args().Execute()
		saSecretErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", appYamlFile, "-n", namespace).Args().Execute()
		o.Expect(saSecretErr).NotTo(o.HaveOccurred())
		exutil.AssertPodToBeReady(oc, name, namespace)

		exutil.By("3) Generate random port and service ip used to create service")
		randomServicePort := int(getRandomNum(6000, 9000))
		clusterIP, svcErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "default", "service", "kubernetes", `-o=jsonpath={.spec.clusterIP}`).Output()
		o.Expect(svcErr).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 15*time.Second, false, func(cxt context.Context) (bool, error) {
			serviceIP = getServiceIP(oc, clusterIP)
			if serviceIP.String() == "172.30.0.0" {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to get one available service IP!")

		exutil.By("4) Create clusterip service with --clusterip")
		servicecreateout, servicecreateerror := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "service", "clusterip", name, "--clusterip", fmt.Sprintf("%v", serviceIP.String()), "--tcp", fmt.Sprintf("%v:8080", randomServicePort)).Output()
		o.Expect(servicecreateerror).NotTo(o.HaveOccurred())
		o.Expect(servicecreateout).Should(o.ContainSubstring(fmt.Sprintf("service/%v created", name)))

		exutil.By("5) Check clusterip service running status")
		if serviceIP.To4() != nil {
			serviceEndpoint = fmt.Sprintf("%v:%v", serviceIP.String(), randomServicePort)
		} else {
			serviceEndpoint = fmt.Sprintf("[%v]:%v", serviceIP.String(), randomServicePort)
		}
		// retry 3 times, sometimes, the endpoint is not ready for accessing.
		err = wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
			servicechkout, servicechkerror = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, name, "--", "curl", "--connect-timeout", "2", serviceEndpoint).Output()
			if err != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Unable to access the %s", serviceEndpoint))
		o.Expect(servicechkerror).NotTo(o.HaveOccurred())
		o.Expect(servicechkout).Should(o.ContainSubstring("Hello OpenShift"))
		servicedelerror := oc.Run("delete").Args("-n", namespace, "svc", name).Execute()
		o.Expect(servicedelerror).NotTo(o.HaveOccurred())

		exutil.By("6) Create clusterip service without --clusterip option")
		servicecreateout, servicecreateerror = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "service", "clusterip", name, "--tcp", fmt.Sprintf("%v:8080", randomServicePort)).Output()
		o.Expect(servicecreateout).Should(o.ContainSubstring(fmt.Sprintf("service/%v created", name)))
		o.Expect(servicecreateerror).NotTo(o.HaveOccurred())

		exutil.By("7) Check clusterip service running status with allotted IP")
		allottedServiceIP, serviceipgetError := oc.WithoutNamespace().Run("get").Args("service", name, "-o=jsonpath={.spec.clusterIP}", "-n", namespace).Output()
		o.Expect(serviceipgetError).NotTo(o.HaveOccurred())
		if serviceIP.To4() != nil {
			serviceEndpoint = fmt.Sprintf("%v:%v", allottedServiceIP, randomServicePort)
		} else {
			serviceEndpoint = fmt.Sprintf("[%v]:%v", allottedServiceIP, randomServicePort)
		}
		// retry 3 times, sometimes, the endpoint is not ready for accessing.
		err = wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 30*time.Second, false, func(cxt context.Context) (bool, error) {
			servicechkout, servicechkerror = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, name, "--", "curl", "--connect-timeout", "2", serviceEndpoint).Output()
			if err != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Unable to access the %s", serviceEndpoint))
		o.Expect(servicechkerror).NotTo(o.HaveOccurred())
		o.Expect(servicechkout).Should(o.ContainSubstring("Hello OpenShift"))
		servicedelerror = oc.Run("delete").Args("-n", namespace, "svc", name).Execute()
		o.Expect(servicedelerror).NotTo(o.HaveOccurred())

		exutil.By("8) Create clusterip service without '--tcp' option")
		servicecreateout, servicecreateerror = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "service", "clusterip", name).Output()
		o.Expect(servicecreateout).Should(o.ContainSubstring("error: at least one tcp port specifier must be provided"))
		o.Expect(servicecreateerror).To(o.HaveOccurred())

		exutil.By("9) Create clusterip service with '--dry-run' option.")
		servicecreateout, servicecreateerror = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", namespace, "service", "clusterip", name, "--tcp", fmt.Sprintf("%v:8080", randomServicePort), "--dry-run=client").Output()
		o.Expect(servicecreateout).Should(o.ContainSubstring(fmt.Sprintf("service/%v created (dry run)", name)))
		o.Expect(servicecreateerror).NotTo(o.HaveOccurred())
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Low-53693-[Apiserver] Identity and disable APIs not required for MVP", func() {
		exutil.By("1. Check the MVP/recommended apis and status from below.")
		apiResource, apiErr := oc.AsAdmin().WithoutNamespace().Run("api-resources").Args("--loglevel=6").Output()
		o.Expect(apiErr).NotTo(o.HaveOccurred())
		if !strings.Contains(apiResource, "security.openshift.io/v1") {
			e2e.Failf("security.openshift.io/v1 api is missing")
		}
		if !strings.Contains(apiResource, "route.openshift.io/v1") {
			e2e.Failf("route.openshift.io/v1 api is missing")
		}

		exutil.By("2. List out disable apis should not present.")
		disabledApis := []string{"build", "apps", "image", "imageregistry", "config", "user", "operator", "template"}
		for _, i := range disabledApis {
			removedapi := i + ".openshift"
			if strings.Contains(apiResource, removedapi) {
				e2e.Failf("disabled %v api is present not removed from microshift", removedapi)
			}
		}
		exutil.By("3. Check the security context of service-ca component which should be non-root")
		security, securityErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-service-ca", "-o", `jsonpath='{.items[*].spec.containers[0].securityContext}'`).Output()
		o.Expect(securityErr).NotTo(o.HaveOccurred())
		o.Expect(security).Should(o.ContainSubstring(`"runAsNonRoot":true,"runAsUser":`))
		podname, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-service-ca", "-o", `jsonpath={.items[*].metadata.name}`).Output()
		o.Expect(podErr).NotTo(o.HaveOccurred())
		execPod, execErr := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-service-ca", podname, "--", "/bin/sh", "-c", `id`).Output()
		o.Expect(execErr).NotTo(o.HaveOccurred())
		secUser, securityErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-service-ca", "-o", `jsonpath='{.items[*].spec.containers[0].securityContext.runAsUser}'`).Output()
		o.Expect(securityErr).NotTo(o.HaveOccurred())
		secUser = strings.Trim(string(secUser), "'")
		o.Expect(execPod).Should(o.ContainSubstring(fmt.Sprintf("uid=%s(%s) gid=0(root) groups=0(root),%s", secUser, secUser, secUser)))

		exutil.By("4. check removal for kube-proxy.")
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		cmd := `iptables-save | grep -iE 'proxy|kube-proxy' || true`
		proxy, proxyErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=default"}, "bash", "-c", cmd)
		o.Expect(proxyErr).NotTo(o.HaveOccurred())
		proxy = regexp.MustCompile(`\n`).ReplaceAllString(string(proxy), "")
		o.Expect(proxy).Should(o.BeEmpty())

		exutil.By("5. check oauth endpoint Curl oauth server url, it should not present.")
		consoleurl, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-console").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(consoleurl).ShouldNot(o.BeEmpty())
		cmd = fmt.Sprintf(`curl -k %v/.well-known/oauth-authorization-server`, consoleurl)
		cmdOut, cmdErr := exec.Command("bash", "-c", cmd).Output()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		o.Expect(cmdOut).Should(o.ContainSubstring("404"))

		exutil.By("Apirequestcount is disabled in microshift.")
		apierr := oc.AsAdmin().WithoutNamespace().Run("get").Args("apirequestcount").Execute()
		if apierr == nil {
			e2e.Failf("Apirequestcount has not disabled")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Medium-54816-[Apiserver] Remove RoleBindingRestrictions API", func() {
		exutil.By("1. Roles bindings restrictions should not work in microshift.")
		roleOutput, roleErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("rolebinding.rbac").Output()
		o.Expect(roleErr).NotTo(o.HaveOccurred())
		o.Expect(roleOutput).ShouldNot(o.BeEmpty())
		roleErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("rolebindingrestriction", "-A").Execute()
		o.Expect(roleErr).To(o.HaveOccurred())

		exutil.By("2. Check the removal of the RoleBindingRestrictions API endpoint by checking the OpenShift API endpoint documentation or running:")
		roleOutput, roleErr = oc.AsAdmin().WithoutNamespace().Run("api-resources").Args().Output()
		o.Expect(roleErr).NotTo(o.HaveOccurred())
		o.Expect(roleOutput).ShouldNot(o.ContainSubstring("RoleBindingRestrictions"))

		exutil.By("3. Create a ClusterRole")
		clusterroleyaml := tmpdir + "/clusterroleyaml"
		clusterroleCMD := fmt.Sprintf(`cat > %v << EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: example-cluster-role-ocp54816
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "watch", "list"]`, clusterroleyaml)
		_, clusterrolecmdErr := exec.Command("bash", "-c", clusterroleCMD).Output()
		o.Expect(clusterrolecmdErr).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete", "-f", clusterroleyaml).Args().Execute()
		clusterroleErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", clusterroleyaml).Args().Execute()
		o.Expect(clusterroleErr).NotTo(o.HaveOccurred())

		exutil.By("4. Created a ClusterRoleBinding to bind the ClusterRole to a service account")
		clusterrolebindingyaml := tmpdir + "/clusterrolebindingyaml"
		clusterrolebindingCMD := fmt.Sprintf(`cat > %v << EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: example-cluster-role-binding-ocp54816
subjects:
- kind: ServiceAccount
  name: example-service-account-ocp54816
  namespace: example-namespace-ocp54816
roleRef:
  kind: ClusterRole
  name: example-cluster-role-ocp54816
  apiGroup: rbac.authorization.k8s.io`, clusterrolebindingyaml)
		_, clusterrolebindingcmdErr := exec.Command("bash", "-c", clusterrolebindingCMD).Output()
		o.Expect(clusterrolebindingcmdErr).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete", "-f", clusterrolebindingyaml).Args().Execute()
		clusterrolebindingErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", clusterrolebindingyaml).Args().Execute()
		o.Expect(clusterrolebindingErr).NotTo(o.HaveOccurred())

		exutil.By("5. Test the ClusterRole and ClusterRoleBinding by using oc to impersonate the service account and check access to pod")
		saOutput, saErr := oc.AsAdmin().WithoutNamespace().Run("auth").Args("can-i", "get", "pods", "--as=system:serviceaccount:example-namespace-ocp54816:example-service-account-ocp54816").Output()
		o.Expect(saErr).NotTo(o.HaveOccurred())
		o.Expect(saOutput).Should(o.ContainSubstring("yes"))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Medium-53972-[Apiserver] Cluster Policy Controller integration", func() {
		namespace := "tmpocp53792"
		caseID := "ocp-53972"

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", namespace, "--ignore-not-found").Execute()

		exutil.By("1.Create temporary namespace")
		namespaceOutput, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("namespace", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(namespaceOutput).Should(o.ContainSubstring("namespace/"+namespace+" created"), namespace+" not created..")

		// There are events in the openshift-kube-controller-manager that show the creation of these SCC labels
		exutil.By("2.Check if the events in the ns openshift-kube-controller-manager show the creation of these SCC labels, e.g., tempocp53792")
		scErr := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 200*time.Second, false, func(cxt context.Context) (bool, error) {
			eventsOutput := getResourceToBeReady(oc, asAdmin, withoutNamespace, "events", "-n", "openshift-kube-controller-manager")
			if strings.Contains(eventsOutput, "CreatedSCCRanges") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(scErr, "Not created SCC ranges for namespaces...")

		// When a namespace is created, the cluster policy controller is in charge of adding SCC labels.
		exutil.By("3.Check the scc annotations should have openshift.io to verify that the namespace added scc annotations Cluster Policy Controller integration")
		namespaceOutput = getResourceToBeReady(oc, asAdmin, withoutNamespace, "ns", namespace, `-o=jsonpath={.metadata.annotations}`)
		o.Expect(namespaceOutput).Should(o.ContainSubstring("openshift.io/sa.scc"), "Not have openshift.io Scc annotations")

		// Cluster policy controller does take care of the resourceQuota, verifying that the quota feature works properly after Cluster Policy Controller integration
		exutil.By("4.Create quota to verify that the quota feature works properly after Cluster Policy Controller integration")
		template := getTestDataFilePath("ocp9853-quota.yaml")
		templateErr := oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(templateErr).NotTo(o.HaveOccurred())

		exutil.By("5.Create multiple secrets to test created ResourceQuota, expect failure for secrets creations that exceed quota limit")
		secretCount, err := oc.Run("get").Args("-n", namespace, "resourcequota", "myquota", "-o", `jsonpath={.status.namespaces[*].status.used.secrets}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		usedCount, _ := strconv.Atoi(secretCount)
		limits := 15
		for i := usedCount; i <= limits; i++ {
			secretName := fmt.Sprintf("%v-secret-%d", caseID, i)
			output, err := oc.Run("create").Args("-n", namespace, "secret", "generic", secretName).Output()
			exutil.By(fmt.Sprintf("5.%d) creating secret %s", i, secretName))
			if i < limits {
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				o.Expect(output).To(o.MatchRegexp("secrets.*forbidden: exceeded quota"))
			}
		}

		exutil.By("6. Create multiple Configmaps to test created ResourceQuota, expect failure for configmap creations that exceed quota limit")
		configmapCount, cmerr := oc.Run("get").Args("-n", namespace, "resourcequota", "myquota", "-o", `jsonpath={.status.namespaces[*].status.used.configmaps}`).Output()
		o.Expect(cmerr).NotTo(o.HaveOccurred())
		usedcmCount, _ := strconv.Atoi(configmapCount)
		limits = 13
		for i := usedcmCount; i <= limits; i++ {
			cmName := fmt.Sprintf("%v-cm-%d", caseID, i)
			output, err := oc.Run("create").Args("-n", namespace, "cm", cmName).Output()
			exutil.By(fmt.Sprintf("6.%d) creating configmaps %s", i, cmName))
			if i < limits {
				o.Expect(err).NotTo(o.HaveOccurred())
			} else {
				o.Expect(output).To(o.MatchRegexp("configmaps.*forbidden: exceeded quota"))
			}
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftBoth-ConnectedOnly-Medium-55394-[Apiserver] MicroShift enable SCC admission for pods", func() {
		namespace := "test-scc-ocp55394"
		testpod := "security-context-demo-ocp55394"
		testpod2 := "security-context-demo-2-ocp55394"

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", namespace, "--ignore-not-found").Execute()

		exutil.By("1.Create temporary namespace")
		namespaceOutput, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("namespace", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(namespaceOutput).Should(o.ContainSubstring("namespace/"+namespace+" created"), namespace+" not created..")

		// Set the security context for a Pod which set the user which is running with uid=1000 gid=3000 groups=200 and processes are running as user 1000, which is the value of runAsUser
		exutil.By("2. Create one pod " + testpod + " with the specified security context.")
		template := getTestDataFilePath("microshift-pods-security-context.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f", template, "-n", namespace).Execute()
		templateErr := oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(templateErr).NotTo(o.HaveOccurred())

		exutil.By("3. Verify that the Pod's " + testpod + " Container is running")
		exutil.AssertPodToBeReady(oc, testpod, namespace)

		// Get a shell to the running Container, check output shows that the processes are running as user 1000, which is the value of runAsUser
		exutil.By("4.1 Verify if the processes are running with the specified user ID 1000 in the pod.")
		execCmdOuptut := ExecCommandOnPod(oc, testpod, namespace, "ps")
		if match, _ := regexp.MatchString(`1000.*sleep 1h`, execCmdOuptut); match {
			e2e.Logf("Processes are running on pod %v with user id 1000 :: %v", testpod, execCmdOuptut)
		} else {
			e2e.Failf("Not able find the processes which are running on pod %v as user 1000 :: %v", testpod, execCmdOuptut)
		}

		// Get a shell to the running Container, check output should shows that user which is running with uid=1000 gid=3000 groups=2000
		exutil.By("4.2 Verify that user is running with specified uid=1000 gid=3000 groups=2000")
		execCmdOuptut = ExecCommandOnPod(oc, testpod, namespace, "id")
		if match, _ := regexp.MatchString(`uid=1000.*gid=3000.*groups=2000`, execCmdOuptut); match {
			e2e.Logf("On pod %v User running with :: %v", testpod, execCmdOuptut)
		} else {
			e2e.Failf("Not able find the user which is running on pod %v with uid=1000 gid=3000 groups=2000 :: %v", testpod, execCmdOuptut)
		}

		// Set the security context again for a Pod which override the user value which is running with uid=1000 to 2000, which is the new value of runAsUser
		exutil.By("5. Create one pod " + testpod2 + " with the specified security context.")
		template = getTestDataFilePath("microshift-pods-security-context2.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f", template, "-n", namespace).Execute()
		templateErr = oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(templateErr).NotTo(o.HaveOccurred())

		exutil.By("6. Verify that the Pod's " + testpod2 + " Container is running")
		exutil.AssertPodToBeReady(oc, testpod2, namespace)

		// Get a shell to the running Container, check output should shows that the processes are running as user 2000, which is the value of runAsUser
		exutil.By("7. Verify that processes are running with the specified user ID 2000 in the pod.")
		execCmdOuptut = ExecCommandOnPod(oc, testpod2, namespace, "ps aux")
		if match, _ := regexp.MatchString(`2000.*ps aux`, execCmdOuptut); match {
			e2e.Logf("Processes are running on pod %v with user id 2000 :: %v", testpod2, execCmdOuptut)
		} else {
			e2e.Failf("Not able find the processes which are running on pod %v as user 2000 :: %v", testpod2, execCmdOuptut)
		}

		// Adding this for https://issues.redhat.com/browse/USHIFT-1384 in >= 4.14
		exutil.By("8. Ensure that pods in different namespaces are launched with different UIDs.")
		namespace1 := "testpod-namespace-1"
		namespace2 := "testpod-namespace-2"

		defer func() {
			oc.DeleteSpecifiedNamespaceAsAdmin(namespace1)
			oc.DeleteSpecifiedNamespaceAsAdmin(namespace2)
		}()

		exutil.By("8.1 Create two different namespaces.")
		oc.CreateSpecifiedNamespaceAsAdmin(namespace1)
		oc.CreateSpecifiedNamespaceAsAdmin(namespace2)

		template = getTestDataFilePath("pod-for-ping.json")

		exutil.By("8.2 Create pods in both namespaces")
		createPod := func(namespace string) string {
			createPodErr := oc.Run("create").Args("-f", template, "-n", namespace).Execute()
			o.Expect(createPodErr).NotTo(o.HaveOccurred())
			podName := getPodsList(oc.AsAdmin(), namespace)
			exutil.AssertPodToBeReady(oc, podName[0], namespace)
			return getResourceToBeReady(oc, asAdmin, withoutNamespace, "pod", "-n", namespace, `-o=jsonpath='{range .items[*]}{@.metadata.name}{" runAsUser: "}{@.spec.containers[*].securityContext.runAsUser}{" fsGroup: "}{@.spec.securityContext.fsGroup}{" seLinuxOptions: "}{@.spec.securityContext.seLinuxOptions.level}{"\n"}{end}'`)
		}

		exutil.By("8.3 Verify pods should have different UID's and desc in both namespaces.")
		// Original pod descriptions
		podDesc1 := createPod(namespace1)
		podDesc2 := createPod(namespace2)

		// Split the descriptions into lines
		lines1 := strings.Split(podDesc1, "\n")
		lines2 := strings.Split(podDesc2, "\n")

		// Initialize a flag to check for differences
		differencesFound := false

		// Iterate through each line and compare the specified fields
		for i := 0; i < len(lines1) && i < len(lines2); i++ {
			fields1 := strings.Fields(lines1[i])
			fields2 := strings.Fields(lines2[i])

			if len(fields1) != len(fields2) {
				e2e.Failf("Number of fields in line %d differ: %d != %d", i+1, len(fields1), len(fields2))
			}

			// Check if fields1 has enough elements before extracting values
			if len(fields1) > 2 {
				// Extract the values of interest
				runAsUser1 := fields1[2]      // Assuming runAsUser is the 3rd field
				fsGroup1 := fields1[4]        // Assuming fsGroup is the 5th field
				seLinuxOptions1 := fields1[6] // Assuming seLinuxOptions is the 7th field

				runAsUser2 := fields2[2]
				fsGroup2 := fields2[4]
				seLinuxOptions2 := fields2[6]

				// Compare the values
				if runAsUser1 != runAsUser2 && fsGroup1 != fsGroup2 && seLinuxOptions1 != seLinuxOptions2 {
					e2e.Logf("Line %d: runAsUser does not match: '%s' != '%s'", i+1, runAsUser1, runAsUser2)
					e2e.Logf("Line %d: fsGroup does not match: '%s' != '%s'", i+1, fsGroup1, fsGroup2)
					e2e.Logf("Line %d: seLinuxOptions do not match: '%s' != '%s'", i+1, seLinuxOptions1, seLinuxOptions2)
					differencesFound = true
				} else {
					e2e.Logf("Line %d: runAsUser does match: '%s' != '%s'", i+1, runAsUser1, runAsUser2)
					e2e.Logf("Line %d: fsGroup does match: '%s' != '%s'", i+1, fsGroup1, fsGroup2)
					e2e.Logf("Line %d: seLinuxOptions does match: '%s' != '%s'", i+1, seLinuxOptions1, seLinuxOptions2)
					differencesFound = false
					break
				}
			}
		}
		// Check if any differences were found and pass or fail accordingly
		if differencesFound {
			e2e.Logf("Both pods in different namespaces have different UIDs and SCC descriptions\n")
		} else {
			e2e.Failf("Both pods in different namespaces don't have different UIDs and SCC descriptions\n")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-NonHyperShiftHOST-MicroShiftBoth-Medium-55480-[Apiserver] Audit logs must be stored and persisted", func() {
		exutil.By("1. Debug node and check the KAS audit log.")
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		o.Expect(masterNode).ShouldNot(o.BeEmpty())
		kaslogfile := "ls -l /var/log/kube-apiserver/audit.log"
		masterDebugNode, debugNodeErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=default"}, "bash", "-c", kaslogfile)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
		o.Expect(masterDebugNode).ShouldNot(o.BeEmpty())
		parts := strings.Fields(masterDebugNode)
		permissions := parts[0]
		owner := parts[2]
		group := parts[3]

		if strings.HasPrefix(permissions, "-rw") && strings.Contains(owner, "root") && strings.Contains(group, "root") {
			e2e.Logf("Test Passed: The file has read & write permissions, owner and group owner is root :: %v", masterDebugNode)
		} else {
			e2e.Failf("Test Failed : The file does not have required permissions")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftBoth-Medium-55677-[Apiserver] MicroShift enable CRDs validation", func() {
		namespace := "test-scc-ocp55677"
		crontab := "my-new-cron-object-ocp55677"
		crontabNew := "my-new-cron-object-ocp55677-2"

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", namespace, "--ignore-not-found").Execute()

		exutil.By("1.Create temporary namespace")
		namespaceOutput, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("namespace", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(namespaceOutput).Should(o.ContainSubstring("namespace/"+namespace+" created"), namespace+" not created..")

		exutil.By("2. Create a CustomResourceDefinition")
		template := getTestDataFilePath("ocp55677-crd.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f", template, "-n", namespace).Execute()
		templateErr := oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(templateErr).NotTo(o.HaveOccurred())

		exutil.By("3. Create custom crontab " + crontab + " object")
		mycrontabyaml := tmpdir + "/my-ocp55677-crontab.yaml"
		mycrontabCMD := fmt.Sprintf(`cat > %v << EOF
apiVersion: "ms.qe.com/v1"
kind: CronTab
metadata:
  name: %v
  namespace: %v
spec:
  cronSpec: "* * * * */5"
  image: my-awesome-cron-image`, mycrontabyaml, crontab, namespace)
		_, myCrontabCmdErr := exec.Command("bash", "-c", mycrontabCMD).Output()
		o.Expect(myCrontabCmdErr).NotTo(o.HaveOccurred())
		mycrontabErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", mycrontabyaml).Args().Execute()
		o.Expect(mycrontabErr).NotTo(o.HaveOccurred())

		exutil.By("4. Check the created crontab :: " + crontab)
		crontabOutput, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("crontab", "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(crontabOutput).Should(o.ContainSubstring(crontab), crontab+" not created..")

		exutil.By("5. Create new custom " + crontabNew + " object with unknown field.")
		mycrontabPruneYaml := tmpdir + "/my-ocp55677-prune-crontab.yaml"
		mycrontabPruneCMD := fmt.Sprintf(`cat > %v << EOF
apiVersion: "ms.qe.com/v1"
kind: CronTab
metadata:
  name: %v
  namespace: %v
spec:
  cronSpec: "* * * * */5"
  image: my-awesome-cron-image
  someRandomField: 42`, mycrontabPruneYaml, crontabNew, namespace)
		_, myCrontabPruneCmdErr := exec.Command("bash", "-c", mycrontabPruneCMD).Output()
		o.Expect(myCrontabPruneCmdErr).NotTo(o.HaveOccurred())
		mycrontabNewErr := oc.AsAdmin().WithoutNamespace().Run("create", "--validate=false", "-f", mycrontabPruneYaml).Args().Execute()
		o.Expect(mycrontabNewErr).NotTo(o.HaveOccurred())

		exutil.By("5. Check the unknown field pruning in " + crontabNew + " crontab object")
		crontabOutput, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("crontab", crontabNew, "-n", namespace, "-o", `jsonpath={.spec}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(crontabOutput, "someRandomField") {
			e2e.Logf("Test case Passed :: Unknown field is pruned crontab object\n :: %v", crontabOutput)
		} else {
			e2e.Logf("Test case Failed:: Unknown field is not pruned in crontab object\n :: %v", crontabOutput)
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-ConnectedOnly-High-56229-[Apiserver] Complete Route API compatibility", func() {
		namespace := "test-ocp56229"
		routeName := "hello-microshift-ocp56229"

		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("ns", namespace, "--ignore-not-found").Execute()
		exutil.By("1.Create temporary namespace")
		namespaceOutput, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("namespace", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(namespaceOutput).Should(o.ContainSubstring("namespace/"+namespace+" created"), "Failed to create namesapce "+namespace)

		exutil.By("2. Create a Route without spec.host and spec.to.kind")
		routeYaml := tmpdir + "/hellomicroshift-56229.yaml"
		routetmpYaml := `apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-microshift-ocp56229
  namespace: test-ocp56229
spec:
 to:
  kind: ""
  name: hello-microshift-ocp56229`
		f, err := os.Create(routeYaml)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", routetmpYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().WithoutNamespace().Run("delete", "-f", routeYaml).Args().Execute()
		routeErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", routeYaml).Args().Execute()
		o.Expect(routeErr).NotTo(o.HaveOccurred())
		var routeJsonOutput string
		var routeType string
		routeTypeErr := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 90*time.Second, false, func(cxt context.Context) (bool, error) {
			routeOutput, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", namespace, "-o", "json").Output()
			routeJsonOutput = gjson.Parse(routeOutput).String()
			routeType = gjson.Get(routeJsonOutput, `status.ingress.0.conditions.0.type`).String()
			if strings.Contains(routeType, "Admitted") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(routeTypeErr, routeName+" type is not set Admitted")

		routeStatus := gjson.Get(routeJsonOutput, `status.ingress.0.conditions.0.status`).String()
		o.Expect(routeStatus).Should(o.ContainSubstring("True"), routeStatus+" status is not set True")
		routeHost := gjson.Get(routeJsonOutput, `spec.host`).String()
		o.Expect(routeHost).Should(o.ContainSubstring(routeName+"-"+namespace+".apps.example.com"), routeName+" host is not set with default hostname")
		routeKind := gjson.Get(routeJsonOutput, `spec.to.kind`).String()
		o.Expect(routeKind).Should(o.ContainSubstring("Service"), routeName+" kind type is not set with default value :: Service")
		routeWildCardPolicy := gjson.Get(routeJsonOutput, `status.ingress.0.wildcardPolicy`).String()
		o.Expect(routeWildCardPolicy).Should(o.ContainSubstring("None"), routeName+" wildcardpolicy is not set with default value :: None")
		routeWeight := gjson.Get(routeJsonOutput, `spec.to.weight`).String()
		o.Expect(routeWeight).Should(o.ContainSubstring("100"), routeName+" weight is not set with default value :: 100")
		e2e.Logf("Route %v created with default host %v and route kind type :: %v, type :: %v with status :: %v and wildcardpolicy :: %v and weight :: %v", routeName, routeHost, routeKind, routeType, routeStatus, routeWildCardPolicy, routeWeight)

		exutil.By("3.Check spec.wildcardPolicy can't be change")
		routewildpolicyYaml := tmpdir + "/hellomicroshift-56229-wildcard.yaml"
		routewildpolicytmpYaml := `apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-microshift-ocp56229
  namespace: test-ocp56229
spec:
 to:
   kind: Service
   name: hello-microshift-ocp56229
 wildcardPolicy: "Subdomain"`
		f, err = os.Create(routewildpolicyYaml)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w = bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", routewildpolicytmpYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		routewildpolicyErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", routewildpolicyYaml, "--server-side", "--force-conflicts").Args().Execute()
		o.Expect(routewildpolicyErr).To(o.HaveOccurred())

		exutil.By("4.Check weight policy can be changed")
		routeWeightYaml := "/tmp/hellomicroshift-56229-weight.yaml"
		routeWeighttmpYaml := `apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-microshift-ocp56229
  namespace: test-ocp56229
spec:
  to:
   kind: Service
   name: hello-microshift-ocp56229
   weight: 10`
		f, err = os.Create(routeWeightYaml)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w = bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", routeWeighttmpYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		routeweightErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", routeWeightYaml, "--server-side", "--force-conflicts").Args().Execute()
		o.Expect(routeweightErr).NotTo(o.HaveOccurred())
		routeWeight, routeErr = oc.AsAdmin().WithoutNamespace().Run("get").Args("route", routeName, "-n", namespace, `-o=jsonpath={.spec.to.weight}`).Output()
		o.Expect(routeErr).NotTo(o.HaveOccurred())
		o.Expect(routeWeight).Should(o.ContainSubstring("10"), routeName+" weight is not set with default value :: 10")

		exutil.By("5. Create ingresss routes")
		template := getTestDataFilePath("ocp-56229-ingress.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f", template, "-n", namespace).Execute()
		templateErr := oc.AsAdmin().Run("create").Args("-f", template, "-n", namespace).Execute()
		o.Expect(templateErr).NotTo(o.HaveOccurred())
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-High-55728-[Apiserver] Clients (including internal clients) must not use an unready Kubernetes apiserver [Disruptive]", func() {
		// set the varaibles
		var (
			caseID           = "55728"
			e2eTestNamespace = "e2e-ushift-apiserver-" + caseID + "-" + exutil.GetRandomString()
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get the clustername")
		clusterName, clusterErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-o", `jsonpath={.clusters[0].name}`).Output()
		o.Expect(clusterErr).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster Name :: %v", clusterName)

		exutil.By("3. Point to the API server referring the cluster name")
		apiserverName, apiErr := oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "-o", `jsonpath={.clusters[?(@.name=="`+clusterName+`")].cluster.server}`).Output()
		o.Expect(apiErr).NotTo(o.HaveOccurred())
		e2e.Logf("Server Name :: %v", apiserverName)

		exutil.By("4. Create a secret to hold a token for the default service account.")
		saSecretYaml := tmpdir + "/sa-secret-ocp55728.yaml"
		saSecrettmpYaml := `apiVersion: v1
kind: Secret
metadata:
  name: default-token-ocp55728
  annotations:
    kubernetes.io/service-account.name: default
type: kubernetes.io/service-account-token`
		f, err := os.Create(saSecretYaml)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()
		w := bufio.NewWriter(f)
		_, err = fmt.Fprintf(w, "%s", saSecrettmpYaml)
		w.Flush()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().WithoutNamespace().Run("delete", "-f", saSecretYaml, "-n", e2eTestNamespace).Args().Execute()
		saSecretErr := oc.AsAdmin().WithoutNamespace().Run("apply", "-f", saSecretYaml, "-n", e2eTestNamespace).Args().Execute()
		o.Expect(saSecretErr).NotTo(o.HaveOccurred())

		exutil.By("4. Get the token value")
		token, tokenerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/default-token-ocp55728", "-n", e2eTestNamespace, "-o", `jsonpath={.data.token}`).Output()
		o.Expect(tokenerr).NotTo(o.HaveOccurred())
		tokenValue, tokenValueErr := base64.StdEncoding.DecodeString(token)
		o.Expect(tokenValueErr).NotTo(o.HaveOccurred())
		o.Expect(tokenValue).ShouldNot(o.BeEmpty())

		exutil.By("5. Restart master node")
		masterNode, masterErr := exutil.GetFirstMasterNode(oc)
		o.Expect(masterErr).NotTo(o.HaveOccurred())
		o.Expect(masterNode).ShouldNot(o.BeEmpty())
		defer clusterNodesHealthcheck(oc, 600, tmpdir+"/ocp55728")
		_, rebooterr := exutil.DebugNodeWithChroot(oc, masterNode, "shutdown", "-r", "+1", "-t", "30")
		o.Expect(rebooterr).NotTo(o.HaveOccurred())

		exutil.By("6. Check apiserver readiness msg")
		apiserverRetrymsg := apiserverReadinessProbe(string(tokenValue), apiserverName)
		o.Expect(apiserverRetrymsg).ShouldNot(o.BeEmpty())
		e2e.Logf("Get retry msg from apiserver during master node restart :: %v", apiserverRetrymsg)
		nodeHealthErr := clusterNodesHealthcheck(oc, 600, tmpdir+"/ocp55728")
		if nodeHealthErr != nil {
			e2e.Failf("Cluster nodes health check failed")
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Medium-54786-[logging] component name presents in klog headers", func() {

		var (
			e2eTestNamespace = "microshift-ocp54786"
			components       = []string{"kube-controller-manager", "kubelet", "kube-apiserver"}
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		exutil.By("3. Checking component name presents in klog headers")
		for _, comps := range components {
			script := `journalctl -u microshift.service|grep -i "microshift.*: ` + comps + `"|tail -1`
			masterNodeLogs, checkLogErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNodes[0], []string{"--quiet=true", "--to-namespace=" + e2eTestNamespace}, "bash", "-c", script)
			o.Expect(checkLogErr).NotTo(o.HaveOccurred())
			count := len(strings.TrimSpace(masterNodeLogs))
			if count > 0 {
				e2e.Logf("Component name presents in klog headers for :: %v :: %v", comps, masterNodeLogs)
			} else {
				e2e.Failf("Component name not presents in klog headers for :: %v :: %v", comps, masterNodeLogs)
			}
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Medium-62959-[Apiserver] Remove search logic for configuration file [Disruptive]", func() {
		var (
			e2eTestNamespace = "microshift-ocp62959"
			chkConfigCmd     = `sudo /usr/bin/microshift show-config --mode effective 2>/dev/null | grep -i memoryLimitMB`
			valCfg           = "180"
			etcConfigYaml    = "/etc/microshift/config.yaml"
			etcConfigYamlbak = "/etc/microshift/config.yaml.bak"
			user             = "redhat"
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift vm")
		fqdnName := getMicroshiftHostname(oc)

		defer func() {
			etcdConfigCMD := fmt.Sprintf(`'
configfile=%v
configfilebak=%v
if [ -f $configfilebak ]; then
	cp $configfilebak $configfile
	rm -f $configfilebak 
else 
	rm -f $configfile 
fi'`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcdConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		defer func() {
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo rm -f ~/.microshift/config.yaml")
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("3. Check default config values for etcd")
		mchkConfigdefault, mchkConfigErr := runSSHCommand(fqdnName, user, chkConfigCmd)
		o.Expect(mchkConfigErr).NotTo(o.HaveOccurred())

		exutil.By("4. Configure the memoryLimitMB field in user config path")
		configDir := "~/.microshift"
		configFile := "config.yaml"
		etcdConfigCMD := fmt.Sprintf(`"mkdir -p %v && touch %v/%v && cat > %v/%v << EOF
etcd:
  memoryLimitMB: %v
EOF"`, configDir, configDir, configFile, configDir, configFile, valCfg)
		_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcdConfigCMD)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("5. Check config values for etcd should not change from default values")
		mchkConfig, mchkConfigErr := runSSHCommand(fqdnName, user, chkConfigCmd)
		o.Expect(mchkConfigErr).NotTo(o.HaveOccurred())
		o.Expect(mchkConfig).Should(o.ContainSubstring(mchkConfigdefault))

		exutil.By("6. Configure the memoryLimitMB field in default config path")
		etcdConfigCMD = fmt.Sprintf(`'
configfile=%v
configfilebak=%v
if [ -f $configfile ]; then
	cp $configfile $configfilebak
fi
cat > $configfile << EOF
etcd:
  memoryLimitMB: %v
EOF'`, etcConfigYaml, etcConfigYamlbak, valCfg)
		_, mchgConfigErr = runSSHCommand(fqdnName, user, "sudo bash -c", etcdConfigCMD)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("7. Check config values for etcd should change from default values")
		mchkConfig, mchkConfigErr = runSSHCommand(fqdnName, user, chkConfigCmd)
		o.Expect(mchkConfigErr).NotTo(o.HaveOccurred())
		o.Expect(mchkConfig).Should(o.ContainSubstring(`memoryLimitMB: ` + valCfg))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Medium-62987-[Apiserver] Remove search logic for data directory [Disruptive]", func() {
		var (
			e2eTestNamespace           = "microshift-ocp62987"
			userDataDir                = `/root/.microshift/data/`
			chkContentUserDatadirCmd   = `sudo ls ` + userDataDir
			globalDataDir              = `/var/lib/microshift/`
			chkContentGlobalDatadirCmd = `sudo ls ` + globalDataDir + `resources/`
			userDataDirCmd             = `sudo mkdir -p ` + userDataDir
			globalDataDirDelCmd        = `sudo find ` + globalDataDir + `resources/ -mindepth 1 -delete`
			userDataDirDelCmd          = `sudo rm -rf ` + userDataDir + `*`
			user                       = "redhat"
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift vm")
		fqdnName := getMicroshiftHostname(oc)

		exutil.By("3. Check user data dir")
		// Check if the directory exists
		chkUserDatadirCmd := fmt.Sprintf(`"if [ -d %v ]; then echo 'Directory exists'; else echo 'Directory does not exist'; fi"`, userDataDir)
		checkDirOutput, checkDirErr := runSSHCommand(fqdnName, user, "sudo bash -c", chkUserDatadirCmd)
		o.Expect(checkDirErr).NotTo(o.HaveOccurred())

		if strings.Contains(checkDirOutput, "Directory exists") {
			// Directory exists, so delete the contents
			_, deldirerr := runSSHCommand(fqdnName, user, userDataDirDelCmd)
			o.Expect(deldirerr).NotTo(o.HaveOccurred())
			chkContentOutput, chkContentErr := runSSHCommand(fqdnName, user, chkContentUserDatadirCmd)
			o.Expect(chkContentErr).NotTo(o.HaveOccurred())
			o.Expect(strings.TrimSpace(chkContentOutput)).To(o.BeEmpty())
		} else {
			// Directory does not exist, so create it
			_, mkdirerr := runSSHCommand(fqdnName, user, userDataDirCmd)
			o.Expect(mkdirerr).NotTo(o.HaveOccurred())
		}

		exutil.By("4. Check global data dir")
		// Check if the directory exists
		chkUserGlobalDatadirCmd := fmt.Sprintf(`"if [ -d %v ]; then echo 'Directory exists'; else echo 'Directory does not exist'; fi"`, globalDataDir)
		checkGlobalDirOutput, checkGlobalDirErr := runSSHCommand(fqdnName, user, "sudo bash -c", chkUserGlobalDatadirCmd)
		o.Expect(checkGlobalDirErr).NotTo(o.HaveOccurred())

		if !strings.Contains(checkGlobalDirOutput, "Directory exists") {
			e2e.Failf("Globaldatadir %v should exist :: %v", globalDataDir, checkGlobalDirOutput)
		}

		// Directory exists, so delete the contents it can restore automatically after restart
		_, deldirerr := runSSHCommand(fqdnName, user, globalDataDirDelCmd)
		o.Expect(deldirerr).NotTo(o.HaveOccurred())
		chkContentOutput, chkContentErr := runSSHCommand(fqdnName, user, chkContentGlobalDatadirCmd)
		o.Expect(chkContentErr).NotTo(o.HaveOccurred())
		o.Expect(strings.TrimSpace(chkContentOutput)).To(o.BeEmpty())

		exutil.By("5. Restart Microshift")
		restartErr := restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("6. Ensure that userdatadir is empty and globaldatadir is restored after Microshift is restarted")
		chkContentOutput, chkContentErr = runSSHCommand(fqdnName, user, chkContentUserDatadirCmd)
		o.Expect(chkContentErr).NotTo(o.HaveOccurred())
		o.Expect(strings.TrimSpace(chkContentOutput)).To(o.BeEmpty())
		e2e.Logf("Userdatadir %v be empty.", userDataDir)
		getlogErr := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 200*time.Second, false, func(cxt context.Context) (bool, error) {
			chkContentOutput, chkContentErr = runSSHCommand(fqdnName, user, chkContentGlobalDatadirCmd)
			if chkContentErr == nil && strings.TrimSpace(chkContentOutput) != "" {
				e2e.Logf("Globaldatadir %v not empty, it is restored :: %v", globalDataDir, chkContentOutput)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(getlogErr, fmt.Sprintf("Globaldatadir %v empty, it is not restored :: %v", globalDataDir, chkContentOutput))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Longduration-NonPreRelease-Medium-63099-make logging config more resilient [Disruptive][Slow]", func() {
		var (
			e2eTestNamespace = "microshift-ocp63099"
			etcConfigYaml    = "/etc/microshift/config.yaml"
			etcConfigYamlbak = "/etc/microshift/config.yaml.bak"
			user             = "redhat"
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift vm")
		fqdnName := getMicroshiftHostname(oc)

		defer func() {
			etcConfigCMD := fmt.Sprintf(`'configfile=%v;
			configfilebak=%v;
			if [ -f $configfilebak ]; then
				cp $configfilebak $configfile; 
				rm -f $configfilebak;
			else
				rm -f $configfile;
			fi'`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("3. Take backup of config file")
		etcConfigCMD := fmt.Sprintf(`'configfile=%v; configfilebak=%v; if [ -f $configfile ]; then cp $configfile $configfilebak; fi'`, etcConfigYaml, etcConfigYamlbak)
		_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		logLevels := []string{"Normal", "normal", "NORMAL", "debug", "DEBUG", "Trace", "trace", "TRACE", "TraceAll", "traceall", "TRACEALL"}
		for stepn, level := range logLevels {
			exutil.By(fmt.Sprintf("%v.1 Configure the logLevel %v in default config path", stepn+4, level))
			etcConfigCMD = fmt.Sprintf(`'
configfile=%v
cat > $configfile << EOF
debugging:
  logLevel: %v
EOF'`, etcConfigYaml, level)

			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

			unixTimestamp := time.Now().Unix()
			exutil.By(fmt.Sprintf("%v.2 Restart Microshift", stepn+4))
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%v.3 Check logLevel should change to %v", stepn+4, level))
			chkConfigCmd := fmt.Sprintf(`sudo journalctl -u microshift -b -S @%vs | grep "logLevel: %v"|grep -iv journalctl|tail -1`, unixTimestamp, level)
			getlogErr := wait.PollUntilContextTimeout(context.Background(), 15*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
				mchkConfig, mchkConfigErr := runSSHCommand(fqdnName, user, chkConfigCmd)
				if mchkConfigErr == nil && strings.Contains(mchkConfig, "logLevel: "+level) {
					e2e.Logf("LogLevel changed to %v :: %v", level, mchkConfig)
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(getlogErr, fmt.Sprintf("LogLevel not changed to %v :: %v", level, mchgConfigErr))
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Longduration-NonPreRelease-Medium-63217-[Apiserver] configurable manifest sources [Disruptive][Slow]", func() {
		var (
			e2eTestNamespace = "microshift-ocp63217"
			etcConfigYaml    = "/etc/microshift/config.yaml"
			etcConfigYamlbak = "/etc/microshift/config.yaml.bak"
			tmpManifestPath  = "/var/lib/microshift/manifests/manifestocp63217/"
			chkConfigCmd     = `sudo /usr/bin/microshift show-config --mode effective 2>/dev/null`
			user             = "redhat"
		)

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift vm")
		fqdnName := getMicroshiftHostname(oc)

		defer func() {
			baseName := "busybox-scenario"
			numbers := []int{1, 2, 3}
			for _, number := range numbers {
				ns := fmt.Sprintf("%s%d-ocp63217", baseName, number)
				oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
			}
			baseName = "hello-openshift-scenario"
			for _, number := range numbers {
				ns := fmt.Sprintf("%s%d-ocp63217", baseName, number)
				oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
			}
		}()

		defer func() {
			etcConfigCMD := fmt.Sprintf(`'configfile=%v;
			configfilebak=%v;
			if [ -f $configfilebak ]; then
				cp $configfilebak $configfile; 
				rm -f $configfilebak;
			else
				rm -f $configfile;
			fi'`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		defer func() {
			dirCmd := "sudo rm -rf /etc/microshift/manifests/kustomization.yaml /etc/microshift/manifests/busybox.yaml " + tmpManifestPath
			_, mchgConfigErr := runSSHCommand(fqdnName, user, dirCmd)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("3. Take backup of config file")
		etcConfigCMD := fmt.Sprintf(`'configfile=%v;
		configfilebak=%v;
		if [ -f $configfile ]; then 
			cp $configfile $configfilebak;
		fi'`, etcConfigYaml, etcConfigYamlbak)
		_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("4. Create tmp manifest path on node")
		_, dirErr := runSSHCommand(fqdnName, user, "sudo mkdir -p "+tmpManifestPath)
		o.Expect(dirErr).NotTo(o.HaveOccurred())

		// set the manifest option value to an empty list should disable loading
		exutil.By("5.1 :: Scenario-1 :: Set an empty list value to the manifest option in config")
		tmpNamespace := "scenario1-ocp63217"
		etcConfigCMD = fmt.Sprintf(`
manifests:
    kustomizePaths: []`)
		changeMicroshiftConfig(etcConfigCMD, fqdnName, etcConfigYaml)

		exutil.By("5.2 :: Scenario-1 :: Create kustomization and deployemnt files")
		newSrcFiles := map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				"/etc/microshift/manifests/",
				"NAMESPACEVAR",
				tmpNamespace,
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				"/etc/microshift/manifests/",
				"NAMESPACEVAR",
				tmpNamespace,
			},
		}
		addKustomizationToMicroshift(fqdnName, newSrcFiles)
		restartErr := restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())
		exutil.By("5.3 :: Scenario-1 :: Check pods after microshift restart")
		podsOp, err := getResource(oc, asAdmin, withoutNamespace, "pod", "-n", "busybox-"+tmpNamespace, "-o=jsonpath={.items[*].metadata.name}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podsOp).To(o.BeEmpty(), "Scenario-1 :: Failed :: Pods are created, manifests are not disabled")
		e2e.Logf("Scenario-1 :: Passed :: Pods should not be created, manifests are disabled")

		// Setting the manifest option value to a single value should only load manifests from that location
		exutil.By("6.1 :: Scenario-2 :: Set a single value to the manifest option in config")
		tmpNamespace = "scenario2-ocp63217"
		etcConfigCMD = fmt.Sprintf(`
  kustomizePaths:
  - /etc/microshift/manifests`)
		changeMicroshiftConfig(etcConfigCMD, fqdnName, etcConfigYaml)

		exutil.By("6.2 :: Scenario-2 :: Create kustomization and deployemnt files")
		newSrcFiles = map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				"/etc/microshift/manifests/",
				"scenario1-ocp63217",
				tmpNamespace,
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				"/etc/microshift/manifests/",
				"scenario1-ocp63217",
				tmpNamespace,
			},
		}

		addKustomizationToMicroshift(fqdnName, newSrcFiles)
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("6.3 :: Scenario-2 :: Check pods after microshift restart")
		podsOutput := getPodsList(oc, "busybox-"+tmpNamespace)
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Scenario-2 :: Failed :: Pods are not created, manifests are not loaded from defined location")
		e2e.Logf("Scenario-2 :: Passed :: Pods are created, manifests are loaded from defined location :: %s", podsOutput[0])

		//  Setting the option value to multiple values should load manifests from all of them.
		exutil.By("7.1 Scenario-3 :: Set multiple values to the manifest option in config")
		etcConfigCMD = fmt.Sprintf(`
manifests:
  kustomizePaths:
  - /etc/microshift/manifests
  - %v`, tmpManifestPath)
		changeMicroshiftConfig(etcConfigCMD, fqdnName, etcConfigYaml)

		tmpNamespace = "scenario3-ocp63217"
		newSrcFiles = map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				"/etc/microshift/manifests/",
				"scenario2-ocp63217",
				tmpNamespace,
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				"/etc/microshift/manifests/",
				"scenario2-ocp63217",
				tmpNamespace,
			},
			"hello-openshift.yaml": {
				"microshift-hello-openshift.yaml",
				tmpManifestPath,
				"NAMESPACEVAR",
				tmpNamespace,
			},
			"kustomization": {
				"microshift-hello-openshift-kustomization.yaml",
				tmpManifestPath,
				"NAMESPACEVAR",
				tmpNamespace,
			},
		}
		exutil.By("7.2 :: Scenario-3 :: Create kustomization and deployemnt files")
		addKustomizationToMicroshift(fqdnName, newSrcFiles)
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("7.3 Scenario-3 :: Check pods after microshift restart")
		podsOutput = getPodsList(oc, "hello-openshift-"+tmpNamespace)
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Scenario-3 :: Failed :: Pods are not created, manifests are not loaded from defined location")
		podsOutput = getPodsList(oc, "busybox-"+tmpNamespace)
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Scenario-3 :: Failed :: Pods are not created, manifests are not loaded from defined location")
		e2e.Logf("Scenario-3 :: Passed :: Pods are created, manifests are loaded from defined location :: %s", podsOutput[0])

		// If the option includes a manifest path that exists but does not contain a kustomization.yaml file, it should be ignored.
		exutil.By("8.1 Scenario-4 :: Set option includes a manifest path that exists but does not contain a kustomization.yaml file")
		_, delFileErr := runSSHCommand(fqdnName, user, "sudo rm "+tmpManifestPath+"kustomization.yaml")
		o.Expect(delFileErr).NotTo(o.HaveOccurred())
		delNsErr := oc.WithoutNamespace().Run("delete").Args("ns", "hello-openshift-scenario3-ocp63217", "--ignore-not-found").Execute()
		o.Expect(delNsErr).NotTo(o.HaveOccurred())
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("8.2 Scenario-4 :: Check pods after microshift restart")
		podsOp, err = getResource(oc, asAdmin, withoutNamespace, "pod", "-n", "hello-openshift-"+tmpNamespace, "-o=jsonpath={.items[*].metadata.name}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podsOp).To(o.BeEmpty(), "Scenario-4 :: Failed :: Pods are created, manifests not ignored defined location")
		e2e.Logf("Scenario-4 :: Passed :: Pods are not created, manifests ignored defined location :: %s", podsOp)

		//  If the option includes a manifest path that does not exist, it should be ignored.
		exutil.By("9.1 Scenario-5 :: Set option includes a manifest path that does not exists")
		_, delDirErr := runSSHCommand(fqdnName, user, "sudo rm -rf "+tmpManifestPath)
		o.Expect(delDirErr).NotTo(o.HaveOccurred())
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("9.2 Scenario-5 :: Check pods after microshift restart")
		podsOp, err = getResource(oc, asAdmin, withoutNamespace, "pod", "-n", "hello-openshift-"+tmpNamespace, "-o=jsonpath={.items[*].metadata.name}")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podsOp).To(o.BeEmpty(), "Scenario-5 :: Failed :: Pods are created, manifests not ignored defined location")
		e2e.Logf("Scenario-5 :: Passed :: Pods are not created, manifests ignored defined location :: %s", podsOp)

		// If the option is not specified, the default locations of /etc/microshift/manifests/kustomization.yaml and /usr/lib/microshift/manifests/kustomization.yaml should be loaded
		exutil.By("10.1 :: Scenario-6 :: Set the manifest option value to an empty for manifest in config")
		etcConfigCMD = fmt.Sprintf(`
manifests:
    kustomizePaths:`)
		changeMicroshiftConfig(etcConfigCMD, fqdnName, etcConfigYaml)
		delNsErr = oc.WithoutNamespace().Run("delete").Args("ns", "busy-scenario3-ocp63217", "--ignore-not-found").Execute()
		o.Expect(delNsErr).NotTo(o.HaveOccurred())
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		exutil.By("10.2 :: Scenario-6 :: Check manifest config")
		pattern := `kustomizePaths:\s*\n\s+-\s+/usr/lib/microshift/manifests\s*\n\s+-\s+/usr/lib/microshift/manifests\.d/\*\s*\n\s+-\s+/etc/microshift/manifests\s*\n\s+-\s+/etc/microshift/manifests\.d/\*`
		re := regexp.MustCompile(pattern)
		mchkConfig, mchkConfigErr := runSSHCommand(fqdnName, user, chkConfigCmd)
		o.Expect(mchkConfigErr).NotTo(o.HaveOccurred())
		match := re.MatchString(mchkConfig)
		if !match {
			e2e.Failf("Manifest config not reset to default :: \n" + mchkConfig)
		}

		exutil.By("10.3 :: Scenario-6 :: Check pods after microshift restart")
		podsOutput = getPodsList(oc, "busybox-"+tmpNamespace)
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Scenario-6 :: Failed :: Pods are not created, manifests are not set to default")
		e2e.Logf("Scenario-6 :: Passed :: Pods should be created, manifests are loaded from default location")
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Longduration-NonPreRelease-Medium-72334-[Apiserver] Make audit log policy configurable for MicroShift [Disruptive][Slow]", func() {
		var (
			e2eTestNamespace     = "microshift-ocp72334-" + exutil.GetRandomString()
			etcConfigYaml        = "/etc/microshift/config.yaml"
			etcConfigYamlbak     = "/etc/microshift/config.yaml.bak"
			user                 = "redhat"
			chkConfigCmd         = `sudo /usr/bin/microshift show-config --mode effective 2>/dev/null`
			defaultProfileCm     = "my-test-default-profile-cm"
			writeRequestBodiesCm = "my-test-writerequestbodies-profile-cm"
			noneProfileCm        = "my-test-none-profile-cm"
			allRequestBodiesCm   = "my-test-allrequestbodies-profile-cm"
			auditLogPath         = "/var/log/kube-apiserver/audit.log"
			writeVerbs           = "(create|delete|patch|update)"
			getVerbs             = "(get|list|watch)"
			fqdnName             = getMicroshiftHostname(oc)
		)

		defer func() {
			etcConfigCMD := fmt.Sprintf(`'configfile=%v;
			configfilebak=%v;
			if [ -f $configfilebak ]; then
				cp $configfilebak $configfile;
				rm -f $configfilebak;
			else
				rm -f $configfile;
			fi'`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("1. Prepare for audit profile setting.")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		e2e.Logf("Take backup of config file")
		etcConfigCMD := fmt.Sprintf(`'configfile=%v;
		configfilebak=%v;
		if [ -f $configfile ]; then 
			cp $configfile $configfilebak;
		fi'`, etcConfigYaml, etcConfigYamlbak)
		_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("2. Set audit profile to Invalid profile")
		etcConfigCMD = fmt.Sprintf(`
apiServer:
 auditLog:
  profile: Unknown`)
		changeMicroshiftConfig(etcConfigCMD, fqdnName, etcConfigYaml)
		restartErr := restartMicroshift(fqdnName)
		o.Expect(restartErr).To(o.HaveOccurred())

		getGrepCMD := func(profileCm, namespace, condition, verbs string, logPath string) string {
			verbGrepCmd := `""`
			if strings.Contains(profileCm, "default") && verbs != `""` {
				verbGrepCmd = fmt.Sprintf(`-hE "\"verb\":\"(%s)\",\"user\":.*(requestObject|responseObject)|\"verb\":\"(%s)\",\"user\":.*(requestObject|responseObject)"`, writeVerbs, getVerbs)
			} else if strings.Contains(profileCm, "writerequest") || strings.Contains(profileCm, "allrequest") {
				verbGrepCmd = fmt.Sprintf(`-hE "\"verb\":\"%s\",\"user\":.*(requestObject|responseObject)"`, verbs)
			}
			return fmt.Sprintf(`sudo grep -i %s %s | grep -i %s | grep %s | grep %s || true`, profileCm, logPath, namespace, condition, verbGrepCmd)
		}

		type logCheckStep struct {
			desc       string
			cmd        string
			conditions string
		}

		type auditProfile struct {
			name        string
			etcConfig   string
			logCountCmd string
			conditions  string
			innerSteps  []*logCheckStep
		}

		steps := []*auditProfile{
			{
				name:        "None",
				etcConfig:   "apiServer:\n  auditLog:\n    profile: None",
				logCountCmd: getGrepCMD(noneProfileCm, e2eTestNamespace, `""`, `""`, auditLogPath),
				conditions:  "==",
			},
			{
				name:      "Default",
				etcConfig: "apiServer:\n  auditLog:",
				innerSteps: []*logCheckStep{
					{
						desc:       "Verify System-Auth logs in profile :: ",
						cmd:        getGrepCMD(defaultProfileCm, e2eTestNamespace, "-i system:authenticated", `""`, auditLogPath),
						conditions: ">",
					},
					{
						desc:       "Verify Verb and User logs in profile :: ",
						cmd:        getGrepCMD(defaultProfileCm, e2eTestNamespace, `""`, getVerbs, auditLogPath),
						conditions: "==",
					},
					{
						desc:       "Verify default logs in profile :: ",
						cmd:        getGrepCMD(defaultProfileCm, e2eTestNamespace, `""`, `""`, auditLogPath),
						conditions: ">",
					},
				},
			},
			{
				name:      "WriteRequestBodies",
				etcConfig: "apiServer:\n  auditLog:\n    profile: WriteRequestBodies",
				innerSteps: []*logCheckStep{
					{
						desc:       "Verify Read logs in profile :: ",
						cmd:        getGrepCMD(writeRequestBodiesCm, e2eTestNamespace, `""`, getVerbs, auditLogPath),
						conditions: "==",
					},
					{
						desc:       "Verify Write logs in profile :: ",
						cmd:        getGrepCMD(writeRequestBodiesCm, e2eTestNamespace, `""`, writeVerbs, auditLogPath),
						conditions: ">",
					},
				},
			},
			{
				name:       "AllRequestBodies",
				etcConfig:  "apiServer:\n  auditLog:\n    profile: AllRequestBodies",
				conditions: ">",
				innerSteps: []*logCheckStep{
					{
						desc:       "Verify Read logs in profile :: ",
						cmd:        getGrepCMD(allRequestBodiesCm, e2eTestNamespace, `""`, getVerbs, auditLogPath),
						conditions: ">",
					},
					{
						desc:       "Verify Write logs in profile :: ",
						cmd:        getGrepCMD(allRequestBodiesCm, e2eTestNamespace, `""`, writeVerbs, auditLogPath),
						conditions: ">",
					},
				},
			},
		}

		i := 2
		for _, step := range steps {
			exutil.By(fmt.Sprintf("%d.1: Set Microshift Audit profile :: %s", i+1, step.name))
			changeMicroshiftConfig(step.etcConfig, fqdnName, etcConfigYaml)
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())

			exutil.By(fmt.Sprintf("%d.2: Verify Microshift profile :: %s", i+1, step.name))
			configOutput, configErr := getMicroshiftConfig(fqdnName, chkConfigCmd, "apiServer.auditLog.profile")
			exutil.AssertWaitPollNoErr(configErr, fmt.Sprintf("Failed to verify Microshift config: %v", configErr))
			o.Expect(configOutput).To(o.ContainSubstring(step.name))

			exutil.By(fmt.Sprintf("%d.3: Verify Microshift audit logs :: %s", i+1, step.name))
			err := oc.AsAdmin().WithoutNamespace().Run("create").Args("cm", fmt.Sprintf("my-test-%s-profile-cm", strings.ToLower(strings.ReplaceAll(step.name, " ", "-"))), "-n", e2eTestNamespace, "--from-literal=key=value").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", fmt.Sprintf("my-test-%s-profile-cm", strings.ToLower(strings.ReplaceAll(step.name, " ", "-"))), "-n", e2eTestNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			j := 3
			for _, innerStep := range step.innerSteps {
				exutil.By(fmt.Sprintf("%d.%d: %s%s", i+1, j+1, innerStep.desc, step.name))
				eventLogs, eventCount, logErr := verifyMicroshiftLogs(fqdnName, innerStep.cmd, innerStep.conditions)
				exutil.AssertWaitPollNoErr(logErr, fmt.Sprintf("Failed to verify Microshift audit logs: %v :: %s :: %v", logErr, eventLogs, eventCount))
				o.Expect(eventCount).To(o.BeNumerically(innerStep.conditions, 0))
				j++
			}
			i++
		}
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Longduration-NonPreRelease-Medium-72340-[Apiserver] Microshift Audit Log File Rotation [Disruptive][Slow]", func() {
		var (
			e2eTestNamespace = "microshift-ocp72340-" + exutil.GetRandomString()
			etcConfigYaml    = "/etc/microshift/config.yaml"
			etcConfigYamlbak = "/etc/microshift/config.yaml.bak"
			user             = "redhat"
			chkConfigCmd     = `sudo /usr/bin/microshift show-config --mode effective 2>/dev/null`
			tmpdir           = "/tmp/" + e2eTestNamespace
			sosReportCmd     = `sudo microshift-sos-report --tmp-dir ` + tmpdir
			newAuditlogPath  = "/home/redhat/kube-apiserver"
			oldAuditlogPath  = "/var/log/kube-apiserver"
			fqdnName         = getMicroshiftHostname(oc)
		)

		defer runSSHCommand(fqdnName, user, "sudo rm -rf ", tmpdir)
		_, cmdErr := runSSHCommand(fqdnName, user, "sudo mkdir -p "+tmpdir)
		o.Expect(cmdErr).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Recovering audit log path")
			clearScript := fmt.Sprintf(`'oldPath=%s;
			newPath=%s;
			if [ -d $newPath ]; then
				sudo rm -rf -- $oldPath;
				sudo rm -rf -- $newPath;
            fi'`, oldAuditlogPath, newAuditlogPath)
			_, cmdErr := runSSHCommand(fqdnName, user, "sudo bash -c", clearScript)
			o.Expect(cmdErr).NotTo(o.HaveOccurred())
			// adding to avoid race conditions
			time.Sleep(100 * time.Millisecond)
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		defer func() {
			e2e.Logf("Recovering microshift config yaml")
			etcConfigCMD := fmt.Sprintf(`'configfile=%v;
			configfilebak=%v;
			if [ -f $configfilebak ]; then
				cp $configfilebak $configfile;
				rm -f $configfilebak;
			else
				rm -f $configfile;
			fi'`, etcConfigYaml, etcConfigYamlbak)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		exutil.By("1. Prepare for audit profile setting.")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		e2e.Logf("Take backup of config file")
		etcConfigCMD := fmt.Sprintf(`'configfile=%v;
		configfilebak=%v;
		if [ -f $configfile ]; then 
			cp $configfile $configfilebak;
		fi'`, etcConfigYaml, etcConfigYamlbak)
		_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())

		exutil.By("2. Check Micoroshift log rotation default values")
		configOutput, configErr := getMicroshiftConfig(fqdnName, chkConfigCmd, "apiServer.auditLog")
		exutil.AssertWaitPollNoErr(configErr, fmt.Sprintf("Failed to verify Microshift config: %v", configErr))
		o.Expect(configOutput).To(o.ContainSubstring(`"maxFileAge":0,"maxFileSize":200,"maxFiles":10,"profile":"Default"`))

		exutil.By("3. Set audit profile to Invalid profile")
		etcConfigInval := "apiServer:\n  auditLog:\n    maxFileAge: inval\n    maxFileSize: invali\n    maxFiles: inval\n"
		changeMicroshiftConfig(etcConfigInval, fqdnName, etcConfigYaml)
		restartErr := restartMicroshift(fqdnName)
		o.Expect(restartErr).To(o.HaveOccurred())

		exutil.By("4. Verify Log rotation values and size")
		etcConfig := "apiServer:\n  auditLog:\n    maxFileAge: 1\n    maxFileSize: 2\n    maxFiles: 2\n    profile: AllRequestBodies"
		changeMicroshiftConfig(etcConfig, fqdnName, etcConfigYaml)
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())
		configOutput, configErr = getMicroshiftConfig(fqdnName, chkConfigCmd, "apiServer.auditLog")
		exutil.AssertWaitPollNoErr(configErr, fmt.Sprintf("Failed to verify Microshift config: %v", configErr))
		o.Expect(configOutput).To(o.ContainSubstring(`"maxFileAge":1,"maxFileSize":2,"maxFiles":2,"profile":"AllRequestBodies"`))

		mstatusErr := wait.PollUntilContextTimeout(context.Background(), 6*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			// Check audit log files in /var/log/kube-apiserver/ directory
			checkAuditLogsCmd := "sudo ls -ltrh /var/log/kube-apiserver/"
			filesOutput, err := runSSHCommand(fqdnName, user, checkAuditLogsCmd)
			if err != nil {
				return false, nil
			}
			// Check if there are two backup files of size 2M and audit.log is correctly managed
			lines := strings.Split(string(filesOutput), "\n")
			backupCount := 0
			for _, line := range lines {
				if strings.Contains(line, "audit-") {
					fields := strings.Fields(line)
					if len(fields) >= 5 && fields[4] == "2.0M" {
						backupCount++
					}
				}
			}
			if backupCount != 2 {
				oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-A").Execute()
				return false, nil
			}
			e2e.Logf("Verification successful: Audit log configuration and files are as expected")
			return true, nil
		})
		exutil.AssertWaitPollNoErr(mstatusErr, fmt.Sprintf("Failed to verify Microshift audit logs: %s", mstatusErr))

		exutil.By("5. Verify Audit log file storage location path dedicated volume")
		_, stopErr := runSSHCommand(fqdnName, user, "sudo systemctl stop microshift")
		o.Expect(stopErr).NotTo(o.HaveOccurred())

		e2e.Logf("Move kube-apiserver log")
		_, moveErr := runSSHCommand(fqdnName, user, "sudo mv /var/log/kube-apiserver ~/")
		o.Expect(moveErr).NotTo(o.HaveOccurred())

		e2e.Logf("Create symlink for audit logs")
		_, symLinkErr := runSSHCommand(fqdnName, user, "sudo ln -s ~/kube-apiserver /var/log/kube-apiserver")
		o.Expect(symLinkErr).NotTo(o.HaveOccurred())

		e2e.Logf("Restart Microshift")
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		e2e.Logf("Gather SOS report logs")
		sosreportStatus := gatherSosreports(fqdnName, user, sosReportCmd, tmpdir)

		// Define the regular expression pattern to extract the file name
		re := regexp.MustCompile(`(/[a-zA-Z0-9/-]+/sosreport-[a-zA-Z0-9-]+\.tar\.xz)`)
		match := re.FindStringSubmatch(sosreportStatus)
		if len(match) > 1 {
			e2e.Logf("File name:", match[1])
		} else {
			e2e.Failf("File name not found in output")
		}

		e2e.Logf("Untart SOS report logs")
		_, tarSosErr := runSSHCommand(fqdnName, user, "sudo tar -vxf "+match[1]+" -C "+tmpdir)
		o.Expect(tarSosErr).NotTo(o.HaveOccurred())

		e2e.Logf("Compare SOS report logs")
		var sosOutput1 string
		var sosOutput2 string
		mSosErr := wait.PollUntilContextTimeout(context.Background(), 6*time.Second, 300*time.Second, false, func(cxt context.Context) (bool, error) {
			sosOutput1, _ = runSSHCommand(fqdnName, user, `sudo find `+tmpdir+` -type l -name kube-apiserver -exec sh -c 'find $(readlink -f {}) -maxdepth 1 -type f | wc -l' \;|tr -d '\n'`)
			sosOutput2, _ = runSSHCommand(fqdnName, user, `sudo find `+tmpdir+` -type d -name kube-apiserver -exec ls -ltrh {} \;|grep -v total|wc -l|tr -d '\n'`)
			// Compare the count of both symbolik link and actual path
			if sosOutput1 != "" && sosOutput2 != "" && sosOutput1 == sosOutput2 {
				e2e.Logf("Both storage paths are identical.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(mstatusErr, fmt.Sprintf("Both storage paths are not identical :: %v. %s::%s", mSosErr, sosOutput1, sosOutput2))
	})

	// author: rgangwar@redhat.com
	g.It("Author:rgangwar-MicroShiftOnly-Longduration-NonPreRelease-Medium-76468-[Apiserver] Drop-in configuration directory [Disruptive]", func() {
		var (
			e2eTestNamespace = "microshift-ocp76468"
			nsString         = "NAMESPACEVAR"
			nsBaseApp        = "base-app-ocp76468"
			nsDevApp         = "dev-app-ocp76468"
			nsPatchesApp     = "patches-app-ocp76468"
			user             = "redhat"
			basePath         = "/etc/microshift"
			configDir        = filepath.Join(basePath, "config.d")
			configFiles      = []string{
				"config.yaml",
				"10-kustomize.yaml",
				"20-kustomize.yaml",
				"10-san.yaml",
				"20-san.yaml",
				"10-kubelet.yaml",
				"20-kubelet.yaml",
			}

			// Manifest paths
			tmpManifestPath = []string{
				filepath.Join(basePath, "manifests.d/my-app/base/"),
				filepath.Join(basePath, "manifests.d/my-app/dev/"),
				filepath.Join(basePath, "manifests.d/my-app/dev/patches/"),
			}
		)

		var etcConfigYaml []string
		for _, file := range configFiles {
			if file == "config.yaml" {
				etcConfigYaml = append(etcConfigYaml, filepath.Join(basePath, file))
			} else {
				etcConfigYaml = append(etcConfigYaml, filepath.Join(configDir, file))
			}
		}

		var etcConfigYamlbak []string
		for _, file := range configFiles {
			if file == "config.yaml" {
				etcConfigYamlbak = append(etcConfigYamlbak, filepath.Join(basePath, file+".bak"))
			} else {
				etcConfigYamlbak = append(etcConfigYamlbak, filepath.Join(configDir, file+".bak"))
			}
		}

		exutil.By("1. Create new namespace for the scenario")
		oc.CreateSpecifiedNamespaceAsAdmin(e2eTestNamespace)
		defer oc.DeleteSpecifiedNamespaceAsAdmin(e2eTestNamespace)

		exutil.By("2. Get microshift VM hostname")
		fqdnName := getMicroshiftHostname(oc)

		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "hello-openshift-dev-app-ocp76468", "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "busybox-base-app-ocp76468", "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "busybox-patches-app-ocp76468", "--ignore-not-found").Execute()
		}()

		// Loop for restoring configs and cleaning up manifests after test completion
		defer func() {
			for i := range etcConfigYaml {
				etcConfigCMD := fmt.Sprintf(
					`'configfile=%v; configfilebak=%v; 
				if [ -f $configfilebak ]; then 
					cp $configfilebak $configfile; 
					rm -f $configfilebak; 
				else 
					rm -f $configfile; 
				fi'`,
					etcConfigYaml[i], etcConfigYamlbak[i],
				)
				_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfigCMD)
				o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			}

			for i := range tmpManifestPath {
				_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo rm -rf "+tmpManifestPath[i])
				o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
			}

			restartErr := restartMicroshift(fqdnName)
			o.Expect(restartErr).NotTo(o.HaveOccurred())
		}()

		// Loop for backing up config files
		exutil.By("3. Backup existing config files")
		for i := range etcConfigYaml {
			etcConfig := fmt.Sprintf(
				`'configfile=%v; configfilebak=%v; 
				if [ -f $configfile ]; then 
					cp $configfile $configfilebak; 
				fi'`,
				etcConfigYaml[i],
				etcConfigYamlbak[i],
			)
			_, mchgConfigErr := runSSHCommand(fqdnName, user, "sudo bash -c", etcConfig)
			o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
		}

		// Create manifest paths for the new configurations
		exutil.By("4. Create tmp manifest path on the node")
		for i := range tmpManifestPath {
			_, dirErr := runSSHCommand(fqdnName, user, "sudo mkdir -p "+tmpManifestPath[i])
			o.Expect(dirErr).NotTo(o.HaveOccurred())
		}

		// Set glob path values to the manifest option in config
		exutil.By("4.1 Scenario1 :: Set glob path values to the manifest option in config 10-kustomization.yaml")
		etcConfig := `
manifests:
  kustomizePaths:
  - /etc/microshift/manifests.d/my-app/*/`
		changeMicroshiftConfig(etcConfig, fqdnName, etcConfigYaml[1])

		// Create kustomization and deployment files
		newSrcFiles := map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				tmpManifestPath[0],
				nsString,
				nsBaseApp,
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				tmpManifestPath[0],
				nsString,
				nsBaseApp,
			},
			"hello-openshift.yaml": {
				"microshift-hello-openshift.yaml",
				tmpManifestPath[1],
				nsString,
				nsDevApp,
			},
			"kustomization": {
				"microshift-hello-openshift-kustomization.yaml",
				tmpManifestPath[1],
				nsString,
				nsDevApp,
			},
		}
		exutil.By("4.2 Create kustomization and deployment files")
		addKustomizationToMicroshift(fqdnName, newSrcFiles)
		restartErr := restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		// Validate pods after microshift restart
		exutil.By("4.3 Check pods after microshift restart")
		podsOutput := getPodsList(oc, "hello-openshift-"+nsDevApp)
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case Scenario1 :: Failed :: Pods are not created, manifests are not loaded from defined location")
		podsOutput = getPodsList(oc, "busybox-"+nsBaseApp)
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case Scenario1 :: Failed :: Pods are not created, manifests are not loaded from defined location")
		e2e.Logf("Test case Scenario1 :: Passed :: Pods are created, manifests are loaded from defined location :: %s", podsOutput[0])

		exutil.By("4.4 Scenario2 :: Set glob path values to the manifest option in config 20-kustomization.yaml")
		etcConfig = `
manifests:
  kustomizePaths:
  - /etc/microshift/manifests.d/my-app/*/patches/`
		changeMicroshiftConfig(etcConfig, fqdnName, etcConfigYaml[2])

		// Create kustomization and deployment files
		newSrcFilesPatches := map[string][]string{
			"busybox.yaml": {
				"microshift-busybox-deployment.yaml",
				tmpManifestPath[2],
				nsBaseApp,
				nsPatchesApp,
			},
			"kustomization.yaml": {
				"microshift-busybox-kustomization.yaml",
				tmpManifestPath[2],
				nsBaseApp,
				nsPatchesApp,
			},
		}
		exutil.By("4.5 Create kustomization and deployment files")
		addKustomizationToMicroshift(fqdnName, newSrcFilesPatches)
		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		// Validate pods after microshift restart
		exutil.By("4.6 Check pods after microshift restart")
		podsOutput = getPodsList(oc, "busybox-patches-app-ocp76468")
		o.Expect(podsOutput[0]).NotTo(o.BeEmpty(), "Test case Scenario1 :: Failed :: Pods are not created, manifests are not loaded from defined location")
		e2e.Logf("Test case Scenario2 :: Passed :: Pods are created, manifests are loaded from defined location :: %s", podsOutput[0])

		exutil.By("4.7 Scenario3 :: Set path values to the apiserver option in config 10-san.yaml and 20-san.yaml")
		san10Content := `
apiServer:
  subjectAltNames:
  - test.api.com`
		changeMicroshiftConfig(san10Content, fqdnName, etcConfigYaml[3])

		san20Content := `
apiServer:
  subjectAltNames:
  - hostZ.api.com`
		changeMicroshiftConfig(san20Content, fqdnName, etcConfigYaml[4])

		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		validateMicroshiftConfig(fqdnName, user, `subjectAltNames:\n\s+-\s+hostZ\.api\.com`)
		e2e.Logf("Test case Scenario3 :: Passed :: San configs are not merged together, san with higher numbers take priority")

		exutil.By("4.7 Scenario4 :: Set path values to the kubelet option in config 10-kubelet.yaml and 20-kubelet.yaml")
		kubeletConfig := `
kubelet:
  another_setting1: True`
		changeMicroshiftConfig(kubeletConfig, fqdnName, etcConfigYaml[0])

		kubelet10Content := `
kubelet:
  another_setting2: True`
		changeMicroshiftConfig(kubelet10Content, fqdnName, etcConfigYaml[5])

		kubelet20Content := `
kubelet:
  another_setting3: True`
		changeMicroshiftConfig(kubelet20Content, fqdnName, etcConfigYaml[6])

		restartErr = restartMicroshift(fqdnName)
		o.Expect(restartErr).NotTo(o.HaveOccurred())

		validateMicroshiftConfig(fqdnName, user, `kubelet:\n\s+another_setting1:\s+true\n\s+another_setting2:\s+true\n\s+another_setting3:\s+true`)
		e2e.Logf("Test case Scenario4 :: Passed :: kubelete configs are merged together")
	})
})
