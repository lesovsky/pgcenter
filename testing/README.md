## Testing

This stuff is related to testing on local development.

- Dockerfile used for making `lesovsky/pgcenter-testing` Docker image. This image contains all necessary Postgres versions and setup scripts. Container started from the image is used for running local tests.
- fixtures.sql - SQL-script which creates all necessary stuff in testing environment
- prepare-test-environment.sh - Shell-script which setup testing environment in a container (prepare configs, start services)
- e2e.sh - Shell-script used in CI for testing high-level functionality.

See these [docs](../doc/development.md) for more info about local development.