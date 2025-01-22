export const iscUtils = {
  setCustomCatalogSource: (customCatalog) => {
    const cmd = `oc get -n openshift-marketplace catalogsource --kubeconfig=${Cypress.env('KUBECONFIG_PATH')} -o=jsonpath='{.items[*].metadata.name}' 2>&1`;
    return cy.exec(cmd, { failOnNonZeroExit: false }).then(result => {
      const out = result.stdout;
      // Determine the catalog source based on the output
      if (out.includes(customCatalog)) {
        return customCatalog;
      } else if (out.includes("qe-app-registry")) {
        return "qe-app-registry";
      } else {
        return "redhat-operators";
      }
    });
  }
};
