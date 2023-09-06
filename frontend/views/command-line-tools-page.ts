export const commandLineToolsPage = {
  goTo: () => {
    cy.visit('/command-line-tools');
    cy.get('.co-m-pane__name').should('exist')
},
  checkDownloadUrl: (hostname: string) => {
    cy.get('[data-test-id="oc - OpenShift Command Line Interface (CLI)"]')
      .nextAll()
      .eq(1)
      .should('exist')
      .within(() => {
        cy.get('li')
          .should('have.length.gt', 0)
          .each((li) => {
            cy.wrap(li)
              .find('a')
              .should('have.attr', 'href')
              .should('match', new RegExp(`^https:\/\/${hostname}`));
          });
      });
    }
}
