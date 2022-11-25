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
    favoriteNamespace: (name: string) => {
        cy.get('[data-test="dropdown-menu-item-link"]').contains(name)
          .next('button.pf-m-favorite').click();
    },

    selectNamespace: (name: string) => {
        namespaceDropdown.clickTheDropdown();
        namespaceDropdown.showSystemProjects();
        namespaceDropdown.filterNamespace(name);
        cy.contains('button', name).click();
    }

}