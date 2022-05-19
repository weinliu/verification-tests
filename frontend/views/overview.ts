export const Overview = {
    navToDashboard: () => cy.visit('/dashboards'),
    closeGuidedTour: () => cy.get('#tour-step-footer-secondary').click(),
    isLoaded: () => cy.get('[data-test-id="dashboard"]', { timeout: 60000 }).should('exist'),
    clickNotificationDrawer: () => cy.get('[data-quickstart-id="qs-masthead-notifications"]').first().click()
}

export namespace OverviewSelectors {
    export const skipTour = '[data-test=tour-step-footer-secondary]'
} 
