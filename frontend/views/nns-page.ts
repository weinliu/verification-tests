import {listPage} from "../upstream/views/list-page";

export const nnsPage = {
    goToNNS: () => {
        cy.contains('Networking').should('be.visible');
        cy.clickNavLink(['Networking', 'NodeNetworkState']);
    },
    filterByRequester: (selector) => {
        listPage.filter.clickFilterDropdown();
        cy.get('#'+selector).check();
    },
    searchByRequester: (selectorKey, selectorVal) => {
        listPage.filter.clickSearchByDropdown();
        cy.get(`button[data-test-dropdown-menu="${selectorKey}"]`).click();
        cy.byLegacyTestID('item-filter').type(selectorVal);
    },
};
