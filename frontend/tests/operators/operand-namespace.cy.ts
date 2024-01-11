import { operatorHubPage } from "views/operator-hub-page"

describe('Display All Namespace Operands for Global Operators', () => {
  const params = {
    ns: 'ocp50153-project',
    operatorName: 'argocd-operator',
    catalogSourceName: 'custom-catalogsource',
    catalogSourceFile: 'custom-catalog-source.json',
    subscriptionName: 'argocd'
  }

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    cy.adminCLI(`oc apply -f ./fixtures/operators/${params.catalogSourceFile}`)
    cy.adminCLI(`oc new-project ${params.ns}`)
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
  })

  after(() => {
    cy.adminCLI(`oc delete subscription ${params.operatorName} -n openshift-operators`);
    cy.adminCLI(`oc delete clusterserviceversion ${params.operatorName}.v0.0.15 -n openshift-operators`);
    cy.adminCLI(`oc delete project ${params.ns}`)
    cy.adminCLI(`oc delete CatalogSource ${params.catalogSourceName} -n openshift-marketplace`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
  })

  it('(OCP-50153,xiyuzhao,UI) - Display All Namespace Operands for Global Operators', {tags: ['e2e','admin']}, () => {
    operatorHubPage.installOperator(params.operatorName, params.catalogSourceName);
    cy.contains('View Operator').click();
    cy.exec(`echo '{
      "kind": "ArgoCD",
      "apiVersion": "argoproj.io/v1alpha1",
      "metadata": {
        "name": "${params.subscriptionName}",
        "namespace": "${params.ns}"
      },
      "spec": {}
    }' | oc create -f - --kubeconfig=${Cypress.env('KUBECONFIG_PATH')}` );
    cy.byLegacyTestID('horizontal-link-All instances').click();

    // checkpoint 1: Check column 'Namespace' is added in list
    cy.get('[data-label="Namespace"]').should('be.visible')
    // checkpoint 2: Check 'All namespace' radio input is selected by dafault
    cy.byTestID('All namespaces-radio-input').should('be.checked')
    // checkpoint 3: Check subscription is listed
    cy.byTestID(params.subscriptionName).should('be.visible')
    // checkpoint 4: Check only corresponding resource is displayed on specific ns
    cy.byTestID('Current namespace only-radio-input').click()
    cy.byTestID(params.subscriptionName).should('not.exist')
  })
})
