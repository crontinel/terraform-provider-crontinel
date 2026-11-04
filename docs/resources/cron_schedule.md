---
page_title: "crontinel_cron_schedule Resource - crontinel"
subcategory: ""
description: |-
  The schedule a job is expected to run on. Alerts when a run doesn't start on time, runs too long, or never finishes.
---

# crontinel_cron_schedule (Resource)

Monitoring starts when the schedule is created. Earlier occurrences aren't checked. Changing `command`, `expression` or `timezone` restarts monitoring at that moment.

## Example Usage

```terraform
resource "crontinel_cron_schedule" "nightly_import" {
  app        = "billing"
  command    = "nightly-import"
  expression = "0 2 * * *"
  timezone   = "UTC"
  grace_seconds       = 300
  max_runtime_seconds = 3600
  tags       = ["billing", "nightly"]
}
```

## Schema

### Required

- `app` (String) App slug. Changing it replaces the monitor.
- `command` (String) The exact command your runs report. This is the monitor's key. Changing it replaces the monitor.
- `expression` (String) Five-field cron expression.
- `timezone` (String) IANA time zone the expression runs in, for example `UTC` or `Europe/Berlin`.

### Optional

- `grace_seconds` (Number) How late a start may be before it alerts, 0 to 86400. Default `300`.
- `max_runtime_seconds` (Number) How long a run may stay open before it alerts as unfinished, 60 to 604800. Default `3600`.
- `enabled` (Boolean) Default `true`.
- `tags` (Set of String) Up to 10 lowercase slugs (letters, digits, `-` and `_`).

### Read-Only

- `id` (String) The monitor id.
- `ping_url` (String, Sensitive) Heartbeat URL for this schedule. The API shows it only when the monitor is created, so it is empty for imported or adopted schedules. Treat it as a secret.
- `version` (String) Changes whenever the record changes.

## Import

```shell
terraform import crontinel_cron_schedule.nightly_import billing/42
```

The id is `<app slug>/<monitor id>`.
