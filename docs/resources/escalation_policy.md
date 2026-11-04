---
page_title: "crontinel_escalation_policy Resource - crontinel"
subcategory: ""
description: |-
  The app's escalation policy: ordered steps that notify more channels when an alert isn't acknowledged.
---

# crontinel_escalation_policy (Resource)

One policy per app, with up to 5 steps. If an alert is not acknowledged or recovered, each step notifies its channels after its wait. Channels named in a step are held back from the first notification until that step. Acknowledging an alert (from the dashboard, or the link in an alert email) stops escalation for that incident.

Only alerts that open after the policy is first added escalate. Alert channels are created in the dashboard; this provider does not manage them, so pass their ids.

## Example Usage

```terraform
resource "crontinel_escalation_policy" "billing" {
  app = "billing"

  steps {
    wait_minutes = 10
    channel_ids  = [12]
  }

  steps {
    wait_minutes = 30
    channel_ids  = [12, 15] # for example a pager channel
  }
}
```

## Schema

### Required

- `app` (String) App slug. Changing it replaces the policy.

### Blocks

- `steps` (Block List, at most 5) In order.
  - `wait_minutes` (Number, Required) Minutes after the previous step (or the first notification), 5 to 1440.
  - `channel_ids` (Set of Number, Required) Alert channel ids of this app or its organization.

### Read-Only

- `id` (String) The app slug.
- `version` (String) Changes whenever the policy changes.

Destroying the resource removes the policy (the app then has no escalation).

## Import

```shell
terraform import crontinel_escalation_policy.billing billing
```
