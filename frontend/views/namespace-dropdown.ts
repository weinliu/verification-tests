import {listPage} from "../upstream/views/list-page";

export const namespaceDropdown = {
    filterByRequester: (selector: string) => {
        listPage.filter.clickFilterDropdown();
        cy.get(selector).click();
    },
    clickTheDropdown: () => {
        cy.byLegacyTestID('namespace-bar-dropdown')
            .within(($div) => {
                cy.get('button').click()
            })
    },
    getProjectsDisplayed: () => {
        return cy.get('li[data-test="dropdown-menu-item-link"]');
    },
    clickDefaultProjectToggle: () => {
        cy.get('span.pf-c-switch__toggle').click()
    },

    filterNamespace: (name: string) => {
        cy.get('[data-test="dropdown-text-filter"]').clear()
          .type(name, { force: true });
    },
    favoriteNamespace: (name: string) => {
        cy.get('[data-test="dropdown-menu-item-link"]').contains(name)
          .next('button.pf-m-favorite').click();
    },

    selectNamespace: (name: string) => {
        namespaceDropdown.clickTheDropdown()
        namespaceDropdown.filterNamespace(name)
        cy.contains('button', name).click()
    }

}