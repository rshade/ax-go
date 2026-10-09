# Quickstart: Known codes include the config package's runtime codes

## Read the list an agent sees

```bash
go run ./examples/integration __schema --format=json | jq -c '.error_envelope.known_codes'
```

```json
["config_invalid","config_max_bytes_invalid","config_option_invalid","config_patch_invalid","config_too_large","confirmation_required","internal_error","validation_error","warnings_as_errors"]
```

## Confirm a config failure is listed (SC-002)

```bash
head -c 2000000 /dev/zero | tr '\0' ' ' \
  | go run ./examples/integration --format=json --config=- 2>&1 >/dev/null \
  | jq -r .error_code
# config_too_large
```

The code it prints is in the list above.

## Match a code in Go without linking the runtime

```go
import "github.com/rshade/ax-go/contract"

if axErr, ok := errors.AsType[*contract.Error](err); ok &&
    axErr.ErrorCode == contract.ErrorCodeConfigTooLarge {
    // shrink the config or raise the cap, then retry
}
```

## Verify

```bash
go test -race ./contract/... ./config/... ./schema/... ./examples/integration/...
make surface-check
```
