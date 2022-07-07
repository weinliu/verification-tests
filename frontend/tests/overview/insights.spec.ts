import { checkErrors } from '../../upstream/support';
import { Overview } from '../../views/overview';
import { Insights } from '../../views/insights';
describe('Insights check', () => {
  before(() => {
    cy.exec(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`);
    cy.logout;
  });

  it('(OCP-48054,admin) Add severity links on insights popover', () => {
    Overview.navToDashboard();
    Overview.isLoaded();
    Insights.openInsightsPopup();
    cy.exec(`oc get clusterversions.config.openshift.io version --template={{.spec.clusterID}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      const clusterID = result.stdout;
      Insights.checkSeverityLinks(`${clusterID}`);
      Insights.checkLinkForInsightsAdvisor(`${clusterID}`);
    });
  });
})
