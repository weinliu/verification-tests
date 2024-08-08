import { listPage } from "../upstream/views/list-page";
import { nodesPage } from '../views/nodes';
import { podsPage } from '../views/pods';

describe('console feature about windows node', () => {
  before(() => {
    cy.cliLogin();
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.cliLogout();
  });

  it('(OCP-25796,yanpzhan,UserInterface) Check windows node related info on console',{tags:['@userinterface','e2e','admin']}, function () {
    cy.hasWindowsNode().then(value => {
      if(value == false){
        this.skip();
      }
    });
    cy.exec(`oc get node -l kubernetes.io/os=windows`).then((result) => {
      let win_nodes_info = result.stdout.split('\n');
      let win_nodes_name = [];

      //Check windows node in node list page
      cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
      nodesPage.goToNodesPage();
      for (let i = 1; i < win_nodes_info.length; i++ ){
        win_nodes_name[i-1] = win_nodes_info[i].split(' ')[0];
        listPage.rows.shouldExist(win_nodes_name[i-1]);
      }

      //Check windows node details info
      let nodeName, nodeOS, nodeOSImage, nodeArch, nodeKernelVersion;
      nodesPage.gotoDetail(win_nodes_name[0]);
      nodeName = win_nodes_name[0];
      nodesPage.checkDetailsField("Node name", nodeName)
      cy.exec(`oc get node ${nodeName} -o jsonpath='{.status.nodeInfo.operatingSystem}{"\t"}{.status.nodeInfo.osImage}{"\t"}{.status.nodeInfo.architecture}{"\t"}{.status.nodeInfo.kernelVersion}'`).then((output) => {
        nodeOS = output.stdout.split('\t')[0];
        nodeOSImage = output.stdout.split('\t')[1];
        nodeArch = output.stdout.split('\t')[2];
        nodeKernelVersion = output.stdout.split('\t')[3];
        nodesPage.checkDetailsField("Operating system", nodeOS)
        nodesPage.checkDetailsField("OS image", nodeOSImage)
        nodesPage.checkDetailsField("Architecture", nodeArch)
        nodesPage.checkDetailsField("Kernel version", nodeKernelVersion)
      });

      //Check windows pod
      let podName, projectName;
      cy.exec(`oc get pods -l app=win-webserver --all-namespaces -o jsonpath='{.items[0].metadata.name}{"\t"}{.items[0].metadata.namespace}'`).then((result) => {
        podName = result.stdout.split('\t')[0];
        projectName = result.stdout.split('\t')[1];
        podsPage.goToPodDetails(projectName, podName);
        nodesPage.checkDetailsField("Name", podName);
        nodesPage.checkDetailsField("Status", "Running");
        cy.get('a:contains("Terminal")').click();
        cy.get('.panel-body').should('exist');
        cy.get('.xterm-screen').should('exist');
      });
    });
  });
})
