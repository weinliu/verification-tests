import { operatorHubPage } from "views/operator-hub-page";
import { Pages } from "views/pages";

describe('Operators Installed nonlatest operator test', () => {
  const params ={
    'ns': 'ocp63222-project',
    'operatorName': 'Service Binding Operator',
    'operatorPkgname': 'rh-service-binding-operator'
  }

  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc new-project ${params.ns}`)
    cy.uiLogin(Cypress.env("LOGIN_IDP"), Cypress.env("LOGIN_USERNAME"), Cypress.env("LOGIN_PASSWORD"));
  });

  after(() => {
    cy.adminCLI(`oc delete project ${params.ns}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-63222,xiyuzhao,UserInterface) Console supports installing non-latest Operator versions	',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    Pages.gotoOperatorHubPage();
    operatorHubPage.checkSourceCheckBox("red-hat");
    cy.get('input[type="text"]').clear().type(`${params.operatorName}{enter}`);
    cy.get('[role="gridcell"]').contains(params.operatorName).click();
    //1.Check Channel and Version dropdown is added in subscription page
    cy.contains('h5', 'Channel')
      .next()
      .find('button')
      .contains("stable");
    cy.contains('h5', 'Version')
      .next()
      .find('button')
      .click({force: true});
    //2.1.Check the selected channel and version can be carried over to 'Install Operator' page
    cy.get('[role="option"]').eq(1).click({force:true});
    cy.get('h5:contains("Version")')
      .next()
      .find('*[class*="toggle__text"]')
      .invoke('text')
      .then(text => text.trim())
      .as('selectedVersion');
    cy.get('[data-test-id="operator-install-btn"]')
      .invoke('attr', 'href')
      .as('installHref');

    cy.get('@selectedVersion').then(selectedVersion => {
      cy.get('@installHref').then(href => {
        cy.visit(String(href));

        cy.get('label:contains("Version")').should('exist');
        cy.get('[data-test="operator-version-select-toggle"] span')
          .should('contain', selectedVersion);
        cy.url()
          .should('contain', selectedVersion)
          .and('contain', 'channel=stable');
      });
    });
    //2.2.Check the Waring message for Manual approval'*/
    cy.get('[data-test="A specific namespace on the cluster-radio-input"]').click();
    cy.get('button#dropdown-selectbox').click();
    cy.contains('span', params.ns).click();
    cy.get('[class*="alert__title"]')
      .eq(0)
      .invoke('text')
      .should('match', /^.*Manual update approval.*not installing.*latest version.*selected channel.*$/);
    //3.For Customer Bug: check operator can be installed successfully after manual approve
    cy.get('[data-test="install-operator"]').click();
    cy.get('[id="operator-install-page"]', { timeout: 120000 }).should('exist');
    cy.contains('Approve', { timeout: 240000 }).click().then(() => {
      cy.contains('View Operator', { timeout: 120000 }).should('be.visible');
    });
    //4.Check the InstallPlan is available for the Manual installed operator in Installed Operator page
    Pages.gotoInstalledOperatorPage();
    cy.byTestID('name-filter-input')
      .clear()
      .type("binding")
      .should(() => expect(Cypress.$(`[data-test="status-text"]`).length).to.eq(1))
      .then(() => {
        cy.get(`[data-test="status-text"]`, { timeout: 120000 })
          .contains('Succeeded')
          .parent().parent().parent()
          .find('a')
          .invoke('attr', 'href')
          .should('include','InstallPlan');
    })
    //5.1.Check a successful icon is added for the Installed Operator
    Pages.gotoOperatorHubPage(params.ns)
    operatorHubPage.checkSourceCheckBox("red-hat");
    cy.get('input[type="text"]').clear().type(`${params.operatorName}{enter}`);
    cy.get('[data-test="success-icon"]').should('exist');
    //5.2.Check the operator subcription page added new section 'Installed Operator', and have 3 new values
    cy.get('[role="gridcell"]').contains(params.operatorName).click();
    cy.get('[class*="description-list__text"]').each(($el, index) => {
      if (index <3){
        const keywords = ['Installed Channel', 'stable', 'Installed Version'];
        const text = $el.text();
        expect(text).to.include(keywords[index]);
      }
    });
  });
})
