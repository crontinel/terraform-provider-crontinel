terraform {
  required_providers {
    crontinel = {
      source = "crontinel/crontinel"
    }
  }
}

# Reads CRONTINEL_API_KEY from the environment (a generated key with monitor:read and monitor:write).
provider "crontinel" {}

# A run that doesn't start within 5 minutes of 02:00 UTC alerts.
resource "crontinel_cron_schedule" "nightly_import" {
  app        = "billing"
  command    = "nightly-import"
  expression = "0 2 * * *"
  timezone   = "UTC"
  tags       = ["billing", "nightly"]
}

# A run that exits 0 but processed nothing fails.
resource "crontinel_outcome_rule" "nightly_import_count" {
  app       = "billing"
  command   = crontinel_cron_schedule.nightly_import.command
  key       = "processed_records"
  kind      = "numeric_min"
  threshold = 1
}

resource "crontinel_heartbeat" "queue_worker" {
  app              = "billing"
  name             = "Queue worker"
  interval_minutes = 5
}
