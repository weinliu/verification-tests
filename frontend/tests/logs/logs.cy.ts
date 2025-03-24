import { testName } from '../../upstream/support';
import { logsPage } from '../../views/logs';
import { guidedTour } from '../../upstream/views/guided-tour';
import { podsPage } from '../../views/pods';

describe('logs related features', () => {
  before(() => {
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.adminCLI(`oc new-project ${testName}`);
  });

  after(() => {
    cy.adminCLI(`oc delete project ${testName}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`, {failOnNonZeroExit: false});
  });
  it('(OCP-69245,yanpzhan,UserInterface) Add option to enable/disable tailing to Pod log viewer',{tags:['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc get pods -n openshift-kube-apiserver -l apiserver=true -ojsonpath='{.items[0].metadata.name}'`).then((result)=> {
     const podname = result.stdout;
     cy.log(podname);
     podsPage.goToPodsLogTab('openshift-kube-apiserver',podname);
    })
    cy.get('div[data-test="no-log-lines"]').should('contain', '1000 lines');
    cy.get('input[data-test="show-full-log"]').click({force: true});
    cy.wait(10000);
    cy.log('Show full logs!');
    cy.get('div[data-test="no-log-lines"]').should('not.contain', '1000 lines');
  }),
  it('(OCP-68420,yanpzhan,UserInterface) Should maintain white-space in pod log on console',{tags:['@userinterface','@e2e','@osd-ccs','@rosa']}, () => {
    cy.adminCLI(`oc create -f ./fixtures/pods/pod-with-white-space-logs.yaml -n ${testName}`);
    cy.visit(`/k8s/ns/${testName}/pods/example/logs`);
    logsPage.selectContainer('container2');
    logsPage.setLogWrap('true');
    cy.get('span[class$=c-log-viewer__text]', {timeout: 60000}).should('contain', 'Log   TEST');
    logsPage.setLogWrap('false');
    cy.get('span[class$=c-log-viewer__text]', {timeout: 60000}).should('contain', 'Log   TEST');
  });
  it('(OCP-54875,yanpzhan,UserInterface) Configure default behavior for "Wrap lines" in log viewers by pod annotation',{tags:['@userinterface','@e2e','@osd-ccs','@rosa']}, () => {
    cy.adminCLI(`oc create -f ./fixtures/pods/example-pod.yaml -n ${testName}`);
    cy.adminCLI(`oc create -f ./fixtures/pods/example-pod-with-wrap-annotation.yaml -n ${testName}`);
    cy.visit(`/k8s/ns/${testName}/pods/examplepod/logs`);
    logsPage.setLogWrap('false');
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
