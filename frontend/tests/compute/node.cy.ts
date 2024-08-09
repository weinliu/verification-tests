import { nodesPage } from "views/nodes";
import { detailsPage } from "upstream/views/details-page";
describe('nodes page', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    nodesPage.goToNodesPage();
    nodesPage.setDefaultColumn();
  });

  after(() => {
    nodesPage.goToNodesPage();
    nodesPage.setDefaultColumn();
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  it('(OCP-69089,yanpzhan,UserInterface) Add uptime info for node on console',{tags:['@userinterface','@e2e','admin','@rosa']}, () => {
    nodesPage.goToNodesPage();
    nodesPage.setAdditionalColumn('Uptime');
    cy.get('th[data-label="Uptime"]').should('exist');
    cy.get('a.co-resource-item__resource-name').first().click();
    cy.get('dt:contains("Uptime")').should('exist');
    detailsPage.selectTab('Details');
    cy.get('dt:contains("Uptime")').should('exist');
  });
})


