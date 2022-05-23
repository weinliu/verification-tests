import {listPage} from "../upstream/views/list-page";

export const namespaceDropdown = {
    filterByRequester: (selector: string) => {
        listPage.filter.clickFilterDropdown();
        cy.get(selector).click();
    },
    clickTheDropdown: () => {
        cy.get('button.co-namespace-dropdown__menu-toggle').click()
    },
    getProjectsDisplayed: () => {
        return cy.get('li.pf-c-menu__list-item')
    },
    clickDefaultProjectToggle: () => {
        cy.get('span.pf-c-switch__toggle').click()
    }
}