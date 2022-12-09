import { listPage } from "../upstream/views/list-page";
export const operatorHubPage = {
  goTo: () => {
    cy.visit('/operatorhub/all-namespaces');
    // the operator hub page is loaded when the count is displayed
    cy.get('.co-catalog-page__num-items').should('exist')
  },
  getAllTileLabels: () => {
    return cy.get('.pf-c-badge')
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
  filter: (name: string) => {
    cy.get('input[type="text"]')
      .clear()
      .type(name)
  },
  // pass operator name that matches the Title on UI
  install: (name: string) => {
    cy.get('input[type="text"]').type(name + "{enter}")
    cy.get('[role="gridcell"]').first().within(noo => {
      cy.contains(name).should('exist').click()
    })
    // ignore warning pop up for community operators
    cy.get('body').then(body => {
      if (body.find('.modal-content').length) {
        cy.byTestID('confirm-action').click()
      }
    })
    cy.get('.co-catalog-page__overlay-actions > .pf-c-button').should('have.attr', 'href').then((href) => {
      cy.visit(String(href))
    })
    cy.byTestID('Enable-radio-input').click()
    cy.byTestID('install-operator').trigger('click')

    cy.visit('k8s/all-namespaces/operators.coreos.com~v1alpha1~ClusterServiceVersion')

    cy.contains(name).parents('tr').within(() => {
      cy.byTestID("status-text", { timeout: 30000 }).should('have.text', "Succeeded")
    })
  },
  installOperator: (operatorName, csName, installNamespace?) => {
    cy.visit(`/operatorhub/subscribe?pkg=${operatorName}&catalog=${csName}&catalogNamespace=openshift-marketplace&targetNamespace=undefined`);
    if (installNamespace) {
      cy.get('[data-test="A specific namespace on the cluster-radio-input"]').click();
      cy.get('button#dropdown-selectbox').click();
      cy.contains('span', `${installNamespace}`).click();
    }
    cy.get('[data-test="install-operator"]').click();
  },
  checkOperatorStatus: (csvName, csvStatus) => {
    cy.get(`[data-test-operator-row="${csvName}"]`).parents('tr').children().contains(`${csvStatus}`, {timeout: 60000});
  },
  removeOperator: (csvName) => {
    listPage.rows.clickKebabAction(`${csvName}`,"Uninstall Operator");
    cy.get('#confirm-action').click();
    cy.get(`[data-test-operator-row="${csvName}"]`).should('not.exist');
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
  export const CUSTOM_CATALOG = "custom-auto-source"
}
