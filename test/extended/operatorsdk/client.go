package operatorsdk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// CLI provides function to call the Operator-sdk CLI
type CLI struct {
	execPath        string
	verb            string
	username        string
	globalArgs      []string
	commandArgs     []string
	finalArgs       []string
	stdin           *bytes.Buffer
	stdout          io.Writer
	stderr          io.Writer
	verbose         bool
	showInfo        bool
	skipTLS         bool
	ExecCommandPath string
	env             []string
}

// NewOperatorSDKCLI initialize the SDK framework
func NewOperatorSDKCLI() *CLI {
	client := &CLI{}
	client.username = "admin"
	client.execPath = "operator-sdk"
	client.showInfo = true
	return client
}

// NewMakeCLI initialize the make framework
func NewMakeCLI() *CLI {
	client := &CLI{}
	client.username = "admin"
	client.execPath = "make"
	client.showInfo = true
	return client
}

// NewMVNCLI initialize the make framework
func NewMVNCLI() *CLI {
	client := &CLI{}
	client.username = "admin"
	client.execPath = "mvn"
	client.showInfo = true
	return client
}

// Run executes given OperatorSDK command verb
func (c *CLI) Run(commands ...string) *CLI {
	in, out, errout := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
	operatorsdk := &CLI{
		execPath:        c.execPath,
		verb:            commands[0],
		username:        c.username,
		showInfo:        c.showInfo,
		ExecCommandPath: c.ExecCommandPath,
		env:             c.env,
	}
	if c.skipTLS {
		operatorsdk.globalArgs = append([]string{"--skip-tls=true"}, commands...)
	} else {
		operatorsdk.globalArgs = commands
	}
	operatorsdk.stdin, operatorsdk.stdout, operatorsdk.stderr = in, out, errout
	return operatorsdk.setOutput(c.stdout)
}

// setOutput allows to override the default command output
func (c *CLI) setOutput(out io.Writer) *CLI {
	c.stdout = out
	return c
}

// Args sets the additional arguments for the OpenShift CLI command
func (c *CLI) Args(args ...string) *CLI {
	c.commandArgs = args
	c.finalArgs = append(c.globalArgs, c.commandArgs...)
	return c
}

func (c *CLI) printCmd() string {
	return strings.Join(c.finalArgs, " ")
}

// ExitError returns the error info
type ExitError struct {
	Cmd    string
	StdErr string
	*exec.ExitError
}

// FatalErr exits the test in case a fatal error has occurred.
func FatalErr(msg interface{}) {
	// the path that leads to this being called isn't always clear...
	fmt.Fprintln(g.GinkgoWriter, string(debug.Stack()))
	e2e.Failf("%v", msg)
}

// Output executes the command and returns stdout/stderr combined into one string
func (c *CLI) Output() (string, error) {
	if c.verbose {
		e2e.Logf("DEBUG: opm %s\n", c.printCmd())
	}
	cmd := exec.Command(c.execPath, c.finalArgs...)
	cmd.Env = os.Environ()
	if c.env != nil {
		cmd.Env = append(cmd.Env, c.env...)
	}
	if c.ExecCommandPath != "" {
		e2e.Logf("set exec command path is %s\n", c.ExecCommandPath)
		cmd.Dir = c.ExecCommandPath
	}
	cmd.Stdin = c.stdin
	if c.showInfo {
		e2e.Logf("Running '%s %s'", c.execPath, strings.Join(c.finalArgs, " "))
	}
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	switch err := err.(type) {
	case nil:
		c.stdout = bytes.NewBuffer(out)
		return trimmed, nil
	case *exec.ExitError:
		e2e.Logf("Error running %v:\n%s", cmd, trimmed)
		return trimmed, &ExitError{ExitError: err, Cmd: c.execPath + " " + strings.Join(c.finalArgs, " "), StdErr: trimmed}
	default:
		FatalErr(fmt.Errorf("unable to execute %q: %v", c.execPath, err))
		// unreachable code
		return "", nil
	}
}

// The method is to get random string with length 8.
func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func replaceContent(filePath, src, target string) {
	input, err := ioutil.ReadFile(filePath)
	if err != nil {
		FatalErr(fmt.Errorf("read file %s failed: %v", filePath, err))
	}
	output := bytes.Replace(input, []byte(src), []byte(target), -1)
	if err = ioutil.WriteFile(filePath, output, 0o755); err != nil {
		FatalErr(fmt.Errorf("write file %s failed: %v", filePath, err))
	}
}

func getContent(filePath string) string {
	content, err := ioutil.ReadFile(filePath)
	o.Expect(err).NotTo(o.HaveOccurred())
	return string(content)
}

func insertContent(filePath, src, insertStr string) {
	input, err := ioutil.ReadFile(filePath)
	if err != nil {
		FatalErr(fmt.Errorf("read file %s failed: %v", filePath, err))
	}
	contents := string(input)
	lines := strings.Split(contents, "\n")
	var newLines []string
	for _, line := range lines {
		newLines = append(newLines, line)
		if strings.Contains(line, src) {
			newLines = append(newLines, insertStr)
		}
	}
	output := []byte(strings.Join(newLines, "\n"))
	if err = ioutil.WriteFile(filePath, output, 0o755); err != nil {
		FatalErr(fmt.Errorf("write file %s failed: %v", filePath, err))
	}
}

func copy(src, target string) error {
	bytesRead, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(target, bytesRead, 0o755)
	if err != nil {
		return err
	}
	return nil
}

func notInList(target string, strArray []string) bool {
	for _, element := range strArray {
		if target == element {
			return false
		}
	}
	return true
}

func logDebugInfo(oc *exutil.CLI, ns string, resource ...string) {
	for _, resourceIndex := range resource {
		e2e.Logf("oc get %s:", resourceIndex)
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(resourceIndex, "-n", ns).Output()
		if strings.Contains(resourceIndex, "event") {
			var warningEventList []string
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if strings.Contains(line, "Warning") {
					warningStr := strings.Split(line, "Warning")[1]
					if notInList(warningStr, warningEventList) {
						warningEventList = append(warningEventList, "Warning"+warningStr)
					}
				}
			}
			e2e.Logf(strings.Join(warningEventList, "\n"))
		} else {
			e2e.Logf(output)
		}
	}
}

// The method is to create one resource with template
func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 15*time.Second, false, func(ctx context.Context) (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "olm-config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("can not process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

type clusterrolebindingDescription struct {
	name      string
	namespace string
	saname    string
	template  string
}

// The method is to create role with template
func (clusterrolebinding *clusterrolebindingDescription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", clusterrolebinding.template,
		"-p", "NAME="+clusterrolebinding.name, "NAMESPACE="+clusterrolebinding.namespace, "SA_NAME="+clusterrolebinding.saname)
	o.Expect(err).NotTo(o.HaveOccurred())
}
