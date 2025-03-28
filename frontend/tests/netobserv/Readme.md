# NetObserv frontend tests guidelines

## Spec files structure 
To help with memory utilization when cypress tests are run, follow below guidelines for tests structure:
- Have each spec file execution time as low as possible, preferrably less than 3mins but not more 3:30 seconds.
- Do not duplicate checks across tests.
- Focus on critical feature specific checks as opposed to checking everything in automation.
- Have each spec files with fewer tests, preferrably <= 4 `It` blocks.
- If tests are taking too long consider fragmenting into multiple `It` blocks and/or multiple spec files.

## Tests naming
- All tests must have all lower case in their naming to maintain alphabetical order of tests execution.

## Flaky tests
- [netflow_cluster_admin_group.cy.ts](netflow_cluster_admin_group.cy.ts) can be a flaky test because when user is added to group and when cluster-admin role is added to the group it takes longer indeterministic amount of time for user to have admin privileges.
- [netflow_export.cy.ts](netflow_export.cy.ts) `should validate exporting table view step` can be flaky because loki query could take longer and file download doesn't start until loki query finishes.
- [netflow_developer_view.cy.ts](netflow_developer_view.cy.ts) Developer view tests can be flaky as tests runs longer and logs in/out multiple times with different user ids.

## Topology tests API data
- When topology tests fail with UI error `undefined` for topology view and if tests uses an API fixture data, it may be a sign the API data may need an update.
- All API data are generated with Query Options `datasource=loki` and filter `Source Namespace="netobserv"` and `bnf=false`
- Before updating the correctness conditions make sure it's not a actual bug in the code and the new views are actually correct.

## NOO tests env variables
Currently tests can read 2 environment variables:
# Set `CYPRESS_NOO_CATALOG_SOURCE=upstream` to install NOO from upstream catalog source image or pass `--env NOO_CATALOG_SOURCE=upstream` with Cypress runner
# Set `CYPRESS_SKIP_NOO_INSTALL=true` to skip installing catalog source and installing the operator or pass `--env SKIP_NOO_INSTALL=true` with Cypress runner
