package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// These tests run real Terraform against fakeAPI. They need the terraform binary and TF_ACC=1:
//
//	TF_ACC=1 go test ./internal/provider/ -run TestAcc -v
var testProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"crontinel": providerserver.NewProtocol6WithError(New("test")()),
}

func testPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run the acceptance tests")
	}
}

func providerBlock(f *fakeAPI) string {
	return fmt.Sprintf("provider \"crontinel\" {\n  url     = %q\n  api_key = %q\n}\n", f.URL, fakeKey)
}

func TestAccCronSchedule(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	cfg := func(expr, command string) string {
		return providerBlock(f) + fmt.Sprintf(`
resource "crontinel_cron_schedule" "x" {
  app        = "billing"
  command    = %q
  expression = %q
  timezone   = "UTC"
  tags       = ["billing", "nightly"]
}`, command, expr)
	}
	var firstID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: cfg("0 2 * * *", "invoices:send"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("crontinel_cron_schedule.x", "grace_seconds", "300"),
					resource.TestCheckResourceAttr("crontinel_cron_schedule.x", "max_runtime_seconds", "3600"),
					resource.TestCheckResourceAttr("crontinel_cron_schedule.x", "enabled", "true"),
					resource.TestCheckResourceAttr("crontinel_cron_schedule.x", "tags.#", "2"),
					resource.TestCheckResourceAttrSet("crontinel_cron_schedule.x", "ping_url"),
					resource.TestCheckResourceAttrSet("crontinel_cron_schedule.x", "version"),
					func(s *terraform.State) error {
						firstID = s.RootModule().Resources["crontinel_cron_schedule.x"].Primary.ID
						return nil
					},
				),
			},
			{ // in-place update keeps the id and the creation-only ping URL
				Config: cfg("30 2 * * *", "invoices:send"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("crontinel_cron_schedule.x", "expression", "30 2 * * *"),
					resource.TestCheckResourceAttrSet("crontinel_cron_schedule.x", "ping_url"),
					func(s *terraform.State) error {
						if id := s.RootModule().Resources["crontinel_cron_schedule.x"].Primary.ID; id != firstID {
							return fmt.Errorf("id changed on update: %s -> %s", firstID, id)
						}
						return nil
					},
				),
			},
			{ // someone edits it in Crontinel: Terraform sees drift, then puts it back
				PreConfig: func() {
					f.mutate("billing", "cron-schedules", firstID, func(r record) { r["grace_seconds"] = 999.0 })
				},
				Config:             cfg("30 2 * * *", "invoices:send"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: cfg("30 2 * * *", "invoices:send"),
				Check:  resource.TestCheckResourceAttr("crontinel_cron_schedule.x", "grace_seconds", "300"),
			},
			{ // the key changed: a different monitor replaces it
				Config: cfg("30 2 * * *", "invoices:send-weekly"),
				Check: func(s *terraform.State) error {
					if id := s.RootModule().Resources["crontinel_cron_schedule.x"].Primary.ID; id == firstID {
						return fmt.Errorf("changing the command should replace the monitor")
					}
					return nil
				},
			},
			{ // import by <app>/<id>
				ResourceName: "crontinel_cron_schedule.x",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return "billing/" + s.RootModule().Resources["crontinel_cron_schedule.x"].Primary.ID, nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"ping_url"},
			},
			{ // a bad expression is reported with the API's words
				Config:      cfg("every night", "invoices:send-weekly"),
				ExpectError: regexp.MustCompile(`five-field cron expression`),
			},
			{Config: cfg("30 2 * * *", "invoices:send-weekly")},
		},
	})
}

func TestAccCronScheduleRecreatedWhenDeletedOutsideTerraform(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	cfg := providerBlock(f) + `
resource "crontinel_cron_schedule" "x" {
  app        = "billing"
  command    = "a"
  expression = "0 2 * * *"
  timezone   = "UTC"
}`
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: cfg, Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["crontinel_cron_schedule.x"].Primary.ID
				return nil
			}},
			{PreConfig: func() { f.remove("billing", "cron-schedules", id) }, Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: cfg, Check: resource.TestCheckResourceAttrSet("crontinel_cron_schedule.x", "id")},
		},
	})
}

func TestAccHeartbeat(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	cfg := func(interval int, extra string) string {
		return providerBlock(f) + fmt.Sprintf(`
resource "crontinel_heartbeat" "x" {
  app              = "billing"
  name             = "Queue worker"
  interval_minutes = %d
  %s
}`, interval, extra)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: cfg(5, ""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_heartbeat.x", "interval_minutes", "5"),
				resource.TestCheckNoResourceAttr("crontinel_heartbeat.x", "grace_minutes"),
				resource.TestCheckResourceAttr("crontinel_heartbeat.x", "enabled", "true"))},
			{Config: cfg(10, "grace_minutes = 3"), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_heartbeat.x", "interval_minutes", "10"),
				resource.TestCheckResourceAttr("crontinel_heartbeat.x", "grace_minutes", "3"))},
			{ResourceName: "crontinel_heartbeat.x", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return "billing/" + s.RootModule().Resources["crontinel_heartbeat.x"].Primary.ID, nil
				}},
			{Config: cfg(0, ""), ExpectError: regexp.MustCompile(`at least 1`)},
			{Config: cfg(10, "grace_minutes = 3")},
		},
	})
}

