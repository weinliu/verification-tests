import { Pages } from "views/pages";
describe('Console plugins features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env("LOGIN_IDP"),Cypress.env("LOGIN_USERNAME"),Cypress.env("LOGIN_PASSWORD"));
  });
  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-69573,yanpzhan,UserInterface) Enable ConsolePlugin v1 CRD storage',{tags:['@userinterface','@e2e', 'admin','@osd-ccs', '@rosa']}, () => {
    Pages.gotoOneCRDDetailsPage('consoleplugins.console.openshift.io');
    const versionData = [
     { name: 'v1alpha1', served: 'true', storage: 'false' },
     { name: 'v1', served: 'true', storage: 'true' },
    ];
    versionData.forEach((version,index) => {
    cy.get(`td:contains("${version.name}")`)
       .eq(index)
       .siblings('td[data-label="Served"]').should('have.text', version.served)
       .siblings('td[data-label="Storage"]').should('have.text', version.storage);
    });
  })
});
