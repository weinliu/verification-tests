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

        // to load all the graphs on the dashboard.
        cy.wait(5000)
        cy.get('#content-scrollable').scrollTo('bottom')
        cy.wait(5000)
    }
}

export const graphSelector = {
    graphBody: '.co-dashboard-card__body--dashboard > div > div'
}
