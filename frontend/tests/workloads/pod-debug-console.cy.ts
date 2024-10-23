import { guidedTour } from './../../upstream/views/guided-tour';
import { listPage } from 'upstream/views/list-page';
import { testName } from 'upstream/support';

describe('Debug console for pods', () => {

  const testParams = {
    namespace: testName,
    filename: 'deployment-with-crashloopbackoff-pod',
    name: 'crash-loop'
  }

  before(() => {
    cy.cliLogin();
    cy.exec(`oc new-project ${testParams.namespace}`);
    cy.exec(`oc apply -f ./fixtures/deployments/${testParams.filename}.yaml -n ${testParams.namespace}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  })

  after(() => {
    cy.adminCLI(`oc delete project ${testParams.namespace}`, {failOnNonZeroExit: false});
  })

  it('(OCP-48000,xiyuzhao,UserInterface), Run Pod in Debug mode',{tags:['@userinterface','@e2e']}, () => {
    // Go and find the CrashLoopBackOff Pod in the deployment page for the application
    cy.visit(`/k8s/ns/${testParams.namespace}/deployments/${testParams.name}-deployment/pods`)

    cy.get(`[data-test="status-text"]`, { timeout: 100000 }).should('contain', 'Crash');
    cy.byButtonText('CrashLoopBackOff').click()
    cy.get(`[data-test*="popup-debug-container-link-${testParams.name}-container"]`).click();

    // On the debug pod detail page; Check title and existence of debug console
    cy.byLegacyTestID(`resource-title`).should('exist')
    cy.contains('temporary pod').should('be.visible')
    cy.get('.co-terminal', {timeout: 60000}).should('be.visible')

    // Get pod name via cli
    cy.visit(`/k8s/ns/${testParams.namespace}/pods/`);
    listPage.filter.by('CrashLoopBackOff');
    //Due to bug OCPBUGS-43652, add step to avoid pod cannot be clicked on Pod List page
    cy.get('[data-test-id="dropdown-button"]').click();
    cy.get(`#name [data-test*=crash-loop]`).should('exist').click();
    cy.get('[data-test-id="horizontal-link-Logs"]').should('exist').click();
    cy.get(`[data-test="debug-container-link"]`).then($a => {
      const message = $a.text();
      expect($a, message).to.have.attr("href").contain("debug");
    })
    cy.get('[data-test="container-select"]').click();
    cy.get('button').contains(`${testParams.name}-container`);

    //Add checkpoint for customer bug OCPBUGS-12244: debug container should not copy main pod network info
    let ipaddress1, ipaddress2;
    cy.adminCLI(`oc get pods -n ${testParams.namespace} -o jsonpath='{.items[0].status.podIP}{"\t"}{.items[1].status.podIP}'`).then((result)=> {
      ipaddress1 = result.stdout.split('\t')[0]
      ipaddress2 = result.stdout.split('\t')[1]
      cy.log(`ip1: ${ipaddress1} \t ip2: ${ipaddress2}`);
      expect(`${ipaddress1}`).to.not.equal(`${ipaddress2}`);
    })
    // Check customer bug for debug container not terminating after closing tab
    cy.visit(`/dashboard`)
    cy.adminCLI(`oc get pod -n ${testParams.namespace}`)
      .should('not.match', /${testParams.name}.*-debug.*/)
  })
})
