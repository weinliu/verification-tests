import { Overview } from '../../views/overview';
import { ClusterSettingPage } from '../../views/cluster-setting';
describe('feature for hypershift provisined cluster', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-48890,yanpzhan,UserInterface) The TopologyMode needs to be passed to the console via console-config.yaml',{tags:['@userinterface','@e2e','@hypershift-hosted','admin']}, () => {
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

  it('(OCP-50239,yanpzhan,UserInterface) Update cluster setting page and overview page for hypershift provisioned cluster',{tags:['@userinterface','@hypershift-hosted','admin']}, () => {
    cy.switchPerspective('Administrator');
    ClusterSettingPage.goToClusterSettingConfiguration();

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
