import { testName } from "upstream/support";
import { nodesPage } from "views/nodes";
import { Pages } from "views/pages";
import { podsMetricsTab, podsPage } from "views/pods";

describe('pod page', () => {
  before(() => {
    cy.cliLogin();
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
    cy.createProject(testName);
    cy.adminCLI(`oc create -f ./fixtures/networkAttachmentDefinition.yaml -n ${testName}`)
    cy.adminCLI(`oc create -f ./fixtures/pods/pod-with-limit-multiInterface.yaml -n ${testName}`);
    cy.exec(`oc get pod -n openshift-sdn --no-headers --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '{print $1;exit}'`).then((result) => {
      const sdnPodName = result.stdout;
      cy.log(sdnPodName);
      cy.exec(`oc exec ${sdnPodName} -n openshift-sdn --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} --container=sdn -i -- ip -4 route show default  | awk '{print $5;exit}'`)
        .then((result) => {
          const networkInterface = result.stdout;
          cy.log(networkInterface)
          var patch = [{
            "op": "replace",
            "path": "/spec/config",
            "value":`{"cniVersion": "0.3.1","name": "ipvlan-host-local","master": "${networkInterface}","type": "ipvlan","ipam": {"type": "host-local","subnet": "22.2.2.0/24"}}`
            }]
          const updateNetworkInterface = JSON.stringify(patch)
          cy.adminCLI(`oc patch network-attachment-definition ipvlan-host-local -n ${testName} --type='json' -p \'${updateNetworkInterface}\'`)
      })
    })
  });

  after(() => {
    cy.cliLogout();
    cy.adminCLI(`oc delete project ${testName}`);
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  it('(OCP-72714,yanpzhan,UserInterface) Show last state for container',{tags:['@userinterface','@e2e','@rosa','@osd-ccs']}, () => {
    cy.exec(`oc create -f ./fixtures/pod-72714.yaml -n ${testName}`);
    cy.wait(20000);
    let containerName, lastState;
    for(let i=0;i<3;i++){
      cy.exec(`oc get pod example-72714 -n ${testName} -ojsonpath='{.status.containerStatuses[${i}].name}{"\t"}{.status.containerStatuses[${i}].lastState}'`).then((result) => {
        containerName = result.stdout.split('\t')[0];
        lastState = result.stdout.split('\t')[1];
        podsPage.goToPodDetails(`${testName}`,'example-72714');
        if (lastState == '{}'){
          podsPage.checkContainerLastStateOnPodPage(`${containerName}`,'-');
	  Pages.gotoOneContainerPage(`${testName}`,'example-72714',`${containerName}`);
          podsPage.checkContainerLastStateOnContainerPage('-');
        }else {
          podsPage.checkContainerLastStateOnPodPage(`${containerName}`,'Terminated');
	  Pages.gotoOneContainerPage(`${testName}`,'example-72714',`${containerName}`);
          podsPage.checkContainerLastStateOnContainerPage('Terminated');
        }
      })
    }
  });
  it('(OCP-53357,xiyuzhao,UserInterface) Pod host IP is visible on Pod details page',{tags:['@userinterface','@e2e','admin','@rosa','@smoke']}, () => {
    const podname = "limitpod-withnetworks"
    podsPage.goToPodDetails(testName,podname)
    cy.get('[data-test="Host IP"]')
      .should('exist')
      .click()
      .should('contain.text', "Host IP");
    cy.get('[data-test-selector="details-item-value__Host IP"]')
      .should('be.visible')
      .then($a => {
        const podHostIP = $a.text();
        cy.exec(`oc get pod ${podname} -n ${testName} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} -o yaml | awk '/hostIP: / {print $2}'`, { failOnNonZeroExit: false })
          .then((output) => {
            expect(output.stdout).to.equal(podHostIP);
          });
        })
  });

  it('(OCP-33771,xiyuzhao,UserInterface) Check limits, requests, and secondary networks to charts in Pod Details	',{tags:['@userinterface','@e2e','admin','@rosa','@osd-ccs']}, () => {
    const resourceName = ['CPU','Memory','Filesystem', 'Network transfer','Pod count'];
    const params ={
      'ns': testName,
      'podname': 'limitpod-withnetworks',
      'networkName': 'ipvlan-host-local'
    };
    // Check chart in Pod Metrics Tab
    podsPage.goToPodDetails(params.ns, params.podname);
    cy.get('[data-test="status-text"]').should('contain.text', 'Running');
    podsPage.goToPodsMetricsTab(params.ns,params.podname);
    podsMetricsTab.checkMetricsLoaded();
    podsMetricsTab.checkMetricsURL(0,/memory/,/kube_pod_resource_limit/);
    podsMetricsTab.checkMetricsURL(1,/cpu/,/kube_pod_resource_limit/);
    podsMetricsTab.clickToMetricsPage(4,/network/);
    cy.get('[aria-label="query results table"] tbody td')
      .as('queryresult')
      .contains(params.networkName);
    // Check chart in Utilization section in Node Overview page
    podsPage.goToPodDetails(params.ns,params.podname);
    cy.get('[data-test-selector="details-item-value__Node"] span a')
      .should('have.attr','href')
      .and('match',/nodes/)
      .then((href) => {
        cy.visit(href)
      });
    cy.get('[data-test-id="utilization-card"]').should('exist');
    resourceName.forEach((name) => {
      cy.get('[data-test="utilization-item-title"]').contains(name);
      cy.get(`[aria-label="View ${name} metrics in query browser"]`)
        .should('not.contain','No datapoints found')
      });
    nodesPage.checkChartURL('CPU',/kube_pod_resource_limit/);
    nodesPage.checkChartURL('Memory',/kube_pod_resource_limit/);
    cy.get('[data-test-id="utilization-item"]')
      .as('utilization')
      .eq(3)
      .contains(/in/);
    cy.get('@utilization').eq(3).contains(/out/);
  });
})
