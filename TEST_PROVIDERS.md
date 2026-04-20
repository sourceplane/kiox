# Test Providers

Run these commands from the repository root.

## Build kiox

```bash
make build
```

## Legacy provider fixture

```bash
./bin/kiox release \
  --manifest testdata/echo-provider/kiox.yaml \
  --dist testdata/echo-provider/dist \
  --output testdata/echo-provider/oci

./bin/kiox init demo-echo -p testdata/echo-provider/oci as echo
./bin/kiox --workspace demo-echo status
./bin/kiox --workspace demo-echo exec echo plan
./bin/kiox workspace delete demo-echo
```

## Multi-document normalized fixture

```bash
./bin/kiox release \
  --manifest testdata/multi-tool-provider/kiox.yaml \
  --dist testdata/multi-tool-provider/dist \
  --output testdata/multi-tool-provider/oci

./bin/kiox init demo-multi -p testdata/multi-tool-provider/oci as echo
./bin/kiox --workspace demo-multi exec echo one two
./bin/kiox --workspace demo-multi exec echo-tool alpha beta
./bin/kiox workspace delete demo-multi
```

## Inline normalized fixture

```bash
./bin/kiox release \
  --manifest testdata/inline-tool-provider/kiox.yaml \
  --dist testdata/inline-tool-provider/dist \
  --output testdata/inline-tool-provider/oci

./bin/kiox init demo-inline -p testdata/inline-tool-provider/oci as inline
./bin/kiox --workspace demo-inline exec inline red blue
./bin/kiox --workspace demo-inline exec inline-tool green gold
./bin/kiox workspace delete demo-inline
```

## Setup provider fixture

```bash
./bin/kiox release \
  --manifest testdata/setup-kubectl/kiox.yaml \
  --dist testdata/setup-kubectl/dist \
  --output testdata/setup-kubectl/oci

./bin/kiox init demo-kubectl -p testdata/setup-kubectl/oci as setup-kubectl
./bin/kiox --workspace demo-kubectl ls
KUBECTL_VERSION=1.29 ./bin/kiox --workspace demo-kubectl -- kubectl version --client
./bin/kiox --workspace demo-kubectl ls
./bin/kiox workspace delete demo-kubectl
```

## Automated tests

```bash
go test ./internal/cmd -run 'TestWorkspaceSupports(NormalizedMultiToolProvider|InlineNormalizedProvider)'
go test ./internal/cmd -run TestListShowsToolInventoryForSetupProviderFlow
go test ./...
```