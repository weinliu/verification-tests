import { nodesPage } from "views/nodes";
import { detailsPage } from "upstream/views/details-page";
describe('nodes page', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  beforeEach(() => {
    nodesPage.goToNodesPage();
    nodesPage.setDefaultColumn();
  });

  after(() => {
    nodesPage.goToNodesPage();
    nodesPage.setDefaultColumn();
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });
  it('(OCP-69089,yanpzhan,UserInterface) Add uptime info for node on console',{tags:['@userinterface','@e2e','admin','@rosa','@hypershift-hosted']}, () => {
    nodesPage.goToNodesPage();
    nodesPage.setAdditionalColumn('Uptime');
    cy.get('th[data-label="Uptime"]').should('exist');
    cy.get('a.co-resource-item__resource-name').first().click();
    cy.get('dt:contains("Uptime")').should('exist');
    detailsPage.selectTab('Details');
    cy.get('dt:contains("Uptime")').should('exist');
  });

  it('(OCP-75487,yapei,UserInterface)Add Architecture column to NodeList and filter for archs)',{tags: ['@userinterface','@e2e','admin','@osd-ccs','@rosa','@hypershift-hosted']}, () => {
    let node_names = [];
    let node_archs = [];
    const node_arch_counts = {};
    nodesPage.goToNodesPage();
    cy.get('th[scope="col"]').should('have.length', 9);
    nodesPage.setAdditionalColumn('Architecture');
    cy.get('td[id="architecture"]').should('exist');
    function get_node_archs() {
      return cy.exec(`oc get node --no-headers --kubeconfig=${Cypress.env('KUBECONFIG_PATH')} | awk -F ' ' '{print $1}'`).then(result => {
        node_names = result.stdout.split('\n');
        node_names.forEach((node) => {
          return cy.adminCLI(`oc get node ${node} -o jsonpath='{.status.nodeInfo.architecture}'`).then(result => {
            const arch = result.stdout;
            node_archs.push(arch);
          });
        })
        return cy.wrap(node_archs);
      })
    }
    function summarize_number_by_arch(archs_list) {
      for (const num of archs_list) {
        node_arch_counts[num] = node_arch_counts[num] ? node_arch_counts[num] + 1 : 1;
      }
      return cy.wrap(node_arch_counts);

    }
    get_node_archs().then(() => {
      summarize_number_by_arch(node_archs).then(() => {
        cy.log(`node_arch_counts: ${JSON.stringify(node_arch_counts)}`);
        let archs = Object.keys(node_arch_counts);
        archs.forEach((arch) => {
          nodesPage.filterBy('Architecture', arch);
          cy.get('tr[data-test-rows="resource-row"]').should('have.length', node_arch_counts[arch]);
        })
      })
    });
  });
})


