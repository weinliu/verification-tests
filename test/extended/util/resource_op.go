package util

// DeleteLabelFromSpecificResource delete the custom label from the specific resource
func DeleteLabelFromSpecificResource(oc *CLI, resourceKindAndName string, resourceNamespace string, labelName string) (string, error) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName, labelName+"-")
	return oc.AsAdmin().WithoutNamespace().Run("label").Args(cargs...).Output()
}

// AddLabelToSpecificResource add the custom label to the specific resource
func AddLabelToSpecificResource(oc *CLI, resourceKindAndName string, resourceNamespace string, labelName string, labelValue string) (string, error) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName, labelName+"="+labelValue, "--overwrite")
	return oc.AsAdmin().WithoutNamespace().Run("label").Args(cargs...).Output()
}

// GetResourceSpecificLabelValue get the specfic label value from the resource and label name
func GetResourceSpecificLabelValue(oc *CLI, resourceKindAndName string, resourceNamespace string, labelName string) (string, error) {
	var cargs []string
	if resourceNamespace != "" {
		cargs = append(cargs, "-n", resourceNamespace)
	}
	cargs = append(cargs, resourceKindAndName, "-o=jsonpath=.metadata.labels."+labelName+"}")
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
