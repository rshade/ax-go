# Quickstart: Unread struct-field gate (slopcheck)

## Run the gate

```bash
make slop-check                      # same as below
go run ./internal/cmd/slopcheck      # from the module root
```

A clean tree prints one line on stdout and exits `0`:

```json
{"status":"pass","configurations":4,"packages":57,"fields":312,"unread":0}
```

## See it fail (SC-002)

Plant an unread field in any table test:

```go
tests := []struct {
    name        string
    expectError bool // assigned below, never read
}{
    {name: "ok", expectError: false},
}
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) { /* never reads tc.expectError */ })
}
```

```bash
go run ./internal/cmd/slopcheck; echo "exit=$?"
```

stdout is empty and the exit is `2`. stderr carries one envelope:

```json
{"error_code":"slopcheck_unread_field", ... ,"context":{"cause":"…expectError (assigned at …)"}, ...}
```

Remove the field and its assignments; the gate passes again.

## Fixing a finding

| The field is… | Do |
| --- | --- |
| meant to be checked | add the assertion that reads it |
| leftover | delete the field and every assignment |
| read through reflection the gate cannot see | the gate already treats whole-value use as a read; if it still reports, the value never reaches that reader, so delete the field |

Never add an allowlist to silence a finding. Policy lives in Go constants
(FR-014), and none ships by default.

## Run the analyzer adapter's tests

```bash
go test -race ./internal/unreadfield/...
```

The adapter (`analyzer.New()`) is exercised by `analysistest` over the shared
fixture module at `internal/unreadfield/testdata/fixtures/`. To add a
classification case, add a package there with a `// want` comment. The parity
test then checks that the gate's core agrees.
