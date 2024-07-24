import { guidedTour } from "upstream/views/guided-tour";
import { operatorHubPage } from "views/operator-hub-page";
import { Pages } from "views/pages";

describe('Operators Installed nonlatest operator test', () => {
  const params ={
    'ns': 'ocp63222-project',
    'operatorName': 'Service Binding Operator',
    'operatorPkgname': 'rh-service-binding-operator'
  }

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc new-project ${params.ns}`)
    cy.switchPerspective('Administrator');
  });

  after(() => {
    cy.adminCLI(`oc delete project ${params.ns}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-63222,xiyuzhao,UserInterface) Console supports installing non-latest Operator versions	',{tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    let selectedVersion;
    function getSelectedVersion() {
      return new Cypress.Promise((resolve) => {
        cy.get('h5:contains("Version")')
          .next()
          .find('button')
          .then(($version) => {
            selectedVersion = $version.text().trim();
            cy.log(`Selected option version: ${selectedVersion}`);
            cy.get('[data-test-id="operator-modal-header"] h5')
              .should('contain.text', selectedVersion)
              .then(() => {
                resolve(selectedVersion);
              });
          });
      });
    };

    Pages.gotoOperatorHubPage();
    operatorHubPage.checkSourceCheckBox("red-hat");
    cy.get('input[type="text"]').clear().type(params.operatorName + "{enter}");
    cy.get('[role="gridcell"]').within(() => {
      cy.contains(params.operatorName).should('exist').click();
    })
    //Check Channel and Version dropdown is added in subscription page
    cy.contains('h5', 'Channel')
      .next()
      .find('button')
      .contains("stable");
    cy.contains('h5', 'Version')
      .next()
      .find('button')
      .click({force: true});
    cy.get('[role="option"]').eq(1).click({force:true});

    /*1.Check the selected channel and version can be carried over to 'Install Operator' page
      2.Check the Waring message for Manual approval'*/
    getSelectedVersion().then((selectedVersion) => {
      cy.get('[data-test-id="operator-install-btn"]').should('have.attr', 'href').then((href) => {
        cy.visit(String(href));
      });
      cy.get('label:contains("Version")')
        .next()
        .find('button')
        .invoke('text')
        .should('contain',selectedVersion);
      cy.url()
        .should('contain',selectedVersion)
        .and('contain','channel=stable');
    });
    cy.get('[data-test="A specific namespace on the cluster-radio-input"]').click();
    cy.get('button#dropdown-selectbox').click();
    cy.contains('span', params.ns).click();
    cy.get('[class*="alert__title"]')
      .eq(0)
      .invoke('text')
      .should('match', /^.*Manual update approval.*not installing.*latest version.*selected channel.*$/);
    cy.get('[data-test="install-operator"]').click();

    // Customer bug: check operator can be installed successfully after manual approve
    cy.get('[id="operator-install-page"]', { timeout: 120000 }).should('exist');
    cy.contains('Approve', { timeout: 120000 }).click();
    cy.contains('View Operator').should('be.visible');

    // Check the Upgrade available for the operator in Installed Operator page
    Pages.gotoInstalledOperatorPage();
    cy.byTestID('name-filter-input').clear().type("binding")
      .should(() => expect(Cypress.$(`[data-test="status-text"]`).length).to.eq(1))
      .then(() => {
        cy.get(`[data-test="status-text"]`, { timeout: 120000 })
          .contains('Succeeded')
          .parent().parent().parent()
          .find('a')
          .invoke('attr', 'href')
          .should('include','InstallPlan');
    })
    // Check the operator subcription page have a new section 'Installed Operator'
    Pages.gotoOperatorHubPage(params.ns)
    operatorHubPage.checkSourceCheckBox("red-hat");
    cy.get('input[type="text"]').clear().type(params.operatorName + "{enter}");
    cy.get('[role="gridcell"]').within(() => {
      cy.get('[data-test="success-icon"]').should('exist');
      cy.contains(params.operatorName).should('exist').click();
    });
    cy.get('[class*="description-list__text"]').each(($el, index) => {
      if (index <3){
        const keywords = ['Installed Channel', 'stable', 'Installed Version'];
        const text = $el.text();
        expect(text).to.include(keywords[index]);
      }
    });
  });
})
