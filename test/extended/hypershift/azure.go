package hypershift

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type azureKMSKey struct {
	keyName      string
	keyVaultName string
	keyVersion   string
}

// The vault key URI is expected to be in the format:
// https://<KEYVAULT_NAME>.vault.azure.net/keys/<KEYVAULT_KEY_NAME>/<KEYVAULT_KEY_VERSION>
func parseAzureVaultKeyURI(vaultKeyURI string) (azureKMSKey, error) {
	parsedURL, err := url.Parse(vaultKeyURI)
	if err != nil {
		return azureKMSKey{}, err
	}

	hostParts := strings.Split(parsedURL.Host, ".")
	if len(hostParts) != 4 {
		return azureKMSKey{}, errors.New("invalid host format")
	}
	keyVaultName := hostParts[0]

	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) != 3 {
		return azureKMSKey{}, errors.New("invalid path format")
	}
	keyName := pathParts[1]
	keyVersion := pathParts[2]

	return azureKMSKey{
		keyName:      keyName,
		keyVaultName: keyVaultName,
		keyVersion:   keyVersion,
	}, nil
}

func getHCPatchForAzureKMS(activeKey, backupKey *azureKMSKey) (string, error) {
	if activeKey == nil && backupKey == nil {
		return "", errors.New("at least one of activeKey or backupKey must be non-nil")
	}

	patch := `
spec:
  secretEncryption:
    kms:
      azure:
`
	if activeKey != nil {
		patch += fmt.Sprintf(`        activeKey:
          keyName: %s
          keyVaultName: %s
          keyVersion: %s
`, activeKey.keyName, activeKey.keyVaultName, activeKey.keyVersion)
	}
	if backupKey != nil {
		patch += fmt.Sprintf(`        backupKey:
          keyName: %s
          keyVaultName: %s
          keyVersion: %s
`, backupKey.keyName, backupKey.keyVaultName, backupKey.keyVersion)
	}
	return patch, nil
}
