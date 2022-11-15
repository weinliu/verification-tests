import { guidedTour } from "upstream/views/guided-tour"
import { listPage } from "upstream/views/list-page"

describe('Check rollout restart and retry in Deployment/DC', () => {
  // --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}
  const params = {
    namespace: 'auto-52579',
    deploymentName: 'hello-openshift',
    dcName: 'hooks',
    deploymentFile: 'deployments.yaml',
    dcFileName: 'deploymentconfig.yaml',
  }

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.createProject(params.namespace)
    cy.byTestID('resource-status').contains('Active')
    cy.exec(`oc apply -f ./fixtures/${params.deploymentFile} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc apply -f ./fixtures/${params.dcFileName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
  })

  after(() => {
    cy.exec(`oc delete project ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.logout()
  })

  it('(OCP-52579, xiangyli) Add Rollout Restart and Retry function to Deployment/Deployment Config', {tags: ['e2e']}, () => {
      // Check point 1: Kebab and action list button click for deployment
      cy.visit(`/k8s/ns/${params.namespace}/apps~v1~Deployment`)
      cy.exec(`oc get deployment/${params.deploymentName} -n ${params.namespace} -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} `)
        .its('stdout')
        .should('not.contain', 'restartedAt')
      cy.exec(`oc rollout history deployment/${params.deploymentName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        .its('stdout')
        .should('not.contain', '2') // There should be only one revision
      listPage.rows.clickKebabAction(params.deploymentName, 'Restart rollout')
      cy.visit(`/k8s/ns/${params.namespace}/deployments/${params.deploymentName}`)
      cy.exec(`oc get deployment/${params.deploymentName} -n ${params.namespace} -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        .its('stdout')
        .should('contain', 'restartedAt')
      cy.exec(`oc rollout history deployment/${params.deploymentName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        .its('stdout')
        .should('contain', '2')
      cy.exec(`oc rollout pause deployment/${params.deploymentName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        .its('stdout')
        .should('contain', 'paused')
      cy.byLegacyTestID('actions-menu-button').click()
      cy.byLegacyTestID('action-items')
        .within(($div) => {
          cy.contains('button', 'Restart rollout')
            .should('be.disabled')
        })
      
      // Check point 2: Everything goes the same for the deploymentconfig except for action name. 
      // 2.1 Check that retry button is disabled at first and the tool tip hovers accordingly. 
      cy.visit(`/k8s/ns/${params.namespace}/apps.openshift.io~v1~DeploymentConfig`)
      cy.get(`[data-test-rows="resource-row"]`)
        .contains(params.dcName)
        .parents('tr')
        .within(() => {
          cy.get('[data-test-id="kebab-button"]').click()
        })
      cy.byLegacyTestID('action-items').within(($div) => {
        cy.get('.pf-m-disabled').should('contain', 'Retry rollout')
      })
      // 2.2 Go to detail page
      cy.visit(`/k8s/ns/${params.namespace}/deploymentconfigs/${params.dcName}/replicationcontrollers`)
      cy.byTestID('status-text').contains('Complete')
      cy.exec(`oc rollout latest dc/${params.dcName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      cy.exec(`oc rollout cancel dc/${params.dcName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      cy.visit(`/k8s/ns/${params.namespace}/deploymentconfigs/${params.dcName}/replicationcontrollers`) // refresh the page
      cy.byTestID('status-text').contains('Failed')
      cy.byLegacyTestID('actions-menu-button').click()
      cy.get('[data-test-action="Retry rollout"]').click()
      cy.byTestID('status-text').contains('Failed').should('not.exist')
      // start to check if the rollout was successful
      cy.exec(`oc rollout status dc/${params.dcName} -n ${params.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
        .its('stdout')
        .should('contain', 'successfully')
    })
})