import { listPage } from '../upstream/views/list-page';

export const projectsPage = {
    navToProjectsPage: () => {
        cy.visit('/k8s/cluster/projects').get('[data-test-id="resource-title"]').should('be.visible')
    },
    filterByRequester: (selector: string) => {
        listPage.filter.clickFilterDropdown();
        cy.get(selector).click();
    },
    filterMyProjects: () => projectsPage.filterByRequester('#me'),
    filterSystemProjects: () => projectsPage.filterByRequester('#system'),
    filterUserProjects: () => projectsPage.filterByRequester('#user'),
    checkProjectExists: (projectname: string) => {
        cy.get(`[data-test-id="${projectname}"]`).should('exist');
    },
    checkProjectNotExists: (projectname: string) => {
        cy.get(`[data-test-id="${projectname}"]`).should('not.exist');
    },
    checkCreationModalHelpText: () => {
        cy.get('form').should('contain', 'project is an alternative');
    },
    checkCreationModalHelpLink: () => {
        cy.get('form a').then(($link) => {
            cy.wrap($link).should('contain', 'about working with projects');
            cy.wrap($link).should('have.attr', 'href').and('contain', 'working-with-projects');
        })
    }
}
