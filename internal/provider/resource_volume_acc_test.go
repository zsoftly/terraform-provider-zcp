package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/zsoftly/zcp-cli/pkg/api/volume"
)

func testAccVolumeConfig(name, region, billingCycle, storageCategory string, size int, project string) string {
	optional := ""
	if project != "" {
		optional += fmt.Sprintf("\n  project = %q", project)
	}
	return fmt.Sprintf(`
data "zcp_region" "r" {
  slug = %q
}

resource "zcp_volume" "test" {
  name             = %q
  cloud_provider   = data.zcp_region.r.cloud_provider
  region           = %q
  billing_cycle    = %q
  storage_category = %q
  size             = %d%s
}
`, region, name, region, billingCycle, storageCategory, size, optional)
}

func testAccCheckVolumeDestroyed(t *testing.T, region string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		svc := volume.NewService(accClient(t))
		project := os.Getenv("ZCP_ACC_PROJECT")
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "zcp_volume" {
				continue
			}
			vols, err := svc.List(context.Background(), region, project)
			if err != nil {
				return fmt.Errorf("listing volumes during destroy check: %w", err)
			}
			for _, v := range vols {
				if v.Slug == rs.Primary.ID {
					return fmt.Errorf("volume %s still exists after destroy", rs.Primary.ID)
				}
			}
		}
		return nil
	}
}

// TestAccVolumeResource_lifecycle covers custom-size create, re-plan zero-diff,
// import (size populated from the API), and destroy.
func TestAccVolumeResource_lifecycle(t *testing.T) {
	region := accEnvDefault("ZCP_ACC_VOLUME_REGION", accRegion())
	storageCategory := accEnv(t, "ZCP_ACC_STORAGE_CATEGORY")
	project := os.Getenv("ZCP_ACC_PROJECT")
	name := "tf-acc-volume"
	billingCycle := "hourly"
	size := 10

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroyed(t, region),
		Steps: []resource.TestStep{
			{
				Config: testAccVolumeConfig(name, region, billingCycle, storageCategory, size, project),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_volume.test", "name", name),
					resource.TestCheckResourceAttr("zcp_volume.test", "size", "10"),
					resource.TestCheckResourceAttrSet("zcp_volume.test", "id"),
					resource.TestCheckResourceAttrSet("zcp_volume.test", "slug"),
				),
			},
			{
				Config:   testAccVolumeConfig(name, region, billingCycle, storageCategory, size, project),
				PlanOnly: true,
			},
			{
				ResourceName:            "zcp_volume.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"project"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["zcp_volume.test"]
					cp := s.RootModule().Resources["data.zcp_region.r"].Primary.Attributes["cloud_provider"]
					return fmt.Sprintf("%s/%s/%s/%s/%s", rs.Primary.ID, cp, region, billingCycle, storageCategory), nil
				},
			},
		},
	})
}

// TestAccVolumeResource_planAndSize covers a named storage tier with explicit
// capacity.
func TestAccVolumeResource_planAndSize(t *testing.T) {
	region := accRegion()
	storageCategory := accEnvDefault("ZCP_ACC_STORAGE_CATEGORY", "nvme")
	plan := accEnv(t, "ZCP_ACC_VOLUME_PLAN")
	project := os.Getenv("ZCP_ACC_PROJECT")
	optional := ""
	if project != "" {
		optional = fmt.Sprintf("\n  project          = %q", project)
	}
	config := fmt.Sprintf(`
data "zcp_region" "r" {
  slug = %q
}

resource "zcp_volume" "test" {
  name             = "tf-acc-volume-plan-size"
  cloud_provider   = data.zcp_region.r.cloud_provider
  region           = %q
  billing_cycle    = "hourly"
  storage_category = %q
  plan             = %q
  size             = 10%s
}
`, region, region, storageCategory, plan, optional)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVolumeDestroyed(t, region),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_volume.test", "plan", plan),
					resource.TestCheckResourceAttr("zcp_volume.test", "size", "10"),
				),
			},
		},
	})
}
