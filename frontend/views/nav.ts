export const sideNav = {
  openNav: (name: string) => {
    cy.contains(`${name}`).then(($elem) => {
      if ($elem.attr('aria-expanded') == 'false') {
        $elem.click();
      }
    })
  },
  checkNoMachineResources: () => {
    sideNav.openNav('Compute');
    cy.get('[data-test="nav"]').should('contain', 'Node');
    cy.get('[data-test="nav"]').should('not.contain', 'Machine');
  }
}
