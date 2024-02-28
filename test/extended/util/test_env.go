package util

import "os"

type TestEnv int

const (
	TestEnvProw TestEnv = 1 << iota
	TestEnvJenkins
	TestEnvLocal
)

func GetTestEnv() TestEnv {
	switch {
	case os.Getenv("JENKINS_HOME") != "":
		return TestEnvJenkins
	case os.Getenv("OPENSHIFT_CI") != "":
		return TestEnvProw
	default:
		return TestEnvLocal
	}
}

func (t TestEnv) IsRunningInProw() bool {
	return t == TestEnvProw
}

func (t TestEnv) IsRunningInJenkins() bool {
	return t == TestEnvJenkins
}

func (t TestEnv) IsRunningLocally() bool {
	return t == TestEnvLocal
}
