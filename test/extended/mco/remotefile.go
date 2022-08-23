package mco

import (
	"fmt"
	"regexp"
	"strings"

	logger "github.com/openshift/openshift-tests-private/test/extended/mco/logext"
)

const (
	statFormat = `--print=Name: %n\nSize: %s\nKind: %F\nPermissions: %04a/%A\nUID: %u/%U\nGID: %g/%G\nLinks: %h\nSymLink: %N\nSelinux: %C\n`
	statParser = `Name: (?P<name>.+)\n` +
		`Size: (?P<size>\d+)\n` +
		`Kind: (?P<kind>.*)\n` +
		`Permissions: (?P<nperm>\d+)/(?P<rwxperm>\S+)\n` +
		`UID: (?P<uidnumber>\d+)/(?P<uidname>\S+)\n` +
		`GID: (?P<gidnumber>\d+)/(?P<gidname>\S+)\n` +
		`Links: (?P<links>\d+)\n` +
		`SymLink: (?P<symlink>.*)\n` +
		`Selinux: (?P<selinux>.*)\n` +
		`.*\n`
	startCat = "{{[[!\n"
	endCat   = "\n!]]}}"
)

// RemoteFile handles files located remotely in a node
type RemoteFile struct {
	node     Node
	fullPath string
	statData map[string]string
	content  string
}

// NewRemoteFile creates a new instance of RemoteFile
func NewRemoteFile(node Node, fullPath string) *RemoteFile {
	return &RemoteFile{node: node, fullPath: fullPath}
}

// Fetch gets the file information from the node
func (rf *RemoteFile) Fetch() error {
	output, err := rf.node.DebugNodeWithChroot("stat", statFormat, rf.fullPath)
	if err != nil {
		return err
	}

	err = rf.digest(output)
	if err != nil {
		return err
	}

	return rf.fetchTextContent()
}

func (rf *RemoteFile) fetchTextContent() error {
	output, err := rf.node.DebugNodeWithChroot("sh", "-c", fmt.Sprintf("echo -n '%s'; cat %s; echo '%s'", startCat, rf.fullPath, endCat))
	if err != nil {
		return err
	}
	// Split by first occurrence of startCat and last occurrence of endCat
	tmpcontent := strings.SplitN(output, startCat, 2)[1]
	// take into account that "cat" introduces a newline at the end
	lastIndex := strings.LastIndex(tmpcontent, endCat)
	rf.content = fmt.Sprintf(tmpcontent[:lastIndex])

	logger.Debugf("remote file %s content is:\n%s", rf.fullPath, rf.content)

	return nil
}

// PushNewPermissions modifies the remote file's permissions, setting the provided new permissions using `chmod newperm`
func (rf *RemoteFile) PushNewPermissions(newperm string) error {
	logger.Infof("Push permissions %s to file %s in node %s", newperm, rf.fullPath, rf.node.GetName())
	_, err := rf.node.DebugNodeWithChroot("sh", "-c", fmt.Sprintf("chmod %s %s", newperm, rf.fullPath))
	if err != nil {
		logger.Errorf("Error: %s", err)
		return err
	}
	return nil
}

// PushNewTextContent modifies the remote file's content
func (rf *RemoteFile) PushNewTextContent(newTextContent string) error {
	logger.Infof("Push content `%s` to file %s in node %s", newTextContent, rf.fullPath, rf.node.GetName())
	_, err := rf.node.DebugNodeWithChroot("sh", "-c", fmt.Sprintf("echo -n '%s' > '%s'", newTextContent, rf.fullPath))
	if err != nil {
		logger.Errorf("Error: %s", err)
		return err
	}
	return nil
}

// GetTextContent return the content of the text file. If the file contains binary data this method cannot be used to retrieve the file's content
func (rf *RemoteFile) GetTextContent() string {
	return rf.content
}

// Diggest the output of the 'stat' command using the 'statFormat' format. And stores the parsed information inside the 'statData' map
// To be able to understand the 'statFormat' format, it uses the 'statParser' regex. Both, 'statFormat' and 'statParser', must be coherent
func (rf *RemoteFile) digest(statOutput string) error {

	logger.Debugf("stat output: %v", statOutput)
	rf.statData = make(map[string]string)
	re := regexp.MustCompile(statParser)
	match := re.FindStringSubmatch(statOutput)
	logger.Debugf("matched stat info: %v", match)
	// check whether matched string found
	if len(match) == 0 {
		return fmt.Errorf("no file stat info matched")
	}

	for i, name := range re.SubexpNames() {
		if i < 0 {
			return fmt.Errorf("Data [%s] could not be parsed from 'stat' output: %s", name, statOutput)
		}
		if i != 0 && name != "" {
			rf.statData[name] = match[i]
		}
	}

	return nil
}

// GetUIDName returns the UID of the file in name format
func (rf *RemoteFile) GetUIDName() string {
	return rf.statData["uidname"]
}

// GetGIDName returns the GID of the file in name format
func (rf *RemoteFile) GetGIDName() string {
	return rf.statData["gidname"]
}

// GetSelinux returns the file's selinux info
func (rf *RemoteFile) GetSelinux() string {
	return rf.statData["selinux"]
}

// GetName returns the name of the file
func (rf *RemoteFile) GetName() string {
	return rf.statData["name"]
}

// GetKind returns a human readable description of the file (regular file, regular empty file, directory, symbolyc link..)
func (rf *RemoteFile) GetKind() string {
	return rf.statData["kind"]
}

// GetNpermissions returns permissions in numeric format (0664). Always 4 digits
func (rf *RemoteFile) GetNpermissions() string {
	return rf.statData["nperm"]
}

// GetUIDNumber the file's UID number
func (rf *RemoteFile) GetUIDNumber() string {
	return rf.statData["uidnumber"]
}

// GetSymLink returns the symlink description of the file (i.e: "'/tmp/actualfile'" if no link, or "'/tmp/linkfile' -> '/tmp/actualfile'" if link.
func (rf *RemoteFile) GetSymLink() string {
	return rf.statData["symlink"]
}

// GetSize returns the size of the file in bytes
func (rf *RemoteFile) GetSize() string {
	return rf.statData["size"]
}

// GetRWXPermissions returns the file permissions in rwx format
func (rf *RemoteFile) GetRWXPermissions() string {
	return rf.statData["rwxperm"]
}

// GetGIDNumber returns the file's GID number
func (rf *RemoteFile) GetGIDNumber() string {
	return rf.statData["gidnumber"]
}

// GetLinks returns the number of hard links
func (rf *RemoteFile) GetLinks() string {
	return rf.statData["links"]
}

// IsDirectory returns true if it is a directory
func (rf *RemoteFile) IsDirectory() bool {
	return rf.GetRWXPermissions()[0] == 'd' && strings.Contains(rf.GetKind(), "directory")
}

// GetTextContentAsList returns the content of the text file as a list of strings, one string per line
func (rf RemoteFile) GetTextContentAsList() []string {
	return strings.Split(rf.content, "\n")
}

// GetFilteredTextContent returns the filetered remote file's text content as a list of strings, one string per line matching the regexp.
func (rf RemoteFile) GetFilteredTextContent(regex string) ([]string, error) {
	content := rf.GetTextContentAsList()

	filteredContent := []string{}
	for _, line := range content {
		match, err := regexp.MatchString(regex, line)
		if err != nil {
			logger.Errorf("Error filtering content lines. Error: %s", err)
			return nil, err
		}

		if match {
			filteredContent = append(filteredContent, line)
		}
	}

	return filteredContent, nil
}
