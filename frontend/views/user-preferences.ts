export const preferNotifications = {
  goToNotificationsTab: () => cy.visit('/user-preferences/notifications').get('#notifications').should('exist'),
  setHideNotifications: () => {
    cy.get("input[id='console.hideUserWorkloadNotifications']").then(($elem) => {
      const $checkedstate = $elem.attr('data-checked-state');
      if($checkedstate == 'false'){
        cy.contains('Hide user workload notifications').click();
        cy.log('Hide notification!');
      }
    })
  }
},
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
