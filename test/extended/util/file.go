package util

import (
	"io"
	"os"

	o "github.com/onsi/gomega"
)

// DuplicateFileToTemp creates a temporary duplicate of the file at srcPath using destPattern for naming,
// returning the path of the duplicate.
func DuplicateFileToTemp(srcPath string, destPattern string) (destPath string) {
	var destFile, srcFile *os.File
	var err error

	srcFile, err = os.Open(srcPath)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer func() {
		o.Expect(srcFile.Close()).NotTo(o.HaveOccurred())
	}()

	destFile, err = os.CreateTemp(os.TempDir(), destPattern)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer func() {
		o.Expect(destFile.Close()).NotTo(o.HaveOccurred())
	}()

	_, err = io.Copy(destFile, srcFile)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(destFile.Sync()).NotTo(o.HaveOccurred())
	return destFile.Name()
}
