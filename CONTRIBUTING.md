# Contributing

## Before you open a PR

```bash
make lint    # go vet + gofmt
make test    # go test ./... -race
```

Both must be clean. CI runs the same commands.

## Changing the tool set

1. Add the handler in `internal/tools/`, register it in the matching `*Tools()` slice.
2. Set `Mutating: true` for anything that is not a GET — the read-only guard keys off it.
3. Add a test in `internal/tools/tools_test.go`. Handler-level tests use an
   `httptest` mock and assert the exact method, path and body New API receives;
   that is where the nested-body and body-carried-id contracts are pinned.
4. If the endpoint is new in a *newer* New API release, sync the route table
   first (`make sync`) so `api_endpoints` can find it.
5. Update `docs/TOOLS.md` and the tool table in both READMEs if the surface
   changed.

## Changing the route table

Never edit `internal/spec/routes_gen.go` or `routes.baseline.json` by hand — both
are generated.

```bash
make sync           # regenerate from the newest upstream release
make sync-baseline  # refresh the baseline
git diff internal/spec
```

If `routesync` fails to parse a route from an upstream release, the bug is in
`internal/spec/parse.go`. New API uses three registration styles; a real release
will exercise all of them. Add a case to `parse_test.go` that reproduces it.

## Reporting a version gap

Open an issue with the output of:

```bash
./bin/routesync --tag <your New API tag> --out '' --check
```

Missing routes appear as `+`, routes upstream removed as `-`.

## Releasing

Releases are cut from `main` by pushing a tag; `.github/workflows/release.yml`
runs GoReleaser, which builds linux/darwin/windows binaries for amd64 and arm64,
writes `checksums.txt`, and publishes the GitHub release.

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

The version stamped into the binaries comes from the tag, so do not build
release artifacts by hand.
