package datasources

import (
	"context"
	"fmt"
	"strings"

	lib "terraform-provider-ceph/internal/provider/lib"
	model "terraform-provider-ceph/internal/provider/models"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &RgwBucketDataSource{}
	_ datasource.DataSourceWithConfigure = &RgwBucketDataSource{}
)

func NewRgwBucketDataSource() datasource.DataSource {
	return &RgwBucketDataSource{}
}

type RgwBucketDataSource struct {
	clientLibs *lib.CephProviderClientLibs
}

func (d *RgwBucketDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_bucket"
}

func (d *RgwBucketDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = model.GetRgwBucketDatasourceSchema()
}

// Configure adds the provider configured client to the data source.
func (d *RgwBucketDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RgwBucketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data model.RgwBucket

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var name = data.Name.ValueString()

	bucket, err := d.clientLibs.Rgw.GetBucketInfo(ctx, admin.Bucket{Bucket: name})

	if err != nil {
		// Check if the error is a "NoSuchBucket" error
		if strings.HasPrefix(err.Error(), "NoSuchBucket") {
			tflog.Debug(ctx, "Bucket "+name+" not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to get bucket info for bucket "+name, err.Error())
		return
	}

	data = model.ToRgwBucket(bucket)

	versioning, err := d.clientLibs.S3.GetBucketVersioning(&s3.GetBucketVersioningInput{
		Bucket: data.Name.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to get bucket versioning", err.Error())
		return
	}
	if versioning.Status != nil && *versioning.Status == s3.BucketVersioningStatusEnabled {
		data.VersioningEnabled = types.BoolValue(true)
	} else {
		data.VersioningEnabled = types.BoolValue(false)
	}

	// Now get bucket policy and set it in the state
	policyJson, err := d.clientLibs.S3.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: &name,
	})
	if err != nil && !strings.HasPrefix(err.Error(), "NoSuchBucketPolicy") {
		resp.Diagnostics.AddError("Failed to get bucket policy", err.Error())
		return
	}
	if policyJson != nil && policyJson.Policy != nil {
		s3BucketPolicy, err := model.UnmarshalBucketPolicy(*policyJson.Policy)

		if err != nil {
			resp.Diagnostics.AddError("Failed to unmarshal bucket policy", err.Error())
			return
		}

		model.ReadS3BucketPolicyIntoBucket(&data, &s3BucketPolicy)
	}

	// Now get bucket lifecycle policy and set it in the state
	lifecyclePolicy, err := d.clientLibs.S3.GetBucketLifecycleConfiguration(&s3.GetBucketLifecycleConfigurationInput{
		Bucket: &name,
	})
	if err != nil && !strings.HasPrefix(err.Error(), "NoSuchLifecycleConfiguration") {
		resp.Diagnostics.AddError("Failed to get bucket lifecycle policy", err.Error())
		return
	}
	if lifecyclePolicy != nil && lifecyclePolicy.Rules != nil {
		model.ReadS3LifecyclePolicyRulesIntoBucket(&data, lifecyclePolicy.Rules)
	}

	// Set state
	diags := resp.State.Set(ctx, &data)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
