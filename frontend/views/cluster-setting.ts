export const ClusterSettingPage = {
  goToClusterSettingDetails: () => cy.visit('/settings/cluster'),
  isLoaded: () => {
    cy.get('.co-cluster-settings', {timeout: 30000}).should('be.visible');
    cy.get('.co-m-pane__body-group', {timeout: 30000}).should('be.visible');
  },
  goToClusterSettingConfiguration: () => cy.visit('/settings/cluster/globalconfig'),
  goToConsolePlugins: () => {
    cy.visit('/k8s/cluster/operator.openshift.io~v1~Console/cluster/console-plugins');
    cy.wait(10000);
    cy.get('tbody tr', {timeout: 30000});
  },
  clickToClustSettingDetailTab: () => cy.get('[data-test-id="horizontal-link-Details"]').click(),
  checkUpstreamUrlDisabled: () => cy.get('button[data-test-id*="upstream-server-url"]').should("have.attr", "aria-disabled").and("eq", "true"),
  checkAlertMsg: (msg) => {
    cy.get('[class*="alert__title"]').should('contain', `${msg}`);
  },
  checkChannelNotEditable: () => cy.get('button[data-test-id="current-channel-update-link"]').should('not.exist'),
  checkNoAutoscalerField: () => cy.get('dt').should('not.contain', 'Cluster autoscaler'),
  checkClusterVersionNotEditable: () => {
    cy.get('[data-test-id="version"]').click();
    ClusterSettingPage.checkUpstreamUrlDisabled();
    cy.get('[data-test="Labels-details-item__edit-button"]').should('not.exist');
    cy.get('[data-test="edit-annotations"]').should('not.exist');
    cy.get('[data-test-id="horizontal-link-YAML"]').click();
    cy.get('.yaml-editor').should('exist');
    cy.get('[id="save-changes"]').should('not.exist');
  },
  checkHiddenConfiguration: () => {
    cy.get('[data-test-id="breadcrumb-link-0"]').click();
    cy.get('.loading-box__loaded').should('exist');
    const configName = ['APIServer','Authentication','DNS','FeatureGate','Networking','OAuth','Proxy','Scheduler'];
    configName.forEach(function (name) {
      cy.get(`[href="/k8s/cluster/config.openshift.io~v1~${name}/cluster"]`).should('not.exist');
    })
  },
  editUpstreamConfig: () => {
    cy.get('[data-test-id*="upstream-server-url"]').click();
    cy.get('[data-test="Custom update service.-radio-input"]').click();
    cy.get('[id="cluster-version-custom-upstream-server-url"]')
      .clear()
      .type('https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/graph');
    cy.get('[data-test="confirm-action"]').click();

  },
  configureChannel: () => {
    cy.get('[data-test-id="cluster-version"]').then(($version) => {
      const text = $version.text();
      const versionString = `stable-${text.split('.').slice(0, 2).join('.')}`
      cy.get('[data-test-id="current-channel-update-link"]').click();
      cy.get('[class*="form-control"]').clear().type(versionString);
    });
    cy.get('[data-test="confirm-action"]').click();
  },
  toggleConsolePlugin: (plugin_name: string, toggle_action: string) => {
    cy.get(`a[data-test="${plugin_name}"]`).parent().parent().parent('tr').within(() => {
      cy.get('button[data-test="edit-console-plugin"]').click({force: true});
      return;
    });
    cy.get('form[class*="modal"]').within(($modal) => {
      cy.get(`input[data-test="${toggle_action}-radio-input"]`).click({force: true});
      cy.get('button[data-test="confirm-action"]').click({force: true});
    });
  }
}
