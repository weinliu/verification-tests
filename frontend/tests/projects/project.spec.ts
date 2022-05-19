import { checkErrors } from '../../upstream/support';
import { listPage } from '../../upstream/views/list-page';
import { projectsPage } from '../../views/projects';
describe('Projects', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  afterEach(() => {
    checkErrors();
  });

  after(() => {
    cy.logout;
  });

  it('(OCP-44210) check description and help text on project creation page', () => {
    projectsPage.navToProjectsPage();
    listPage.clickCreateYAMLbutton();
    projectsPage.checkCreationModalHelpText();
    projectsPage.checkCreationModalHelpLink();
  });
})
