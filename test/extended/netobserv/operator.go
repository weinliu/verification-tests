package netobserv

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/yaml"
)

// SubscriptionObjects objects are used to create operators via OLM
type SubscriptionObjects struct {
	OperatorName     string
	Namespace        string
	OperatorGroup    string // the file used to create operator group
	Subscription     string // the file used to create subscription
	PackageName      string
	CatalogSource    *CatalogSourceObjects `json:",omitempty"`
	OperatorPodLabel string
}

// CatalogSourceObjects defines the source used to subscribe an operator
type CatalogSourceObjects struct {
	Channel         string `json:",omitempty"`
	SourceName      string `json:",omitempty"`
	SourceNamespace string `json:",omitempty"`
}

// OperatorNamespace struct to handle creation of namespace
type OperatorNamespace struct {
	Name              string
	NamespaceTemplate string
}

type version struct {
	Operator struct {
		Branch  string `yaml:"branch"`
		TagName string `yaml:"tagName"`
	} `yaml:"operator"`
	FlowlogsPipeline struct {
		Image string `yaml:"image"`
	} `yaml:"flowlogs-pipeline"`
	ConsolePlugin struct {
		Image string `yaml:"image"`
	} `yaml:"consolePlugin"`
}

// deploy/undeploys network-observability operator given action is true/false
func (versions *version) deployNetobservOperator(action bool, tempdir *string) error {

	var (
		deployCmd string
		err       error
	)

	if action {
		err = versions.gitCheckout(tempdir)
		if err != nil {
			return err
		}
		defer os.RemoveAll(*tempdir)
		e2e.Logf("cloned git repo successfully at %s", *tempdir)
		var vers string
		if versions.Operator.TagName == "" {
			vers = "main"
		} else {
			vers = versions.Operator.TagName
		}
		deployCmd = "VERSION=" + vers + " make deploy"
	} else {
		e2e.Logf("undeploying operator")
		deployCmd = "make undeploy"
	}

	cmd := exec.Command("bash", "-c", fmt.Sprintf("cd %s && %s", *tempdir, deployCmd))
	err = cmd.Run()

	if err != nil {
		e2e.Logf("Failed action: %s for network-observability operator - err %s", deployCmd, err.Error())
		return err

	}
	return nil
}

// parses version.yaml and converts to version struct
func (versions *version) versionMap() error {
	componentVersions := "version.yaml"
	versionsFixture := exutil.FixturePath("testdata", "netobserv", componentVersions)
	vers, err := os.ReadFile(versionsFixture)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(vers, &versions)
	if err != nil {
		return err
	}

	e2e.Logf("versions in versionMap are %s", versions)

	return nil
}

// clones operator git repo and switches to tag if specified in version.yaml
func (versions *version) gitCheckout(tempdir *string) error {
	var err error
	*tempdir, _ = ioutil.TempDir("", "netobserv")
	operatorDir := "network-observability-operator"
	operatorRepo := fmt.Sprintf("https://github.com/netobserv/%s.git", operatorDir)

	repo, err := git.PlainClone(*tempdir, false, &git.CloneOptions{
		URL:           operatorRepo,
		ReferenceName: "refs/heads/main",
		SingleBranch:  true,
	})

	if err != nil {
		e2e.Logf("failed to clone git repo %s: %s", operatorRepo, err)
		return err
	}

	e2e.Logf("cloned git repo for %s successfully at %s", operatorDir, *tempdir)

	tree, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Checkout our tag
	if versions.Operator.TagName != "" {
		e2e.Logf("Deploying tag %s\n", versions.Operator.TagName)
		err = tree.Checkout(&git.CheckoutOptions{
			Branch: plumbing.ReferenceName("refs/tags/" + versions.Operator.TagName),
		})

		if err != nil {
			return err
		}
		os.Setenv("VERSION", versions.Operator.TagName)
	}
	return nil
}

