import { Overview } from "../../views/overview";
import { Branding } from "../../views/branding";
describe("Branding check", () => {
  before(() => {
    cy.login(
      Cypress.env("LOGIN_IDP"),
      Cypress.env("LOGIN_USERNAME"),
      Cypress.env("LOGIN_PASSWORD")
    );
  });

  it("(OCP-48357) Switch the OCP branding logo and title to Red Hat OpenShift logo and title", {tags: ['e2e']}, () => {
    Overview.goToDashboard();
    Overview.checkUpperLeftLogo();
    Overview.toggleAbout();
    Branding.checkAboutModalLogo();
    cy.logout();
    Branding.checkLoginPageLogo();
  });
});
