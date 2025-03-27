import {listPage} from "../upstream/views/list-page";

export const namespaceDropdown = {
    filterByRequester: (selector: string) => {
        listPage.filter.clickFilterDropdown();
        cy.get(selector).click();
    },
    clickTheDropdown: () => {
        cy.get('[data-test-id="namespace-bar-dropdown"] button')
          .contains("Project:")
          .click({ force: true });
    },
    getProjectsDisplayed: () => {
        return cy.get('li[data-test="dropdown-menu-item-link"]');
    },
    showSystemProjects: () => {
        cy.get('input[data-test="showSystemSwitch"]').then(($toggleinput) =>{
            const on = $toggleinput.attr('data-checked-state');
            if(on == 'false'){
                cy.log('show system project switch off, will switch on');
                cy.wrap($toggleinput).click({force: true});
            }
            else {
                cy.log('show system project switch already on');
            }
        })
    },
    hideSystemProjects:() => {
        cy.get('input[data-test="showSystemSwitch"]').then(($toggleinput) =>{
            const on = $toggleinput.attr('data-checked-state');
            if(on == 'true'){
                cy.log('show system project switch on, will switch off');
                cy.wrap($toggleinput).click({force: true});
            }
            else {
                cy.log('show system project switch already off');
            }
        })
    },
    filterNamespace: (name: string) => {
        cy.get('[data-test="dropdown-text-filter"]').clear()
          .type(name, { force: true });
    },
    addFavoriteNamespace: (name: string) => {
        cy.get('span').contains(name)
          .parent()
          .parent()
          .parent('li[data-test="dropdown-menu-item-link"]')
          .within($li => {
            cy.get('button[aria-label="not starred"]').click()
          });
    },
    removeFavoriteNamespace: (name: string) => {
        cy.get('span').contains(name)
          .parent()
          .parent()
          .parent('li[data-test="dropdown-menu-item-link"]')
          .within($li => {
            cy.get('button[aria-label="starred"]').click()
          });
    },
    selectNamespace: (name: string) => {
        namespaceDropdown.clickTheDropdown();
        namespaceDropdown.showSystemProjects();
        namespaceDropdown.filterNamespace(name);
        cy.contains('button', name).click();
    }

}