import { Overview, statusCard } from '../../views/overview';

describe('Dynamic Plugins notification features', () => {
  const testParams = {
    fileName: 'example-dynamic-plugin.yaml',
    pluginName: 'example'
  }

  let enabled = 0
  let total = 1

  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc create -f ./fixtures/${testParams.fileName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":["${testParams.pluginName}"]}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

  })

  after(() => {
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc patch console.operator cluster -p '{"spec":{"plugins":null}}' --type merge --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc delete consoleplugin ${testParams.pluginName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.logout()
  })

  it('(OCP-52366, xiangyli) ocp52366-failure add Dyamic Plugins to Cluster Overview Status card and notification drawer', {tags: ['e2e','admin']}, () => {
    Overview.goToDashboard()
    cy.get('[data-status-id="Dynamic Plugins-secondary-status"]').contains('Degraded')
    Overview.goToDashboard()
    statusCard.togglePluginPopover()
    cy.get(".pf-c-popover__body").within(($div) => {
      cy.get('a:contains(View all)').should('have.attr', 'href', '/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins')
      cy.contains(`${enabled}/${total} enabled`).should('exist')
    })
    cy.get('[data-quickstart-id="qs-masthead-notifications"]').eq(0).click()
    cy.contains('Dynamic plugin error').should('exist')
    cy.byButtonText('View plugin').click()
    cy.byLegacyTestID('resource-title').contains(testParams.pluginName)
  });
})
