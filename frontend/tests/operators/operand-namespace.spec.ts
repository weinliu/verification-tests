import { operatorHubPage } from "views/operator-hub-page"

describe('Display All Namespace Operands for Global Operators', () => {
  
  const params = {
    anotherNamespace: 'argocd-another',
    operatorName: 'argocd-operator',
    catalogSourceName: 'custom-catalogsource',
    catalogSourceFile: 'custom-catalog-source.json',
    operandFile: 'operands.yaml',
    argocd: 'argocd'
  }

  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc apply -f ./fixtures/${params.catalogSourceFile} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc new-project ${params.anotherNamespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
  })
  
  after(() => {
    cy.exec(`oc delete project ${params.anotherNamespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc delete csv argocd-operator.v0.0.15 -n openshift-operators --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.logout()
  })

  it('(OCP-50153, xiangyli, admin) - Display All Namespace Operands for Global Operators', {tags: ['e2e','admin']}, () => {
    // install the operator
    operatorHubPage.installOperator(params.operatorName, params.catalogSourceName)
    // wait for operator to install
    cy.contains('View Operator').click()

    // install the operands
    cy.exec(`oc apply -f ./fixtures/${params.operandFile} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)

    // check namespace radio button is selected for operand on the operator page
    cy.byLegacyTestID('horizontal-link-olm~All instances').click()
    // checkpoint 1: Improved Column
    cy.get('[data-label="Namespace"]').should('be.visible')
    // checkpoint 2: All namespace radio input is selected by dafault
    cy.byTestID('All namespaces-radio-input').should('be.checked')
    // checkpoint 2: Two resources are shown when All Namespaces option is selected
    cy.byTestID(params.argocd).should('be.visible')
    // checkpoint 3: Only one corresponding resource is shown on specific ns with Current Namespace Only selected
    cy.byLegacyTestID('horizontal-link-olm~All instances').click()
    cy.byTestID('Current namespace only-radio-input').check()
    cy.byTestID(params.argocd).should('not.exist')
  })
})
