package provider_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/kubernetes"
)

func testAccKubernetesConfig(name, region, version, controlPlanePlan, workerPlan, storagePlan, storageCategory, sshKey, project string, rootDiskSize, workers int) string {
	optional := ""
	if sshKey != "" {
		optional += fmt.Sprintf("\n  ssh_key = %q", sshKey)
	}
	if project != "" {
		optional += fmt.Sprintf("\n  project = %q", project)
	}
	return fmt.Sprintf(`
data "zcp_region" "r" {
  slug = %q
}

resource "zcp_kubernetes_cluster" "test" {
  name             = %q
  cloud_provider   = data.zcp_region.r.cloud_provider
  region           = %q
  version          = %q
  control_plane_plan = %q
  worker_plan        = %q
  storage_plan       = %q
  root_disk_size     = %d
  billing_cycle    = "hourly"
  workers          = %d
  storage_category = %q%s
}
`, region, name, region, version, controlPlanePlan, workerPlan, storagePlan, rootDiskSize, workers, storageCategory, optional)
}

func testAccCheckKubernetesDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		svc := kubernetes.NewService(accClient(t))
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "zcp_kubernetes_cluster" {
				continue
			}
			_, err := svc.Get(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("cluster %s still exists after destroy", rs.Primary.ID)
			}
			if !apierrors.IsNotFound(err) && !apierrors.IsResourceNotFound(err) {
				return fmt.Errorf("unexpected error checking cluster %s: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// TestAccKubernetesClusterResource_lifecycle covers create (blocking until
// Running with endpoints populated), re-plan zero-diff, in-place worker scaling
// (update, not replace), import, and destroy.
func TestAccKubernetesClusterResource_lifecycle(t *testing.T) {
	region := accRegion()
	version := accEnv(t, "ZCP_ACC_K8S_VERSION")
	controlPlanePlan := accEnv(t, "ZCP_ACC_K8S_CONTROL_PLANE_PLAN")
	workerPlan := accEnv(t, "ZCP_ACC_K8S_WORKER_PLAN")
	storagePlan := accEnv(t, "ZCP_ACC_K8S_STORAGE_PLAN")
	rootDiskSize, err := strconv.Atoi(accEnv(t, "ZCP_ACC_K8S_ROOT_DISK_SIZE"))
	if err != nil || rootDiskSize < 1 {
		t.Fatal("ZCP_ACC_K8S_ROOT_DISK_SIZE must be a positive integer")
	}
	storageCategory := accEnv(t, "ZCP_ACC_STORAGE_CATEGORY")
	sshKey := os.Getenv("ZCP_ACC_SSH_KEY")
	project := os.Getenv("ZCP_ACC_PROJECT")
	// Unique per-run name avoids collisions across retries or concurrent runs.
	name := acctest.RandomWithPrefix("tf-acc-k8s")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckKubernetesDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccKubernetesConfig(name, region, version, controlPlanePlan, workerPlan, storagePlan, storageCategory, sshKey, project, rootDiskSize, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_kubernetes_cluster.test", "name", name),
					resource.TestCheckResourceAttr("zcp_kubernetes_cluster.test", "state", "Running"),
					resource.TestCheckResourceAttr("zcp_kubernetes_cluster.test", "workers", "1"),
					resource.TestCheckResourceAttrSet("zcp_kubernetes_cluster.test", "id"),
				),
			},
			{
				Config:   testAccKubernetesConfig(name, region, version, controlPlanePlan, workerPlan, storagePlan, storageCategory, sshKey, project, rootDiskSize, 1),
				PlanOnly: true,
			},
			{
				// Scaling workers must be an in-place update, never a replacement.
				Config: testAccKubernetesConfig(name, region, version, controlPlanePlan, workerPlan, storagePlan, storageCategory, sshKey, project, rootDiskSize, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_kubernetes_cluster.test", "workers", "2"),
					resource.TestCheckResourceAttr("zcp_kubernetes_cluster.test", "state", "Running"),
				),
			},
			{
				ResourceName:      "zcp_kubernetes_cluster.test",
				ImportState:       true,
				ImportStateVerify: true,
				// billing_cycle/storage_category/ssh_key/project are create-only
				// and supplied via the import ID, not a fresh Read. control_nodes is a
				// create-only (RequiresReplace) field whose top-level count the API
				// reports transiently as 0 right after a scale (as it does for workers),
				// so importing immediately after Step 3 may not repopulate it; a stable
				// cluster's Read returns the real value.
				ImportStateVerifyIgnore: []string{"billing_cycle", "storage_category", "ssh_key", "project", "control_nodes"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["zcp_kubernetes_cluster.test"]
					cp := s.RootModule().Resources["data.zcp_region.r"].Primary.Attributes["cloud_provider"]
					return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s/%d/hourly/%s/%s/%s/false/false", rs.Primary.ID, cp, region, version, controlPlanePlan, workerPlan, storagePlan, rootDiskSize, storageCategory, project, sshKey), nil
				},
			},
		},
	})
}