func TestAccOutcomeRule(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	cfg := func(threshold, kind, extra string) string {
		return providerBlock(f) + fmt.Sprintf(`
resource "crontinel_outcome_rule" "x" {
  app       = "billing"
  command   = "invoices:send"
  key       = "processed_records"
  kind      = %q
  threshold = %s
  %s
}`, kind, threshold, extra)
	}
	var firstID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: cfg("1", "numeric_min", ""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_outcome_rule.x", "missing", "unknown"),
				func(s *terraform.State) error {
					firstID = s.RootModule().Resources["crontinel_outcome_rule.x"].Primary.ID
					return nil
				})},
			{Config: cfg("5", "numeric_min", `missing = "fail"`), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_outcome_rule.x", "threshold", "5"),
				resource.TestCheckResourceAttr("crontinel_outcome_rule.x", "missing", "fail"),
				func(s *terraform.State) error { // threshold and missing update in place
					if id := s.RootModule().Resources["crontinel_outcome_rule.x"].Primary.ID; id != firstID {
						return fmt.Errorf("id changed on an in-place update")
					}
					return nil
				})},
			{Config: cfg("5", "numeric_max", `missing = "fail"`), Check: func(s *terraform.State) error { // kind identifies the rule: replaced
				if id := s.RootModule().Resources["crontinel_outcome_rule.x"].Primary.ID; id == firstID {
					return fmt.Errorf("changing kind should replace the rule")
				}
				return nil
			}},
			{ResourceName: "crontinel_outcome_rule.x", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return "billing/" + s.RootModule().Resources["crontinel_outcome_rule.x"].Primary.ID, nil
				}},
			{Config: cfg("1", "nope", ""), ExpectError: regexp.MustCompile(`must be one of`)},
			{Config: cfg("5", "numeric_max", `missing = "fail"`)}, // leave a valid configuration for cleanup
		},
	})
}

func TestAccMaintenanceWindow(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	once := providerBlock(f) + `
resource "crontinel_cron_schedule" "m" {
  app        = "billing"
  command    = "m"
  expression = "0 2 * * *"
  timezone   = "UTC"
}
resource "crontinel_maintenance_window" "x" {
  app         = "billing"
  name        = "Release window"
  type        = "once"
  starts_at   = "2030-12-01T22:00:00+06:00"
  ends_at     = "2030-12-01T23:30:00+06:00"
  monitor_ids = [crontinel_cron_schedule.m.id]
  tags        = ["release"]
}`
	weekly := providerBlock(f) + `
resource "crontinel_maintenance_window" "x" {
  app              = "billing"
  name             = "Release window"
  type             = "weekly"
  days             = [0, 6]
  start_time       = "03:00"
  duration_minutes = 60
}`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			// The server returns UTC; the +06:00 spelling in the configuration must not be a perpetual change.
			{Config: once, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_maintenance_window.x", "starts_at", "2030-12-01T22:00:00+06:00"),
				resource.TestCheckResourceAttr("crontinel_maintenance_window.x", "monitor_ids.#", "1"))},
			{Config: once, PlanOnly: true},
			// Changing type updates the same window in place.
			{Config: weekly, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_maintenance_window.x", "duration_minutes", "60"),
				resource.TestCheckResourceAttr("crontinel_maintenance_window.x", "days.#", "2"),
				resource.TestCheckNoResourceAttr("crontinel_maintenance_window.x", "starts_at"))},
			{ResourceName: "crontinel_maintenance_window.x", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return "billing/" + s.RootModule().Resources["crontinel_maintenance_window.x"].Primary.ID, nil
				}},
			{Config: providerBlock(f) + `
resource "crontinel_maintenance_window" "x" {
  app       = "billing"
  name      = "Release window"
  type      = "once"
  starts_at = "next tuesday"
  ends_at   = "2030-12-01T23:30:00+00:00"
}`, ExpectError: regexp.MustCompile(`not an RFC 3339 time`)},
			{Config: weekly}, // leave a valid configuration for cleanup
		},
	})
}

func TestAccEscalationPolicy(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	cfg := func(second string) string {
		return providerBlock(f) + fmt.Sprintf(`
resource "crontinel_escalation_policy" "x" {
  app = "billing"
  steps {
    wait_minutes = 10
    channel_ids  = [2]
  }
  %s
}`, second)
	}
	two := `steps {
    wait_minutes = 30
    channel_ids  = [2, 3]
  }`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: cfg(""), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("crontinel_escalation_policy.x", "id", "billing"),
				resource.TestCheckResourceAttr("crontinel_escalation_policy.x", "steps.#", "1"))},
			{Config: cfg(two), Check: resource.TestCheckResourceAttr("crontinel_escalation_policy.x", "steps.#", "2")},
			{ResourceName: "crontinel_escalation_policy.x", ImportState: true, ImportStateId: "billing", ImportStateVerify: true},
			{ // removed outside Terraform: the policy is planned again
				PreConfig: func() { f.setEscalation("billing", []any{}) },
				Config:    cfg(two), PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{Config: cfg(two), Check: resource.TestCheckResourceAttr("crontinel_escalation_policy.x", "steps.#", "2")},
		},
	})
}

func TestAccWrongKeyIsExplained(t *testing.T) {
	testPreCheck(t)
	f := newFakeAPI()
	defer f.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{{
			Config:      fmt.Sprintf("provider \"crontinel\" {\n  url = %q\n  api_key = \"ct_mcp_wrong\"\n}\nresource \"crontinel_heartbeat\" \"x\" {\n  app = \"billing\"\n  name = \"x\"\n  interval_minutes = 5\n}", f.URL),
			ExpectError: regexp.MustCompile(`unauthenticated`),
		}},
	})
}
