import { nav } from '../../upstream/views/nav';

declare global {
    namespace Cypress {
        interface Chainable<Subject> {
            switchPerspective(perspective: string);
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