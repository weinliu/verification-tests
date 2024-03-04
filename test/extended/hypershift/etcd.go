package hypershift

import (
	"fmt"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

func iamRoleTrustPolicyForEtcdBackup(accountId, saIssuer, hcpNs string) string {
	trustPolicy := fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::%s:oidc-provider/%s"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "%s:sub": "system:serviceaccount:%s:etcd-backup-sa"
                }
            }
        }
    ]
}`, accountId, saIssuer, saIssuer, hcpNs)
	e2e.Logf("Role trust policy for etcd backup:\n%s", trustPolicy)
	return trustPolicy
}
