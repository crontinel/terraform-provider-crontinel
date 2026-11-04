# Terraform provider for Crontinel

Manage [Crontinel](https://crontinel.com) cron schedules, heartbeats and outcome rules as code.

```hcl
resource "crontinel_cron_schedule" "nightly_import" {
  app        = "billing"
  command    = "nightly-import"
  expression = "0 2 * * *"
  timezone   = "UTC"
}

resource "crontinel_outcome_rule" "nightly_import_count" {
  app       = "billing"
  command   = crontinel_cron_schedule.nightly_import.command
  key       = "processed_records"
  kind      = "numeric_min"
  threshold = 1
}
```

Documentation is in [`docs/`](docs/index.md) and on the Terraform Registry. Needs a generated Crontinel key with `monitor:read` and `monitor:write`.

## Develop

```bash
go build ./... && go test ./...
go build -o /tmp/tf-provider/terraform-provider-crontinel .
```

Point Terraform at the local build with a CLI config:

```hcl
provider_installation {
  dev_overrides { "crontinel/crontinel" = "/tmp/tf-provider" }
  direct {}
}
```

```bash
TF_CLI_CONFIG_FILE=./dev.tfrc CRONTINEL_API_KEY=ct_mcp_... CRONTINEL_URL=http://127.0.0.1:8000 terraform plan
```

## Release

Push a tag like `v0.1.0`. GitHub Actions builds, signs the checksums with the repository's GPG key and publishes the release. Repository secrets: `GPG_PRIVATE_KEY`, `GPG_PASSPHRASE`.

MIT license.
