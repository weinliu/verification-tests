package workloads

import (
	"context"
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
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-apps] Workloads", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: yinzhou@redhat.com
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:yinzhou-High-28001-bug 1749478 KCM should recover when its temporary secrets are deleted [Disruptive]", func() {
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
		err = oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", oc.Namespace(), "--import-mode=PreserveOriginal").Execute()
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
		err = oc.WithoutNamespace().Run("delete").Args("is", "--all", "-n", oc.Namespace()).Execute()
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
		err = oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "p43092-1", "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		checkPodStatus(oc, "deployment=hello-openshift", "p43092-1", "Running")

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
		err = oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", "p43099", "--import-mode=PreserveOriginal").Execute()
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
	g.It("HyperShiftMGMT-Author:yinzhou-High-43035-KCM use internal LB to avoid outages during kube-apiserver rollout [Disruptive]", func() {
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
	g.It("HyperShiftMGMT-NonPreRelease-PstChkUpgrade-Author:yinzhou-Medium-55823-make sure split the route controllers out from OCM", func() {
		checkMessage := []string{
			"ingress-to-route",
			"ingress-ip",
		}

		g.By("Retreive pods from openshift-route-controller-manager namespace")
		routeControllerPodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-route-controller-manager", "-l", "app=route-controller-manager", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		routeControllerPodList := strings.Fields(routeControllerPodNames)

		if ok := waitForAvailableRsRunning(oc, "deployment", "route-controller-manager", "openshift-route-controller-manager", "3"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("route-controller-manager pods are not running as expected")
		}

		g.By("Check the ingress-ip and ingress-to-route are started under project openshift-route-controller-manager")
		for _, routeControllerPodName := range routeControllerPodList {
			out, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-route-controller-manager", "pod/"+routeControllerPodName).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, v := range checkMessage {
				if strings.Contains(out, v) {
					e2e.Logf("Find the expected log from the pod %v", routeControllerPodName)
					break
				}
			}
		}

		g.By("Check the ingress-ip and ingress-to-route should no see from OCM")
		out, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-controller-manager", "-l", "app=openshift-controller-manager-a").Output()
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
	// author: yinzhou@redhat.com
	g.It("HyperShiftMGMT-NonPreRelease-Longduration-Author:yinzhou-Low-60194-Make sure KCM KS operator is rebased onto the latest version of Kubernetes", func() {
		g.By("Get the latest version of Kubernetes")
		ocVersion, versionErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[0].status.nodeInfo.kubeletVersion}").Output()
		o.Expect(versionErr).NotTo(o.HaveOccurred())
		kubenetesVersion := strings.Split(strings.Split(ocVersion, "+")[0], "v")[1]
		kuberVersion := strings.Split(kubenetesVersion, ".")[0] + "." + strings.Split(kubenetesVersion, ".")[1]
		g.By("Check the KCM operator is rebased with latest version")
		kcmPodOprator, descKCMErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", "openshift-kube-controller-manager-operator", "-l", "app=kube-controller-manager-operator").Output()
		o.Expect(descKCMErr).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(kuberVersion, kcmPodOprator); !matched {
			e2e.Failf("KCM operator not rebased with latest Kubernetes\n")
		}
		g.By("Check the KS operator is rebased with latest version")
		ksPodOprator, descKSErr := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", "openshift-kube-scheduler-operator", "-l", "app=openshift-kube-scheduler-operator").Output()
		o.Expect(descKSErr).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(kuberVersion, ksPodOprator); !matched {
			e2e.Failf("KS operator not rebased with latest Kubernetes\n")
		}
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Critical-63962-Verify MaxUnavailableStatefulSet feature is available via TechPreviewNoUpgrade", func() {
		g.By("Check if the cluster is TechPreviewNoUpgrade")
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip for featuregate set as TechPreviewNoUpgrade")
		}
		// Get kubecontrollermanager pod name & check if the feature gate is enabled
		kcmPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-kube-controller-manager", "-l", "app=kube-controller-manager", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		kcmPodOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", kcmPodName, "-n", "openshift-kube-controller-manager", "-o=jsonpath={.spec.containers[0].args}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(kcmPodOut, "--feature-gates=MaxUnavailableStatefulSet=true")).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	// This is a techpreviewNoUpgrade feature, so added NonHyperShiftHOST
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:knarra-Critical-63694-Verify MaxUnavailableStatefulSet feature works fine", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		statefulset63694 := filepath.Join(buildPruningBaseDir, "statefulset_63694.yaml")

		g.By("Check if the cluster is TechPreviewNoUpgrade")
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip for featuregate not set as TechPreviewNoUpgrade")
		}
		// Create statefulset

		g.By("create new namespace")
		oc.SetupProject()
		ns63694 := oc.Namespace()

		ssCreationErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", statefulset63694, "-n", ns63694).Execute()
		o.Expect(ssCreationErr).NotTo(o.HaveOccurred())

		// Check all pod related to statefulset are running
		if ok := waitForAvailableRsRunning(oc, "statefulset", "web", ns63694, "5"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("All pods related to statefulset web are not running")
		}

		// Trigger a rolling upgrade by changing the image
		patch := `[{"op":"replace", "path":"/spec/template/spec/containers/0/image", "value":"quay.io/openshifttest/nginx-alpine@sha256:f78c5a93df8690a5a937a6803ef4554f5b6b1ef7af4f19a441383b8976304b4c"}]`
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("statefulset", "web", "-n", ns63694, "--type=json", "-p", patch).Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		// Run oc get pods command in background
		cmd2, backgroundBuf2, _, err := oc.AsAdmin().Run("get").Args("pods", "-n", ns63694, "-w").Background()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer cmd2.Process.Kill()

		// Verify that pods have been rolled out in order
		err = wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
			eventDetails := []string{"web-4.*ContainerCreating", "web-3.*ContainerCreating", "web-2.*ContainerCreating", "web-1.*ContainerCreating", "web-0.*ContainerCreating"}
			for _, event := range eventDetails {
				if matched, _ := regexp.MatchString(event, backgroundBuf2.String()); !matched {
					e2e.Logf("Waiting for all set of StatefulSet pods to be rolled out...\n")
					return false, nil
				}
			}
			e2e.Logf("StatefulSet pods have rolled out successfully")
			return true, nil
		})
		if err != nil {
			e2e.Logf("Backgroundbuf2 is %s", backgroundBuf2.String())
			e2e.Failf("Timeout waiting for StatefulSet pods rollout: %v\n", err)
		}

	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-26247-Alert when pod has a PodDisruptionBudget with minAvailable 1 disruptionsAllowed 0", func() {
		ns26247 := oc.Namespace()

		g.By("create deploy")
		deployCreationErr := oc.WithoutNamespace().Run("create").Args("deployment", "deploy26247", "-n", ns26247, "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(deployCreationErr).NotTo(o.HaveOccurred())

		g.By("create pdb")
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("poddisruptionbudget", "pdb26247", "--selector=app=deploy26247", "--min-available=1", "-n", ns26247).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check PodDisruptionBudgetAtLimit alert")
		token, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PodDisruptionBudgetAtLimit"}'`, token, "pdb26247", 600)

		g.By("scale the deploy to replicas 0")
		err = oc.Run("scale").Args("deploy", "deploy26247", "--replicas=0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("check PodDisruptionBudgetLimit alert")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PodDisruptionBudgetLimit"}'`, token, "pdb26247", 600)
	})
	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:yinzhou-High-67765-Make sure rolling update logic to exclude unsetting nodes [Serial]", func() {
		if exutil.IsSNOCluster(oc) {
			g.Skip("It is sno cluster, so skip it")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns67765 := oc.Namespace()

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		daemonSetYaml := filepath.Join(buildPruningBaseDir, "daemonset-origin.yaml")
		daemonSetUpdateYaml := filepath.Join(buildPruningBaseDir, "daemonset-update.yaml")

		g.By("Set taint for the first node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", nodeList.Items[0].Name, "dedicated:NoSchedule-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "node", nodeList.Items[0].Name, "dedicated=special-user:NoSchedule").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Create the daemonset with toleration")
		err = oc.Run("create").Args("-f", daemonSetYaml, "-n", ns67765).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the daemonset should be deployed to all work nodes")
		waitForDaemonsetPodsToBeReady(oc, ns67765, "hello-openshift")
		daemonsetRunningNum := getDaemonsetDesiredNum(oc, ns67765, "hello-openshift")
		o.Expect(daemonsetRunningNum).To(o.Equal(len(nodeList.Items)))

		g.By("Update the daemonset with non-exist works toleration")
		err = oc.Run("replace").Args("-f", daemonSetUpdateYaml, "-n", ns67765).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the deamomset should be deployed to all exist work nodes")
		waitForDaemonsetPodsToBeReady(oc, ns67765, "hello-openshift")
		daemonsetRunningNumNew := getDaemonsetDesiredNum(oc, ns67765, "hello-openshift")
		o.Expect(daemonsetRunningNumNew).To(o.Equal(len(nodeList.Items) - 1))
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-69072-Infinite PODs loop creation with NodeAffinity status [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		project69072Yaml := filepath.Join(buildPruningBaseDir, "project-69072.yaml")
		deployment69072Yaml := filepath.Join(buildPruningBaseDir, "deployment-69072.yaml")

		g.By("Create new namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", "infinite-pod-creation-69072").Execute()
		projectCreationErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", project69072Yaml).Execute()
		o.Expect(projectCreationErr).NotTo(o.HaveOccurred())

		g.By("Create deployment")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("deployment", "infinite-pod-creation-69072").Execute()
		deploymentCreationErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deployment69072Yaml, "-n", "infinite-pod-creation-69072").Execute()
		o.Expect(deploymentCreationErr).NotTo(o.HaveOccurred())

		g.By("Verify that pods created by deployment are running")
		checkPodStatus(oc, "app=infinite-pod-creation-69072", "infinite-pod-creation-69072", "Running")

		g.By("Retreive master node name from the cluster")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		g.By("Patch the deployment with the node name other than worker node")
		patchOptions := fmt.Sprintf(`{"spec":{"template":{"spec":{"nodeName": "%s"}}}}`, masterNodes[0])
		_, patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", "-n", "infinite-pod-creation-69072", "infinite-pod-creation-69072", "--type=merge", "-p", patchOptions).Output()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		g.By("Verify that no pods with status NodeAffinity present")
		podStatusOutput, podOutErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "infinite-pod-creation-69072").Output()
		o.Expect(podOutErr).NotTo(o.HaveOccurred())
		o.Expect(podStatusOutput).NotTo(o.ContainSubstring("NodeAffinity"))
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-69211-Validate no deployer sa when baselineCapabilitySet to None [Serial]", func() {
		// Skip the test if baselinecaps is set to v4.13 or v4.14
		if isBaselineCapsSet(oc, "None") && !isEnabledCapability(oc, "Build") && !isEnabledCapability(oc, "DeploymentConfig") {
			// Verify deployer sa is not present
			saOutput, saErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", "-A").Output()
			o.Expect(saErr).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(saOutput, "deployer")).To(o.BeFalse())
			g.By("Create a new project & verify that there is no builder and deployer sa's present")
			oc.SetupProject()
			projectSAOutput, projectSAError := oc.AsAdmin().WithoutNamespace().Run("get").Args("sa", "-n", oc.Namespace()).Output()
			o.Expect(projectSAError).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(projectSAOutput, "deployer")).To(o.BeFalse())
			o.Expect(strings.Contains(projectSAOutput, "builder")).To(o.BeFalse())
		}
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-69870-ClusterResourceQuota should not been stuck in delete state when using foreground deletion cascading strategy [Flaky]", func() {
		workloadsBaseDir := exutil.FixturePath("testdata", "workloads")
		templateYaml := filepath.Join(workloadsBaseDir, "clusterresroucequota.yaml")

		g.By("Create the cluster resource quota")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", templateYaml).Execute()
		_, temOutErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", templateYaml).Output()
		o.Expect(temOutErr).NotTo(o.HaveOccurred())
		g.By("Delete the cluster resource quota")
		temDeleteErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("--cascade=foreground", "clusterresourcequota", "blue").Execute()
		o.Expect(temDeleteErr).NotTo(o.HaveOccurred())
		_, temOutput, temOutErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterresourcequota", "blue").Outputs()
		o.Expect(temOutErr).To(o.HaveOccurred())
		o.Expect(temOutput).To(o.ContainSubstring("not found"))
	})

})

