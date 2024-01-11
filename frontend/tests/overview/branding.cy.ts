import { Overview } from "../../views/overview";
import { Branding } from "../../views/branding";
import { guidedTour } from '../../upstream/views/guided-tour';

describe("Branding check", () => {
  before(() => {
    cy.login(Cypress.env("LOGIN_IDP"),Cypress.env("LOGIN_USERNAME"),Cypress.env("LOGIN_PASSWORD"));
    guidedTour.close();
  });

  it("(OCP-48357,yanpzhan,UI) Switch the OCP branding logo and title to Red Hat OpenShift logo and title", {tags: ['e2e','@rosa']}, () => {
    Overview.goToDashboard();
    Overview.checkUpperLeftLogo();
    Overview.toggleAbout();
    Branding.checkAboutModalLogo();
    cy.uiLogout();
    Branding.checkLoginPageLogo();
  });
});
