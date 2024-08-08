describe('servicemonitor', () => {
  it('(OCP-53843,xiyuzhao,UserInterface) Add client certificate and key path to servicemonitor of console and console-operator',{tags:['@userinterface','e2e','admin','@rosa','@osd-ccs']}, () => {
    cy.exec(`oc get servicemonitor console-operator -n openshift-console-operator -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '/certFile:|keyFile:/ {print $2}'`).then((result)=> {
      expect(result.stdout).contains('tls.crt');
      expect(result.stdout).contains('tls.crt');
    });
    cy.exec(`oc get servicemonitor console -n openshift-console -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '/certFile:|keyFile:/ {print $2}'`).then((result)=> {
      expect(result.stdout).contains('tls.crt');
      expect(result.stdout).contains('tls.crt');
    });
  })
})