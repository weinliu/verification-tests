import { Overview } from '../../views/overview';
describe('features for get started resources', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-73109,yanpzhan,UserInterface) Update "Explore new features and capabilities" on "Geting started resources" card', {tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    Overview.goToDashboard();
    cy.exec(`oc get packagemanifests.packages.operators.coreos.com --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | grep rhods-operator`, {failOnNonZeroExit: false}).then((result) => {
      if(result.stdout.includes('rhods')){
	Overview.ExploreNewFeature('OpenShift AI','Red Hat OpenShift AI');
      }else{
	cy.contains('OpenShift AI').should('not.exist');
      }
    });
    Overview.goToDashboard();
    cy.exec(`oc get packagemanifests.packages.operators.coreos.com --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | grep lightspeed-operator`, {failOnNonZeroExit: false}).then((result) => {
      if(result.stdout.includes('lightspeed')){
        Overview.ExploreNewFeature('OpenShift LightSpeed','OpenShift LightSpeed Operator');
      }else{
	cy.contains('OpenShift LightSpeed').should('not.exist');
      }
    });
  });

  it('(OCP-73803,yapei,UserInterface)Add user-impersonation to QuickStart and new langs to ExploreAdminFeatures',{tags: ['e2e','admin','@osd-ccs','@rosa']}, () => {
    Overview.goToDashboard();
    cy.get('button[data-test~="user-impersonation"]').as('user-impersonate-button').should('exist');
    cy.get('@user-impersonate-button').click();
    cy.get('[data-test="quickstart drawer"]').should('exist');
    cy.exec(`oc get packagemanifests.packages.operators.coreos.com --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | grep lightspeed-operator`, {failOnNonZeroExit: false}).then((result) => {
      if (result.stdout.includes('lightspeed')) {
        Overview.ExploreNewFeature('OpenShift LightSpeed','OpenShift LightSpeed Operator');
      } else {
	      cy.get('a[data-test~="new-translations"]')
          .should('have.attr', 'href')
          .and('equal', '/user-preferences/language')
      }
    });
  });
})
