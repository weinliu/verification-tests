import { guidedTour } from './../../upstream/views/guided-tour';

describe('console-route', () => {
  const params = {
    'host': null
  };

  before(() => {
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    guidedTour.close();
    cy.adminCLI('oc get route console -n openshift-console -o template --template="{{.spec.host}}"')
      .then(result => {
        params['host'] = result.stdout;
      });
  });

  after(() => {
    cy.adminCLI('oc patch ingress.config cluster --type json -p \'[{"op": "remove", "path": "/spec/appsDomain"}]\'');
  });

  it('(OCP-64619,yanpzhan,UserInterface)console route should be re-generated using cluster domain',{tags:['@userinterface','@e2e', 'admin', '@rosa', '@osd-ccs'] }, () => {
    cy.adminCLI('oc get route console -n openshift-console -ojsonpath="{.metadata.annotations}"')
      .then(result => expect(result.stdout).contains(`"haproxy.router.openshift.io/timeout":"5m"`));
    cy.adminCLI('oc patch route console -n openshift-console --type json -p \'[{"op":"replace","path":"/spec/host","value":"example.com"}]\'');
    cy.adminCLI('oc get route console -n openshift-console -o template --template="{{.spec.host}}"')
      .then(result => expect(result.stdout).contains(params.host));
    cy.adminCLI('oc patch ingress.config cluster --type merge -p \'{"spec":{"appsDomain":"testdomain.com"}}\'');
    cy.adminCLI('oc delete route console -n openshift-console');
    cy.wait(5000);
    cy.checkCommandResult('oc get route -n openshift-console', 'console', { retries: 5, interval: 5000 });
    cy.adminCLI('oc get route console -n openshift-console -o template --template="{{.spec.host}}"')
      .then(result => expect(result.stdout).contains(params.host));
  });
});
