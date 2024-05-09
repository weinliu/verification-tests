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
- [netflow_export.cy.ts](netflow_export.cy.ts) can be a flaky test because loki query could take longer and file download doesn't start until loki query finishes.
