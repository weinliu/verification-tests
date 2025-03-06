export const preferNotifications = {
  goToNotificationsTab: () => {
    cy.visit('/user-preferences/notifications');
    cy.wait(20000);
    cy.get('[data-test="tab notifications"]').invoke('attr', 'aria-selected').should('eq', 'true');
  },
  toggleNotifications: (action: string) => {
    cy.get("input[id='console.hideUserWorkloadNotifications']").then(($elem) => {
      const checkedstate = $elem.attr('data-checked-state');
      if(checkedstate === 'true'){
        cy.log('the "Hide user workload notifications" input is currently checked');
        if(action === 'hide'){
          cy.log('nothing to do since it already checked')
        } else if(action === 'enable') {
          cy.log('uncheck "Hide user workload notifications"');
          cy.contains('Hide user workload notifications').click();
        }
      } else if (checkedstate === 'false')
        cy.log('the "Hide user workload notifications" input is currently not checked');
        if(action === 'hide'){
          cy.log('check "Hide user workload notifications"');
          cy.contains('Hide user workload notifications').click();
        } else if(action === 'enable') {
          cy.log('nothing to do since it already un-checked')
        }
      })
  },
}
export const consoleTheme = {
  setDarkTheme: () => {
    cy.get('div[data-test*="onsole.theme"] > div > button').click();
    cy.contains('span', 'Dark').click()
  },
  setLightTheme: () => {
    cy.get('div[data-test*="console.theme"] > div > button').click();
    cy.contains('span', 'Light').click()
  },
  setSystemDefaultTheme: () => {
    cy.get('div[data-test*="console.theme"] > div > button').click();
    cy.contains('span', 'System default').click()
  }
}

export const userPreferences = {
  navToGeneralUserPreferences: () => {
    cy.get('button[data-test="user-dropdown-toggle"]').click({force: true});
    cy.get('span').contains('User Preferences').click({force: true});
    cy.get('.co-user-preference-page-content__tab-content', {timeout: 20000}).should('be.visible');
    cy.get('ul[role="tablist"] a[data-test="tab general"]').click();
  },
  checkExactMatchDisabledByDefault: () => {
    cy.get('input[id="console.enableExactSearch"]').should('have.attr', 'data-checked-state', 'false');
  },
  toggleExactMatch: (action: string) => {
    cy.get('input[id="console.enableExactSearch"]').as('enableExactMatchInput').then(($elem) => {
      const checkedstate = $elem.attr('data-checked-state');
      switch(checkedstate){
        case 'true':
          if(action === 'enable'){
            cy.log('exact match already enabled, nothing to do!');
          } else if( action === 'disable'){
            cy.log('exact match already enabled, disabling');
            cy.get('@enableExactMatchInput').click();
          }
        case 'false':
          if(action === 'enable') {
            cy.log('exact match currently disabled, enabling');
            cy.get('@enableExactMatchInput').click();
          } else if (action === 'disable') {
            cy.log('exact match currently disabled, nothing to do!');
          }
      }
    })
  },
  toggleLightspeed: (action: string) => {
    cy.get('input[id="console.hideLightspeedButton"]').as('hideLightspeed').then(($elem) => {
      const checkedstate = $elem.attr('data-checked-state');
      if(checkedstate === 'true') {
        if(action === 'hide'){
          cy.log('Lightspeed button is already hidden, nothing to do!');
	} else if(action === 'display')	{
          cy.log('Lightspeed button is hidden, click to display.');
          cy.get('@hideLightspeed').click();
	}
      } else if(checkedstate === 'false') {
        if(action === 'display') {
          cy.log('Lightspeed button is already displayed, nothing to do!');
        } else if (action === 'hide') {
          cy.log('Lightspeed button is displayed, click to hide.');
          cy.get('@hideLightspeed').click();
	}
      }
    })
  },
  checkLightspeedModal: (userRole: string) => {
    cy.get('.lightspeed__popover-button').click();
    cy.contains('h1', 'Meet Openshift Lightspeed').should('exist');
    cy.contains('Benefits').should('exist');
    if(userRole === 'normal-user') {
      cy.contains('Must have administrator access').should('exist');
      cy.contains('Get started in OperatorHub').should('not.exist');
    }
    if(userRole === 'cluster-admin') {
      cy.contains('Must have administrator access').should('not.exist');
      cy.contains('Get started in OperatorHub').scrollIntoView().click();
      cy.get('[data-test-id="operator-install-btn"]').should('exist');
      cy.contains('h1', 'OpenShift Lightspeed Operator', {timeout: 30000}).should('exist');
    }
    cy.visit('/');
    cy.get('.lightspeed__popover-button').click();
    cy.contains('button', "Don't show again").scrollIntoView().click();
    cy.url().should('include',`/user-preferences`);
  },
  getLanguageOptions: () => {
    cy.get('ul > li > a[data-test="tab language"]').click({force: true});
    cy.get('input#default-language-checkbox').uncheck();
    cy.get('[data-test="console.preferredLanguage field"] button[class*="toggle"]').click();
    return cy.get('button[role="option"]');
  },
  chooseDifferentLanguage: (lang: string) => {
    cy.visit('/user-preferences/language');
    cy.get('input#default-language-checkbox').uncheck();
    cy.get('[data-test="console.preferredLanguage field"] button[class*="toggle"]').click();
    cy.byButtonText(lang).click({force: true});
  }
}