var _ = g.Describe("[sig-cli] Workloads kube-controller-manager on Microshift", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	// author: knarra@redhat.com
	g.It("MicroShiftOnly-Author:knarra-Medium-56673-Enable the Openshift flavor of the kube-controller-manager [Disruptive]", func() {
		g.By("Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		defer func() {
			_, removalStatusErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo rm -rf /etc/microshift/config.yaml")
			o.Expect(removalStatusErr).NotTo(o.HaveOccurred())
			restartMicroshiftService(oc, masterNodes[0])
		}()

		g.By("Copy config.yaml.default to config.yaml and modify loglevel")
		mConfigCmd := fmt.Sprintf(`
sudo cp /etc/microshift/config.yaml.default /etc/microshift/config.yaml
cat > /etc/microshift/config.yaml << EOF
debugging:
  logVLevel: 2
EOF`)

		_, contentCreationStatusErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", mConfigCmd)
		o.Expect(contentCreationStatusErr).NotTo(o.HaveOccurred())

		g.By("Restart microshift service")
		restartMicroshiftService(oc, masterNodes[0])

		g.By("check if openshift context is nil")
		openshiftContextStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo journalctl -u microshift | grep kube-controller-manager | grep openshift-config")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(openshiftContextStatus, "--openshift-config=\"\"")).To(o.BeTrue())

		g.By("Verify if all the flags listed below are present")
		kcmFlags, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo journalctl -u microshift | grep kube-controller-manager | grep FLAG")
		o.Expect(err).NotTo(o.HaveOccurred())

		kcmFlagDetails := []string{"--enable-dynamic-provisioning=\"true\"", "--allocate-node-cidrs=\"true\"", "--configure-cloud-routes=\"false\"", "--use-service-account-credentials=\"true\"", "--leader-elect=\"false\"", "--leader-elect-retry-period=\"3s\"", "--leader-elect-resource-lock=\"leases\"", "--controllers=\"[*,-bootstrapsigner,-tokencleaner,-ttl]\"", "--cluster-signing-duration=\"720h0m0s\"", "--secure-port=\"10257\"", "--cert-dir=\"/var/run/kubernetes\"", "--root-ca-file=\"/var/lib/microshift/certs/ca-bundle/service-account-token-ca.crt\"", "--service-account-private-key-file=\"/var/lib/microshift/resources/kube-apiserver/secrets/service-account-key/service-account.key\"", "--cluster-signing-cert-file=\"/var/lib/microshift/certs/kubelet-csr-signer-signer/csr-signer/ca.crt\"", "--cluster-signing-key-file=\"/var/lib/microshift/certs/kubelet-csr-signer-signer/csr-signer/ca.key\"", "--kube-api-qps=\"150\"", "--kube-api-burst=\"300\""}
		for _, kcmFlag := range kcmFlagDetails {
			o.Expect(strings.Contains(kcmFlags, kcmFlag)).To(o.BeTrue())
		}

	})

})
