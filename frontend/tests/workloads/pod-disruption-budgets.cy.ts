import { pdbListPage } from '../../views/pod-disruption-budgets';
import { button  } from '../../views/button';

describe('PDB List Page and Detail Page Test', () => {
  const deploymentParams = {
    name: 'example-deployment'
  }

  const testParams = {
    fileName: 'deployments',
    projectName: 'ocp50657'
  }

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  })

  after(() => {
    cy.adminCLI(`oc delete project ${testParams.projectName}`)
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  })

  it('(OCP-50657,yapei,UserInterface) Add support for PDB(Pod Disruption Budget)',{tags:['@userinterface','e2e','admin']}, () => {
    cy.adminCLI(`oc new-project ${testParams.projectName}`)
    cy.adminCLI(`oc create -f ./fixtures/${testParams.fileName}.yaml -n ${testParams.projectName}`);
    const pdbParams = {
      name: 'example-pdb',
      value: '6'
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
    cy.get('[name="availability requirement value"]').clear().type('0');
    button.saveChanges();
    cy.adminCLI(`oc get pdb ${pdbParams.name} -n ${testParams.projectName} -o yaml`)
      .its('stdout')
      .should('contain', 'maxUnavailable: 0');
  });
})
