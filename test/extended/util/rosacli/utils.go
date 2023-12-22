package rosacli

func removeFromStringSlice(slice []string, value string) []string {
	foundIndex := -1
	for i, item := range slice {
		if item == value {
			foundIndex = i
			break
		}
	}
	if foundIndex >= 0 {
		slice = append(slice[:foundIndex], slice[foundIndex+1:]...)
	}
	return slice
}

func sliceContains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func appendToStringSliceIfNotExist(slice []string, value string) []string {
	if !sliceContains(slice, value) {
		slice = append(slice, value)
	}
	return slice
}