// waitForPackagemanifestAppear waits for the packagemanifest to appear in the cluster
// chSource: bool value, true means the packagemanifests' source name must match the so.CatalogSource.SourceName, e.g.: oc get packagemanifests xxxx -l catalog=$source-name
func (so *SubscriptionObjects) waitForPackagemanifestAppear(oc *exutil.CLI, chSource bool) {
	args := []string{"-n", so.CatalogSource.SourceNamespace, "packagemanifests"}
	if chSource {
		args = append(args, "-l", "catalog="+so.CatalogSource.SourceName)
	} else {
		args = append(args, so.PackageName)
	}
	err := wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, false, func(context.Context) (done bool, err error) {
		packages, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
		if err != nil {
			msg := fmt.Sprintf("%v", err)
			if strings.Contains(msg, "No resources found") || strings.Contains(msg, "NotFound") {
				return false, nil
			}
			return false, err
		}
		if strings.Contains(packages, so.PackageName) {
			return true, nil
		}
		e2e.Logf("Waiting for packagemanifest/%s to appear", so.PackageName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Packagemanifest %s is not availabile", so.PackageName))
}

// setCatalogSourceObjects set the default values of channel, source namespace and source name if they're not specified
func (so *SubscriptionObjects) setCatalogSourceObjects(oc *exutil.CLI) {
	// set channel
	if so.CatalogSource.Channel == "" {
		so.CatalogSource.Channel = "stable"
	}

	// set source namespace
	if so.CatalogSource.SourceNamespace == "" {
		so.CatalogSource.SourceNamespace = "openshift-marketplace"
	}

	// set source and check if the packagemanifest exists or not
	if so.CatalogSource.SourceName != "" {
		so.waitForPackagemanifestAppear(oc, true)
	} else {
		catsrc, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("catsrc", "-n", so.CatalogSource.SourceNamespace, "qe-app-registry").Output()
		if catsrc != "" && !(strings.Contains(catsrc, "NotFound")) {
			so.CatalogSource.SourceName = "qe-app-registry"
			so.waitForPackagemanifestAppear(oc, true)
		} else {
			so.waitForPackagemanifestAppear(oc, false)
			source, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests", so.PackageName, "-o", "jsonpath={.status.catalogSource}").Output()
			if err != nil {
				e2e.Logf("error getting catalog source name: %v", err)
			}
			so.CatalogSource.SourceName = source
		}
	}
}

// SubscribeOperator is used to subcribe the CLO and EO
func (so *SubscriptionObjects) SubscribeOperator(oc *exutil.CLI) {
	// check if the namespace exists, if it doesn't exist, create the namespace
	_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), so.Namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			e2e.Logf("The project %s is not found, create it now...", so.Namespace)
			namespaceTemplate := exutil.FixturePath("testdata", "logging", "subscription", "namespace.yaml")
			namespaceFile, err := processTemplate(oc, "-f", namespaceTemplate, "-p", "NAMESPACE_NAME="+so.Namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(context.Context) (done bool, err error) {
				output, err := oc.AsAdmin().Run("apply").Args("-f", namespaceFile).Output()
				if err != nil {
					if strings.Contains(output, "AlreadyExists") {
						return true, nil
					}
					return false, err
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't create project %s", so.Namespace))
		}
	}

	// check the operator group, if no object found, then create an operator group in the project
	og, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", so.Namespace, "og").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	msg := fmt.Sprintf("%v", og)
	if strings.Contains(msg, "No resources found") {
		// create operator group
		ogFile, err := processTemplate(oc, "-n", so.Namespace, "-f", so.OperatorGroup, "-p", "OG_NAME="+so.Namespace, "NAMESPACE="+so.Namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(context.Context) (done bool, err error) {
			output, err := oc.AsAdmin().Run("apply").Args("-f", ogFile, "-n", so.Namespace).Output()
			if err != nil {
				if strings.Contains(output, "AlreadyExists") {
					return true, nil
				}
				return false, err
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't create operatorgroup %s in %s project", so.Namespace, so.Namespace))
	}

	// check subscription, if there is no subscription objets, then create one
	sub, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", so.Namespace, so.PackageName).Output()
	if err != nil {
		msg := fmt.Sprint("v%", sub)
		if strings.Contains(msg, "NotFound") {
			so.setCatalogSourceObjects(oc)
			//create subscription object
			subscriptionFile, err := processTemplate(oc, "-n", so.Namespace, "-f", so.Subscription, "-p", "PACKAGE_NAME="+so.PackageName, "NAMESPACE="+so.Namespace, "CHANNEL="+so.CatalogSource.Channel, "SOURCE="+so.CatalogSource.SourceName, "SOURCE_NAMESPACE="+so.CatalogSource.SourceNamespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 60*time.Second, false, func(context.Context) (done bool, err error) {
				output, err := oc.AsAdmin().Run("apply").Args("-f", subscriptionFile, "-n", so.Namespace).Output()
				if err != nil {
					if strings.Contains(output, "AlreadyExists") {
						return true, nil
					}
					return false, err
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can't create subscription %s in %s project", so.PackageName, so.Namespace))
		}
	}
	//WaitForDeploymentPodsToBeReady(oc, so.Namespace, so.OperatorName)
}

func deleteNamespace(oc *exutil.CLI, ns string) {
	err := oc.AdminKubeClient().CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	err = wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 180*time.Second, false, func(context.Context) (bool, error) {
		_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Namespace %s is not deleted in 3 minutes", ns))
}

func (so *SubscriptionObjects) uninstallOperator(oc *exutil.CLI) {
	resource{"subscription", so.PackageName, so.Namespace}.clear(oc)
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", so.Namespace, "csv", "-l", "operators.coreos.com/"+so.PackageName+"."+so.Namespace+"=").Execute()
	// do not remove namespace openshift-logging and openshift-operators-redhat, and preserve the operatorgroup as there may have several operators deployed in one namespace
	// for example: loki-operator and elasticsearch-operator
	if so.Namespace != "openshift-logging" && so.Namespace != "openshift-operators-redhat" && so.Namespace != "openshift-operators" && so.Namespace != "openshift-netobserv-operator" && !strings.HasPrefix(so.Namespace, "e2e-test-") {
		deleteNamespace(oc, so.Namespace)
	}
}

func checkOperatorChannel(oc *exutil.CLI, operatorNamespace string, operatorName string) (string, error) {
	channelName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", operatorName, "-n", operatorNamespace, "-o=jsonpath={.spec.channel}").Output()
	if err != nil {
		return "", err
	}
	return channelName, nil
}

func checkOperatorSource(oc *exutil.CLI, operatorNamespace string, operatorName string) (string, error) {
	channelName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", operatorName, "-n", operatorNamespace, "-o=jsonpath={.spec.source}").Output()
	if err != nil {
		return "", err
	}
	return channelName, nil
}

func checkOperatorStatus(oc *exutil.CLI, operatorNamespace string, operatorName string) bool {
	err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespace", operatorNamespace).Execute()
	if err == nil {
		err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", operatorName, "-n", operatorNamespace).Execute()
		if err1 == nil {
			csvName, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", operatorName, "-n", operatorNamespace, "-o=jsonpath={.status.installedCSV}").Output()
			o.Expect(err2).NotTo(o.HaveOccurred())
			o.Expect(csvName).NotTo(o.BeEmpty())
			err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 360*time.Second, false, func(context.Context) (bool, error) {
				csvState, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", csvName, "-n", operatorNamespace, "-o=jsonpath={.status.phase}").Output()
				if err != nil {
					return false, err
				}
				return csvState == "Succeeded", nil
			})
			return err == nil
		}
	}
	e2e.Logf("%s operator will be created by tests", operatorName)
	return false
}

func (ns *OperatorNamespace) deployOperatorNamespace(oc *exutil.CLI) {
	e2e.Logf("Creating Netobserv operator namespace")
	nsParameters := []string{"--ignore-unknown-parameters=true", "-f", ns.NamespaceTemplate}
	exutil.ApplyClusterResourceFromTemplate(oc, nsParameters...)
}
