export const operatorHubPage = {
  goTo: () => {
      cy.visit('/operatorhub/all-namespaces');
  },
  getAllTileLabels: () => {
    return cy.get('.pf-c-badge')
  },
    // the operator hub page is loaded when the count is displayed
  isLoaded: () => {
      cy.get('.co-catalog-page__num-items').should('exist')
  },
  checkCustomCatalog: (name: string) => {
      cy.get('form[data-test-group-name="catalogSourceDisplayName"]')
          .find(`[data-test="catalogSourceDisplayName-${name}"]`)
  },
  checkSourceCheckBox: (name: string) => {
    cy.get('form[data-test-group-name="catalogSourceDisplayName"]')
        .find(`[data-test="catalogSourceDisplayName-${name}"]`)
        .find('[type="checkbox"]').check()
  },
  uncheckSourceCheckBox: (name: string) => {
    cy.get('form[data-test-group-name="catalogSourceDisplayName"]')
        .find(`[data-test="catalogSourceDisplayName-${name}"]`)
        .find('[type="checkbox"]').uncheck()
  }
};

export namespace OperatorHubSelector {
  export const SOURCE_MAP = new Map([
        ["certified", "Certified"],
        ["community", "Community"],
        ["red-hat", "Red Hat"],
        ["marketplace", "Marketplace"],
        ["custom-auto-source", "Custom-Auto-Source"]
      ]);
  export const CUSTOM_CATALOG= "custom-auto-source"
}
