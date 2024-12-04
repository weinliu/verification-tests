export const MC = {
  configurationFilesSection: (criteria: string) => {
    cy.get('[data-test-section-heading="Configuration files"]').should(criteria);
  },
  checkConfigurationFileDetails: (path, mode, overwrite: string, content) => {
    cy.get('[data-test-section-heading="Configuration files"]').scrollIntoView();
    cy.get('p').contains(path).should('exist').scrollIntoView();;
    cy.get('button[aria-label="public~Info"]').first().click();
    cy.contains(mode).should('exist');
    cy.contains(overwrite).should('exist');
    cy.get('code').first().should(($code) => {
      const text = $code.text();
      expect(text).to.include(decodeURIComponent(content).replace(/^(data:,)/, '').slice(0,5));
    })
  },
}