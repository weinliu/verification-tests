import { Overview } from '../../views/overview';
import { ClusterSettingPage } from '../../views/cluster-setting';
describe('feature for hypershift provisined cluster', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.logout;
  });

  it('(OCP-48890,yanpzhan) The TopologyMode needs to be passed to the console via console-config.yaml', {tags: ['e2e','admin']}, () => {
    let $topologyMode;
    cy.exec(`oc get infrastructures.config.openshift.io cluster --template={{.status.controlPlaneTopology}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      $topologyMode = result.stdout;
    });
    cy.exec(`oc get cm console-config -n openshift-console --template={{.data}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((output) => {
      expect(output.stdout).to.include($topologyMode);
    });

    let cpt;
    cy.window().then((win: any) => {
      cpt = win.SERVER_FLAGS.controlPlaneTopology;
      expect(cpt).to.equal($topologyMode);
    });
  });

  it('(OCP-50239,yanpzhan) Update cluster setting page and overview page for hypershift provisioned cluster', {tags: ['e2e','admin']}, () => {
    ClusterSettingPage.goToClusterSettingConfiguration();
    //set win.SERVER_FLAGS.controlPlaneTopology to External to simulate hypershift provisioned cluster
    cy.window().then((win: any) => {
      win.SERVER_FLAGS.controlPlaneTopology = 'External';
    });

    //Check on cluster setting detail page and cluster version page
    ClusterSettingPage.clickToClustSettingDetailTab();
    ClusterSettingPage.checkAlertMsg("Control plane is hosted");
    ClusterSettingPage.checkUpstreamUrlDisabled();
    ClusterSettingPage.checkChannelNotEditable();
    ClusterSettingPage.checkNoAutoscalerField();
    ClusterSettingPage.checkClusterVersionNotEditable();

    //Check on cluster setting configuration page
    ClusterSettingPage.checkHiddenConfiguration();

    //Check on overview page
    Overview.navToOverviewPage();
    Overview.checkControlplaneStatusHidden();
    Overview.checkGetStartIDPConfHidden();
  });

})
