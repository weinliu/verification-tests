import { commandLineToolsPage } from "../../views/command-line-tools-page";
import { guidedTour } from './../../upstream/views/guided-tour';


describe('command line tools page', () => {
  const params = {
    'servingCertKeyPairSecret': 'etcd-client',
    'customerDomain1':'console-route-custom1.qe1.devcluster.openshift.com',
  }

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
  });

  after(() => {
    cy.adminCLI('oc patch ingress.config cluster --type json -p \'[{"op": "remove", "path": "/spec/componentRoutes"}]\'');
  })

  it('(OCP-64621,yanpzhan,UserInterface)cutomized "downloads" route should be updated and honored in CLI download links',{tags:['@userinterface','@e2e','admin']}, () => {
    cy.adminCLI(`oc patch ingress.config cluster --type merge -p '{"spec":{"componentRoutes":[{"name":"downloads","namespace":"openshift-console","hostname":"${params.customerDomain1}","servingCertKeyPairSecret":{"name":"${params.servingCertKeyPairSecret}"}}]}}'`)
    commandLineToolsPage.goTo();
    commandLineToolsPage.checkDownloadUrl(params.customerDomain1)
  })

})

