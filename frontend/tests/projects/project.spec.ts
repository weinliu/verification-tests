import { checkErrors } from '../../upstream/support';
import { listPage } from '../../upstream/views/list-page';
import { projectsPage } from '../../views/projects';
describe('Projects (OCP-44210)', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  afterEach(() => {
    checkErrors();
  });

  after(() => {
    cy.logout;
  });

  it('check description and help text on project creation page (OCP-44210)', () => {
    projectsPage.navToProjectsPage();
    listPage.clickCreateYAMLbutton();
    projectsPage.checkCreationModalHelpText();
    projectsPage.checkCreationModalHelpLink();
  });
})
