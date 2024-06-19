describe('user analytics from console', () => {
  it('OCP-73409 - Configure and load default Segment Api Key and proxy', {tags: ['e2e','@admin','@rosa','@osd-ccs']}, () => {
    const segment_API_HOST = `"SEGMENT_API_HOST":"console.redhat.com/connections/api/v1"`;
    const segment_JS_HOST = `"SEGMENT_JS_HOST":"console.redhat.com/connections/cdn"`;
    cy.adminCLI(`oc get cm telemetry-config -n openshift-console-operator -o jsonpath={.data}`)
      .its('stdout')
      .should('include', segment_API_HOST)
      .and('include',segment_JS_HOST);
    const cm_segment_API_HOST = `SEGMENT_API_HOST: console.redhat.com/connections/api/v1`;
    const cm_segment_JS_HOST = `SEGMENT_JS_HOST: console.redhat.com/connections/cdn`;
    cy.adminCLI(`oc get cm console-config -n openshift-console -o jsonpath={.data}`)
      .its('stdout')
      .should('include', cm_segment_API_HOST)
      .should('include', cm_segment_JS_HOST);
  });
})