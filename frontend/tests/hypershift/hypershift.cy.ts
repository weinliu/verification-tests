import { searchPage } from '../../views/search';
import { sideNav } from '../../views/nav';
import { crds } from '../../views/crds';
describe('Check on hypershift provisined cluster', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-50740,yanpzhan,UserInterface) Remove manchine related resources for HyperShift Provisioned Clusters',{tags:['@userinterface','@hypershift-hosted','admin']}, () => {
    sideNav.checkNoMachineResources();
    searchPage.checkNoMachineResources();
    crds.checkNoMachineResources();
  });

  it('(OCP-51733,yanpzhan,UserInterface) Check no idp alert for temporary administrative user on HyperShift Provisioned Clusters',{tags:['@userinterface','@hypershift-hosted','admin']}, () => {
    cy.contains('logged in as a temporary administrative user').should('not.exist');
    cy.get('div').should('not.contain', 'allow others to log in');
  });
})
