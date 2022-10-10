import { guidedTour } from '../../upstream/views/guided-tour';
import { consoleTheme } from '../../views/user-preferences.ts';

describe('dark-theme related feature', () => {
  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  });

  after(() => {
    cy.logout;
  });

  it('(OCP-49134,admin,yanpzhan) Support dark theme for admin console', () => {
    cy.visit('/user-preferences');
    consoleTheme.setLightTheme();
    cy.get('.pf-theme-dark').should('not.exist');
    consoleTheme.setDarkTheme();
    cy.get('.pf-theme-dark').should('exist');
    consoleTheme.setSystemDefaultTheme();
  });

})
