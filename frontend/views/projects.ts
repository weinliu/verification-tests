import { listPage } from '../upstream/views/list-page';

export const projectsPage = {
    goToProjectsPage: () => {
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
    },
    createProject: (project_name) => {
        cy.get('input[data-test="input-name"]').type(project_name);
        cy.get('[class*="modal-box__close"]').parent('[role="dialog"]').within(($div) => {
            cy.get('button').contains('Create').click({force: true});
        });
        cy.get('[class*="modal-box__body"]').should('not.exist');
    }
}

export const namespacePage = {
    createNS: (ns_name, network_policy) => {
        cy.get('input[data-test="input-name"]').type(ns_name);
        cy.get('[class*="modal-box__close"]').parent('[role="dialog"]').within(($div) => {
            cy.get('button[class*=menu-toggle]').click();
            cy.get('span').contains(network_policy).click();
        });
        cy.get('button[id="confirm-action"]').click();
    }
}
