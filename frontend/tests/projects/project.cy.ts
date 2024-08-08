import { listPage } from '../../upstream/views/list-page';
import { projectsPage } from '../../views/projects';
describe('Projects', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  it('(OCP-44210,yanpzhan,UserInterface) check description and help text on project creation page',{tags:['@userinterface','e2e','@osd-ccs','@rosa','@smoke']}, () => {
    projectsPage.goToProjectsPage();
    listPage.clickCreateYAMLbutton();
    projectsPage.checkCreationModalHelpText();
    projectsPage.checkCreationModalHelpLink();
  });
})
