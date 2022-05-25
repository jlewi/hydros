package eksutil

import (
	"os"
	"os/exec"
)

// InitCluster accepts a path to a cluster config file and makes a call to eksctl to create an EKS cluster
func InitCluster(clusterConfigPath string, outputKubeconfigPath string) error {
	cmd := exec.Command(
		"eksctl",
		"create",
		"cluster",
		"--config-file",
		clusterConfigPath,
		"--kubeconfig",
		outputKubeconfigPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err
}
