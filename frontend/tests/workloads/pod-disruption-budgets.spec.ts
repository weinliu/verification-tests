import { pdbListPage } from './../../views/pod-disruption-budgets';

describe('PDB List Page and Detail Page Test', () => {
  const deploymentParams = {
    name: 'example-deployment'
  }

  const testParams = {
    fileName: 'deployments',
    projectName: 'ocp50657'
  }

  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.exec(`oc new-project ${testParams.projectName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc create -f ./fixtures/${testParams.fileName}.yaml -n ${testParams.projectName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  })

  after(() => {
    cy.logout();
    cy.exec(`oc delete project ${testParams.projectName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
  })

  it('(OCP-50657, xiangyli) - Add support for PDB (Pod Disruption Budget)', {tags: ['e2e','admin']}, () => {
    const pdbParams = {
      name: 'example-pdbd',
      label: 'app=name',
      value: '6',
      editValue: '50657'
    }
    cy.visit('/k8s/all-namespaces/poddisruptionbudgets');
    cy.get('[data-test-id="resource-title"]').contains('PodDisruptionBudgets');
    cy.get('thead').contains(/^Name(.*)Namespace(.*)Selector(.*)Availability(.*)Allowed disruptions(.*)Created$/);

    cy.visit(`/k8s/ns/${testParams.projectName}/deployments/${deploymentParams.name}`);
    cy.byLegacyTestID('actions-menu-button').click({force: true});
    cy.byButtonText('Add PodDisruptionBudget').click({force: true});
    pdbListPage.createPDB(pdbParams);

    cy.visit(`/k8s/ns/${testParams.projectName}/poddisruptionbudgets/${pdbParams.name}`);
    cy.get('[data-test-selector="details-item-value__Name"]').contains(pdbParams.name);
    cy.get('.co-m-horizontal-nav__menu').contains(/Details|YAML|Pods/);

    cy.visit(`/k8s/ns/${testParams.projectName}/deployments/${deploymentParams.name}`);
    cy.byLegacyTestID('actions-menu-button').click({force: true});
    cy.byButtonText('Edit PodDisruptionBudget').click({force: true});
    cy.get(`input[value=${pdbParams.value}]`).clear().type(pdbParams.editValue);
    cy.byTestID('yaml-view-input').click();
    cy.get('.mtk16').contains(pdbParams.editValue);
    cy.get('[id="save-changes"]').click();
  });
})
