import { guidedTour } from "upstream/views/guided-tour"
import { listPage } from "upstream/views/list-page"

describe('Check rollout restart and retry in Deployment/DC', () => {
  const params = {
    namespace: 'ocp52579-project',
    deploymentName: 'example-deployment',
    dcName: 'hooks',
    deploymentFile: 'deployments.yaml',
    dcFileName: 'deploymentconfig.yaml',
  }

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.createProject(params.namespace);
    cy.byTestID('resource-status').contains('Active');
    cy.adminCLI(`oc apply -f ./fixtures/${params.deploymentFile} -n ${params.namespace}`);
    cy.adminCLI(`oc apply -f ./fixtures/${params.dcFileName} -n ${params.namespace}`);
  })

  after(() => {
     cy.adminCLI(`oc delete project ${params.namespace}`);
  })

  it('(OCP-52579,xiyuzhao,UserInterface) Add Rollout Restart and Retry function to Deployment/Deployment Config', {tags: ['e2e','@rosa']}, () => {
    const checkRolloutState = (judge: string) => {
      cy.adminCLI(`oc get deployment/${params.deploymentName} -n ${params.namespace} -o yaml`)
        .its('stdout')
        .should(judge, 'restartedAt');
      cy.adminCLI(`oc rollout history deployment/${params.deploymentName} -n ${params.namespace}`)
        .its('stdout')
        .should(judge, '2');
    }
    // Check 'Restart rollout' action in Deployment page
    cy.visit(`/k8s/ns/${params.namespace}/apps~v1~Deployment`);
    checkRolloutState('not.contain');
    listPage.rows.clickKebabAction(params.deploymentName, 'Restart rollout');
    listPage.rows.clickRowByName(params.deploymentName);
    checkRolloutState('contain');
    cy.adminCLI(`oc rollout pause deployment/${params.deploymentName} -n ${params.namespace}`)
      .its('stdout')
      .should('contain', 'paused');
    cy.byLegacyTestID('actions-menu-button').click();
    cy.get('[data-test-action="Restart rollout"] button').should('be.disabled');
    // Check 'Retry rollout' in Deployment Configs page
    cy.visit(`/k8s/ns/${params.namespace}/apps.openshift.io~v1~DeploymentConfig`);
    listPage.filter.byName(params.dcName);
    cy.byLegacyTestID('kebab-button').click();
    cy.get('[data-test-action="Retry rollout"] button').should('be.disabled');
    /* Check 'Retry rollout' function on DeploymentConfig details - ReplicationControllers page
      'Retry rollout' is enabled when the latest revision of the ReplicationControllers is in Failed state */
    cy.visit(`/k8s/ns/${params.namespace}/deploymentconfigs/${params.dcName}/replicationcontrollers`);
    cy.byTestID('status-text').contains('Complete');
    cy.adminCLI(`oc rollout latest dc/${params.dcName} -n ${params.namespace}`);
    cy.adminCLI(`oc rollout cancel dc/${params.dcName} -n ${params.namespace}`);
    cy.wait(50000);
    cy.byTestID('status-text').contains('Failed');
    cy.reload(true);
    cy.byLegacyTestID('actions-menu-button').click();
    cy.byTestActionID('Retry rollout').click();
    cy.wait(15000);
    cy.adminCLI(`oc rollout status dc/${params.dcName} -n ${params.namespace}`)
      .its('stdout')
      .should('contain', 'successfully');
    })
})