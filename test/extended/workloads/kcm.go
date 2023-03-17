package workloads

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-apps] Workloads", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-Longduration-Author:yinzhou-High-28001-bug 1749478 KCM should recover when its temporary secrets are deleted [Disruptive]", func() {
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
		var refer string
		err = wait.Poll(2*time.Second, 20*time.Second, func() (bool, error) {
			refer, err = oc.WithoutNamespace().Run("get").Args("rs", "-o=jsonpath={.items[0].metadata.ownerReferences}", "-n", "p43092-1").Output()
			if err != nil {
				e2e.Logf("Fail to get rs, error: %s. Trying again", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "RS not found")

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
		o.Expect(err).NotTo(o.HaveOccurred())
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
		err = wait.Poll(20*time.Second, 200*time.Second, func() (bool, error) {
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
		exutil.AssertWaitPollNoErr(err, "Failed to get the events")
		g.By("check the clusterrole should not be deleted")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole", "foo43099").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("foo43099", output); matched {
			e2e.Logf("Still could find the clusterrole:\n%s", output)
		}
	})

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-Author:yinzhou-High-43035-KCM use internal LB to avoid outages during kube-apiserver rollout [Disruptive]", func() {
		infra, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructures", "cluster", "-o=jsonpath={.status.infrastructureTopology}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if infra == "SingleReplica" {
			g.Skip("This is a SNO cluster, skip.")
		}
		g.By("Get the route")
		output, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-server").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeS := strings.Split(strings.Split(output, "api")[1], ":")[0]
		internalLB := "server: https://api-int" + routeS

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
		defer exutil.DebugNodeWithChroot(oc, leaderKcm, "mv", "/home/kube-apiserver-pod.yaml", "/etc/kubernetes/manifests/")
		_, err = exutil.DebugNodeWithChroot(oc, leaderKcm, "mv", "/etc/kubernetes/manifests/kube-apiserver-pod.yaml", "/home/")
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

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:yinzhou-Medium-55823-make sure split the route controllers out from OCM", func() {
		g.By("Check the ingress-ip and ingress-to-route are started under project openshift-route-controller-manager")
		out, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-route-controller-manager", "-l", "app=route-controller-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage := []string{
			"ingress-to-route",
			"ingress-ip",
		}
		for _, v := range checkMessage {
			if !strings.Contains(out, v) {
				e2e.Failf("can't see route contrller on openshift-route-controller-manager")
			}
		}
		g.By("Check the ingress-ip and ingress-to-route should no see from OCM")
		out, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-controller-manager", "-l", "app=openshift-controller-manager-a").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, v := range checkMessage {
			if strings.Contains(out, v) {
				e2e.Failf("Still see the controller on openshift-controller-manager")
			}
		}
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Medium-56176-oc debug cronjob should fail with a meaningful error", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		cronjobF := filepath.Join(buildPruningBaseDir, "cronjob56176.yaml")
		g.By("create new namespace")
		oc.SetupProject()

		g.By("create cronjob")
		err := oc.Run("create").Args("-f", cronjobF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("check if cronjob has been created")
		cronCreationOutput, err := oc.Run("get").Args("cronjob").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(cronCreationOutput).To(o.ContainSubstring("cronjob56176"))
		debugErr, err := oc.Run("debug").Args("cronjob/cronjob56176").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(debugErr).NotTo(o.ContainSubstring("v1.CronJob is not supported by debug"))
	})

	// author: knarra@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:knarra-Medium-56179-KCM Alert PodDisruptionBudget At and Limit do not alert with maxUnavailable or MinAvailable by percentage [Disruptive]", func() {
		isSNO := exutil.IsSNOCluster(oc)
		if isSNO {
			g.Skip("Skip Testing on SNO since there is no quorum guard pod available here")
		}
		g.By("Get the first master nodename")
		masterNodeName, err := exutil.GetFirstMasterNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Corodon one of the master node")
		defer func() {
			uncordonErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", masterNodeName).Execute()
			o.Expect(uncordonErr).NotTo(o.HaveOccurred())
			checkPodStatus(oc, "app=guard", "openshift-etcd", "Running")
		}()

		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", masterNodeName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Get the quorum guard pod name on the master node which was cordoned")
		etcdPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-etcd", "-l", "app=guard", fmt.Sprintf(`-o=jsonpath={.items[?(@.spec.nodeName=='%s')].metadata.name}`, masterNodeName)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete one of the etcd quorum pod")
		podDeletionErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", etcdPodName, "-n", "openshift-etcd").Execute()
		o.Expect(podDeletionErr).NotTo(o.HaveOccurred())

		token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(5*time.Second, 100*time.Second, func() (bool, error) {
			output, _, err := oc.AsAdmin().NotShowInfo().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "prometheus-k8s-0", "-c", "prometheus", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=ALERTS").Outputs()
			if err != nil {
				e2e.Logf("Can't get alerts, error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("KubePodNotReady", output); matched {
				e2e.Logf("Verify that kubepodnotready alert has been triggered\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get kubepodnotready alert via prometheus"))
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Critical-54195-Enable CronJobTimeZone feature and verify that it works fine", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		createCronJob := filepath.Join(buildPruningBaseDir, "cronjob54195.yaml")
		cronJobIncorrectTz := filepath.Join(buildPruningBaseDir, "cronjob54195ic.yaml")
		cronJobNoTz := filepath.Join(buildPruningBaseDir, "cronjob54195notz.yaml")

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		//Test CronJobTimeZone

		g.By("Create cronJob With TimeZone")
		cronCreationErr := oc.Run("create").Args("-f", createCronJob).Execute()
		o.Expect(cronCreationErr).NotTo(o.HaveOccurred())

		g.By("Verify that cronJob has been created successfully")
		pollErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			cronjobCreated, cronDisplayErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "cronjob54195", "-n", oc.Namespace(), "-o=jsonpath={.metadata.name}").Output()
			if cronDisplayErr != nil {
				e2e.Logf("No cronjob is present: %s. Trying again", cronDisplayErr)
				return false, nil
			}
			if matched, _ := regexp.MatchString("cronjob54195", cronjobCreated); matched {
				e2e.Logf("Cronjob has been created successfully\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("No job has been created"))

		checkForTimezone, timeZoneDisplayErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "cronjob54195", "-n", oc.Namespace(), "-o=jsonpath={.spec.timeZone}").Output()
		o.Expect(timeZoneDisplayErr).NotTo(o.HaveOccurred())
		o.Expect(checkForTimezone).To(o.ContainSubstring("Asia/Calcutta"))

		g.By("Verify job has been created successfully")
		pollErr = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			jobName, jobCreationErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("job", "-n", oc.Namespace()).Output()
			if jobCreationErr != nil {
				e2e.Logf("No job is present: %s. Trying again", jobCreationErr)
				return false, nil
			}
			if matched, _ := regexp.MatchString("cronjob54195", jobName); matched {
				e2e.Logf("Job has been created successfully\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("No job has been created"))

		//Test CronJob with incorrect timeZone

		g.By("Verify that cronjob could not be created with incorrect time zone set")
		incorrectCreation, incorrectCreationErr := oc.Run("create").Args("-f", cronJobIncorrectTz).Output()
		o.Expect(incorrectCreationErr).To(o.HaveOccurred())
		o.Expect(incorrectCreation).To(o.ContainSubstring("unknown time zone Asia/china"))

		//Test CronJob with out timeZone param set

		g.By("Verify that cronjob could be created with out timeZone param")
		noTzCreationErr := oc.Run("create").Args("-f", cronJobNoTz).Execute()
		o.Expect(noTzCreationErr).NotTo(o.HaveOccurred())

		g.By("Verify that cronJob has been created successfully with out timezone param set")
		pollErr = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			cronjobCreatedNoTz, cronNoTzDisplayErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "cronjob54195notz", "-n", oc.Namespace(), "-o=jsonpath={.metadata.name}").Output()
			if cronNoTzDisplayErr != nil {
				e2e.Logf("No cronjob is present: %s. Trying again", cronNoTzDisplayErr)
				return false, nil
			}
			if matched, _ := regexp.MatchString("cronjob54195notz", cronjobCreatedNoTz); matched {
				e2e.Logf("Cronjob has been created successfully\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("No job has been created"))
	})

	// author: knarra@redhat.com
	g.It("Author:knarra-High-54196-Create cronjob by retreiving current time by its timezone", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		createCronJobT := filepath.Join(buildPruningBaseDir, "cronjob_54196.yaml")

		g.By("Retreive schedule and timeZoneName")
		schedule, timeZoneName := getTimeFromTimezone(oc)
		e2e.Logf("Schedule is %s", schedule)
		e2e.Logf("timeZoneName is %s", timeZoneName)
		o.Expect(schedule).NotTo(o.BeEmpty())
		o.Expect(timeZoneName).NotTo(o.BeEmpty())

		//Test CronJobTimeZone
		testCronJobTimeZone := cronJobCreationTZ{
			cName:     "cronjob54196",
			namespace: oc.Namespace(),
			schedule:  schedule,
			timeZone:  timeZoneName,
			template:  createCronJobT,
		}

		// Create test project
		g.By("Create cronJob With TimeZone")
		testCronJobTimeZone.createCronJobWithTimeZone(oc)

		g.By("Verify that cronJob has been created successfully")
		pollErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			cronjobCreated, cronCreationErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "cronjob54196", "-n", oc.Namespace(), "-o=jsonpath={.metadata.name}").Output()
			if cronCreationErr != nil {
				e2e.Logf("No cronjob is present: %s. Trying again", cronCreationErr)
				return false, nil
			}
			if matched, _ := regexp.MatchString("cronjob54196", cronjobCreated); matched {
				e2e.Logf("Cronjob has been created successfully\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("No cronjob has been created"))

		checkForTimezone, timeZoneDisplayErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjob", "cronjob54196", "-n", oc.Namespace(), "-o=jsonpath={.spec.timeZone}").Output()
		o.Expect(timeZoneDisplayErr).NotTo(o.HaveOccurred())
		o.Expect(checkForTimezone).To(o.ContainSubstring(timeZoneName))

		g.By("Verify job has been created")
		pollErr = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			jobName, jobCreationErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("job", "-n", oc.Namespace()).Output()
			if jobCreationErr != nil {
				e2e.Logf("No job is present: %s. Trying again", jobCreationErr)
				return false, nil
			}
			if matched, _ := regexp.MatchString("cronjob54196", jobName); matched {
				e2e.Logf("Job has been created successfully\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("No job has been created"))
	})
})
