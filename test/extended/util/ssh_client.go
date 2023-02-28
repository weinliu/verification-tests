package util

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	o "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type SshClient struct {
	User       string
	Host       string
	Port       int
	PrivateKey string
}

func (sshClient *SshClient) getConfig() (*ssh.ClientConfig, error) {
	pemBytes, err := ioutil.ReadFile(sshClient.PrivateKey)
	if err != nil {
		e2e.Logf("Pem byte failed:%v", err)
	}
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		e2e.Logf("Parse key failed:%v", err)
	}
	config := &ssh.ClientConfig{
		User: sshClient.User,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}
	return config, err
}

// Run runs cmd on the remote host.
func (sshClient *SshClient) Run(cmd string) error {
	config, err := sshClient.getConfig()
	o.Expect(err).NotTo(o.HaveOccurred())

	connection, err := ssh.Dial("tcp", fmt.Sprintf("%v:%v", sshClient.Host, sshClient.Port), config)
	if err != nil {
		e2e.Logf("%v dial failed: %v", sshClient.Host, err)
		return err
	}
	defer connection.Close()

	session, err := connection.NewSession()
	defer session.Close()
	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stdoutBuf
	if err != nil {
		e2e.Logf("Failed to create session: %v", err)
	}
	err = session.Run(cmd)
	if err != nil {
		e2e.Logf("Failed to run cmd [%s] : \n %v", cmd, stdoutBuf)
		return err
	}
	e2e.Logf("Executed cmd [%s] output:\n %s", cmd, stdoutBuf)
	return err
}

// Run runs cmd on the remote host.
func (sshClient *SshClient) RunOutput(cmd string) (string, error) {
	config, err := sshClient.getConfig()
	o.Expect(err).NotTo(o.HaveOccurred())

	connection, err := ssh.Dial("tcp", fmt.Sprintf("%v:%v", sshClient.Host, sshClient.Port), config)
	if err != nil {
		e2e.Logf("%v dial failed: %v", sshClient.Host, err)
		return "", err
	}
	defer connection.Close()

	session, err := connection.NewSession()
	defer session.Close()
	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stdoutBuf
	if err != nil {
		e2e.Logf("Failed to create session: %v", err)
	}
	err = session.Run(cmd)
	if err != nil {
		e2e.Logf("Failed to run cmd [%s] : \n %v", cmd, stdoutBuf)
		return "", err
	}
	return stdoutBuf.String(), err
}

func GetPrivateKey() (string, error) {
	privateKey := os.Getenv("SSH_CLOUD_PRIV_KEY")
	if privateKey == "" {
		privateKey = filepath.Join("../internal/config/keys/", "openshift-qe.pem")
	}
	if _, err := os.Stat(privateKey); os.IsNotExist(err) {
		return "", fmt.Errorf("private key file not found: %s", privateKey)
	}
	return privateKey, nil
}

func GetPublicKey() (string, error) {
	publicKey := os.Getenv("SSH_CLOUD_PUB_KEY")
	if publicKey == "" {
		publicKey = filepath.Join("../internal/config/keys/", "openshift-qe.pub")
	}
	if _, err := os.Stat(publicKey); os.IsNotExist(err) {
		return "", fmt.Errorf("public key file not found: %s", publicKey)
	}
	return publicKey, nil
}
