export const dashboard = {
    visit: () => {
        cy.visit('/monitoring/dashboards')
        cy.byTestID('dashboard-dropdown').should('exist').click()
    },
    visitDashboard: (dashboardName: string) => {
        cy.visit(`/monitoring/dashboards/${dashboardName}`)

        cy.get('#refresh-interval-dropdown-dropdown').should('exist').then(btn => {
            cy.wrap(btn).click().then(drop => {
                cy.contains('15 seconds').should('exist').click()
            })
        })

        // to load all the graphs on the dashboard
        cy.wait(5000)
        cy.get('#content-scrollable').scrollTo('bottom')
        cy.wait(5000)
    }
}

export namespace dashboardSelectors {
    export const flowStatsToggle = '[data-test-id=panel-flowlogs-pipeline-statistics] > .pf-c-button'
    export const ebpfStatsToggle = '[data-test-id=panel-e-bpf-agent-statistics] > .pf-c-button'
    export const operatorStatsToggle = '[data-test-id=panel-operator-statistics] > .pf-c-button'
    export const resourceStatsToggle = '[data-test-id=panel-resource-usage] > .pf-c-button'
}

export const graphSelector = {
    graphBody: '.co-dashboard-card__body--dashboard > div > div'
}

export const appsInfra = [
    "applications-chart",
    "infrastructure-chart"
]

Cypress.Commands.add('checkDashboards', (names) => {
    for (let i = 0; i < names.length; i++) {
        cy.byTestID(names[i]).should('exist', { timeout: 120000 })
            .find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state', { timeout: 120000 })
    }
})

declare global {
    namespace Cypress {
        interface Chainable {
            checkDashboards(names: string[]): Chainable<Element>
        }
    }
}
