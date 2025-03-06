import { Pages } from "views/pages";
describe('Console plugins features', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.uiLogin(Cypress.env("LOGIN_IDP"),Cypress.env("LOGIN_USERNAME"),Cypress.env("LOGIN_PASSWORD"));
  });
  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-69573,yanpzhan,UserInterface) Enable ConsolePlugin v1 CRD storage',{tags:['@userinterface','@e2e', 'admin','@osd-ccs', '@rosa','@hypershift-hosted']}, () => {
    Pages.gotoOneCRDDetailsPage('consoleplugins.console.openshift.io');
    const versionData = [
     { name: 'v1', served: 'true', storage: 'true' }
    ];
    versionData.forEach((version) => {
    cy.get(`td:contains("${version.name}")`)
       .eq(0)
       .next().should('have.text', version.served)
       .next().should('have.text', version.storage);
    });
  })
});
