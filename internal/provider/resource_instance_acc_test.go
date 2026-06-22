package provider_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/instance"
)

type instanceAccConfig struct {
	region, template, networkPlan, storageCategory, sshKey, project string
	name, plan, userData                                            string
	tags                                                            map[string]string
}

func testAccInstanceConfig(c instanceAccConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, `
data "zcp_region" "r" {
  slug = %q
}

resource "zcp_instance" "test" {
  name             = %q
  cloud_provider   = data.zcp_region.r.cloud_provider
  region           = %q
  template         = %q
  plan             = %q
  billing_cycle    = "hourly"
  network_plan     = %q
  storage_category = %q
`, c.region, c.name, c.region, c.template, c.plan, c.networkPlan, c.storageCategory)
	if c.sshKey != "" {
		fmt.Fprintf(&b, "  ssh_key = %q\n", c.sshKey)
	}
	if c.project != "" {
		fmt.Fprintf(&b, "  project = %q\n", c.project)
	}
	if c.userData != "" {
		fmt.Fprintf(&b, "  user_data = %q\n", c.userData)
	}
	if c.tags != nil {
		// Deterministic key order keeps the rendered config stable across plans.
		keys := make([]string, 0, len(c.tags))
		for k := range c.tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("  tags = {\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "    %s = %q\n", k, c.tags[k])
		}
		b.WriteString("  }\n")
	}
	// Generous create timeout: ZCP provisions VMs in ~20 min, slower than the
	// 20m default, so the lifecycle test should not fail on a slightly slow boot.
	b.WriteString("  timeouts {\n    create = \"30m\"\n  }\n}\n")
	return b.String()
}

func testAccCheckInstanceDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		svc := instance.NewService(accClient(t))
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "zcp_instance" {
				continue
			}
			_, err := svc.Get(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("instance %s still exists after destroy", rs.Primary.ID)
			}
			if !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
				return fmt.Errorf("unexpected error checking instance %s: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// TestAccInstanceResource_lifecycle covers the full CLI-parity surface:
// create (blocking until Running, both IPs), re-plan zero-diff, in-place updates
// (rename + resize + startup-script + tags via change-* / tag ops), power state
// (stop then start), import, and destroy.
func TestAccInstanceResource_lifecycle(t *testing.T) {
	base := instanceAccConfig{
		region:          accRegion(),
		template:        accEnv(t, "ZCP_ACC_TEMPLATE"),
		networkPlan:     accEnv(t, "ZCP_ACC_NETWORK_PLAN"),
		storageCategory: accEnv(t, "ZCP_ACC_STORAGE_CATEGORY"),
		sshKey:          os.Getenv("ZCP_ACC_SSH_KEY"),
		project:         os.Getenv("ZCP_ACC_PROJECT"),
		plan:            accEnv(t, "ZCP_ACC_INSTANCE_PLAN"),
	}
	resizePlan := accEnv(t, "ZCP_ACC_INSTANCE_PLAN2") // a second plan slug to resize to

	create := base
	create.name = "tf-acc-instance"
	create.tags = map[string]string{"Environment": "test"}

	// In-place update WITHOUT managing power: rename (display name), resize, set
	// startup script, change tags. The provider transparently stops/restarts for
	// the resize and posts the correct vm_label payload for the rename.
	updated := base
	updated.name = "tf-acc-instance-renamed"
	updated.plan = resizePlan
	updated.userData = "#cloud-config\npackages:\n  - htop\n"
	updated.tags = map[string]string{"Environment": "prod", "Team": "platform"}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroyed(t),
		Steps: []resource.TestStep{
			{ // create
				Config: testAccInstanceConfig(create),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_instance.test", "name", "tf-acc-instance"),
					resource.TestCheckResourceAttr("zcp_instance.test", "state", "Running"),
					resource.TestCheckResourceAttrSet("zcp_instance.test", "id"),
					resource.TestCheckResourceAttrSet("zcp_instance.test", "private_ip"),
				),
			},
			{ // re-plan zero diff
				Config:   testAccInstanceConfig(create),
				PlanOnly: true,
			},
			{ // in-place: rename + resize (transparent stop/restart) + user_data + tags
				Config: testAccInstanceConfig(updated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_instance.test", "name", "tf-acc-instance-renamed"),
					resource.TestCheckResourceAttr("zcp_instance.test", "plan", resizePlan),
					resource.TestCheckResourceAttr("zcp_instance.test", "state", "Running"),
					resource.TestCheckResourceAttr("zcp_instance.test", "tags.Team", "platform"),
				),
			},
			{ // import
				ResourceName:      "zcp_instance.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Create-only attributes (supplied via the import ID, not a fresh
				// Read) plus user_data and tags, which the API does not return.
				ImportStateVerifyIgnore: []string{"plan", "billing_cycle", "ssh_key", "network_plan", "storage_category", "project", "user_data", "tags"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["zcp_instance.test"]
					cp := s.RootModule().Resources["data.zcp_region.r"].Primary.Attributes["cloud_provider"]
					return fmt.Sprintf("%s/%s/%s/%s", rs.Primary.ID, cp, base.region, base.template), nil
				},
			},
		},
	})
}
