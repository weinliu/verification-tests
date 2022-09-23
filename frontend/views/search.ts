export const searchPage = {
  navToSearchPage: () => cy.visit('/search/all-namespaces'),
  checkNoMachineResources: () => {
    searchPage.navToSearchPage();
    cy.get('button.pf-c-select__toggle').click();
    cy.get('[placeholder="Select Resource"]').type("machine");
    cy.contains('No results found');
  }
}
