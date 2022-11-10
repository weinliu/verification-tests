import { nav } from '../../upstream/views/nav';

declare global {
    namespace Cypress {
        interface Chainable<Subject> {
            switchPerspective(perspective: string);
            cliLogin();
            cliLogout();
        }
    }
}

Cypress.Commands.add("switchPerspective", (perspective: string) => {

    /* if side bar is collapsed then expand it
    before switching perspecting */
    cy.get('body').then((body) => {
        if (body.find('.pf-m-collapsed').length > 0) {
            cy.get('#nav-toggle').click()
        }
    });
    nav.sidenav.switcher.changePerspectiveTo(perspective);
    nav.sidenav.switcher.shouldHaveText(perspective);
});

Cypress.Commands.add("cliLogin", () => {
  cy.exec(`oc login -u ${Cypress.env('LOGIN_USERNAME')} -p ${Cypress.env('LOGIN_PASSWORD')} ${Cypress.env('HOST_API')} --insecure-skip-tls-verify=true`).then(result => {
    cy.log(result.stderr)
    cy.log(result.stdout)
    expect(result.stderr).to.be.empty
  });
});

Cypress.Commands.add("cliLogout", () => {
  cy.exec(`oc logout`).then(result => {
    cy.log(result.stderr)
    cy.log(result.stdout)
    expect(result.stderr).to.be.empty
  });
});
