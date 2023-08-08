package rosacli

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
)

type runner struct {
	cmds      []string
	cmdArgs   []string
	format    string
	color     string
	debug     bool
	sensitive bool
}

func NewRunner() *runner {
	runner := &runner{
		format: "text",
		debug:  false,
		color:  "auto",
	}

	return runner
}
func (r *runner) Sensitive(sensitive bool) *runner {
	r.sensitive = sensitive
	return r
}

func (r *runner) Debug(debug bool) *runner {
	r.debug = debug
	return r
}

func (r *runner) Color(color string) *runner {
	r.color = color
	return r
}

func (r *runner) OutputFormat() *runner {
	cmdArgs := r.cmdArgs
	if r.format == "json" || r.format == "yaml" {
		cmdArgs = append(cmdArgs, "--output", r.format)
	}

	r.cmdArgs = cmdArgs
	return r
}

func (r *runner) CloseFormat() *runner {
	r.format = "text"
	return r
}

func (r *runner) JsonFormat(jsonOutput bool) *runner {
	cmdArgs := r.cmdArgs
	if jsonOutput {
		cmdArgs = append(cmdArgs, "--output", "json")
	}

	r.cmdArgs = cmdArgs
	return r
}

func (r *runner) Cmd(commands ...string) *runner {
	r.cmds = commands
	return r
}

func (r *runner) CmdFlags(cmdFlags ...string) *runner {
	var cmdArgs []string
	cmdArgs = append(cmdArgs, cmdFlags...)
	if r.debug {
		cmdArgs = append(cmdArgs, "--debug")
	}
	if r.color != "auto" {
		cmdArgs = append(cmdArgs, "--color", r.color)
	}

	r.cmdArgs = cmdArgs
	return r
}

func (r *runner) AddCmdFlags(cmdFlags ...string) *runner {
	cmdArgs := append(r.cmdArgs, cmdFlags...)
	r.cmdArgs = cmdArgs
	return r
}

func (r *runner) UnsetBoolFlag(flag string) *runner {
	var newCmdArgs []string
	cmdArgs := r.cmdArgs
	for _, vv := range cmdArgs {
		if vv == flag {
			continue
		}
		newCmdArgs = append(newCmdArgs, vv)
	}

	r.cmdArgs = newCmdArgs
	return r
}

func (r *runner) UnsetFlag(flag string) *runner {
	cmdArgs := r.cmdArgs
	flagIndex := 0
	for n, key := range cmdArgs {
		if key == flag {
			flagIndex = n
			break
		}
	}

	cmdArgs = append(cmdArgs[:flagIndex], cmdArgs[flagIndex+2:]...)
	r.cmdArgs = cmdArgs
	return r
}

func (r *runner) ReplaceFlag(flag string, value string) *runner {
	cmdArgs := r.cmdArgs
	for n, key := range cmdArgs {
		if key == flag {
			cmdArgs[n+1] = value
			break
		}
	}

	r.cmdArgs = cmdArgs
	return r
}
func (r *runner) Run() (bytes.Buffer, error) {
	rosacmd := "rosa"
	cmdElements := r.cmds
	if len(r.cmdArgs) > 0 {
		cmdElements = append(cmdElements, r.cmdArgs...)
	}

	var output bytes.Buffer
	var err error
	retry := 0
	for {
		if retry > 4 {
			err = fmt.Errorf("executing failed: %s", output.String())
			return output, err
		}

		logger.Infof("Running command: rosa %s", strings.Join(cmdElements, " "))
		output.Reset()
		cmd := exec.Command(rosacmd, cmdElements...)
		cmd.Stdout = &output
		cmd.Stderr = cmd.Stdout

		err = cmd.Run()
		if !r.sensitive {
			logger.Infof("Get Combining Stdout and Stder is :\n%s", output.String())
		}

		if strings.Contains(output.String(), "Not able to get authentication token") {
			retry = retry + 1
			logger.Warnf("[Retry] Not able to get authentication token!! Wait and sleep 5s to do the %d retry", retry)
			time.Sleep(5 * time.Second)
			continue
		}
		return output, err
	}
}

func (r *runner) runCMD(command []string) (bytes.Buffer, error) {
	var output bytes.Buffer
	var err error

	if !r.sensitive {
		logger.Infof("Running command: %s", strings.Join(command, " "))
	} else {
		logger.Infof("%s command is running", command[0])
	}
	output.Reset()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = &output
	cmd.Stderr = cmd.Stdout

	err = cmd.Run()
	logger.Infof("Get Combining Stdout and Stder is :\n%s", output.String())

	return output, err

}
