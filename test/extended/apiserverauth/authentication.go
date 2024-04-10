package apiserverauth

import (
	"crypto/tls"
	base64 "encoding/base64"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-auth] Authentication", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLIWithoutNamespace("default")

	// author: xxia@redhat.com
	// It is destructive case, will make co/authentical Available=False for a while, so adding [Disruptive]
	// If the case duration is greater than 10 minutes and is executed in serial (labelled Serial or Disruptive), add Longduration
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:xxia-Medium-29917-Deleted authentication resources can come back immediately [Disruptive]", func() {
		// Temporarily skip for sno env. Will follow up to investigate robust enhancement.
		exutil.SkipForSNOCluster(oc)
		g.By("Delete namespace openshift-authentication")
		err := oc.WithoutNamespace().Run("delete").Args("ns", "openshift-authentication").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Waiting for the namespace back, it should be back immediate enough. If it is not back immediately, it is bug")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("ns", "openshift-authentication").Output()
			if err != nil {
				e2e.Logf("Fail to get namespace openshift-authentication, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("openshift-authentication.*Active", output); matched {
				e2e.Logf("Namespace is back:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "openshift-authentication is not back")

		g.By("Waiting for oauth-openshift pods back")
		// Observation: 2nd time deletion of pods brings them back to 'Running' state sooner compare to 1st time deletion,
		// so deleting the auth pods if they are still in Terminating state
		output, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("oauth-openshift.*Terminating", output); matched {
			err := oc.WithoutNamespace().Run("delete").Args("pods", "--all", "-n", "openshift-authentication").Execute()
			o.Expect(err).NotTo(o.HaveOccurred(), "Couldn't delete pods of openshift-authentication")
		}
		// It needs some time to wait for pods recreated and Running, so the Poll parameters are a little larger
		err = wait.Poll(15*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication").Output()
			if err != nil {
				e2e.Logf("Fail to get pods under openshift-authentication, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("oauth-openshift.*Running", output); matched {
				e2e.Logf("Pods are back:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod of openshift-authentication is not back")

		g.By("Waiting for the clusteroperator back to normal")
		// It needs more time to wait for clusteroperator back to normal. In test, the max time observed is up to 4 mins, so the Poll parameters are larger
		err = wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("co", "authentication").Output()
			if err != nil {
				e2e.Logf("Fail to get clusteroperator authentication, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("True.*False.*False", output); matched {
				e2e.Logf("clusteroperator authentication is back to normal:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "clusteroperator authentication is not back to normal")

		g.By("Delete authentication.operator cluster")
		err = oc.WithoutNamespace().Run("delete").Args("authentication.operator", "cluster").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Waiting for authentication.operator back")
		// It needs more time to wait for authentication.operator back. In test, the max time observed is up to 10 mins, so the Poll parameters are larger
		// There is open bug https://issues.redhat.com/browse/OCPBUGS-10525 related to the deleted auth resources not being in normal state immediately
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("authentication.operator", "--no-headers").Output()
			if err != nil {
				e2e.Logf("Fail to get authentication.operator cluster, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("^cluster ", output); matched {
				e2e.Logf("authentication.operator cluster is back:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "authentication.operator cluster is not back")

		g.By("Delete project openshift-authentication-operator")
		err = oc.WithoutNamespace().Run("delete").Args("project", "openshift-authentication-operator").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Waiting for project openshift-authentication-operator back")
		// It needs more time to wait for project openshift-authentication-operator back. In test, the max time observed is up to 6 mins, so the Poll parameters are larger
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("project", "openshift-authentication-operator").Output()
			if err != nil {
				e2e.Logf("Fail to get project openshift-authentication-operator, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("openshift-authentication-operator.*Active", output); matched {
				e2e.Logf("project openshift-authentication-operator is back:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "project openshift-authentication-operator is not back")

		g.By("Waiting for the openshift-authentication-operator pod back")
		// Observation: 2nd time deletion of pods brings them back to 'Running' state sooner compare to 1st time deletion,
		// so deleting the openshift-authentication-operator pods if they are still in terminating state
		output, err = oc.WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("authentication-operator.*Terminating", output); matched {
			err := oc.WithoutNamespace().Run("delete").Args("pods", "--all", "-n", "openshift-authentication-operator").Execute()
			o.Expect(err).NotTo(o.HaveOccurred(), "Couldn't delete pods of openshift-authentication-operator")
		}
		// It needs some time to wait for pods recreated and Running, so the Poll parameters are a little larger
		err = wait.Poll(15*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication-operator").Output()
			if err != nil {
				e2e.Logf("Fail to get pod under openshift-authentication-operator, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("authentication-operator.*Running", output); matched {
				e2e.Logf("Pod is back:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod of openshift-authentication-operator is not back")
	})

	// author: pmali@redhat.com
	// It is destructive case, will make co/authentical Available=False for a while, so adding [Disruptive]

	g.It("NonHyperShiftHOST-Author:pmali-High-33390-Network Stability check every level of a managed route [Disruptive] [Flaky]", func() {
		g.By("Check pods under openshift-authentication namespace is available")
		err := wait.Poll(1*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-authentication").Output()
			if err != nil {
				e2e.Logf("Fail to get pods under openshift-authentication, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("oauth-openshift.*Running", output); matched {
				e2e.Logf("Pods are in Running state:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod of openshift-authentication is not Running")

		// Check authentication operator, If its UP and running that means route and service is also working properly. No need to check seperately Service and route endpoints.
		g.By("Check authentication operator is available")
		err = wait.Poll(1*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "authentication", "-o=jsonpath={.status.conditions[0].status}").Output()
			if err != nil {
				e2e.Logf("Fail to get authentication.operator cluster, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("False", output); matched {
				e2e.Logf("authentication.operator cluster is UP:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "authentication.operator cluster is not UP")

		//Check service endpoint is showing correct error

		buildPruningBaseDir := exutil.FixturePath("testdata", "apiserverauth")

		g.By("Check service endpoint is showing correct error")
		networkPolicyAllow := filepath.Join(buildPruningBaseDir, "allow-same-namespace.yaml")

		g.By("Create AllowNetworkpolicy")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-authentication", "-f"+networkPolicyAllow).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-authentication", "-f="+networkPolicyAllow).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check authentication operator after allow network policy change
		err = wait.Poll(2*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "authentication", "-o=jsonpath={.status.conditions[0].message}").Output()

			if err != nil {
				e2e.Logf("Fail to get authentication.operator cluster, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains(output, "OAuthServerServiceEndpointAccessibleControllerDegraded") {
				e2e.Logf("Allow network policy applied successfully:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Allow network policy applied failure")

		g.By("Delete allow-same-namespace Networkpolicy")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-authentication", "-f="+networkPolicyAllow).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//Deny all trafic for route
		g.By("Check route is showing correct error")

		networkPolicyDeny := filepath.Join(buildPruningBaseDir, "deny-network-policy.yaml")

		g.By("Create Deny-all Networkpolicy")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-authentication", "-f="+networkPolicyDeny).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-authentication", "-f="+networkPolicyDeny).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Check authentication operator after network policy change
		err = wait.Poll(2*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "authentication", "-o=jsonpath={.status.conditions[0].message}").Output()

			if err != nil {
				e2e.Logf("Fail to get authentication.operator cluster, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains(output, "OAuthServerRouteEndpointAccessibleControllerDegraded") {
				e2e.Logf("Deny network policy applied:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Deny network policy not applied")

		g.By("Delete Networkpolicy")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-authentication", "-f="+networkPolicyDeny).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: ytripath@redhat.com
	g.It("NonPreRelease-Longduration-Author:ytripath-Medium-20804-Support ConfigMap injection controller [Disruptive] [Slow]", func() {
		oc.SetupProject()

		// Check the pod service-ca is running in namespace openshift-service-ca
		podDetails, err := oc.AsAdmin().Run("get").WithoutNamespace().Args("po", "-n", "openshift-service-ca").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		matched, _ := regexp.MatchString("service-ca-.*Running", podDetails)
		o.Expect(matched).Should(o.Equal(true))

		// Create a configmap --from-literal and annotating it with service.beta.openshift.io/inject-cabundle=true
		err = oc.Run("create").Args("configmap", "my-config", "--from-literal=key1=config1", "--from-literal=key2=config2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("annotate").Args("configmap", "my-config", "service.beta.openshift.io/inject-cabundle=true").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// wait for service-ca.crt to be created in configmap
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("configmap", "my-config", `-o=json`).Output()
			if err != nil {
				e2e.Logf("Failed to get configmap, error: %s. Trying again", err)
				return false, nil
			}
			if strings.Contains(output, "service-ca.crt") {
				e2e.Logf("service-ca injected into configmap successfully\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "service-ca.crt not found in configmap")

		oldCert, err := oc.Run("get").Args("configmap", "my-config", `-o=jsonpath={.data.service-ca\.crt}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Delete secret signing-key in openshift-service-ca project
		podOldUID, err := oc.AsAdmin().Run("get").WithoutNamespace().Args("po", "-n", "openshift-service-ca", "-o=jsonpath={.items[0].metadata.uid}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("delete").WithoutNamespace().Args("-n", "openshift-service-ca", "secret", "signing-key").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			// sleep for 200 seconds to make sure related clusteroperators start to change
			time.Sleep(200 * time.Second)
			e2e.Logf("Waiting for kube-controller-manager/kube-scheduler/kube-apiserver to rotate. The output may show some errors before completion, this is expected")
			err := wait.Poll(15*time.Second, 20*time.Minute, func() (bool, error) {
				// The renewing of signing-key will lead to cluster components to rotate automatically. Some components' pods complete rotation in relatively short time. Some other components take longer time. Below control plane components take the longest time.
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "kube-controller-manager", "kube-scheduler", "kube-apiserver").Output()
				// Just retry, need not o.Expect(err).NotTo(o.HaveOccurred()) here
				matched1, _ := regexp.MatchString("kube-controller-manager.* True *False *False", output)
				matched2, _ := regexp.MatchString("kube-scheduler.* True *False *False", output)
				matched3, _ := regexp.MatchString("kube-apiserver.* True *False *False", output)
				if matched1 && matched2 && matched3 {
					e2e.Logf("The clusteroperators are in good status. Wait 100s to double check.")
					// Sleep for 100 seconds then double-check if all still stay good. This is needed to know the rotation really completes.
					time.Sleep(100 * time.Second)
					output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "kube-controller-manager", "kube-scheduler", "kube-apiserver").Output()
					e2e.Logf("\n" + output)
					matched1, _ := regexp.MatchString("kube-controller-manager.* True *False *False", output)
					matched2, _ := regexp.MatchString("kube-scheduler.* True *False *False", output)
					matched3, _ := regexp.MatchString("kube-apiserver.* True *False *False", output)
					if matched1 && matched2 && matched3 {
						e2e.Logf("Double check shows the clusteroperators stay really in good status. The rotation really completes")
						return true, nil
					}
				} else {
					// The clusteroperators are still transitioning
					e2e.Logf("\n" + output)
					e2e.Logf("The clusteroperators are still transitioning")
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err, "These clusteroperators are still not back after waiting for 20 minutes\n")
		}()

		// Waiting for the pod to be Ready, after several minutes(10 min ?) check the cert data in the configmap
		g.By("Waiting for service-ca to be ready, then check if cert data is updated")
		err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
			podStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("po", "-n", "openshift-service-ca", `-o=jsonpath={.items[0].status.containerStatuses[0].ready}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			podUID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("po", "-n", "openshift-service-ca", "-o=jsonpath={.items[0].metadata.uid}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			if podStatus == `true` && podOldUID != podUID {
				// We need use AsAdmin() otherwise it will frequently hit "error: You must be logged in to the server (Unauthorized)"
				// before the affected components finish pod restart after the secret deletion, like kube-apiserver, oauth-apiserver etc.
				// Still researching if this is a bug
				newCert, _ := oc.AsAdmin().Run("get").Args("configmap", "my-config", `-o=jsonpath={.data.service-ca\.crt}`).Output()
				matched, _ := regexp.MatchString(oldCert, newCert)
				if !matched {
					g.By("Cert data has been updated")
					return true, nil
				}
			}
			return false, err
		})
		exutil.AssertWaitPollNoErr(err, "Cert data not updated after waiting for 5 mins")
	})

	// author: rugong@redhat.com
	// It is destructive case, will change scc restricted, so adding [Disruptive]
	g.It("WRS-ConnectedOnly-Author:rugong-Medium-20052-New field forbiddenSysctls for SCC", func() {
		oc.SetupProject()
		username := oc.Username()
		scc := "scc-test-20052"
		// In 4.11 and above, we should use SCC "restricted-v2"
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("scc", "restricted-v2", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		re := regexp.MustCompile("(?m)[\r\n]+^  (uid|resourceVersion):.*$")
		output = re.ReplaceAllString(output, "")
		output = strings.NewReplacer("MustRunAsRange", "RunAsAny", "name: restricted-v2", "name: "+scc).Replace(output)

		path := "/tmp/scc_restricted_20052.yaml"
		err = ioutil.WriteFile(path, []byte(output), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(path)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", path).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Deleting the test SCC before exiting the scenario")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", path).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("scc", scc, "-p", `{"allowedUnsafeSysctls":["kernel.msg*"]}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("adm", "policy").Args("remove-scc-from-user", scc, username).Execute()
		err = oc.AsAdmin().Run("adm", "policy").Args("add-scc-to-user", scc, username).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		BaseDir := exutil.FixturePath("testdata", "apiserverauth")
		podYaml := filepath.Join(BaseDir, "pod_with_sysctls.yaml")
		err = oc.Run("create").Args("-f", podYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		output, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("scc", scc, "-p", `{"forbiddenSysctls":["kernel.msg*"]}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("sysctl overlaps with kernel.msg"))
		e2e.Logf("oc patch scc failed, this is expected.")

		err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", path).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Restore the SCC successfully.")

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("scc", scc, "-p", `{"forbiddenSysctls":["kernel.msg*"]}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("delete").Args("po", "busybox").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", podYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("scc", scc, "-p", `{"allowedUnsafeSysctls":["kernel.msg*"]}`, "--type=merge").Execute()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf("oc patch scc failed, this is expected.")

		err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", path).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Restore the SCC successfully.")

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("scc", scc, "-p", `{"forbiddenSysctls":["kernel.shm_rmid_forced"]}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("delete").Args("po", "busybox").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.Run("create").Args("-f", podYaml).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("unable to validate against any security context constraint"))
		e2e.Logf("Failed to create pod, this is expected.")
	})

	// author: rugong@redhat.com
	// It is destructive case, will change scc restricted, so adding [Disruptive]
	g.It("WRS-ConnectedOnly-Author:rugong-Medium-20050-New field allowedUnsafeSysctls for SCC [Disruptive]", func() {
		// In 4.11 and above, we should use SCC "restricted-v2"
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("scc", "restricted-v2", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// uid and resourceVersion must be removed, otherwise "Operation cannot be fulfilled" error will occur when running oc replace in later steps
		re := regexp.MustCompile("(?m)[\r\n]+^  (uid|resourceVersion):.*$")
		output = re.ReplaceAllString(output, "")
		path := "/tmp/scc_restricted_20050.yaml"
		err = ioutil.WriteFile(path, []byte(output), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(path)

		oc.SetupProject()
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		BaseDir := exutil.FixturePath("testdata", "apiserverauth")
		podYaml := filepath.Join(BaseDir, "pod-with-msgmax.yaml")
		output, err = oc.Run("create").Args("-f", podYaml).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`unsafe sysctl "kernel.msgmax" is not allowed`))
		e2e.Logf("Failed to create pod, this is expected.")

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("scc", "restricted-v2", `--type=json`, `-p=[{"op": "add", "path": "/allowedUnsafeSysctls", "value":["kernel.msg*"]}]`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Restoring the restricted SCC before exiting the scenario")
			err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", path).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err = oc.Run("create").Args("-f", podYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: rugong@redhat.com
	// It is destructive case, will change oauth cluster and the case execution duration is greater than 5 min, so adding [Disruptive] and [NonPreRelease]
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:rugong-Medium-22434-RequestHeader IDP consumes header values from requests of auth proxy [Disruptive]", func() {
		configMapPath, err := os.MkdirTemp("/tmp/", "tmp_22434")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(configMapPath)
		caFileName := "/ca-bundle.crt"
		configMapName := "my-request-header-idp-configmap"
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", "openshift-config", "cm/admin-kubeconfig-client-ca", "--confirm", "--to="+configMapPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(configMapPath + caFileName)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", configMapName, "--from-file=ca.crt="+configMapPath+caFileName, "-n", "openshift-config").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Removing configmap before exiting the scenario.")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", configMapName, "-n", "openshift-config").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		oauthClusterYamlPath := "/tmp/oauth_cluster_22434.yaml"
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("oauth", "cluster", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// uid and resourceVersion must be removed, otherwise "Operation cannot be fulfilled" error will occur when running oc replace in later steps
		re := regexp.MustCompile("(?m)[\r\n]+^  (uid|resourceVersion):.*$")
		output = re.ReplaceAllString(output, "")
		err = ioutil.WriteFile(oauthClusterYamlPath, []byte(output), 0644)
		defer os.Remove(oauthClusterYamlPath)
		baseDir := exutil.FixturePath("testdata", "apiserverauth")
		idpPath := filepath.Join(baseDir, "RequestHeader_IDP.yaml")
		idpStr, err := ioutil.ReadFile(idpPath)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Replacing oauth cluster yaml [spec] part.")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("oauth", "cluster", "--type=merge", "-p", string(idpStr)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Restoring oauth cluster yaml before exiting the scenario.")
			err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", oauthClusterYamlPath).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `authentication status has not yet changed to {"Progressing": "True"} in 240 seconds`)
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `authentication status has not yet changed to {"Available": "True", "Progressing": "False", "Degraded": "False"} in 240 seconds`)
		e2e.Logf("openshift-authentication pods are all running.")

		g.By("Preparing file client.crt and client.key")
		output, err = oc.AsAdmin().WithoutNamespace().Run("config").Args("view", "--context", "admin", "--minify", "--raw").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		reg := regexp.MustCompile("(?m)[\r\n]+^    (client-certificate-data):.*$")
		clientCertificateData := reg.FindString(output)
		reg = regexp.MustCompile("(?m)[\r\n]+^    (client-key-data):.*$")
		clientKeyData := reg.FindString(output)
		reg = regexp.MustCompile("[^ ]+$")
		crtEncode := reg.FindString(clientCertificateData)
		o.Expect(crtEncode).NotTo(o.BeEmpty())
		keyEncode := reg.FindString(clientKeyData)
		o.Expect(keyEncode).NotTo(o.BeEmpty())
		crtDecodeByte, err := base64.StdEncoding.DecodeString(crtEncode)
		o.Expect(err).NotTo(o.HaveOccurred())
		keyDecodeByte, err := base64.StdEncoding.DecodeString(keyEncode)
		o.Expect(err).NotTo(o.HaveOccurred())
		crtPath := "/tmp/client_22434.crt"
		keyPath := "/tmp/client_22434.key"
		err = ioutil.WriteFile(crtPath, crtDecodeByte, 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(crtPath)
		err = ioutil.WriteFile(keyPath, keyDecodeByte, 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.Remove(keyPath)
		e2e.Logf("File client.crt and client.key are prepared.")

		// generate first request
		cert, err := tls.LoadX509KeyPair(crtPath, keyPath)
		o.Expect(err).NotTo(o.HaveOccurred())
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					Certificates:       []tls.Certificate{cert},
				},
			},
			// if the client follows redirects automatically, it will encounter this error "http: no Location header in response"
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		oauthRouteHost, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "oauth-openshift", "-n", "openshift-authentication", "-o", "jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		requestURL := "https://" + oauthRouteHost + "/oauth/authorize?response_type=token&client_id=openshift-challenging-client"
		request, err := http.NewRequest("GET", requestURL, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		ssoUser1 := "testUser1"
		xRemoteUserDisplayName := "testDisplayName1"
		request.Header.Add("SSO-User", ssoUser1)
		request.Header.Add("X-Remote-User-Display-Name", xRemoteUserDisplayName)
		g.By("First request is sending, waiting a response.")
		response1, err := client.Do(request)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer response1.Body.Close()
		// check user & identity & oauthaccesstoken
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("user", ssoUser1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("identity", "my-request-header-idp:"+ssoUser1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("oauthaccesstoken").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(ssoUser1))
		e2e.Logf("First response is gotten, user & identity & oauthaccesstoken are expected.")
		defer func() {
			g.By("Removing user " + ssoUser1)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("user", ssoUser1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Removing identity my-request-header-idp:" + ssoUser1)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("identity", "my-request-header-idp:"+ssoUser1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("Logging in with access_token.")
		location := response1.Header.Get("Location")
		u, err := url.Parse(location)
		o.Expect(err).NotTo(o.HaveOccurred())
		subStrArr := strings.Split(u.Fragment, "&")
		accessToken := ""
		for i := 0; i < len(subStrArr); i++ {
			if strings.Contains(subStrArr[i], "access_token") {
				accessToken = strings.Replace(subStrArr[i], "access_token=", "", 1)
				break
			}
		}
		o.Expect(accessToken).NotTo(o.BeEmpty())
		// The --token command modifies the file pointed to the env KUBECONFIG, so I need a temporary file for it to modify
		oc.SetupProject()
		err = oc.Run("login").Args("--token", accessToken).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Log in with access_token successfully.")

		// generate second request
		requestURL = "https://" + oauthRouteHost + "/oauth/authorize?response_type=token&client_id=openshift-challenging-client"
		request, err = http.NewRequest("GET", requestURL, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		ssoUser2 := "testUser2"
		xRemoteUserLogin := "testUserLogin"
		request.Header.Add("SSO-User", ssoUser2)
		request.Header.Add("X-Remote-User-Login", xRemoteUserLogin)
		g.By("Second request is sending, waiting a response.")
		response2, err := client.Do(request)
		defer response2.Body.Close()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("user", xRemoteUserLogin).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("identity", "my-request-header-idp:"+ssoUser2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Second response is gotten, user & identity are expected.")
		defer func() {
			g.By("Removing user " + xRemoteUserLogin)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("user", xRemoteUserLogin).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Removing identity my-request-header-idp:" + ssoUser2)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("identity", "my-request-header-idp:"+ssoUser2).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					Certificates:       []tls.Certificate{cert},
				},
			},
		}
		// generate third request
		requestURL = "https://" + oauthRouteHost + "/oauth/token/request"
		request, err = http.NewRequest("GET", requestURL, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		xRemoteUser := "testUser3"
		request.Header.Add("X-Remote-User", xRemoteUser)
		g.By("Third request is sending, waiting a response.")
		response3, err := client.Do(request)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer response3.Body.Close()
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("user", xRemoteUser).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("identity", "my-request-header-idp:"+xRemoteUser).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		bodyByte, err := ioutil.ReadAll(response3.Body)
		respBody := string(bodyByte)
		o.Expect(respBody).To(o.ContainSubstring("Display Token"))
		e2e.Logf("Third response is gotten, user & identity & display_token are expected.")
		defer func() {
			g.By("Removing user " + xRemoteUser)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("user", xRemoteUser).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Removing identity my-request-header-idp:" + xRemoteUser)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("identity", "my-request-header-idp:"+xRemoteUser).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
	})

	// author: rugong@redhat.com
	// Adding the NonHyperShiftHOST label because developers appear to not fix the known bug https://issues.redhat.com/browse/OCPBUGS-3873
	g.It("WRS-NonHyperShiftHOST-Author:rugong-Low-37697-Allow Users To Manage Their Own Tokens", func() {
		oc.SetupProject()
		user1Name := oc.Username()
		userOauthAccessTokenYamlPath, err := os.MkdirTemp("/tmp/", "tmp_37697")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(userOauthAccessTokenYamlPath)
		userOauthAccessTokenYamlName := "userOauthAccessToken.yaml"
		userOauthAccessTokenName1, err := oc.Run("get").Args("useroauthaccesstokens", "-ojsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		userOauthAccessTokenYaml, err := oc.Run("get").Args("useroauthaccesstokens", userOauthAccessTokenName1, "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = ioutil.WriteFile(userOauthAccessTokenYamlPath+"/"+userOauthAccessTokenYamlName, []byte(userOauthAccessTokenYaml), 0644)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", userOauthAccessTokenYamlPath+"/"+userOauthAccessTokenYamlName).Execute()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf("User cannot create useroauthaccesstokens by yaml file of his own, this is expected.")

		// switch to another user, try to get and delete previous user's useroauthaccesstokens
		oc.SetupProject()
		err = oc.Run("get").Args("useroauthaccesstokens", userOauthAccessTokenName1).Execute()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf("User cannot list other user's useroauthaccesstokens, this is expected.")
		err = oc.Run("delete").Args("useroauthaccesstoken", userOauthAccessTokenName1).Execute()
		o.Expect(err).To(o.HaveOccurred())
		e2e.Logf("User cannot delete other user's useroauthaccesstoken, this is expected.")

		baseDir := exutil.FixturePath("testdata", "apiserverauth")
		clusterRoleTestSudoer := filepath.Join(baseDir, "clusterrole-test-sudoer.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterRoleTestSudoer).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterroles", "test-sudoer-37697").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("clusterrolebinding", "test-sudoer-37697", "--clusterrole=test-sudoer-37697", "--user="+oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrolebinding", "test-sudoer-37697").Execute()
		e2e.Logf("Clusterroles and clusterrolebinding were created successfully.")

		err = oc.Run("get").Args("useroauthaccesstokens", "--as="+user1Name, "--as-group=system:authenticated:oauth").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("delete").Args("useroauthaccesstoken", userOauthAccessTokenName1, "--as="+user1Name, "--as-group=system:authenticated:oauth").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("A user of 'impersonate' permission can get and delete other user's useroauthaccesstoken, this is expected.")

		shaToken, err := oc.Run("whoami").Args("-t").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		userOauthAccessTokenName2, err := oc.Run("get").Args("useroauthaccesstokens", "-ojsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("delete").Args("useroauthaccesstokens", userOauthAccessTokenName2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Need wait a moment to ensure the token really becomes invalidated
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			err = oc.Run("login").Args("--token=" + shaToken).Execute()
			if err != nil {
				e2e.Logf("The token is now invalidated after its useroauthaccesstoken is deleted for a while.")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Timed out in invalidating a token after its useroauthaccesstoken is deleted for a while")
	})

	// author: gkarager@redhat.com
	g.It("NonHyperShiftHOST-Author:gkarager-Medium-49757-Missing content in default RBAC role, rolebinding, clusterrole and clusterrolebinding can be restored automatically when apiserver restarts [Disruptive]", func() {
		tmpCaseFilePath, err := os.MkdirTemp("/tmp/", "tmp_49757")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpCaseFilePath)

		g.By("Checking default RBAC resource annotations")
		clusterRole, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole.rbac", "system:build-strategy-docker", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterRole).To(o.ContainSubstring("rbac.authorization.kubernetes.io/autoupdate: \"true\""))
		// Storing original clusterrole configuration in clusterrole.yaml file for future use
		clusterRoleYaml := filepath.Join(tmpCaseFilePath, "clusterrole.yaml")
		// uid and resourceVersion must be removed, otherwise "Operation cannot be fulfilled" error will occur when running oc replace in later steps
		re := regexp.MustCompile("(?m)[\r\n]+^  (uid|resourceVersion):.*$")
		clusterRole = re.ReplaceAllString(clusterRole, "")
		err = os.WriteFile(clusterRoleYaml, []byte(clusterRole), 0o644)
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterRoleBinding, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebinding.rbac", "system:oauth-token-deleters", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterRoleBinding).To(o.ContainSubstring("rbac.authorization.kubernetes.io/autoupdate: \"true\""))
		// Storing original clusterrolebinding configuration in clusterrolebinding.yaml file for future use
		clusterRoleBindingYaml := filepath.Join(tmpCaseFilePath, "clusterrolebinding.yaml")
		clusterRoleBinding = re.ReplaceAllString(clusterRoleBinding, "")
		err = os.WriteFile(clusterRoleBindingYaml, []byte(clusterRoleBinding), 0o644)
		o.Expect(err).NotTo(o.HaveOccurred())

		clusterRole, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole.authorization.openshift.io", "system:build-strategy-docker", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterRole).To(o.ContainSubstring("openshift.io/reconcile-protect: \"false\""))

		clusterRoleBinding, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebinding.authorization.openshift.io", "system:oauth-token-deleters", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterRoleBinding).To(o.ContainSubstring("openshift.io/reconcile-protect: \"false\""))

		g.By("Modifying cluster default RBAC resources")
		patchYaml := `{"rules":[{"apiGroups":["","build.openshift.io"],"resources":["builds/docker","builds/optimizeddocker"],"verbs":["get"]}]}`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterrole.rbac", "system:build-strategy-docker", "--type=merge", "-p", patchYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Restoring clusterrole.rbac before exiting the scenario.")
			err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", clusterRoleYaml).Execute()
		}()

		patchYaml = `{"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"system:authenticated"}]}`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterrolebinding.rbac", "system:oauth-token-deleters", "--type=merge", "-p", patchYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Restoring clusterrolebinding.rbac before exiting the scenario.")
			err = oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", clusterRoleBindingYaml).Execute()
		}()

		g.By("Restarting openshift-apiserver pods")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "--all", "-n", "openshift-apiserver").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "openshift-apiserver", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, `openshift-apiserver status has not yet changed to {"Available": "True", "Progressing": "False", "Degraded": "False"} in 240 seconds`)
		e2e.Logf("openshift-apiserver pods are all rotated and running.")

		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole.rbac", "system:build-strategy-docker", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("- get"))
		o.Expect(output).To(o.ContainSubstring("- create"))

		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrolebinding.rbac", "system:oauth-token-deleters", "-o", "yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("system:unauthenticated"))
		e2e.Logf("The deleted parts in both clusterrole.rbac and clusterrolebinding.rbac are restored.")
	})

	// author: gkarager@redhat.com
	g.It("Author:gkarager-Medium-52324-Pod security admission autolabelling opting out keeps old labels/does not sync", func() {
		g.By("1. Create a namespace as normal user")
		oc.SetupProject()
		testNameSpace := oc.Namespace()

		g.By("2. Check the project labels")
		output, err := oc.Run("get").Args("project", testNameSpace, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// for 4.12 and below, default project lables are "warn", "audit"
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/enforce\":\"restricted\""))

		g.By("3. Create a standalone pod with oc run")
		output, err = oc.Run("run").Args("hello-openshift", "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("Warning"))
		e2e.Logf("Waiting for all pods of hello-openshift application to be ready ...")
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, testNameSpace, 30*time.Second, 10*time.Minute)

		g.By("4. Opt out of autolabelling")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", testNameSpace, "security.openshift.io/scc.podSecurityLabelSync=false").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Adding label to the namespace failed")

		g.By("5. Add privileged scc to user")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default", "-n", testNameSpace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("6. Check labels")
		output, err = oc.Run("get").Args("project", testNameSpace, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/enforce\":\"restricted\""))
		o.Expect(output).To(o.ContainSubstring("\"security.openshift.io/scc.podSecurityLabelSync\":\"false\""))

		g.By("7. Opt into autolabelling explicitly")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", testNameSpace, "security.openshift.io/scc.podSecurityLabelSync=true", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Adding label to the namespace failed")

		g.By("8. Check labels")
		output, err = oc.Run("get").Args("project", testNameSpace, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/enforce\":\"privileged\""))
		o.Expect(output).To(o.ContainSubstring("\"security.openshift.io/scc.podSecurityLabelSync\":\"true\""))

		g.By("9. Remove the privileged scc from user")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-scc-from-user", "privileged", "-z", "default", "-n", testNameSpace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("10. Add the restricted scc to the user")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "restricted", "-z", "default", "-n", testNameSpace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("11. Check labels")
		output, err = oc.Run("get").Args("project", testNameSpace, "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/enforce\":\"baseline\""))
	})

	// author: yinzhou@redhat.com
	g.It("WRS-ConnectedOnly-Author:yinzhou-LEVEL0-High-10662-Cannot run process via user root in the container when using MustRunAsNonRoot as the RunAsUserStrategy", func() {
		oc.SetupProject()
		namespace := oc.Namespace()
		username := oc.Username()

		// "nonroot-v2" SCC has "MustRunAsNonRoot" as the RunAsUserStrategy, assigning it to the user
		defer oc.AsAdmin().Run("adm", "policy").Args("remove-scc-from-user", "nonroot-v2", username).Execute()
		err := oc.AsAdmin().Run("adm", "policy").Args("add-scc-to-user", "nonroot-v2", username).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		baseDir := exutil.FixturePath("testdata", "apiserverauth")
		podTemplate := filepath.Join(baseDir, "pod-scc-runAsUser.yaml")

		g.By("Create new pod with runAsUser=0")
		image := "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83"
		err = exutil.ApplyResourceFromTemplateWithNonAdminUser(oc, "--ignore-unknown-parameters=true", "-f", podTemplate, "-p", "NAME="+"pod10662-1", "NAMESPACE="+namespace, "IMAGE="+image, "USERID=0")
		o.Expect(err).Should(o.HaveOccurred())

		g.By("Create new pod without runAsUser specified and the pod's image is built to run as USER 0 by default")
		// Below image is built to run as USER 0 by default. This can be checked either by "podman inspect <image> | grep User" or "podman run <image> whoami"
		err = oc.Run("run").Args("pod10662-3", "--image", image, `--overrides={"spec":{"securityContext":{"fsGroup":1001}}}`).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check status of pod without runAsUser specified and the pod's image is built to run as USER 0 by default")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("describe").Args("pod/pod10662-3").Output()
			if err != nil {
				e2e.Logf("Fail to describe the pod status, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("container has runAsNonRoot and image will run as root", output); matched {
				e2e.Logf(`Got expected output "container has runAsNonRoot and image will run as root"`)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Timed out to get expected error message")

		g.By("Create new pod with runAsUser != 0")
		pod1 := exutil.Pod{Name: "pod10662-2", Namespace: namespace, Template: podTemplate, Parameters: []string{"IMAGE=" + image, "USERID=97777"}}
		pod1.Create(oc)
		g.By("Check status of pod with runAsUser != 0")
		output, err := oc.Run("get").Args("pod/pod10662-2", "-o=jsonpath=-o=jsonpath={.status.phase}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Running"))
	})

	// author: gkarager@redhat.com
	g.It("NonPreRelease-PreChkUpgrade-Author:gkarager-Medium-55213-Upgrade should succeed when custom SCC is created with readOnlyRootFilesystem set to true", func() {
		g.By("Create a custom SCC that has `readOnlyRootFilesystem: true`")
		baseDir := exutil.FixturePath("testdata", "apiserverauth")
		customSCC := filepath.Join(baseDir, "custom_scc.yaml")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f" + customSCC).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		// This case intentionally does not the post-upgrade check. Successful upgrade itself is the post-upgrade expected result.
	})

	// author: yinzhou@redhat.com
	g.It("WRS-Author:yinzhou-Medium-55675-Group member should not lose rights after other members join the group", func() {
		g.By("Creat new namespace")
		oc.SetupProject()
		user1Name := oc.Username()
		g.By("Creat new group")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("group", "g55675").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "new", "g55675").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Add first user to the group")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "add-users", "g55675", user1Name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Adding clusterrole to the group")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-group", "cluster-admin", "g55675").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-group", "cluster-admin", "g55675").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Waiting for 120 secs, at beginning may hit the error or success intermittently within 2 mins")
		time.Sleep(120 * time.Second)

		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("groups", "add-users", "g55675", "user2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
			for j := 1; j < 31; j++ {
				if oc.Run("get").Args("nodes").Execute() != nil {
					break
				}
				if j == 30 {
					// Continuous success for 30 times. This can help us believe user 1's right is stable
					return true, nil
				}
				time.Sleep(1 * time.Second)
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "After add member to group, hit err, member right is broken")
	})

	// author: dmukherj@redhat.com
	g.It("WRS-ConnectedOnly-Author:dmukherj-LEVEL0-High-47941-User should not be allowed to create privileged ephemeral container without required privileges", func() {
		g.By("1. Create a namespace as normal user")
		oc.SetupProject()
		testNamespace := oc.Namespace()
		username := oc.Username()

		g.By("2. Changing the pod security profile to privileged")
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", testNamespace, "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "pod-security.kubernetes.io/audit=privileged", "pod-security.kubernetes.io/warn=privileged", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Adding label to namespace failed")

		g.By("3. Creating new role for ephemeral containers")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("role", "role-ephemeralcontainers", "--verb=get,list,watch,update,patch", "--resource=pods/ephemeralcontainers", "-n", testNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "role role-ephemeralcontainers creation failed")

		g.By("4. Adding role to the user")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-role-to-user", "role-ephemeralcontainers", username, "--role-namespace", testNamespace, "-n", testNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "policy addition of role-ephemeralcontainers to testuser failed")

		g.By("5. Running the hello-openshift image")
		output, err := oc.Run("run").Args("-n", testNamespace, "hello-openshift", "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("Warning"))
		e2e.Logf("Waiting for hello-openshift pod to be ready ...")
		exutil.AssertAllPodsToBeReadyWithPollerParams(oc, testNamespace, 30*time.Second, 10*time.Minute)

		baseDir := exutil.FixturePath("testdata", "apiserverauth")
		podJson := filepath.Join(baseDir, "sample-pod-ephemeral-container-complex.json")

		//It should fail, currently there is a bug (https://issues.redhat.com/browse/OCPBUGS-7181) associated with it
		err = oc.Run("replace").Args("--raw", "/api/v1/namespaces/"+testNamespace+"/pods/hello-openshift/ephemeralcontainers", "-f", podJson).Execute()
		o.Expect(err).To(o.HaveOccurred(), "Addition of privileged ephemeral containers without required privileges must fail")

		g.By("6. Adding scc to the user")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "default", "-n", testNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "policy addition of scc as privileged in the namespace failed")

		err = oc.Run("replace").Args("--raw", "/api/v1/namespaces/"+testNamespace+"/pods/hello-openshift/ephemeralcontainers", "-f", podJson).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Addition of privileged ephemeral containers failed")
		// It needs more time to wait for Ephemeral Container to come to Running state, so the Poll parameters are larger
		err = wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("po", "-o", "jsonpath={.items[*].status.ephemeralContainerStatuses}").Output()
			if err != nil {
				e2e.Logf("Fail to describe the container status, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("running", output); matched {
				e2e.Logf("Ephemeral Container is in Running state:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to check the ephemeral container status")

		output, err = oc.Run("rsh").Args("-n", testNamespace, "-c", "ephemeral-pod-debugger", "hello-openshift", "ps", "-eF").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to rsh into the ephemeral container")
		o.Expect(output).To(o.ContainSubstring("sleep 360d"))
		o.Expect(output).To(o.ContainSubstring("/hello_openshift"))

		output, err = oc.Run("logs").Args("-n", testNamespace, "-c", "ephemeral-pod-debugger", "hello-openshift").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "logging of container ephemeral-pod-debugger failed")
		o.Expect(output).To(o.ContainSubstring("root"))

		g.By("7. Removing scc from the user")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-scc-from-user", "privileged", "-z", "default", "-n", testNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "policy removal of scc as privileged in the namespace failed")

		err = oc.Run("rsh").Args("-n", testNamespace, "-c", "ephemeral-pod-debugger", "hello-openshift").Execute()
		o.Expect(err).To(o.HaveOccurred(), "rsh into ephemeral containers without required privileges should fail")
	})

	// author: zxiao@redhat.com
	g.It("ROSA-ARO-OSD_CCS-Author:zxiao-LEVEL0-High-22470-The basic challenge will be shown when user pass the X-CSRF-TOKEN http header", func() {
		e2e.Logf("Using OpenShift cluster with a default identity provider that supports 'challenge: true'")

		g.By("1. Get authentication url")
		rawAuthServerJson, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("--raw=/.well-known/oauth-authorization-server").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		authUrl := gjson.Get(rawAuthServerJson, `issuer`).String()
		o.Expect(authUrl).To(o.ContainSubstring("https://"))

		g.By("2. Check if the basic challenge will be shown when no X-CSRF-TOKEN http header")
		proxy := ""
		var proxyURL *url.URL
		if os.Getenv("http_proxy") != "" {
			proxy = os.Getenv("http_proxy")
		} else if os.Getenv("https_proxy") != "" {
			proxy = os.Getenv("https_proxy")
		}

		if proxy != "" {
			proxyURL, err = url.Parse(proxy)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			proxyURL = nil
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		requestURL := authUrl + "/oauth/authorize?response_type=token&client_id=openshift-challenging-client"
		respond1, err := httpClient.Get(requestURL)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer respond1.Body.Close()
		o.Expect(respond1.StatusCode).To(o.Equal(401))
		body, err := ioutil.ReadAll(respond1.Body)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(body)).To(o.MatchRegexp(`A non-empty X-CSRF-Token header is required to receive basic-auth challenges`))

		g.By("3. Check if the basic challenge will be shown when give X-CSRF-TOKEN http header")
		request, err := http.NewRequest("GET", requestURL, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		request.Header.Set("X-CSRF-Token", "1")

		respond2, err := httpClient.Do(request)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer respond2.Body.Close()
		o.Expect(respond2.StatusCode).To(o.Equal(401))
		respondAuthHeader := respond2.Header.Get("Www-Authenticate")
		o.Expect(respondAuthHeader).To(o.ContainSubstring(`Basic realm="openshift"`))
	})

	//author: dmukherj@redhat.com
	g.It("Author:dmukherj-LEVEL0-Critical-52452-Payload namespaces do not respect pod security admission autolabel's opt-in/opt-out", func() {
		g.By("1. Check the project labels for payload namespace openshift-monitoring")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("project", "openshift-monitoring", "-o=jsonpath={.metadata.labels}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/audit\":\"privileged\""))
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/enforce\":\"privileged\""))
		o.Expect(output).To(o.ContainSubstring("\"pod-security.kubernetes.io/warn\":\"privileged\""))

		g.By("2. Opt out of the autolabelling explicitly")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", "openshift-monitoring", "security.openshift.io/scc.podSecurityLabelSync=false", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred(), "Opt out of the autolabelling in openshift-monitoring namespace failed")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", "openshift-monitoring", "security.openshift.io/scc.podSecurityLabelSync-").Execute()

		g.By("3. Change any one label's value")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", "openshift-monitoring", "pod-security.kubernetes.io/warn=restricted", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4. Wait for some time till the project labels for openshift-monitoring namespace are restored to original values")
		err = wait.Poll(15*time.Second, 5*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("project", "openshift-monitoring", "-o=jsonpath={.metadata.labels}").Output()
			if err != nil {
				e2e.Logf("Failed to get project labels of openshift-monitoring namespace: %s. Trying again", err)
				return false, nil
			}
			matched1, _ := regexp.MatchString("\"pod-security.kubernetes.io/audit\":\"privileged\"", output)
			matched2, _ := regexp.MatchString("\"pod-security.kubernetes.io/enforce\":\"privileged\"", output)
			matched3, _ := regexp.MatchString("\"pod-security.kubernetes.io/warn\":\"privileged\"", output)
			if matched1 && matched2 && matched3 {
				e2e.Logf("Changes of pod-security.kubernetes.io labels are auto reverted to original values\n")
				return true, nil
			} else {
				e2e.Logf("Changes of pod-security.kubernetes.io labels are not auto reverted to original values yet\n")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Timed out to check whether the pod-security.kubernetes.io labels of openshift-monitoring namespace are restored or not")
	})
})
