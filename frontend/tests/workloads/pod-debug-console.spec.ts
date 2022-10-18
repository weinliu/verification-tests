import { guidedTour } from './../../upstream/views/guided-tour';
import { nav } from '../../upstream/views/nav';

describe('Debug console for pods', () => {

  const testParams = {
    namespace: 'pod-debug-console-48000',
    name: 'nodejs-ex-git',
    gitURL: 'https://github.com/sclorg/nodejs-ex.git',
    invalidCommand: 'star a wktw',
    waitTime: 50000,
    debugConsoleLoadTime: 60000
  }

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
    guidedTour.isOpen()
    guidedTour.close()
    cy.createProject(testParams.namespace)
  })

  after(() => {
    cy.exec(`oc delete project ${testParams.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
    cy.logout()
  })

  it('(OCP-48000, xiangyli), Run Pod in Debug mode', () => {
    // Import the nodejs-ex.git and run the invalid command to cause CrashLoopBackoof && Twice
    nav.sidenav.switcher.changePerspectiveTo('Developer')
    cy.visit(`/import/ns/${testParams.namespace}`)
    cy.byLegacyTestID('git-form-input-url').clear().type(testParams.gitURL)
    cy.get('#form-input-git-url-field-helper').contains('Validated')
    cy.get('#form-input-name-field').clear().type(testParams.name)
    cy.get('#form-input-image-imageEnv-NPM_RUN-field').clear().type(testParams.invalidCommand)
    cy.byLegacyTestID('submit-button').click({force: true})
    // Make sure the deployment is created
    cy.byLegacyTestID('nodejs-ex-git').should('be.visible')

    // Go and find the CrashLoopBackOff Pod in the deployment page for the application
    cy.visit(`/k8s/ns/${testParams.namespace}/deployments/${testParams.name}/pods`)

    cy.byTestID('status-text').should('contain', 'Crash')
    cy.byButtonText('CrashLoopBackOff')
      .click()
    cy.byTestID(`popup-debug-container-link-${testParams.name}`)
      .click()

    // On the debug pod detail page; Check title and existence of debug console
    cy.byLegacyTestID(`resource-title`).should('exist')
    cy.contains('temporary pod').should('be.visible')
    cy.get('.co-terminal', {timeout: testParams.debugConsoleLoadTime})
      .should('be.visible')

    // Get pod name via cli
    let podName: string
    let rows: string[]
    cy.exec(`oc get pod -n ${testParams.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      .then((res) => {
        rows = res.stdout.split('\n').slice(1)
        podName = rows[1].split(' ')[0]
        cy.log(podName)
        // Check drop down menu in logs for CrashLoopBackOff Pod
        cy.visit(`/k8s/ns/${testParams.namespace}/pods/${podName}/logs`)
        cy.get(`a:contains(Debug container)`).then($a => {
          const message = $a.text();
          expect($a, message).to.have.attr("href").contain("debug");
        })
        cy.byLegacyTestID('dropdown-button').click()
        cy.get(`#${testParams.name}-link`).contains(testParams.name)
      })

    // Check customer bug for debug container not terminating after closing tab
    cy.visit(`/dashboard`)
    cy.exec(`oc get pod -n ${testParams.namespace} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`)
      .should('not.match', /${testParams.name}.*-debug.*/)
  })
})