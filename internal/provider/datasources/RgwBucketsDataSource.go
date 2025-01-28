package datasources

import (
	"context"
	"fmt"
	"strings"

	lib "terraform-provider-ceph/internal/provider/lib"
	model "terraform-provider-ceph/internal/provider/models"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &RgwBucketsDataSource{}
	_ datasource.DataSourceWithConfigure = &RgwBucketsDataSource{}
)

func NewRgwBucketsDataSource() datasource.DataSource {
	return &RgwBucketsDataSource{}
}

type RgwBucketsDataSource struct {
	clientLibs *lib.CephProviderClientLibs
}

type RgwBucketsDataSourceModel struct {
	Name    types.String      `tfsdk:"name"`
	Owner   types.String      `tfsdk:"owner"`
	Buckets []model.RgwBucket `tfsdk:"buckets"`
}

func (d *RgwBucketsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_buckets"
}

func (d *RgwBucketsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{ // request
				Optional: true,
			},
			"owner": schema.StringAttribute{ // request
				Optional: true,
			},
			"buckets": schema.ListNestedAttribute{ // response
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: model.GetRgwBucketDatasourceSchema().Attributes,
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *RgwBucketsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Add a nil check when handling ProviderData because Terraform
	// sets that data after it calls the ConfigureProvider RPC.
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

func (d *RgwBucketsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RgwBucketsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var name = data.Name.ValueString()
	var owner = data.Owner.ValueString()
	bucketNames, err := d.clientLibs.Rgw.ListBuckets(ctx)

	if err != nil {
		resp.Diagnostics.AddError("Failed to list RGW buckets", err.Error())
		return
	}

	data.Buckets = []model.RgwBucket{}

	for _, bucketName := range bucketNames {
		bucket, err := d.clientLibs.Rgw.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})

		if err != nil {
			resp.Diagnostics.AddError("Failed to get bucket info for bucket "+bucketName, err.Error())
			return
		}

		if name != "" && !strings.Contains(bucket.Bucket, name) {
			continue
		}

		if owner != "" && bucket.Owner != owner {
			continue
		}

		data.Buckets = append(data.Buckets, model.ToRgwBucket(bucket))
	}

	// Set state
	diags := resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
