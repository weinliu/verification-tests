export const Deployment = {
  checkAlert: () => {
    cy.get('h4')
      .should('include.text', 'DeploymentConfig is being deprecated with OpenShift 4.14');
    cy.get('div.pf-v5-c-alert__description a')
      .should('include.text', 'Learn more about Deployments')
      .should('have.attr', 'href')
      .and('include', '/deployments')
  },
  checkDeploymentFilesystem: (deploymentName, nameSpace, containerIndex, readOnlyValue) => {
    cy.exec(`oc get deployment ${deploymentName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -n ${nameSpace} -ojsonpath="{.spec.template.spec.containers[${containerIndex}].securityContext}"`, {failOnNonZeroExit: false}).then(result => {
      expect(result.stdout).contains(`"readOnlyRootFilesystem":${readOnlyValue}`)
      });
  },
  checkPodStatus: (nameSpace, label, podStatus) => {
    cy.exec(`oc get pods -n ${nameSpace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -l ${label}`, {failOnNonZeroExit: false}).then(result => {
      expect(result.stdout).contains(`${podStatus}`)
    });
  },
  checkDetailItem: (key, value) => {
    cy.contains('dt', `${key}`).next({timeout: 60000}).should('contain', `${value}`);
  }
}
