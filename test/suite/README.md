# Configuration Test Suite

The suite drives the public CLI with canonical version 1 workflows. Offline checks cover strict parsing,
resolved YAML/JSON, overrides, plan output, removed compatibility flags, and
the workflow-only command surface. Live mode adds SDK upload, policy
enforcement, create, inspect, exec, list, and delete against the selected
gateway. Automatic cleanup is exercised by `test/test-flow.sh`; interactive TTY
remains a manual controlling-terminal check documented in the repository README.

```bash
make test-suite
make test-suite-live
./test/suite/run.sh --filter=plan
./test/suite/run.sh --verbose
```

Add durable canonical fixtures under `test/configs/`; use temporary files in a
test when the malformed input itself is the behavior under test.
