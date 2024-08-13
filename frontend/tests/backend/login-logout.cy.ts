import { guidedTour } from "upstream/views/guided-tour";

describe('login and logout', () => {
  it('(OCP-73149,yapei,UserInterface)do not reveal system info before login completion and should delete token after user logout', {tags: ['@userinterface','@e2e','@rosa','@osd-ccs','@wrs']}, () => {
    const up_pair = Cypress.env('LOGIN_USERS').split(',');
    const login_username = up_pair[up_pair.length - 1].split(':')[0]
    const login_password = up_pair[up_pair.length - 1].split(':')[1]
    const verifyOnLoginPage = () => {
      cy.contains('Log in').should('exist');
      cy.get('.idp').its('length').should('be.at.least', 1);
    };
    // oauthaccesstoken not exist before login via console
    // and got cleared after log out
    const verifyAccessTokenExists = (contains: boolean) => {
      if (contains) {
        cy.adminCLI('oc get oauthaccesstoken').then(result =>{
          expect(result.stdout).to.contain(login_username);
        })
      } else {
        cy.adminCLI('oc get oauthaccesstoken').then(result => {
          expect(result.stdout).to.not.contain(login_username);
        })
      }
    }

    cy.visit('/api-explorer');
    verifyOnLoginPage();
    cy.visit('/k8s/cluster/projects');
    verifyOnLoginPage();
    verifyAccessTokenExists(false);

    cy.login(Cypress.env('LOGIN_IDP'), login_username, login_password);
    guidedTour.close();
    cy.switchPerspective('Administrator');
    cy.visit('/k8s/cluster/projects');
    verifyAccessTokenExists(true);

    cy.uiLogout();
    verifyOnLoginPage();
    verifyAccessTokenExists(false);
  });
});
