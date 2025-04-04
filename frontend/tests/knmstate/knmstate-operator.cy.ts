import { knmstateUtils } from "../../views/knmstate-utils";
import { intPolicy, nncpPage } from "../../views/nncp-page";
import { nnsPage } from "../../views/nns-page";
import {searchPage} from "../../views/search";

describe('knmstate operator console plugin related features', () => {

  before( function() {
    cy.isPlatformSuitableForNMState().then(value => {
      if(value == false){
        this.skip();
      }
    });
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.cliLogin();
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
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("3. Check NNS");
    nnsPage.checkNNS(this.nodeList, true, "linux-bridge", nncpTest.intName, nncpTest.ip);
    
    cy.log("4. Edit NNCP");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testBridge123', nncps);

    cy.log("5. Check NNCP status again");
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("6. Check NNS again");
    nnsPage.checkNNS(this.nodeList, false, "linux-bridge", nncpTest.intName);
    
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
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("3. Check NNS");
    nnsPage.checkNNS(this.nodeList, true, "bond", nncpTest.intName);

    cy.log("4. Edit NNCP");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testBond123', nncps);

    cy.log("5. Check NNCP status again");
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("6. Check NNS again");
    nnsPage.checkNNS(this.nodeList, false, "bond", nncpTest.intName);

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
    nncpPage.checkNNCPStatus(pName, this.workerList.length+" Available");

    cy.log("3. Check NNS, only configured on the selected node");
    nnsPage.checkNNS(this.workerList, true, "linux-bridge", nncpTest.intName);

    cy.log("4. Edit NNCP to remove the interface");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testNodeSelector123', nncps);

    cy.log("5. Check NNCP status again");
    nncpPage.checkNNCPStatus(pName, this.workerList.length+" Available");

    cy.log("6. Check NNS again");
    nnsPage.checkNNS(this.nodeList, false, "linux-bridge", nncpTest.intName);

    cy.log("7. Delete NNCP");
    nncpPage.deleteNNCP(pName);
    cy.byTestID(pName).should('not.exist');
  });

  it('(OCP-64987,qiowang,SDN) Create NNCP for adding multiple interfaces on web console(from form)', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP");
    let pName = "pname-64987";
    let nncpTest1: intPolicy = {
        intName: "bridge02",
        intState: "Up",
        intType: "Bridge",
        ipv4Enable: true,
        ip: "112.9.9.9",
        prefixLen: "30",
        disAutoDNS: false,
        disAutoRoutes: false,
        disAutoGW: false,
        port: "",
        br_stpEnable: true,
        bond_cpMacFrom: "",
        bond_aggrMode: "",
        action: "",
    };
    let nncpTest2: intPolicy = {
        intName: "bond002",
        intState: "Up",
        intType: "Bonding",
        ipv4Enable: true,
        ip: "",
        prefixLen: "",
        disAutoDNS: false,
        disAutoRoutes: false,
        disAutoGW: false,
        port: "",
        br_stpEnable: false,
        bond_cpMacFrom: "",
        bond_aggrMode: "active-backup",
        action: "",
    };
    let nncps: intPolicy[] = [];
    nncps.push(nncpTest1);
    nncps.push(nncpTest2);
    nncpPage.creatNNCPFromForm(pName, 'testMulti123', nncps);

    cy.log("2. Check NNCP status");
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("3. Check NNS");
    nnsPage.checkNNS(this.nodeList, true, "linux-bridge", nncpTest1.intName);
    nnsPage.checkNNS(this.nodeList, true, "bond", nncpTest2.intName);

    cy.log("4. Edit NNCP");
    nncpTest1.action = "editOld";
    nncpTest1.intState = "Absent";
    nncpTest2.action = "editOld";
    nncpTest2.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'testMulti123', nncps);

    cy.log("5. Check NNCP status again");
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("6. Delete NNCP");
    nncpPage.deleteNNCP(pName);
    cy.byTestID(pName).should('not.exist');
  });

  it('(OCP-64852,qiowang,SDN) Check NodeNetworkConfigurationPolicy Page', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP for dummy interface via CLI");
    cy.adminCLI(`oc create -f ./fixtures/knmstate-nncp-64852.yaml`);
    let pName1 = "nncp-64852-1";
    let pName2 = "nncp-64852-2";

    cy.log("2. Check NNCP status on NodeNetworkConfigurationPolicy Page");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    nncpPage.checkNNCPStatus(pName1, this.nodeList.length+" Available");
    nncpPage.checkNNCPStatus(pName2, "Failing");

    cy.log("3. Filter NNCP with Enactment states on NodeNetworkConfigurationPolicy Page");
    nncpPage.filterByRequester("Available");
    cy.byTestID(pName1).should('exist');
    cy.byTestID(pName2).should('not.exist');
    searchPage.clearAllFilters();
    nncpPage.filterByRequester("Failing");
    cy.byTestID(pName1).should('not.exist');
    cy.byTestID(pName2).should('exist');
    searchPage.clearAllFilters();

    cy.log("4. Search NNCP with Name on NodeNetworkConfigurationPolicy Page");
    nncpPage.searchByRequester("NAME", pName1);
    cy.byTestID(pName1).should('exist');
    cy.byTestID(pName2).should('not.exist');
    searchPage.clearAllFilters();

    cy.log("5. Search NNCP with Label on NodeNetworkConfigurationPolicy Page");
    nncpPage.searchByRequester("LABEL", "test=err-value");
    cy.byTestID(pName1).should('not.exist');
    cy.byTestID(pName2).should('exist');
    searchPage.clearAllFilters();

    cy.log("6. Click NNCP Name on NodeNetworkConfigurationPolicy Page");
    cy.byTestID(pName1).click();
    cy.byLegacyTestID('resource-title').contains(pName1).should('exist');

    cy.log("7. Click NNCP Enactment states on NodeNetworkConfigurationPolicy Page");
    cy.visit("k8s/cluster/nmstate.io~v1~NodeNetworkConfigurationPolicy/");
    cy.byTestID(pName1).parents('tr').within(() => {
      cy.get('td[id="status"]').get('button[type="button"]').first().click();
    });
    cy.get('[aria-label="Policy enactments drawer"]').should('exist');
    cy.get('[aria-label="Policy enactments drawer"]').get('button[aria-label="Close"]').click();

    cy.log("8. Delete NNCP via CLI");
    cy.adminCLI(`oc delete -f ./fixtures/knmstate-nncp-64852.yaml`);

    cy.log("9. Check NNCP on web console");
    cy.byTestID(pName1).should('not.exist');
    cy.byTestID(pName2).should('not.exist');
  });

  it('(OCP-64853,qiowang,SDN) Check NodeNetworkState Page', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP for bridge interface on web console");
    let pName = "pname-64853";
    let nncpTest: intPolicy = {
        intName: "bridge03",
        intState: "Up",
        intType: "Bridge",
        ipv4Enable: true,
        ip: "113.9.9.9",
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
    nncpPage.creatNNCPFromForm(pName, "test123", nncps, "kubernetes.io/hostname", this.workerList[0]);

    cy.log("2. Check NNCP via web console and CLI");
    nncpPage.checkNNCPStatus(pName, "1 Available");
    cy.adminCLI("oc get nncp "+pName, {failOnNonZeroExit: false}).then(result => {
      expect(result.stdout).to.contain('Available');
    });
    cy.adminCLI("oc get nnce "+this.workerList[0]+"."+pName, {failOnNonZeroExit: false}).then(result => {
      expect(result.stdout).to.contain('Available');
    });

    cy.log("3. Filter NNS with interface type on NodeNetworkState Page");
    nnsPage.goToNNS();
    nnsPage.filterByRequester("linux-bridge");
    this.nodeList.forEach((node) => {
      if (node == this.workerList[0]) {
        cy.byTestID(node).should('exist');
      } else {
        cy.byTestID(node).should('not.exist');
      };
    });
    searchPage.clearAllFilters();

    cy.log("4. Search NNS with IP address on NodeNetworkState Page");
    nnsPage.searchByRequester("ip-address", nncpTest.ip);
    this.nodeList.forEach((node) => {
      if (node == this.workerList[0]) {
        cy.byTestID(node).should('exist');
      } else {
        cy.byTestID(node).should('not.exist');
      };
    });
    searchPage.clearAllFilters();

    cy.log("5. Click NNS Name on NodeNetworkState Page");
    cy.byTestID(this.workerList[0]).click();
    cy.byLegacyTestID('resource-title').contains(this.workerList[0]).should('exist');

    cy.log("6. Edit NNCP to remove the interface on web console");
    nncpTest.action = "editOld";
    nncpTest.intState = "Absent";
    nncpPage.editNNCPFromForm(pName, 'test123', nncps);

    cy.log("7. Check NNCP via web console and CLI");
    nncpPage.checkNNCPStatus(pName, "1 Available");
    cy.adminCLI("oc get nncp "+pName, {failOnNonZeroExit: false}).then(result => {
      expect(result.stdout).to.contain('Available');
    });
    cy.adminCLI("oc get nnce "+this.workerList[0]+"."+pName, {failOnNonZeroExit: false}).then(result => {
      expect(result.stdout).to.contain('Available');
    });

    cy.log("8. Delete NNCP on web console");
    nncpPage.deleteNNCP(pName);
    cy.byTestID(pName).should('not.exist');

    cy.log("9. Check NNCP vi CLI");
    cy.adminCLI("oc get nncp "+pName, {failOnNonZeroExit: false}).then(result => {
      expect(result.stderr).to.contain('not found');
    });
    cy.adminCLI("oc get nnce "+this.workerList[0]+"."+pName, {failOnNonZeroExit: false}).then(result => {
      expect(result.stderr).to.contain('not found');
    });
  });

  it('(OCP-64803,qiowang,SDN) Create/Edit NNCP on web console(from yaml)', {tags: ['e2e','admin','@smoke']}, function () {
    cy.log("1. Create NNCP");
    let pName = "nncp-64803";
    let file = "knmstate-nncp-64803.yaml"
    nncpPage.createNNCPWithYAML(file, pName);

    cy.log("2. Check NNCP status");
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("3. Check NNS");
    nnsPage.checkNNS(this.nodeList, true, "dummy", "dummy3");

    cy.log("4. Edit NNCP");
    // there is no good way to edit yaml from UI now, so use oc patch to edit yaml here
    let patchContent = `[{"op": "replace", "path": "/spec/desiredState/interfaces", "value": [{"name": "dummy3", "type": "dummy", "state": "absent"}]}]`
    cy.exec(`oc patch nncp ${pName} --type=json -p '${patchContent}' --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false });

    cy.log("5. Check NNCP status again");
    nncpPage.checkNNCPStatus(pName, this.nodeList.length+" Available");

    cy.log("6. Check NNS again");
    nnsPage.checkNNS(this.nodeList, false, "dummy");

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
