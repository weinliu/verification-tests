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
    cy.get('button[id="console.theme"]').click();
    cy.contains('button', 'Dark').click()
  },
  setLightTheme: () => {
    cy.get('button[id="console.theme"]').click();
    cy.contains('button', 'Light').click()
  },
  setSystemDefaultTheme: () => {
    cy.get('button[id="console.theme"]').click();
    cy.contains('button', 'System default').click()
  }
}
