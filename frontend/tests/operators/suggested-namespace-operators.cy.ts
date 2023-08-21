import { listPage } from "upstream/views/list-page";
import { operatorHubPage } from "../../views/operator-hub-page";
import { guidedTour } from './../../upstream/views/guided-tour';

describe('Operators Installed page test', () => {
  const params ={
    'catalog':'redhat-operators',
    'project':'open-cluster-management',
    'advancedCluster': 'advanced-cluster-management'
  }
  
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    });

  after(() => {
    cy.adminCLI(`oc delete ns ${params.project}`);
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false })
    });

  it('(OCP-27681,bandrade) Operator Developer can define a target namespace',{tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {

    operatorHubPage.installOperator(`${params.advancedCluster}`,`${params.catalog}`);
    cy.get('[aria-valuetext="Loading..."]').should('exist');
    cy.visit(`/k8s/ns/${params.project}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    operatorHubPage.checkOperatorStatus('Advanced Cluster Management for Kubernetes', 'Succeeded');
    });

})
