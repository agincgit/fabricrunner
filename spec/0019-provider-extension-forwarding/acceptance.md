# Provider extension forwarding acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestExtensionForwardedUnderNamespace` — each adapter forwards its own
  namespaced entries to the provider request;
- `TestForeignNamespaceIgnored` — an entry namespaced for another provider is
  neither forwarded nor an error;
- `TestExtensionCannotOverrideCoreField` — an entry colliding with a
  core-owned field is rejected rather than winning;
- `TestCoreTypesUnchanged` — `ModelRequest` gains no provider-specific typed
  field;
- `TestExtensionCarriesRequestClassification` — extension content is classified
  with the request;
- `TestConformanceSuiteCoversForwarding` — a third-party adapter failing any of
  the above fails the suite; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
