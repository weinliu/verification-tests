export const Overview = {
  goToDashboard: () => cy.visit('/dashboards'),
  closeGuidedTour: () => cy.get('#tour-step-footer-secondary').click(),
  isLoaded: () => cy.get('[data-test-id="dashboard"]', { timeout: 60000 }).should('exist'),
  clickNotificationDrawer: () => cy.get('[data-quickstart-id="qs-masthead-notifications"]').first().click(),
  toggleAbout: () => {
    cy.get('[data-test="help-dropdown-toggle"]').first().click();
    cy.get("button").contains("About").click();
  },
  checkUpperLeftLogo: () => {
    cy.get("img").should(
      "have.attr",
      "src",
      "static/assets/openshift-logo.svg"
    );
  },
  navToOverviewPage: () => {
    cy.get('[data-quickstart-id="qs-nav-home"]').click();
    cy.get('[href="/dashboards"]').click();
    Overview.isLoaded();
  },
  checkControlplaneStatusHidden: () => cy.get('[data-test="Control Plane"]').should('not.exist'),
  checkGetStartIDPConfHidden: () => cy.get('[data-test="item identity-providers"]').should('not.exist')
};
export const quotaCard = {
  checkQuotaCollapsed: (quotaname) => cy.get(`a[data-test-id="${quotaname}"]`).parents('button[aria-expanded="false"]').should('exist'),
  checkQuotaExpanded: (quotaname) => cy.get(`a[data-test-id="${quotaname}"]`).parents('button[aria-expanded="true"]').should('exist'),
  expandQuota: (quotaname) => cy.get(`a[data-test-id="${quotaname}"]`).parents('button[aria-expanded="false"]').children('span').first().click(),
  checkResourceQuotaInfo: (quotaname, resourceinfo, quotainfo?: string) => {
    cy.get(`a[data-test-id="${quotaname}"]`).parents('.pf-l-stack__item').contains(`${resourceinfo}`).then(($elem) =>    {
      if (quotainfo)
        expect($elem).to.contain.text(`${quotainfo}`);
    })
  },
  checkResourceChartListed: (quotaname, quotainfo) => {
    quotaCard.checkResourceQuotaInfo(`${quotaname}`,`${quotainfo}`);
  },
}
export namespace OverviewSelectors {
  export const skipTour = "[data-test=tour-step-footer-secondary]";
}

export const statusCard = {
  togglePluginPopover: () => {
    cy.get('[data-item-id="Dynamic Plugins-health-item"]').within(() => {
      cy.byButtonText('Dynamic Plugins').click({force: true})
    })
  }
}
