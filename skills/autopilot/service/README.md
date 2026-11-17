# autopilot

A Go service that runs a feature request from a chat message to a pull request.

## Build and test

    make build
    make test
    make secret-scan

## Run on a local machine

    make run-local AUTOPILOT_SETTINGS=<workspace folder>/.workspace/autopilot

The service reads the workspace registry from `<workspace folder>/.workspace/`: `settings.json` and one
`projects/<project>.json` file per project. See `docs/setup.md` and `deploy/examples/`.

`autopilot serve` runs the service; `docs/setup.md` lists every setup step and check command.

## License

MIT. See LICENSE and NOTICE.
