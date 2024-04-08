import { knmstateUtils } from "../../views/knmstate-utils";
import { intPolicy, nncpPage } from "../../views/nncp-page";
import { nnsPage } from "../../views/nns-page";

describe('knmstate operator console plugin related features', () => {

  before( function() {
    cy.isPlatformSuitableForNMState().then(value => {
      if(value == false){
        this.skip();
      }
    });
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.switchPerspective('Administrator');
    cy.log('delete nmstate instance and uninstall knmstate operator if existed before installing');
    knmstateUtils.deleteNMStateInstace();
    knmstateUtils.uninstall();
    knmstateUtils.install();
    knmstateUtils.createNMStateInstace();
  });

  beforeEach( function () {
    cy.log("get node information");
    cy.exec("oc get node --no-headers | awk '{print $1}'", {failOnNonZeroExit: false}).then(result => {
      cy.wrap(result.stdout.split('\n')).as('nodeList');
    });
    cy.exec("oc get node -l node-role.kubernetes.io/worker --no-headers | awk '{print $1}'", {failOnNonZeroExit: false}).then(result1 => {
      cy.wrap(result1.stdout.split('\n')).as('workerList');
    });
  });

  it('(OCP-64784,qiowang,SDN) Verify NMState cosole plugin operator installation(GUI)', {tags: ['e2e','admin','@smoke']}, () => {
    nncpPage.goToNNCP();
    cy.byLegacyTestID('resource-title').contains('NodeNetworkConfigurationPolicy');
    nnsPage.goToNNS();
    cy.byLegacyTestID('resource-title').contains('NodeNetworkState');
  });

  it('(OCP-64981,qiowang,SDN) Create NNCP for adding bridge on web console(from form)', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP");
    let pName = "pname-64981";
    let nncpTest: intPolicy = {
        intName: "bridge01",
        intState: "Up",
        intType: "Bridge",
        ipv4Enable: true,
        ip: "111.9.9.9",
        prefixLen: "24",
        disAutoDNS: false,
        disAutoRoutes: false,
        disAutoGW: false,
        port: "",
        br_stpEnable: true,
        bond_cpMacFrom: "",
        bond_aggrMode: "",
        action: "",
    };
    let nncps: intPolicy[] = [];
    nncps.push(nncpTest);
    nncpPage.creatNNCPFromForm(pName, "testBridge123", nncps);

    cy.log("2. Check NNCP status");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.get(`[data-test-rows="resource-row"]`).contains(pName).parents('tr').within(() => {
      cy.get('td[id="status"]').contains(" "+this.nodeList.length+" Available", {timeout: 30000});
    });

    cy.log("3. Check NNS");
    nnsPage.goToNNS();
    for (let i = 0; i < this.nodeList.length; i++) {
      cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
        cy.get('td[id="network-interface"]').contains("linux-bridge").click();
      });
      cy.get('div[role="dialog"]').contains(nncpTest.intName).should('exist');
      cy.get('div[role="dialog"]').contains(nncpTest.ip).should('exist');
      cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
    };
    
    cy.log("4. Edit NNCP");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testBridge123', nncps);

    cy.log("5. Check NNCP status again");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.get(`[data-test-rows="resource-row"]`).contains(pName).parents('tr').within(() => {
      cy.get('td[id="status"]').contains(" "+this.nodeList.length+" Available", {timeout: 30000});
    });

    cy.log("6. Check NNS again");
    nnsPage.goToNNS();
    for (let i = 0; i < this.nodeList.length; i++) {
      cy.exec(`oc debug node/${this.nodeList[i]} -- chroot /host bash -c nmcli -f TYPE dev | grep bridge | grep -v ovs`, {failOnNonZeroExit: false}).then(result => {
        if(result.stdout.includes('bridge')){
          cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
            cy.get('td[id="network-interface"]').contains("linux-bridge").click();
          });
          cy.get('div[role="dialog"]').contains(nncpTest.intName).should('not.exist');
          cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
        } else {
          cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
            cy.get('td[id="network-interface"]').contains("linux-bridge").should('not.exist');
          });
        };
      });
    };
    
    cy.log("7. Delete NNCP");
    nncpPage.deleteNNCP(pName);
    cy.byTestID(pName).should('not.exist');
  });

  it('(OCP-64982,qiowang,SDN) Create NNCP for adding bond on web console(from form)', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP");
    let pName = "pname-64982";
    let nncpTest: intPolicy = {
        intName: "bond001",
        intState: "Up",
        intType: "Bonding",
        ipv4Enable: true,
        ip: "",
        prefixLen: "",
        disAutoDNS: true,
        disAutoRoutes: true,
        disAutoGW: true,
        port: "",
        br_stpEnable: false,
        bond_cpMacFrom: "",
        bond_aggrMode: "802.3ad",
        action: "",
    };
    let nncps: intPolicy[] = [];
    nncps.push(nncpTest);
    nncpPage.creatNNCPFromForm(pName, 'testBond123', nncps);

    cy.log("2. Check NNCP status");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.get(`[data-test-rows="resource-row"]`).contains(pName).parents('tr').within(() => {
      cy.get('td[id="status"]').contains(" "+this.nodeList.length+" Available", {timeout: 30000});
    });

    cy.log("3. Check NNS");
    nnsPage.goToNNS();
    for (let i = 0; i < this.nodeList.length; i++) {
      cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
        cy.get('td[id="network-interface"]').contains("bond").click();
      });
      cy.get('div[role="dialog"]').contains(nncpTest.intName).should('exist');
      cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
    };

    cy.log("4. Edit NNCP");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testBond123', nncps);

    cy.log("5. Check NNCP status again");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.get(`[data-test-rows="resource-row"]`).contains(pName).parents('tr').within(() => {
      cy.get('td[id="status"]').contains(" "+this.nodeList.length+" Available", {timeout: 30000});
    });

    cy.log("6. Check NNS again");
    nnsPage.goToNNS();
    for (let i = 0; i < this.nodeList.length; i++) {
      cy.exec(`oc debug node/${this.nodeList[i]} -- chroot /host bash -c nmcli -f TYPE dev | grep bond`, {failOnNonZeroExit: false}).then(result => {
        if(result.stdout.includes('bond')){
          cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
            cy.get('td[id="network-interface"]').contains("bond").click();
          });
          cy.get('div[role="dialog"]').contains(nncpTest.intName).should('not.exist');
          cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
        } else {
          cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
            cy.get('td[id="network-interface"]').contains("bond").should('not.exist');
          });
        };
      });
    };

    cy.log("7. Delete NNCP");
    nncpPage.deleteNNCP(pName);
    cy.byTestID(pName).should('not.exist');
  });

  it('(OCP-64820,qiowang,SDN) Verify can configure node selector on web console(from form)', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP with node selector");
    let pName = "pname-64820";
    let nncpTest: intPolicy = {
        intName: "bridge02",
        intState: "Up",
        intType: "Bridge",
        ipv4Enable: false,
        ip: "",
        prefixLen: "",
        disAutoDNS: false,
        disAutoRoutes: false,
        disAutoGW: false,
        port: "",
        br_stpEnable: false,
        bond_cpMacFrom: "",
        bond_aggrMode: "",
        action: "",
    };
    let nncps: intPolicy[] = [];
    nncps.push(nncpTest);
    nncpPage.creatNNCPFromForm(pName, "testNodeSelector123", nncps, "node-role.kubernetes.io/worker", "");

    cy.log("2. Check NNCP status");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.get(`[data-test-rows="resource-row"]`).contains(pName).parents('tr').within(() => {
      cy.get('td[id="status"]').contains(" "+this.workerList.length+" Available", {timeout: 30000});
    });

    cy.log("3. Check NNS, only configured on the selected node");
    nnsPage.goToNNS();
    for (let i = 0; i < this.nodeList.length; i++) {
      let isFind = false;
      for (let j = 0; j < this.workerList.length; j++) {
        if (this.nodeList[i] == this.workerList[j]) {
          isFind = true;
          break;
        };
      };
      if (isFind) {
        cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
          cy.get('td[id="network-interface"]').contains("linux-bridge").click();
        });
        cy.get('div[role="dialog"]').contains(nncpTest.intName).should('exist');
        cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
      } else {
        cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
          cy.get('td[id="network-interface"]').contains("linux-bridge").should('not.exist');
        });
      };
    };

    cy.log("4. Edit NNCP to remove the interface");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testNodeSelector123', nncps);

    cy.log("5. Check NNCP status again");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.get(`[data-test-rows="resource-row"]`).contains(pName).parents('tr').within(() => {
      cy.get('td[id="status"]').contains(" "+this.workerList.length+" Available", {timeout: 30000});
    });

    cy.log("6. Check NNS again");
    nnsPage.goToNNS();
    for (let i = 0; i < this.nodeList.length; i++) {
      cy.exec(`oc debug node/${this.nodeList[i]} -- chroot /host bash -c nmcli -f TYPE dev | grep bridge | grep -v ovs`, {failOnNonZeroExit: false}).then(result => {
        if(result.stdout.includes('bridge')){
          cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
            cy.get('td[id="network-interface"]').contains("linux-bridge").click();
          });
          cy.get('div[role="dialog"]').contains(nncpTest.intName).should('not.exist');
          cy.get('div[role="dialog"]').get('[aria-label="Close"]').last().click();
        } else {
          cy.byTestID(this.nodeList[i]).parents('tbody').within(() => {
            cy.get('td[id="network-interface"]').contains("linux-bridge").should('not.exist');
          });
        };
      });
    };

    cy.log("7. Delete NNCP");
    nncpPage.deleteNNCP(pName);
    cy.byTestID(pName).should('not.exist');
  });

  after(() => {
    knmstateUtils.deleteNMStateInstace();
    knmstateUtils.uninstall();
    cy.exec(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false })
    cy.uiLogout;
  });
  
});
