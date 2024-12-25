import { guidedTour } from '../../upstream/views/guided-tour';
describe('cookies related feature', () => {
  before(() => {
    cy.login(Cypress.env("LOGIN_IDP"),Cypress.env("LOGIN_USERNAME"),Cypress.env("LOGIN_PASSWORD"));
    guidedTour.close();
  });

  it('(OCP-75895,yanpzhan,UserInterface) Console cookies csrf-token and openshift-session-token have SameSite option',{tags:['@userinterface','@e2e','@rosa','@osd-ccs','@hypershift-hosted']}, () => {
    cy.getCookie('csrf-token').should('have.property','sameSite','strict');
    cy.getCookie('openshift-session-token').should('have.property','sameSite','strict');
  });
});  
