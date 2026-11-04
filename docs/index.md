---
page_title: "Provider: Crontinel"
description: |-
  Manage Crontinel cron schedules, heartbeats and outcome rules as code.
---

# Crontinel provider

[Crontinel](https://crontinel.com) is an outcome monitor for background work: it alerts when a job doesn't start, doesn't finish, or finishes without doing the work. This provider manages the monitors that define what "done" means, through the [configuration API](https://docs.crontinel.com/reference/config-api/).

## Authentication

Create a key under **AI connections** in the Crontinel dashboard with the `monitor:read` and `monitor:write` scopes, limited to the apps you manage. The owner of the key needs operator access to those apps. This is not an app ingest key.

```terraform
terraform {
  required_providers {
    crontinel = {
      source = "crontinel/crontinel"
    }
  }
}

provider "crontinel" {
  # api_key = "ct_mcp_..."   # or set CRONTINEL_API_KEY
  # url     = "https://app.crontinel.com"   # or set CRONTINEL_URL
}
```

## Schema

- `api_key` (String, Sensitive) The generated key. Defaults to the `CRONTINEL_API_KEY` environment variable.
- `url` (String) Crontinel origin. Defaults to `CRONTINEL_URL`, then `https://app.crontinel.com`.

## Behavior to know

- Each resource names its app with the app slug from the dashboard URL.
- A save is refused if the record was changed in Crontinel after Terraform last read it, so Terraform never silently overwrites someone else's edit. Run `terraform plan` again to see the difference.
- Creating a resource whose key already exists in Crontinel updates that record instead of failing. Use `terraform import` first if you want to review the difference.
- API limits apply: 20 cron schedules and 20 outcome rules per app, and your plan's monitor limit.
- Not covered yet: quality checks, maintenance windows, escalation policies, alert channels and deletes of mutes.
