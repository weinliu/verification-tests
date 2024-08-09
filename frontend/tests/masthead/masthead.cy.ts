import { guidedTour } from "upstream/views/guided-tour";
import { masthead } from '../../views/masthead';
describe('masthead related', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-60809,yanpzhan,UserInterface) Add customer feedback to console',{tags:['@userinterface','@e2e','admin','@smoke']}, () => {
    masthead.openHelpItem('Share Feedback');
    masthead.checkFeedbackModal();
    masthead.cancelFeedback();
  });
})
