import { Overview } from '../../views/overview';
import { searchPage } from '../../views/search';
import { sideNav } from '../../views/nav';
import { crds } from '../../views/crds';
describe('Check on hypershift provisined cluster', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.logout;
  });

  it('(OCP-50740,yanpzhan) Remove manchine related resources for HyperShift Provisioned Clusters', {tags: ['HyperShiftGUEST','admin']}, () => {
    sideNav.checkNoMachineResources();
    searchPage.checkNoMachineResources();
    crds.checkNoMachineResources();
  });

  it('(OCP-51733,yanpzhan) Check no idp alert for temporary administrative user on HyperShift Provisioned Clusters', {tags: ['HyperShiftGUEST','admin']}, () => {
  cy.contains('logged in as a temporary administrative user').should('not.exist');
  cy.get('div').should('not.contain', 'allow others to log in');
  });
})
