package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/zsoftly/zcp-cli/pkg/api/apierrors"
	"github.com/zsoftly/zcp-cli/pkg/api/project"
	"github.com/zsoftly/zcp-cli/pkg/api/role"
)

func testAccProjectConfig(name, description, purpose string) string {
	optional := ""
	if description != "" {
		optional += fmt.Sprintf("\n  description = %q", description)
	}
	if purpose != "" {
		optional += fmt.Sprintf("\n  purpose = %q", purpose)
	}
	return fmt.Sprintf(`
resource "zcp_project" "test" {
  name = %q%s
}
`, name, optional)
}

func testAccCheckProjectDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		svc := project.NewService(accClient(t))
		projects, err := svc.List(context.Background())
		if err != nil {
			return fmt.Errorf("listing projects during destroy check: %w", err)
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "zcp_project" {
				continue
			}
			for _, p := range projects {
				if p.Slug == rs.Primary.ID {
					return fmt.Errorf("project %s still exists after destroy", rs.Primary.ID)
				}
			}
		}
		return nil
	}
}

func TestAccProjectResource_lifecycle(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-project")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig(name, "created by terraform acceptance test", "provider validation"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_project.test", "name", name),
					resource.TestCheckResourceAttr("zcp_project.test", "description", "created by terraform acceptance test"),
					resource.TestCheckResourceAttr("zcp_project.test", "purpose", "provider validation"),
					resource.TestCheckResourceAttrSet("zcp_project.test", "id"),
				),
			},
			{
				Config: testAccProjectConfig(name, "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_project.test", "description", "created by terraform acceptance test"),
					resource.TestCheckResourceAttr("zcp_project.test", "purpose", "provider validation"),
				),
			},
			{
				Config:   testAccProjectConfig(name, "", ""),
				PlanOnly: true,
			},
		},
	})
}

func testAccRoleConfig(name string, permissions []string) string {
	permissionBlock := ""
	for _, p := range permissions {
		permissionBlock += fmt.Sprintf("\n    %q,", p)
	}
	return fmt.Sprintf(`
resource "zcp_role" "test" {
  name = %q
  permissions = [%s
  ]
}
`, name, permissionBlock)
}

func testAccCheckRoleDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		svc := role.NewService(accClient(t))
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "zcp_role" {
				continue
			}
			_, err := svc.Get(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("role %s still exists after destroy", rs.Primary.ID)
			}
			if !apierrors.IsNotFound(err) &&
				!apierrors.IsResourceNotFound(err) &&
				!strings.Contains(strings.ToLower(err.Error()), "no query results for model") {
				return fmt.Errorf("unexpected error checking role %s: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

func TestAccRoleResource_lifecycle(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-role")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRoleDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: testAccRoleConfig(name, []string{"project-read"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("zcp_role.test", "name", name),
					resource.TestCheckResourceAttr("zcp_role.test", "permissions.#", "1"),
					resource.TestCheckResourceAttrSet("zcp_role.test", "id"),
				),
			},
			{
				Config:   testAccRoleConfig(name, []string{"project-read"}),
				PlanOnly: true,
			},
		},
	})
}

func TestAccRoleResource_emptyPermissionsValidation(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-role")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccRoleConfig(name, []string{}),
				ExpectError: regexp.MustCompile("At least one permission is required|Not enough values"),
			},
		},
	})
}
