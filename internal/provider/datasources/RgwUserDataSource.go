package datasources

import (
	"context"
	"fmt"

	lib "terraform-provider-ceph/internal/provider/lib"
	model "terraform-provider-ceph/internal/provider/models"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

var (
	_ datasource.DataSource              = &RgwUserDataSource{}
	_ datasource.DataSourceWithConfigure = &RgwUserDataSource{}
)

func NewRgwUserDataSource() datasource.DataSource {
	return &RgwUserDataSource{}
}

type RgwUserDataSource struct {
	clientLibs *lib.CephProviderClientLibs
}

func (d *RgwUserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user"
}

func (d *RgwUserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = model.GetRgwUserDatasourceSchema()
}

// Configure adds the provider configured client to the data source.
func (d *RgwUserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	clientLibs, ok := req.ProviderData.(*lib.CephProviderClientLibs)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephProviderClientLibs, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.clientLibs = clientLibs
}

func (d *RgwUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data model.RgwUser

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var uid = data.Id.ValueString()

	if uid == "" {
		resp.Diagnostics.AddError("uid is required", "uid is required")
		return
	}

	user, err := d.clientLibs.Rgw.GetUser(ctx, admin.User{ID: uid})

	if err != nil {
		resp.Diagnostics.AddError("Failed to get user info for user "+uid, err.Error())
		return
	}

	data = model.ToRgwUser(user)

	// Set state
	diags := resp.State.Set(ctx, &data)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
