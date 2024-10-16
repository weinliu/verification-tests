import { Deployment } from "views/deployment";
describe('console configuration tests', () => {
  const capabilities_enabled_data = `{"name":"LightspeedButton","visibility":{"state":"Enabled"}}`
  const capabilities_disabled_data = `{"name":"LightspeedButton","visibility":{"state":"Disabled"}}`
  const patch_consoleoperator_lightspeed_enabled = `oc patch console.operator cluster -p '{"spec":{"customization":{"capabilities":[{"name": "LightspeedButton","visibility":{"state":"Enabled"}},{"name": "GettingStartedBanner","visibility":{"state":"Enabled"}}]}}}' --type merge`;
  const patch_consoleoperator_lightspeed_disabled = `oc patch console.operator cluster -p '{"spec":{"customization":{"capabilities":[{"name": "LightspeedButton","visibility":{"state":"Disabled"}},{"name": "GettingStartedBanner","visibility":{"state":"Enabled"}}]}}}' --type merge`;
  const query_consoleoperator_cmd = `oc get console.operator cluster -o jsonpath='{.spec.customization.capabilities}'`;
  const query_configmap_data = `oc get cm console-config -n openshift-console -o jsonpath='{.data}'`;
  after(() => {
    cy.adminCLI(`${patch_consoleoperator_lightspeed_enabled}`)
      .its('stdout')
      .should('include', 'patched')
    cy.adminCLI(`${query_consoleoperator_cmd}`)
      .its('stdout')
      .should('include', capabilities_enabled_data)
  })
  it('(OCP-53787,yanpzhan,UserInterface) Backend changes to add nodeArchitectures value to console-config file', {tags: ['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    let $architectureType;
    cy.exec(`oc get nodes -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '/architecture:/ {print $2}' | sort | uniq`, { failOnNonZeroExit: false }).then((result) => {
      $architectureType = result.stdout;
    });

    cy.exec(`oc get configmaps console-config -n openshift-console -o yaml --kubeconfig ${Cypress.env('KUBECONFIG_PATH')} | awk '$1 == "-"{ if (key == "nodeArchitectures:") print $NF; next } {key=$1}'`, { failOnNonZeroExit: false }).then((output) => {
      expect(output.stdout).to.include($architectureType);
    });
  });

  it("(OCP-69183,yanpzhan,UserInterface) Set readOnlyRootFilesystem field for both console and console operator related containers", {tags: ['@userinterface','@e2e','admin','@osd-ccs','@rosa']}, () => {
    // check console operator deployment
    Deployment.checkDeploymentFilesystem('console-operator','openshift-console-operator',0,true);
    Deployment.checkPodStatus('openshift-console-operator','name=console-operator','Running');
    // check console deployment
    Deployment.checkDeploymentFilesystem('console','openshift-console',0,false);
    Deployment.checkPodStatus('openshift-console','component=ui','Running');
    // check downloads deployment
    Deployment.checkDeploymentFilesystem('downloads','openshift-console',0,false);
    Deployment.checkPodStatus('openshift-console','component=downloads','Running');
  });

  it('(OCP-73409,yapei,UserInterface)Configure and load default Segment Api Key and proxy', {tags: ['@userinterface','@e2e','@admin','@rosa','@osd-ccs']}, () => {
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

  it('(OCP-75320,yapei,UserInterface)Cluster wide setting for showing/hiding Lightspeed button', {tags:['@userinterface','@e2e','@admin','@rosa','@osd-ccs']}, () => {
    const explain_consoleoperator = `oc explain consoles.operator.openshift.io.spec.customization.capabilities`;
    const patch_invalid_state = `oc patch console.operator cluster -p '{"spec":{"customization":{"capabilities":[{"name": "LightspeedButton","visibility":{"state":"Tested"}}]}}}'  --type merge`;
    const patch_another_entry = `oc patch console.operator cluster --type='json' -p='[{"op": "add", "path": "/spec/customization/capabilities/-", "value":{"name": "TestCap","visibility":{"state":"Enabled"}}}]'`;
    const patch_unsupported_name = `oc patch console.operator cluster -p '{"spec":{"customization":{"capabilities":[{"name": "TestCap","visibility":{"state":"Enabled"}}]}}}'  --type merge`;
    cy.adminCLI(`${explain_consoleoperator}`)
      .its('stdout')
      .should('match',/name.*required/)
      .and('match',/visibility.*required/)
    cy.adminCLI(`${query_consoleoperator_cmd}`)
      .its('stdout')
      .should('include', capabilities_enabled_data)
    cy.adminCLI(`${query_configmap_data}`)
      .its('stdout')
      .should('match', /name: LightspeedButton.*state: Enabled/)
    cy.adminCLI(`${patch_consoleoperator_lightspeed_disabled}`)
      .its('stdout')
      .should('include', 'patched')
    cy.adminCLI(`${query_consoleoperator_cmd}`)
      .its('stdout')
      .should('include', capabilities_disabled_data)
    cy.adminCLI(`${query_configmap_data}`)
      .its('stdout')
      .should('match',/name: LightspeedButton.*state: Disabled/)
    // some negative tests
    cy.adminCLI(`${patch_invalid_state}`,{failOnNonZeroExit: false})
      .its('stderr')
      .should('match', /Unsupported value.*Tested.*supported values.*Enabled.*Disabled/)
    cy.adminCLI(`${patch_another_entry}`,{failOnNonZeroExit: false})
      .its('stderr')
      .should('match', /Too many.*must have at most 2 item/)
    cy.adminCLI(`${patch_unsupported_name}`,{failOnNonZeroExit: false})
      .its('stderr')
      .should('match', /Unsupported value.*TestCap.*supported values.*LightspeedButton.*GettingStartedBanner/)
  });

})
