import testTelemetryConfigMap from '../../fixtures/cluster-monitoring-config-disable-telemeter-client.json';

describe('Telemetry cases', () => {
  const user_configuration_data = 'telemeterClient:\\n  enabled: false';
  const [ login_idp, login_username, login_password ] = [ Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD') ]
  before(() => {
    cy.configureClusterMonitoringConfig(user_configuration_data, testTelemetryConfigMap);
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${login_username}`);
  });
  after(() => {
    cy.restoreClusterMonitoringConfig();
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${login_username}`);
  })
  it('(OCP-72611,yapei,UserInterface)Disable segment analytics when cluster telemetry is disabled',{tags:['@userinterface','@e2e','admin']}, () => {
    // wait for some time until operator takes cluster-monitoring-config changes
    cy.wait(10000);
    cy.intercept(
      {
        method: 'POST',
        url: 'https://console.redhat.com/connections/api/v1/*'
      },
      {}
    ).as('postRequest');
    cy.uiLogin(login_idp, login_username, login_password);
    cy.wait(10000);
    // the POST request should not be sent
    cy.get('@postRequest.all').then((interceptions) => {
      expect(interceptions).to.have.length(0);
    });
  });
})