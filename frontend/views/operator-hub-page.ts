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
  },
  // pass operator name that matches the Title on UI
  install: (name: string) => {
    cy.get('input[type="text"]').type(name+"{enter}")
    cy.get('[role="gridcell"]').first().within(noo => {
      cy.contains(name).should('exist').click()
    })
    cy.byTestID('confirm-action').click()
    cy.get('.co-catalog-page__overlay-actions > .pf-c-button').should('have.attr', 'href').then((href) => {
      cy.visit(String(href))
    })
    cy.byTestID('Enable-radio-input').click()
    cy.byTestID('install-operator').trigger('click')

    cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')

    // newly installed operator will always be on top of the list
    cy.get('[data-id="0-0"]').within(()=> {
      cy.byTestID("status-text", {timeout: 30000}).should('have.text', "Succeeded")
    })
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
