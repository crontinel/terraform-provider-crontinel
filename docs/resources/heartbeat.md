---
page_title: "crontinel_heartbeat Resource - crontinel"
subcategory: ""
description: |-
  A heartbeat monitor. Alerts when no ping arrives within the interval plus grace.
---

# crontinel_heartbeat (Resource)

## Example Usage

```terraform
resource "crontinel_heartbeat" "queue_worker" {
  app              = "billing"
  name             = "Queue worker"
  interval_minutes = 5
  grace_minutes    = 2
}
```

## Schema

### Required

- `app` (String) App slug. Changing it replaces the monitor.
- `name` (String) The monitor's key. Changing it replaces the monitor.
- `interval_minutes` (Number) Expected minutes between pings, 1 to 10080.

### Optional

- `grace_minutes` (Number) Extra minutes before alerting, 1 to 1440. Leave unset to use the server default; the value in effect is shown. Removing it later keeps the current value.
- `enabled` (Boolean) Default `true`.
- `tags` (Set of String) Up to 10 lowercase slugs.

### Read-Only

- `id` (String) The monitor id.
- `version` (String) Changes whenever the record changes.

## Import

```shell
terraform import crontinel_heartbeat.queue_worker billing/7
```
