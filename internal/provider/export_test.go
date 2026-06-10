// export_test.go exposes package-internal constructors for use in
// external test packages. This file is only compiled during test builds.
package provider

import "github.com/hashicorp/terraform-plugin-framework/datasource"

// NewProjectDataSourceWithLister creates a projectDataSource pre-wired with
// the given lister; available only in test binaries.
func NewProjectDataSourceWithLister(l projectLister) datasource.DataSource {
	return &projectDataSource{svc: l}
}

// NewRegionDataSourceWithLister creates a regionDataSource pre-wired with
// the given lister; available only in test binaries.
func NewRegionDataSourceWithLister(l regionLister) datasource.DataSource {
	return &regionDataSource{svc: l}
}

// NewTemplateDataSourceWithLister creates a templateDataSource pre-wired with
// the given lister; available only in test binaries.
func NewTemplateDataSourceWithLister(l templateLister) datasource.DataSource {
	return &templateDataSource{svc: l}
}

// NewPlanDataSourceWithLister creates a planDataSource pre-wired with
// the given lister; available only in test binaries.
func NewPlanDataSourceWithLister(l planLister) datasource.DataSource {
	return &planDataSource{svc: l}
}
