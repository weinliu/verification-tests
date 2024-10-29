import { guidedTour } from "upstream/views/guided-tour";
import { searchPage } from "views/search";

describe('show shortname in console resourse badge', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc create -f ./fixtures/fence-agents-remediation.medik8s.io_fenceagentsremediationtemplates.yaml`)
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  })

  after(() => {
    cy.adminCLI(`oc delete -f ./fixtures/fence-agents-remediation.medik8s.io_fenceagentsremediationtemplates.yaml`)
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);

  })

  it('(OCP-67717,xiyuzhao,UserInterface)Correct shortname in console resource barge',{tags:['@userinterface','@e2e','admin','@rosa','@osd-ccs']}, () => {
    searchPage.navToSearchPage();
    cy.get('input[placeholder="Resources"]').clear().type(`far`);
    cy.get(`.co-m-resource-icon.co-m-resource-fenceagentsremediationtemplate`)
      .should('have.text', 'FAR');
  })
})