import { Overview } from '../../views/overview';
import { Insights } from '../../views/insights';
describe('Insights check', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });

  it('(OCP-48054,yanpzhan) Add severity links on insights popover', {tags: ['e2e','admin']}, () => {
    Overview.goToDashboard();
    Overview.isLoaded();
    Insights.openInsightsPopup();
    cy.exec(`oc get clusterversions.config.openshift.io version --template={{.spec.clusterID}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      const clusterID = result.stdout;
      Insights.checkSeverityLinks(`${clusterID}`);
      Insights.checkLinkForInsightsAdvisor(`${clusterID}`);
    });
  });
})
