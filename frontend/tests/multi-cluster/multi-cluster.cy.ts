import { masthead } from "../../views/masthead";

xdescribe('multi cluster testing', () => {
  const managed_cluster_kubeadmin_pwd = Cypress.env('MANAGED_CLUSTER_KUBEADMIN_PWD');
  const managed_cluster_name = Cypress.env('MANAGED_CLUSTER_NAME');
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.adminCLI(`oc patch console.operator cluster -p '{"spec":{"logLevel":"Debug","operatorLogLevel":"Debug"}}' --type merge`)
      .its('stdout')
      .should('contain', 'patched')
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc patch console.operator cluster -p '{"spec":{"logLevel":"Normal","operatorLogLevel":"Normal"}}' --type merge`)
      .its('stdout')
      .should('contain', 'patched')
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  })
  it.skip('OCP-47169 - Console Multi-Cluster Support Phase 2 - Unite Consoles (ACM, OCP)', () => {
    // check required resources are created
    cy.adminCLI(`oc get cm -n openshift-console`)
      .its('stdout')
      .should('contain', `${managed_cluster_name}-managed-cluster-api-server-cert`)
      .and('contain', `${managed_cluster_name}-managed-cluster-oauth-server-cert`)
  });

  it.skip('OCP-56687 - console operator syncs "copiedCSVsDisabled" flags from managed clusters', () => {
    // check cm/managed-clusters
    cy.adminCLI(`oc get cm managed-clusters -n openshift-console -o yaml`)
      .its('stdout')
      .should('contain', 'copiedCSVsDisabled: false')
      .and('contain', 'apiServer')
      .and('contain', 'oauth')
  });

  it.skip('OCP-59569 - Add multi cluster menu filter capability', () => {
    masthead.clusterDropdownToggle();
    masthead.filterClusters('testingmulti');
    cy.contains('No cluster found').should('be.visible');
    masthead.clearFilters();
    masthead.changeToCluster(`${managed_cluster_name}`);
    // auth against managed cluster
    cy.contains('kube:admin').should('be.visible').click();
    cy.get('#inputUsername').type('kubeadmin');
    cy.get('#inputPassword').type(managed_cluster_kubeadmin_pwd);
    cy.get('button[type="submit"]').click();
    cy.byTestID('username').should('be.visible');
    masthead.clusterDropdownToggle();
    masthead.filterClusters(managed_cluster_name);
    masthead.changeToCluster(managed_cluster_name);
    // view managed cluster pages
    cy.clickNavLink(['Workloads', 'Pods']);
    cy.clickNavLink(['Networking', 'Services']);
    cy.clickNavLink(['Operators', 'Installed Operators']);
  });

  it.skip('OCP-53847 - Use cluster proxy for managed cluster API server requests', () => {
    // wait for logs saved
    cy.wait(60000)
    // view console logs
    cy.adminCLI(`oc logs deploy/console -n openshift-console`)
      .its('stdout')
      .should('match', /https:\/\/cluster-proxy-addon-user\.multicluster-engine\.svc:9092\/d-test\/.*projects/)
      .and('match', /https:\/\/cluster-proxy-addon-user\.multicluster-engine\.svc:9092\/d-test\/.*pods/)
      .and('match', /https:\/\/cluster-proxy-addon-user\.multicluster-engine\.svc:9092\/d-test\/.*services/)
  });
})
