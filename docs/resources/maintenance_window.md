---
page_title: "crontinel_maintenance_window Resource - crontinel"
subcategory: ""
description: |-
  A maintenance window: alerts in its scope send no notifications and don't escalate while it is open.
---

# crontinel_maintenance_window (Resource)

While a window is open, alerts in its scope send no notifications and don't use up escalation steps. Incidents are still recorded. When a window ends and an incident in scope is still open and unacknowledged, it gets one normal notification.

## Example Usage

```terraform
# A one-off release window.
resource "crontinel_maintenance_window" "release" {
  app         = "billing"
  name        = "Release window"
  type        = "once"
  starts_at   = "2026-12-01T22:00:00+00:00"
  ends_at     = "2026-12-01T23:30:00+00:00"
  monitor_ids = [crontinel_cron_schedule.invoices.id]
}

# Every Sunday at 03:00 in the app's time zone, for an hour.
resource "crontinel_maintenance_window" "patching" {
  app              = "billing"
  name             = "Sunday patching"
  type             = "weekly"
  days             = [0]
  start_time       = "03:00"
  duration_minutes = 60
}
```

## Schema

### Required

- `app` (String) App slug. Changing it replaces the window.
- `name` (String) The window's key, up to 100 characters. Changing it replaces the window.
- `type` (String) `once` or `weekly`.

### Optional

- `starts_at`, `ends_at` (String) For `once`: RFC 3339 times with an offset. `ends_at` must be after `starts_at`. The same moment written in another offset is not a change. After `terraform import`, the first plan may show the time rewritten in your offset; applying it changes nothing.
- `days` (Set of Number) For `weekly`: days of the week, 0 (Sunday) to 6 (Saturday).
- `start_time` (String) For `weekly`: `HH:MM` in the app's time zone.
- `duration_minutes` (Number) For `weekly`: 5 to 10080.
- `monitor_ids` (Set of Number) Limit the window to these monitors. Leave unset to cover the whole app.
- `tags` (Set of String) Up to 10 lowercase slugs.

### Read-Only

- `id` (String) The window id.
- `version` (String) Changes whenever the record changes.

## Import

```shell
terraform import crontinel_maintenance_window.release billing/<window id>
```

List window ids with `GET /api/v1/apps/<slug>/maintenance-windows`.
