import { Deployment } from "views/deployment";

describe("console related deployments test", () => {
  it("(OCP-69183,yanpzhan,UserInterface) Set readOnlyRootFilesystem field for both console and console operator related containers", {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    // check console operator deployment
    Deployment.checkDeploymentFilesystem('console-operator','openshift-console-operator',0,true);
    Deployment.checkPodStatus('openshift-console-operator','name=console-operator','Running');
    // check console deployment
    Deployment.checkDeploymentFilesystem('console','openshift-console',0,false);
    Deployment.checkPodStatus('openshift-console','component=ui','Running');
    // check downloads deployment
    Deployment.checkDeploymentFilesystem('downloads','openshift-console',0,false);
    Deployment.checkPodStatus('openshift-console','component=downloads','Running');
  });
});
