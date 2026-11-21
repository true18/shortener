# staticlint

`staticlint` runs a focused set of Go analyzers over the packages supplied on the command line. From the repository root, check all packages with:

```sh
go run ./cmd/staticlint ./...
```

The command includes these standard `go/analysis/passes` checks:

- `appends` checks suspicious append calls; `asmdecl` checks assembly declarations; `assign` finds ineffective assignments; `atomic` checks common misuse of `sync/atomic`.
- `bools` checks suspicious boolean expressions; `buildtag` checks build constraints; `cgocall` checks invalid cgo calls; `composite` checks unkeyed composite literals.
- `copylocks` finds accidental copies of values containing locks; `errorsas` checks `errors.As`; `ifaceassert` checks impossible type assertions; `lostcancel` finds missed context cancel calls.
- `nilfunc` checks comparisons against function values; `printf` checks formatting calls; `shift` checks suspicious shifts; `sigchanyzer` checks signal channel usage.
- `stdmethods` checks standard method signatures; `stringintconv` checks suspicious string/integer conversions; `structtag` validates struct tags; `testinggoroutine` checks unsafe testing calls from goroutines.
- `tests` checks test and benchmark declarations; `timeformat` checks time layouts; `unmarshal` checks pointers passed to unmarshalling functions; `unreachable` finds unreachable code; `unusedresult` finds discarded results from selected functions.

All analyzers in the Staticcheck `SA` class are registered from the package's analyzer collection, rather than maintained as a local list. The `S1000` simplification check is included from Staticcheck's `simple` class.

Two additional public analyzers are included: `bodyclose` reports HTTP response bodies that are not closed, and `nilerr` reports suspicious returns where an error value is discarded in favor of nil.

The project-specific `noosexit` analyzer reports a direct call to the actual `os.Exit` function from a top-level `main` function in a package named `main`. It uses type information, so import aliases are handled and unrelated functions named `Exit` are ignored. Startup code should return errors to `main` for reporting instead of terminating from the application entrypoint.

The analyzer's own behavior is tested with `analysistest`, including aliased imports, non-main functions, non-main packages, nested function literals, and user-defined `Exit` functions.
