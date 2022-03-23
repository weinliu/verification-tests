export const preferNotifications = {
  navToNotificationsTab: () => cy.visit('/user-preferences/notifications').get('#notifications').should('exist'),
  setHideNotifications: () => {
    cy.get("input[id='console.hideUserWorkloadNotifications']").then(($elem) => {
      const $checkedstate = $elem.attr('data-checked-state');
      if($checkedstate == 'false'){
        cy.contains('Hide user workload notifications').click();
        cy.log('Hide notification!');
      }
    })
  }
}
