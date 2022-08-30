package workloads

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-apps] Workloads", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: yinzhou@redhat.com
	g.It("Longduration-Author:yinzhou-High-28001-bug 1749478 KCM should recover when its temporary secrets are deleted [Disruptive]", func() {
		var namespace = "openshift-kube-controller-manager"
		var temporarySecretsList []string

		g.By("get all the secrets in kcm project")
		output, err := oc.AsAdmin().Run("get").Args("secrets", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		secretsList := strings.Fields(output)

		g.By("filter out all the none temporary secrets")
		for _, secretsname := range secretsList {
			secretsAnnotations, err := oc.AsAdmin().Run("get").Args("secrets", "-n", namespace, secretsname, "-o=jsonpath={.metadata.annotations}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString("kubernetes.io/service-account.name", secretsAnnotations); matched {
				continue
			} else {
				secretOwnerKind, err := oc.AsAdmin().Run("get").Args("secrets", "-n", namespace, secretsname, "-o=jsonpath={.metadata.ownerReferences[0].kind}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Compare(secretOwnerKind, "ConfigMap") == 0 {
					continue
				} else {
					temporarySecretsList = append(temporarySecretsList, secretsname)
				}
			}
		}

		g.By("delete all the temporary secrets")
		for _, secretsD := range temporarySecretsList {
			_, err = oc.AsAdmin().Run("delete").Args("secrets", "-n", namespace, secretsD).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Check the KCM operator should be in Progressing")
		e2e.Logf("Checking kube-controller-manager operator should be in Progressing in 100 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-controller-manager", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-controller-manager operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-controller-manager operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-controller-manager", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-controller-manager operator is not becomes available in 1500 seconds")
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-High-43039-openshift-object-counts quota dynamically updating as the resource is deleted", func() {
		g.By("Test for case OCP-43039 openshift-object-counts quota dynamically updating as the resource is deleted")
		g.By("create new namespace")
		oc.SetupProject()

		g.By("Create quota in the project")
		err := oc.AsAdmin().Run("create").Args("quota", "quota43039", "--hard=openshift.io/imagestreams=10", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the quota")
		output, err := oc.WithoutNamespace().Run("describe").Args("quota", "quota43039", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("openshift.io/imagestreams  0     10", output); matched {
			e2e.Logf("the quota is :\n%s", output)
		}

		g.By("create apps")
		err = oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("check the imagestream in the project")
		output, err = oc.WithoutNamespace().Run("get").Args("imagestream", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("hello-openshift", output); matched {
			e2e.Logf("the image stream is :\n%s", output)
		}

		g.By("check the quota again")
		output, err = oc.WithoutNamespace().Run("describe").Args("quota", "quota43039", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("openshift.io/imagestreams  1     10", output); matched {
			e2e.Logf("the quota is :\n%s", output)
		}

		g.By("delete all the resource")
		err = oc.WithoutNamespace().Run("delete").Args("all", "--all", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("make sure all the imagestream are deleted")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, err = oc.WithoutNamespace().Run("get").Args("is", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf("Fail to get is, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("No resources found", output); matched {
				e2e.Logf("ImageStream has been deleted:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "ImageStream has been not deleted")

		g.By("Check the quota")
		output, err = oc.WithoutNamespace().Run("describe").Args("quota", "quota43039", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("openshift.io/imagestreams  0     10", output); matched {
			e2e.Logf("the quota is :\n%s", output)
		}
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-43092-Namespaced dependents try to use cross-namespace owner references will be deleted", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deploydpT := filepath.Join(buildPruningBaseDir, "deploy_duplicatepodsrs.yaml")

		g.By("Create the first namespace")
		err := oc.WithoutNamespace().Run("new-project").Args("p43092-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().Run("delete").Args("project", "p43092-1").Execute()

		g.By("Create app in the frist project")
		err = oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "p43092-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get the rs references")
		refer, err := oc.WithoutNamespace().Run("get").Args("rs", "-o=jsonpath={.items[0].metadata.ownerReferences}", "-n", "p43092-1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the second namespace")
		err = oc.WithoutNamespace().Run("new-project").Args("p43092-2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().Run("delete").Args("project", "p43092-2").Execute()

		testrs := deployduplicatepods{
			dName:      "hello-openshift",
			namespace:  "p43092-2",
			replicaNum: 1,
			template:   deploydpT,
		}
		g.By("Create the test rs")
		testrs.createDuplicatePods(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("rs/hello-openshift", "-n", "p43092-2", "--type=json", "-p", "[{\"op\": \"add\" , \"path\" : \"/metadata/ownerReferences\", \"value\":"+refer+"}]").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("wait until the rs deleted")
		err = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().Run("get").Args("rs", "-n", "p43092-2").Output()
			if err != nil {
				e2e.Logf("Fail to get rs, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("No resources found", output); matched {
				e2e.Logf("RS has been deleted:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "RS has been not deleted")

		g.By("check the event")
		eve, err := oc.WithoutNamespace().Run("get").Args("events", "-n", "p43092-2").Output()
		if matched, _ := regexp.MatchString("OwnerRefInvalidNamespace", eve); matched {
			e2e.Logf("found the events :\n%s", eve)
		}
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-43099-Cluster-scoped dependents with namespaced kind owner references will trigger warning Event [Flaky]", func() {
		g.By("Create the first namespace")
		err := oc.WithoutNamespace().Run("new-project").Args("p43099").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().Run("delete").Args("project", "p43099").Execute()

		g.By("Create app in the frist project")
		err = oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "p43099").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get the rs references")
		refer, err := oc.WithoutNamespace().Run("get").Args("rs", "-o=jsonpath={.items[0].metadata.ownerReferences}", "-n", "p43099").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the clusterrole")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("clusterrole", "foo43099", "--verb=get,list,watch", "--resource=pods,pods/status").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole/foo43099").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterrole/foo43099", "--type=json", "-p", "[{\"op\": \"add\" , \"path\" : \"/metadata/ownerReferences\", \"value\":"+refer+"}]").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("wait until check the events")
		err = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", "default").Output()
			if err != nil {
				e2e.Logf("Fail to get events, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("Warning.*OwnerRefInvalidNamespace.*clusterrole/foo43099", output); matched {
				e2e.Logf("Found the event:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "clusterrole/foo43099 is not found")
		g.By("check the clusterrole should not be deleted")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole", "foo43099").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("foo43099", output); matched {
			e2e.Logf("Still could find the clusterrole:\n%s", output)
		}
	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-High-43035-KCM use internal LB to avoid outages during kube-apiserver rollout [Disruptive]", func() {
		g.By("Get the route")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route/console", "-n", "openshift-console", "-o=jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeS := strings.Split(output, "apps")
		internalLB := "server: https://api-int" + routeS[1]

		g.By("Check the configmap in project openshift-kube-controller-manager")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "controller-manager-kubeconfig", "-n", "openshift-kube-controller-manager", "-oyaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(internalLB, output); matched {
			e2e.Logf("use the internal LB :\n%s", output)
		} else {
			e2e.Failf("Does not use the internal LB: %v", output)
		}

		g.By("Get the master with KCM leader")
		leaderKcm := getLeaderKCM(oc)
		g.By("Remove the apiserver pod from KCM leader master")
		defer oc.AsAdmin().Run("debug").Args("node/"+leaderKcm, "--", "chroot", "/host", "mv", "/home/kube-apiserver-pod.yaml", "/etc/kubernetes/manifests/").Execute()
		err = oc.AsAdmin().Run("debug").Args("node/"+leaderKcm, "--", "chroot", "/host", "mv", "/etc/kubernetes/manifests/kube-apiserver-pod.yaml", "/home/").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-kube-apiserver", "pod/"+"kube-apiserver-"+leaderKcm).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the KCM operator")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().Run("get").Args("co", "kube-controller-manager").Output()
			if err != nil {
				e2e.Logf("Fail to get clusteroperator kube-controller-manager, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
				e2e.Logf("clusteroperator kube-controller-manager is abnormal:\n%s", output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "clusteroperator kube-controller-manager is abnormal")

	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-High-50255-make sure disabled JobTrackingWithFinalizers", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		cronjobF := filepath.Join(buildPruningBaseDir, "cronjob50255.yaml")
		g.By("create new namespace")
		oc.SetupProject()

		g.By("create cronjob")
		err := oc.Run("create").Args("-f", cronjobF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("check when pod running should not have finalizer")
		err = wait.Poll(5*time.Second, 600*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("pod", "-o=jsonpath='{.items[?(@.status.phase == \"Running\")].metadata.name}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString("cronjob50255", output); !matched {
				e2e.Logf("Failed to find running pods")
				return false, nil
			}
			result, err := oc.Run("get").Args("pod", "-o=jsonpath='{.items[?(@.status.phase == \"Running\")].metadata.finalizers}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("the finalizer output is %v", result)
			o.Expect(result).NotTo(o.ContainSubstring("job-tracking"))
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to get running pods for cronjob")
	})
	// author: yinzhou@redhat.com
	g.It("Longduration-NonPreRelease-Author:yinzhou-High-51843-PodDisruptionBudgetAtLimit should not Warning alert when CR replica count is zero", func() {
		g.By("create new namespace")
		oc.SetupProject()
		g.By("create app")
		ns := oc.Namespace()
		err := oc.WithoutNamespace().Run("new-app").Args("httpd", "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create pdb")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("poddisruptionbudget", "pdb51843", "--selector=deployment=httpd", "--max-unavailable=1", "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("scale down the app")
		err = oc.Run("scale").Args("deploy", "httpd", "--replicas=0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		g.By("check should no PodDisruptionBudgetAtLimit warning")
		err = wait.Poll(60*time.Second, 900*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", "\""+fmt.Sprintf("Authorization: Bearer %v", token)+"\"", "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query? --data-urlencode 'query=ALERTS{alertname=\"PodDisruptionBudgetAtLimit\"}'").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString(ns, output); matched {
				e2e.Logf("PodDisruptionBudgetAtLimit warning found for project %v", ns)
				return true, nil
			}
			e2e.Logf("Do not get alert , try next time")
			return false, nil

		})
		exutil.AssertWaitPollWithErr(err, "Could not get alert from prometheus")
	})

})
