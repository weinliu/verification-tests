import { Overview, notificationDrawer } from '../../views/overview';
import { Insights } from '../../views/insights';
import { ClusterSettingPage } from '../../views/cluster-setting';

describe('Insights check', () => {
  before(() => {
    cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
    cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'));
  });

  beforeEach(() => {
    Overview.goToDashboard();
    Overview.isLoaded();
  })

  after(() => {
    cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`);
  });

  it('(OCP-48054,yanpzhan,UserInterface) Add severity links on insights popover',{tags:['@userinterface','e2e','admin','@osd-ccs','@rosa']}, () => {
    Insights.openInsightsPopup();
    cy.exec(`oc get clusterversions.config.openshift.io version --template={{.spec.clusterID}} --kubeconfig ${Cypress.env('KUBECONFIG_PATH')}`, { failOnNonZeroExit: false }).then((result) => {
      const clusterID = result.stdout;
      Insights.checkSeverityLinks(`${clusterID}`);
      Insights.checkLinkForInsightsAdvisor(`${clusterID}`);
    });
  });

  it('(OCP-47571,yapei,UserInterface) Show Cluster Support Level',{tags:['@userinterface','e2e','admin']}, () => {
    let sla_text, cluster_id;
    // get clusterID
    let include_unknown = true;
    cy.adminCLI(`oc get clusterversion version -o jsonpath='{.spec.clusterID}'`)
      .then((result) => {
        cluster_id = result.stdout;
        cy.log(cluster_id)
      })
    // define function
    const checkSLAInNotificationDrawer = () => {
      cy.log(`SLA text is ${sla_text}`)
      Overview.clickNotificationDrawer();
      notificationDrawer.collapseCriticalAlerts();
      notificationDrawer.collapseOtherAlerts();
      notificationDrawer.expandRecommendations();
      cy.get('a.co-external-link')
        .contains('Get support')
        .should('exist')
        .and('have.attr', 'href', `https://console.redhat.com/openshift/details/${cluster_id}`)
    }

    // SLA text always shown on Overview and Cluster Settings page
    cy.byLegacyTestID('sla-text')
      .within(() => {
        cy.get('.co-select-to-copy')
          .invoke('text')
          .as('sla_text_value')
          .then(value => {
            sla_text = value
            if(!sla_text.includes('Unknown')){
              // when SLA text is not Unknown, we also show info in notification drawer
              include_unknown = false
            } else {
              cy.log('SLA text is Unknown, skip checking in notification drawer')
            }
          })
      }).then(() => {
        if (!include_unknown) {
          cy.log(cluster_id)
          checkSLAInNotificationDrawer()
        }
      })

    cy.visit('/settings/cluster');
    ClusterSettingPage.isLoaded();
    cy.get('@sla_text_value').should('exist');
  });
});
