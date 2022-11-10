package util

import (
	"io/ioutil"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

/*
YamlReplace define a YAML modification given.
Example:

	YamlReplace {
		Path: 'spec.template.spec.imagePullSecrets',
		Value: '- name: notmatch-secret',
	}
*/
type YamlReplace struct {
	Path  string // path to modify or create value
	Value string // a string literal or YAML string (ex. 'name: frontend') to be set under the given path
}

/*
ModifyYamlFileContent modify the content of YAML file given the file path and a list of YamlReplace struct.
Example:
ModifyYamlFileContent(file, []YamlReplace {

		{
			Path: 'spec.template.spec.imagePullSecrets',
			Value: '- name: notmatch-secret',
		},
	})
*/
func ModifyYamlFileContent(file string, replacements []YamlReplace) {
	input, err := ioutil.ReadFile(file)
	if err != nil {
		e2e.Failf("read file %s failed: %v", file, err)
	}

	var doc yaml.Node
	if err = yaml.Unmarshal(input, &doc); err != nil {
		e2e.Failf("unmarshal yaml for file %s failed: %v", file, err)

	}

	for _, replacement := range replacements {
		path := strings.Split(replacement.Path, ".")
		value := yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: replacement.Value,
		}
		setYamlValue(&doc, path, value)
	}

	output, err := yaml.Marshal(doc.Content[0])
	if err != nil {
		e2e.Failf("marshal yaml for file %s failed: %v", file, err)
	}

	if err = ioutil.WriteFile(file, output, 0o755); err != nil {
		e2e.Failf("write file %s failed: %v", file, err)
	}
}

// setYamlValue set (or create if path not exist) a leaf yaml.Node according to given path
func setYamlValue(root *yaml.Node, path []string, value yaml.Node) {
	if len(path) == 0 {
		var valueParsed yaml.Node
		if err := yaml.Unmarshal([]byte(value.Value), &valueParsed); err == nil {
			*root = *valueParsed.Content[0]
		} else {
			*root = value
		}
		return
	}
	key := path[0]
	rest := path[1:]
	switch root.Kind {
	case yaml.DocumentNode:
		setYamlValue(root.Content[0], path, value)
	case yaml.MappingNode:
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == key {
				setYamlValue(root.Content[i+1], rest, value)
				return
			}
		}
		// key not found
		root.Content = append(root.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: key,
		}, &yaml.Node{
			Kind: yaml.MappingNode,
		})
		l := len(root.Content)
		setYamlValue(root.Content[l-1], rest, value)
	case yaml.SequenceNode:
		index, err := strconv.Atoi(key)
		if err != nil {
			e2e.Failf("string to int failed: %v", err)
		}
		if index < len(root.Content) {
			setYamlValue(root.Content[index], rest, value)
		}
	}
}
