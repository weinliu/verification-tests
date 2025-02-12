const feature_name_map = {
  'OpenShift AI': 'Red Hat OpenShift AI',
  'OpenShift Lightspeed': 'OpenShift Lightspeed Operator'
  }
export const Overview = {
  goToDashboard: () => {
    cy.visit('/dashboards');
    cy.get('[data-test-id="status-card"]').should('be.visible');
  },
  closeGuidedTour: () => cy.get('#tour-step-footer-secondary').click(),
  isLoaded: () => cy.get('[data-test-id="dashboard"]', { timeout: 60000 }).should('exist'),
  clickNotificationDrawer: () => cy.get('[data-quickstart-id="qs-masthead-notifications"]').first().click(),
  toggleAbout: () => {
    cy.get('[data-test="help-dropdown-toggle"]').first().click();
    cy.get("button").contains("About").click();
  },
  checkUpperLeftLogo: () => {
    cy.get("img").should('have.attr', 'src').and('contain', 'openshift-logo.svg');
  },
  navToOverviewPage: () => {
    cy.get('[data-quickstart-id="qs-nav-home"]').click();
    cy.get('[href="/dashboards"]').click();
    Overview.isLoaded();
  },
  checkControlplaneStatusHidden: () => cy.get('[data-test="Control Plane"]').should('not.exist'),
  checkGetStartIDPConfHidden: () => cy.get('[data-test="item identity-providers"]').should('not.exist'),
  ExploreNewFeature: (featureName) => {
    let operatorName = feature_name_map[`${featureName}`];
    cy.log('operator name: '+`${operatorName}`);
    cy.contains(`${featureName}`).click();
    cy.get('[data-test-id="operator-install-btn"]').should('exist');
    cy.contains('h1', `${operatorName}`, {timeout: 30000}).should('exist');
  }
};
export const quotaCard = {
  checkQuotaCollapsed: (quotaname) => cy.get(`a[data-test-id="${quotaname}"]`).parents('button[aria-expanded="false"]').should('exist'),
  checkQuotaExpanded: (quotaname) => cy.get(`a[data-test-id="${quotaname}"]`).parents('button[aria-expanded="true"]').should('exist'),
  expandQuota: (quotaname) => cy.get(`a[data-test-id="${quotaname}"]`).parents('button[aria-expanded="false"]').children('span').first().click(),
  checkResourceQuotaInfo: (quotaname, resourceinfo, quotainfo?: string) => {
    cy.get(`a[data-test-id="${quotaname}"]`).parents('[class*=l-stack__item]').contains(`${resourceinfo}`).then(($elem) =>    {
      if (quotainfo)
        expect($elem).to.contain.text(`${quotainfo}`);
    })
  },
  checkResourceChartListed: (quotaname, quotainfo) => {
    quotaCard.checkResourceQuotaInfo(`${quotaname}`,`${quotainfo}`);
  },
};
export namespace OverviewSelectors {
  export const skipTour = "[data-test=tour-step-footer-secondary]";
};
export const statusCard = {
  isLoaded: () => {
    cy.get('[data-test-id="status-card"]')
  },
  checkAlertItem: (alert: string, checks: string) => {
    cy.get('.co-status-card__alert-item-header').contains(`${alert}`).should(`${checks}`);
  },
  toggleItemPopover: (item: string) => {
    cy.get(`button[data-test="${item}"]`, {timeout: 30000}).click({force: true});
  },
  secondaryStatus: (item: string, status: string) => {
    cy.get(`[data-status-id="${item}-secondary-status"]`,{timeout: 60000}).contains(status);
  }
};
export const notificationDrawer = {
  toggleNotificationItem: (item: string, toggle: string)=> {
    let action_flag = toggle === 'expand' ? 'true' : 'false'
    cy.get('[class*=notification-drawer__group-toggle-title]')
      .contains(`${item}`)
      .parent('button')
      .as('itemButton')
      .invoke('attr', 'aria-expanded')
      .then((expanded) => {
        if(expanded != action_flag){
          cy.get('@itemButton').click();
        }
      })
  },
  collapseCriticalAlerts: () => {
    notificationDrawer.toggleNotificationItem('Critical Alerts', 'collapse')
  },
  collapseOtherAlerts: () => {
    notificationDrawer.toggleNotificationItem('Other Alerts', 'collapse')
  },
  expandRecommendations: () => {
    notificationDrawer.toggleNotificationItem('Recommendations', 'expand')
  },
};
