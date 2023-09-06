export const Deployment = {
  checkAlert: () => {
    cy.get('[aria-label="Info Alert"] .pf-c-alert__title')
      .should('include.text', 'DeploymentConfig is being deprecated with OpenShift 4.14');
    cy.get('[aria-label="Info Alert"] .pf-c-alert__description a')
      .should('include.text', 'Learn more about Deployments')
      .should('have.attr', 'href')
      .and('include', '/deployments')
  }
}