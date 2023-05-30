import { testName } from '../../upstream/support';
import { logsPage } from '../../views/logs';
import { guidedTour } from '../../upstream/views/guided-tour';

describe('logs related features', () => {
  before(() => {
    cy.cliLogin();
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.exec(`oc new-project ${testName}`);
  });

  after(() => {
    cy.exec(`oc delete project ${testName}`);
    cy.cliLogout();
  });

  it('(OCP-54875,yanpzhan) Configure default behavior for "Wrap lines" in log viewers by pod annotation', {tags: ['e2e','@osd-ccs','@rosa']}, () => {
    cy.exec(`oc create -f ./fixtures/pods/example-pod.yaml -n ${testName}`);
    cy.exec(`oc create -f ./fixtures/pods/example-pod-with-wrap-annotation.yaml -n ${testName}`);
    cy.visit(`/k8s/ns/${testName}/pods/examplepod/logs`);
    logsPage.checkLogWraped('false');
    cy.visit(`/k8s/ns/${testName}/pods/wraplogpod/logs`);
    logsPage.checkLogWraped('true');
    logsPage.setLogWrap('false');
    cy.visit(`/k8s/ns/${testName}/pods/examplepod/logs`);
    logsPage.checkLogWraped('false');
    cy.visit(`/k8s/ns/${testName}/pods/wraplogpod/logs`);
    logsPage.checkLogWraped('true');
  });
})
