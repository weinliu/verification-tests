import { Overview } from "../../views/overview";
import { Branding } from "../../views/branding";
import { guidedTour } from '../../upstream/views/guided-tour';

describe("Branding check", () => {
  it("(OCP-48357,yanpzhan,UserInterface) Switch the OCP branding logo and title to Red Hat OpenShift logo and title",{tags:['@userinterface','@e2e','@rosa','@smoke','@hypershift-hosted']}, () => {
    cy.visit('/');
    Branding.checkLoginPageLogo();
    cy.uiLogin(Cypress.env("LOGIN_IDP"),Cypress.env("LOGIN_USERNAME"),Cypress.env("LOGIN_PASSWORD"));
    guidedTour.close();
    Overview.goToDashboard();
    Overview.checkUpperLeftLogo();
    Overview.toggleAbout();
    Branding.checkAboutModalLogo();
  });
});
