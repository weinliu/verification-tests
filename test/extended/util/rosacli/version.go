package rosacli

import (
	"bytes"
	"fmt"

	semver "github.com/hashicorp/go-version"
)

type VersionService interface {
	ReflectVersions(result bytes.Buffer, hostedCP bool) (versionList *OpenShiftVersionList, err error)
	ReflecttClassicVersions(output bytes.Buffer) (*OpenShiftVersionList, error)
	ReflectHostedCPVersions(output bytes.Buffer) (*OpenShiftVersionList, error)
	ListVersions(hostedCP bool, flags ...string) (versionList *OpenShiftVersionList, output bytes.Buffer, err error)
	ListClassicVersions(flags ...string) (*OpenShiftVersionList, bytes.Buffer, error)
	ListHostedCPVersions(flags ...string) (*OpenShiftVersionList, bytes.Buffer, error)
}

var _ VersionService = &versionService{}

type versionService Service

type OpenShiftVersion struct {
	Version           string `json:"VERSION,omitempty"`
	Default           string `json:"DEFAULT,omitempty"`
	AvailableUpgrades string `json:"AVAILABLE UPGRADES,omitempty"`
}
type ClassicVersion OpenShiftVersion

type HostedCPVersion struct {
	Version string `json:"VERSION,omitempty"`
	Default string `json:"DEFAULT,omitempty"`
}

// Struct for the unified version output
type OpenShiftVersionList struct {
	OpenShiftVersion []OpenShiftVersion `json:"OpenShiftVersion,omitempty"`
}

// Struct for the 'rosa list version' output
type ClassicVersionList struct {
	ClassicVersions []ClassicVersion `json:"ClassicVersions,omitempty"`
}

// Struct for the 'rosa list version --hosted-cp' output
type HostedCPVersionList struct {
	HostedCPVersions []HostedCPVersion `json:"HostedCPVersions,omitempty"`
}

// Reflect versions
func (v *versionService) ReflectVersions(result bytes.Buffer, hostedCP bool) (versionList *OpenShiftVersionList, err error) {
	versionList = &OpenShiftVersionList{}
	if hostedCP {
		theMap := v.Client.Parser.TableData.Input(result).Parse().Output()
		for _, hvItem := range theMap {
			hVersion := &HostedCPVersion{}
			version := OpenShiftVersion{}
			err = MapStructure(hvItem, hVersion)
			if err != nil {
				return versionList, err
			}
			version.Version = hVersion.Version
			version.Default = hVersion.Default
			versionList.OpenShiftVersion = append(versionList.OpenShiftVersion, version)
		}

	} else {
		theMap := v.Client.Parser.TableData.Input(result).Parse().Output()
		for _, cvItem := range theMap {
			cVersion := &ClassicVersion{}
			version := OpenShiftVersion{}
			err = MapStructure(cvItem, cVersion)
			if err != nil {
				return versionList, err
			}
			version.Version = cVersion.Version
			version.Default = cVersion.Default
			version.AvailableUpgrades = cVersion.AvailableUpgrades
			versionList.OpenShiftVersion = append(versionList.OpenShiftVersion, version)
		}
	}
	return versionList, err
}

// Pasrse the result of 'rosa list version' to the ClassicVersionList struct
func (v *versionService) ReflecttClassicVersions(output bytes.Buffer) (*OpenShiftVersionList, error) {
	versionList, err := v.ReflectVersions(output, false)
	return versionList, err
}

// Pasrse the result of 'rosa list version --hosted-cp' to the ClassicVersionList struct
func (v *versionService) ReflectHostedCPVersions(output bytes.Buffer) (*OpenShiftVersionList, error) {
	versionList, err := v.ReflectVersions(output, true)
	return versionList, err
}

// list version `rosa list version` or `rosa list version --hosted-cp`
func (v *versionService) ListVersions(hostedCP bool, flags ...string) (versionList *OpenShiftVersionList, output bytes.Buffer, err error) {
	listVersion := v.Client.Runner.
		Cmd("list", "versions").
		CmdFlags(flags...)

	if hostedCP {
		listVersion.AddCmdFlags("--hosted-cp")
	}

	output, err = listVersion.Run()
	if err != nil {
		return versionList, output, err
	}

	if hostedCP {
		versionList, err = v.ReflectHostedCPVersions(output)

	} else {
		versionList, err = v.ReflecttClassicVersions(output)

	}
	return versionList, output, err
}

// list classic version `rosa list version`
func (v *versionService) ListClassicVersions(flags ...string) (*OpenShiftVersionList, bytes.Buffer, error) {
	versionList, output, err := v.ListVersions(false, flags...)
	if err != nil {
		return &OpenShiftVersionList{}, output, err
	}
	return versionList, output, err
}

// list classic version `rosa list version --hosted-cp`
func (v *versionService) ListHostedCPVersions(flags ...string) (*OpenShiftVersionList, bytes.Buffer, error) {
	versionList, output, err := v.ListVersions(true, flags...)
	if err != nil {
		return &OpenShiftVersionList{}, output, err
	}
	return versionList, output, err
}

func ParseVersion(version string) (string, error) {
	parsedVersion, err := semver.NewVersion(version)
	if err != nil {
		return "", err
	}
	versionSplit := parsedVersion.Segments64()
	return fmt.Sprintf("%d.%d", versionSplit[0], versionSplit[1]), nil
}

func ParseVersionXYStream(version string) (xStream int, yStream int, err error) {
	parsedVersion, err := semver.NewVersion(version)
	if err != nil {
		return 0, 0, err
	}
	versionSplit := parsedVersion.Segments64()
	return int(versionSplit[0]), int(versionSplit[1]), nil
}

// This function will find the Y-1 OCP version based on the passed version
func (vl OpenShiftVersionList) FindNearestBackwardYVersion(version string) (string, error) {
	parsedVersion, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("failed to parse version %q: %w", version, err)
	}
	versionSplit := parsedVersion.Segments64()

	for _, v := range vl.OpenShiftVersion {
		parsedVersion, err := semver.NewVersion(v.Version)
		if err != nil {
			return "", fmt.Errorf("failed to parse version %q: %w", v.Version, err)
		}
		vSplit := parsedVersion.Segments64()
		if vSplit[0] == versionSplit[0] && vSplit[1] == versionSplit[1]-1 {
			return v.Version, nil
		}
	}
	return "", fmt.Errorf("no backward version found for %s", version)
}

// Find the latest(biggest) version based on the passed Y-stream
func (vl OpenShiftVersionList) FindLatestVersionWithinYStream(version string) (string, error) {
	parsedBaseVersion, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("failed to parse version %q: %w", version, err)
	}
	baseVersionSplit := parsedBaseVersion.Segments64()

	var latestVersion string
	for _, v := range vl.OpenShiftVersion {
		parsedVersion, err := semver.NewVersion(v.Version)
		if err != nil {
			return "", fmt.Errorf("failed to parse version %q: %w", v.Version, err)
		}
		vSplit := parsedVersion.Segments64()
		if vSplit[0] == baseVersionSplit[0] && vSplit[1] == baseVersionSplit[1] {
			if latestVersion == "" {
				latestVersion = v.Version
			} else {
				parsedLatestVersion, err := semver.NewVersion(latestVersion)
				if err != nil {
					return "", fmt.Errorf("failed to parse version %q: %w", v.Version, err)
				}
				if parsedLatestVersion.LessThan(parsedVersion) {
					latestVersion = v.Version
				}
			}
		}
	}
	if latestVersion == "" {
		return "", fmt.Errorf("no latest version found for %s", version)
	}
	return latestVersion, nil
}
