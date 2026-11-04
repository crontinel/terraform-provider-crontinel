---
page_title: "crontinel_outcome_rule Resource - crontinel"
subcategory: ""
description: |-
  What a run must report to count as done.
---

# crontinel_outcome_rule (Resource)

A run that exits 0 but reports less than the rule requires fails, and opens an alert. This is the check that catches a job that "succeeded" without doing the work.

`app`, `command`, `job_name`, `environment`, `expression`, `key` and `kind` identify the rule; changing any of them replaces it. Only `threshold` and `missing` update in place. The rule applies to runs received after it is saved.

## Example Usage

```terraform
resource "crontinel_outcome_rule" "invoices_sent" {
  app       = "billing"
  command   = "invoices:send"
  key       = "processed_records"
  kind      = "numeric_min"
  threshold = 1
}

# A backup newer than 26 hours.
resource "crontinel_outcome_rule" "backup_fresh" {
  app       = "billing"
  command   = "backup:run"
  key       = "latest_artifact"
  kind      = "freshness"
  threshold = 93600
}
```

## Schema

### Required

- `app` (String) App slug.
- `command` (String) The command the run reports.
- `key` (String) Metric or timestamp name in the run receipt, for example `processed_records`.
- `kind` (String) `numeric_min`, `numeric_max` or `freshness`.
- `threshold` (Number) The bound, or the maximum age in seconds for `freshness`. 0 to 31536000.

### Optional

- `job_name`, `environment`, `expression` (String) Narrow the rule to receipts that also match these.
- `missing` (String) What a run counts as when it reports no value: `unknown` (its own alert, never a success) or `fail`. Default `unknown`.

### Read-Only

- `id` (String) The rule id.
- `version` (String) Changes whenever the record changes.

## Import

```shell
terraform import crontinel_outcome_rule.invoices_sent billing/<rule id>
```

List rule ids with `GET /api/v1/apps/<slug>/outcome-rules`.
