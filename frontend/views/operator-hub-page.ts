import { listPage } from "../upstream/views/list-page";
export const operatorHubPage = {
  goTo: () => {
    cy.visit('/operatorhub/all-namespaces');
    // the operator hub page is loaded when the count is displayed
    cy.get('.co-catalog-page__num-items').should('exist')
  },
  goToWithNamespace: (ns: string) => {
    cy.visit(`/operatorhub/ns/${ns}`);
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
    cy.get('form[data-test-group-name="catalogSourceDisplayName"]', {timeout: 60000})
      .find(`[data-test="catalogSourceDisplayName-${name}"]`)
      .find('[type="checkbox"]').check()
  },
  uncheckSourceCheckBox: (name: string) => {
    cy.get('form[data-test-group-name="catalogSourceDisplayName"]', {timeout: 60000})
      .find(`[data-test="catalogSourceDisplayName-${name}"]`)
      .find('[type="checkbox"]').uncheck()
  },
  checkInstallStateCheckBox: (state: string) =>{
    cy.get('form[data-test-group-name="installState"]')
      .find(`[data-test="installState-${state}"]`)
      .find('[type="checkbox"]')
      .check();
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
    cy.get(`[data-test-operator-row="${csvName}"]`)
      .parents('tr')
      .children()
      .contains(`${csvStatus}`, {timeout: 60000});
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

export const Operand = {
  switchToFormView: () =>{
    cy.get('#form').scrollIntoView().click();
  },
  switchToYAMLView: () => {
    cy.get('#yaml').scrollIntoView().click();
  },
  submitCreation: () => {
    cy.byTestID("create-dynamic-form").scrollIntoView().click();
  },
  expandSpec: (id: string) =>{
    cy.get(`#${id}`)
      .scrollIntoView()
      .should('have.attr', 'aria-expanded', 'false')
      .click();
  },
  collapseSpec: (id: string) => {
    cy.get(`#${id}`)
      .scrollIntoView()
      .should('have.attr', 'aria-expanded', 'true')
      .click();
  },
  clickAddNodeConfigAdvanced: () => {
    cy.get('#root_spec_nodeConfigAdvanced_add-btn')
      .scrollIntoView()
      .click();
    // this will expand 'Advanced configuration' where we set all affinities
    cy.get('#root_spec_nodeConfigAdvanced_accordion-content')
      .within(() => {
        cy.get('button.pf-c-expandable-section__toggle')
          .first()
          .click()
      })
  },
  setRandomType: () => {
    cy.get('#root_spec_nodeConfigAdvanced_0_type').click();
    cy.get('#all-link').click()
  },
  expandNodeConfigAdvanced: () => {
    Operand.expandSpec('root_spec_nodeConfigAdvanced_accordion-toggle')
  },
  expandNodeAffinity: () => {
    Operand.expandSpec('root_spec_nodeConfigAdvanced_0_nodeAffinity_accordion-toggle')
  },
  expandPodAffinity: () => {
    Operand.expandSpec('root_spec_nodeConfigAdvanced_0_podAffinity_accordion-toggle')
  },
  expandPodAntiAffinity: () => {
    Operand.expandSpec('root_spec_nodeConfigAdvanced_0_podAntiAffinity_accordion-toggle')
  },
  collapseNodeAffinity: () => {
    Operand.collapseSpec('root_spec_nodeConfigAdvanced_0_nodeAffinity_accordion-toggle')
  },
  collapsePodAffinity: () => {
    Operand.collapseSpec('root_spec_nodeConfigAdvanced_0_podAffinity_accordion-toggle')
  },
  collapsePodAntiAffinity: () => {
    Operand.collapseSpec('root_spec_nodeConfigAdvanced_0_podAntiAffinity_accordion-toggle')
  },
  nodeAffinityAddRequired: (key: string, operator: string, value: string) => {
    cy.get('#root_spec_nodeConfigAdvanced_0_nodeAffinity_accordion-content')
      .within(() => {
        cy.byButtonText('Add required').click();
      })
    cy.get('.co-affinity-term')
      .last()
      .within(() => {
        cy.byButtonText('Add expression').click();
        Operand.addExpression(key, operator, value);
      })
  }, 
  nodeAffinityAddPreferred: (weight: string,key: string, operator: string, value: string) => {
    cy.get('#root_spec_nodeConfigAdvanced_0_nodeAffinity_accordion-content')
      .within(() => {
        cy.byButtonText('Add preferred').click()
      });
    cy.get('.co-affinity-term')
    .last()
    .within(() => {
      Operand.setWeight(weight);
      cy.byButtonText('Add expression').click();
      Operand.addExpression(key, operator, value);
    })
  },
  podAffinityAddRequired: (tpkey: string,key: string, operator: string, value: string) => {
    cy.get('#root_spec_nodeConfigAdvanced_0_podAffinity_accordion-content')
      .within(() => {
        cy.byButtonText('Add required').click()
      })
    cy.get('.co-affinity-term')
    .last()
    .within(() => {
      Operand.setTopologyKey(tpkey);
      cy.byButtonText('Add expression').click();
      Operand.addExpression(key, operator, value);
    })    
  },
  podAntiAffinityAddPreferred: (weight: string, tpkey: string, key: string, operator: string, value: string) => {
    cy.get('#root_spec_nodeConfigAdvanced_0_podAntiAffinity_accordion-content')
      .within(() => {
        cy.byButtonText('Add preferred').click()
      })
    cy.get('.co-affinity-term')
    .last()
    .within(() => {
      Operand.setWeight(weight);
      Operand.setTopologyKey(tpkey);
      cy.byButtonText('Add expression').click();
      Operand.addExpression(key, operator, value);
    }) 
  },
  setWeight: (weight: string) => {
    cy.get('.co-affinity-term__weight-input')
      .last()
      .within(() => {
        cy.get('input').clear().type(weight)
      })
  },
  setTopologyKey: (key: string) =>{
    cy.get('#topology-undefined').last().clear().type(key);
  },
  addExpression: (key: string, operator: string, value?: string) => {
    cy.get('.key-operator-value__name-field')
      .last()
      .within(() => {
        cy.get('input').clear().type(key)
      })
    cy.get('.key-operator-value__operator-field')
      .last()
      .within(() => {
        cy.byLegacyTestID('dropdown-button').click();
        cy.get(`button[data-test-dropdown-menu="${operator}"]`).click();
      })
    if(value) {
      cy.get('.key-operator-value__value-field')
      .last()
      .within(() => {
        cy.get('input').clear().type(value)
      })
    }
  }
}

export const installedOperatorPage ={
  goToWithNS: (ns: string) => {
    cy.visit(`/k8s/ns/${ns}/operators.coreos.com~v1alpha1~ClusterServiceVersion`);
    cy.get('[aria-label="Installed Operators"]').should('exist');
    }
}