export const Branding = {
  checkAboutModalLogo: () => {
    cy.get("div[class$='about-modal-box__brand']").find("img").should("have.attr", "src").and("contain", "red-hat-fedora.svg");
    cy.get('[aria-label="Close Dialog"]').click();
  },
  checkLoginPageLogo: () => {
    cy.get("svg").should("have.attr", "title", "Red Hat OpenShift logo");
  },
  closeModal: () => {
    cy.get('body').then(($body) => {
      if ($body.find(`[aria-label="Close Dialog"]`).length > 0) {
        cy.get('[aria-label="Close Dialog"]').click();
      }
    });
  }
};
