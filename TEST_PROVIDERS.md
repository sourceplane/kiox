# Test Providers

Run these commands from the repository root.

## Build tinx

```bash
make build
```

## Legacy provider fixture

```bash
./bin/tinx release \
  --manifest testdata/echo-provider/tinx.yaml \
  --dist testdata/echo-provider/dist \
  --output testdata/echo-provider/oci

./bin/tinx init demo-echo -p testdata/echo-provider/oci as echo
./bin/tinx --workspace demo-echo status
./bin/tinx --workspace demo-echo exec echo plan
./bin/tinx workspace delete demo-echo
```

## Multi-document normalized fixture

```bash
./bin/tinx release \
  --manifest testdata/multi-tool-provider/tinx.yaml \
  --dist testdata/multi-tool-provider/dist \
  --output testdata/multi-tool-provider/oci

./bin/tinx init demo-multi -p testdata/multi-tool-provider/oci as echo
./bin/tinx --workspace demo-multi exec echo one two
./bin/tinx --workspace demo-multi exec echo-tool alpha beta
./bin/tinx workspace delete demo-multi
```

## Inline normalized fixture

```bash
./bin/tinx release \
  --manifest testdata/inline-tool-provider/tinx.yaml \
  --dist testdata/inline-tool-provider/dist \
  --output testdata/inline-tool-provider/oci

./bin/tinx init demo-inline -p testdata/inline-tool-provider/oci as inline
./bin/tinx --workspace demo-inline exec inline red blue
./bin/tinx --workspace demo-inline exec inline-tool green gold
./bin/tinx workspace delete demo-inline
```

## Setup provider fixture

```bash
./bin/tinx release \
  --manifest testdata/setup-kubectl/tinx.yaml \
  --dist testdata/setup-kubectl/dist \
  --output testdata/setup-kubectl/oci

./bin/tinx init demo-kubectl -p testdata/setup-kubectl/oci as setup-kubectl
./bin/tinx --workspace demo-kubectl ls
KUBECTL_VERSION=1.29 ./bin/tinx --workspace demo-kubectl -- kubectl version --client
./bin/tinx --workspace demo-kubectl ls
./bin/tinx workspace delete demo-kubectl
```

## Automated tests

```bash
go test ./internal/cmd -run 'TestWorkspaceSupports(NormalizedMultiToolProvider|InlineNormalizedProvider)'
go test ./internal/cmd -run TestListShowsToolInventoryForSetupProviderFlow
go test ./...
```