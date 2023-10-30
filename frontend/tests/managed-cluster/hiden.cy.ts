import { guidedTour } from '../../upstream/views/guided-tour';

describe("Features on managed cluster such as ROSA/OSD", () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(
      Cypress.env("LOGIN_IDP"),
      Cypress.env("LOGIN_USERNAME"),
      Cypress.env("LOGIN_PASSWORD")
    );
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it("(OCP-68228,yanpzhan) Update button is disabled on ROSA/OSD cluster", {tags: ['e2e','@osd-ccs','@rosa']}, function() {
    cy.visit('settings/cluster');
    cy.get('[data-test-id="horizontal-link-Details"]').should('be.visible');
    let brand;
    cy.window({log: true}).then((win: any) => {
      brand = win.SERVER_FLAGS.branding;
      cy.log(`${brand}`);
      if(brand == 'rosa' || brand == 'dedicated'){
	cy.log('Testing on Rosa/OSD cluster!');
	cy.get('button[data-test-id="current-channel-update-link"]').should('not.exist');
      } else {
	cy.log('Not Rosa/OSD cluster. Skip!');
      }
    });
    })
})
