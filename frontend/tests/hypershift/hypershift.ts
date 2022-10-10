import { Overview } from '../../views/overview';
import { searchPage } from '../../views/search';
import { sideNav } from '../../views/nav';
import { crds } from '../../views/crds';
describe.skip('Check on hypershift provisined cluster', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.logout;
  });

  it('(OCP-50740,admin,HyperShiftGUEST,yanpzhan) Remove manchine related resources for HyperShift Provisioned Clusters', () => {
    sideNav.checkNoMachineResources();
    searchPage.checkNoMachineResources();
    crds.checkNoMachineResources();
  });

  it('(OCP-51733,admin,HyperShiftGUEST,yanpzhan) Check no idp alert for temporary administrative user on HyperShift Provisioned Clusters', () => {
  cy.contains('logged in as a temporary administrative user').should('not.exist');
  cy.get('div').should('not.contain', 'allow others to log in');
  });
})
