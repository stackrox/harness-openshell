# Configuration Test Suite

The suite drives the public CLI with canonical
`harness.openshell.dev/v1alpha1` workflows. Offline checks cover strict parsing,
resolved YAML/JSON, overrides, plan output, removed compatibility flags, and
the `init`/`doctor` surfaces. Live mode adds SDK create, describe, exec, list,
and delete against the selected gateway.

```bash
make test-suite
make test-suite-live
./test/suite/run.sh --filter=plan
./test/suite/run.sh --verbose
```

Add durable canonical fixtures under `test/configs/`; use temporary files in a
test when the malformed input itself is the behavior under test.
