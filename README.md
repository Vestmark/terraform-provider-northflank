# terraform-provider-northflank

> **Unofficial community provider.** This project is maintained by [Vestmark](https://vestmark.com)
> and is not affiliated with, endorsed by, or supported by Northflank Ltd.
> The Northflank name and logo are trademarks of Northflank Ltd.
> The API spec included in this repository is reproduced from the public endpoint
> `https://api.northflank.com/v1/swagger-json` for interoperability purposes.

Terraform provider for [Northflank](https://northflank.com) — currently targeting
**secret group management** to allow variable handoff between Terraform provisioning
and Northflank services.

## Resources

| Resource | Description |
|---|---|
| `northflank_secret` | Manages a Northflank project secret group (environment variables) |

## Data Sources

| Data Source | Description |
|---|---|
| `northflank_secret` | Reads an existing Northflank project secret group |

## Example usage

```hcl
provider "northflank" {
  # Or: export NORTHFLANK_API_TOKEN=...
  api_token = var.northflank_api_token
}

resource "northflank_secret" "app" {
  project_id  = "my-project"
  name        = "App Secrets"
  secret_type = "environment"
  priority    = 10

  variables = {
    DB_PASSWORD = random_password.db.result
    DB_HOST     = aws_db_instance.main.endpoint
  }
}
```

See [examples/](examples/) for more complete configurations.

## Import

Secret groups can be imported using `<project_id>/<secret_id>`:

```sh
terraform import northflank_secret.app my-project/app-secrets
```

## Development

### Prerequisites

- [mise](https://mise.jdx.dev/) — manages the Go toolchain and CLI tools
- Terraform >= 1.0

### Setup

```sh
# Install Go and dev tools
mise install

# Build the provider binary
make build

# Run unit tests (no Northflank account required)
make test
```

### Local testing with dev_overrides

1. Install the provider to your GOBIN:
   ```sh
   make install
   ```

2. Add a `dev_overrides` block to `~/.terraformrc`:
   ```hcl
   provider_installation {
     dev_overrides {
       "vestmark-infra/northflank" = "/home/<you>/go/bin"
     }
     direct {}
   }
   ```

3. Run `terraform plan` / `apply` directly — **do not run `terraform init`** with overrides active.

### Acceptance tests (require a Northflank account)

```sh
export NORTHFLANK_API_TOKEN=<your-token>
export NORTHFLANK_TEST_PROJECT=<existing-project-id>
make testacc
```

### Regenerating the API client

The Go client in [internal/nfapi/](internal/nfapi/) is auto-generated from the
Northflank OpenAPI spec. To regenerate after an API update:

```sh
# Re-fetch the latest spec and normalize it
bash openapi/fetch.sh

# Re-run oapi-codegen
make generate
```

The normalized spec lives at [openapi/northflank-openapi.json](openapi/northflank-openapi.json)
and is committed to the repo. The raw downloaded spec (`northflank-raw.json`) is gitignored.

## Architecture

```
main.go                        — provider entry point (providerserver.Serve)
internal/
  nfapi/                       — GENERATED: oapi-codegen output (do not edit)
  client/                      — thin wrapper: auth injection, ErrNotFound sentinel,
                                 stable SecretGroup model
  provider/                    — Terraform Plugin Framework resources & data sources
openapi/
  fetch.sh + normalize.py      — spec fetch + hybrid Swagger-2/OA3 normalization
  northflank-openapi.json      — normalized spec (committed)
  oapi-codegen.yaml            — codegen config (project-secrets only)
```

The [Northflank API](https://northflank.com/docs/v1/api) spec is published at
`https://api.northflank.com/v1/swagger-json` and normalized before generation
because it is a hybrid Swagger-2 / OpenAPI-3 document (Swagger-2 response shapes
with OpenAPI-3 requestBody and pathItems).
