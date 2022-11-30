package util

import "strings"

// DeleteLabelsFromSpecificResource deletes the custom labels from the specific resource
func DeleteLabelsFromSpecificResource(oc *CLI, resourceKindAndName string, resourceNamespace string, labelNames ...string) (string, error) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName)
	cargs = append(cargs, StringsSliceElementsAddSuffix(labelNames, "-")...)
	return oc.AsAdmin().WithoutNamespace().Run("label").Args(cargs...).Output()
}

// AddLabelsToSpecificResource adds the custom labels to the specific resource
func AddLabelsToSpecificResource(oc *CLI, resourceKindAndName string, resourceNamespace string, labels ...string) (string, error) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName)
	cargs = append(cargs, labels...)
	cargs = append(cargs, "--overwrite")
	return oc.AsAdmin().WithoutNamespace().Run("label").Args(cargs...).Output()
}

// GetResourceSpecificLabelValue gets the specified label value from the resource and label name
func GetResourceSpecificLabelValue(oc *CLI, resourceKindAndName string, resourceNamespace string, labelName string) (string, error) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName, "-o=jsonpath={.metadata.labels."+labelName+"}")
	return oc.AsAdmin().WithoutNamespace().Run("get").Args(cargs...).Output()
}

// StringsSliceContains judges whether the strings Slice contains specific element, return bool and the first matched index
// If no matched return (false, 0)
func StringsSliceContains(stringsSlice []string, element string) (bool, int) {
	for index, strElement := range stringsSlice {
		if strElement == element {
			return true, index
		}
	}
	return false, 0
}

// StringsSliceElementsHasPrefix judges whether the strings Slice contains an element which has the specific prefix
// returns bool and the first matched index
// sequential order: -> sequentialFlag: "true"
// reverse order:    -> sequentialFlag: "false"
// If no matched return (false, 0)
func StringsSliceElementsHasPrefix(stringsSlice []string, elementPrefix string, sequentialFlag bool) (bool, int) {
	if len(stringsSlice) == 0 {
		return false, 0
	}
	if sequentialFlag {
		for index, strElement := range stringsSlice {
			if strings.HasPrefix(strElement, elementPrefix) {
				return true, index
			}
		}
	} else {
		for i := len(stringsSlice) - 1; i >= 0; i-- {
			if strings.HasPrefix(stringsSlice[i], elementPrefix) {
				return true, i
			}
		}
	}
	return false, 0
}

// StringsSliceElementsAddSuffix returns a new string slice all elements with the specific suffix added
func StringsSliceElementsAddSuffix(stringsSlice []string, suffix string) []string {
	if len(stringsSlice) == 0 {
		return []string{}
	}
	var newStringsSlice = make([]string, 0, 10)
	for _, element := range stringsSlice {
		newStringsSlice = append(newStringsSlice, element+suffix)
	}
	return newStringsSlice
}
