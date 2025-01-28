package resources

import (
	"context"
	"fmt"
	"strings"

	lib "terraform-provider-ceph/internal/provider/lib"
	model "terraform-provider-ceph/internal/provider/models"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &RgwBucketResource{}
	_ resource.ResourceWithConfigure   = &RgwBucketResource{}
	_ resource.ResourceWithImportState = &RgwBucketResource{}
)

type RgwBucketResource struct {
	clientLibs *lib.CephProviderClientLibs
}

func NewRgwBucketResource() resource.Resource {
	return &RgwBucketResource{}
}

// Metadata returns the resource type name.
func (r *RgwBucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_bucket"
}

// Schema defines the schema for the resource.
func (r *RgwBucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = model.GetRgwBucketResourceSchema()
}

// Configure implements resource.ResourceWithConfigure.
func (r *RgwBucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.clientLibs = clientLibs
}

// Create creates the resource and sets the initial Terraform state.
func (r *RgwBucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data model.RgwBucket

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var placement = data.PlacementRule.ValueString()

	bucket := s3.CreateBucketInput{}
	bucket.SetBucket(data.Name.ValueString())

	if placement != "" {
		bucket.CreateBucketConfiguration = &s3.CreateBucketConfiguration{}

		bucket.CreateBucketConfiguration.SetLocationConstraint(*r.clientLibs.S3.Config.Region + ":" + placement)
	}

	_, err := r.clientLibs.S3.CreateBucket(&bucket)
	if err != nil {
		resp.Diagnostics.AddError("CreateBucket failed", err.Error())
		return
	}

	var status string
	if data.VersioningEnabled.ValueBool() {
		status = s3.BucketVersioningStatusEnabled
	} else {
		status = s3.BucketVersioningStatusSuspended
	}
	_, err = r.clientLibs.S3.PutBucketVersioning(&s3.PutBucketVersioningInput{
		Bucket: data.Name.ValueStringPointer(),
		VersioningConfiguration: &s3.VersioningConfiguration{
			Status: &status,
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to enable versioning", err.Error())
		return
	}

	if len(data.Permissions) > 0 {
		s3BucketPolicy := model.GenerateS3BucketPolicyFromBucket(&data)
		s3BucketPolicyJson, err := model.MarshalBucketPolicy(&s3BucketPolicy)
		if err != nil {
			resp.Diagnostics.AddError("Failed to generate bucket policy", err.Error())
			return
		}
		_, err = r.clientLibs.S3.PutBucketPolicy(&s3.PutBucketPolicyInput{
			Bucket: data.Name.ValueStringPointer(),
			Policy: &s3BucketPolicyJson,
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to put bucket policy", err.Error())
			return
		}
	}

	if len(data.LifecycleDelete) > 0 {
		s3LifecyclePolicy := model.GenerateS3LifecyclePolicyFromBucket(&data)
		_, err = r.clientLibs.S3.PutBucketLifecycleConfiguration(&s3.PutBucketLifecycleConfigurationInput{
			Bucket:                 data.Name.ValueStringPointer(),
			LifecycleConfiguration: &s3LifecyclePolicy,
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to put bucket lifecycle policy", err.Error())
			return
		}
	}

	// Now re-fetch the bucket with RGW
	bucketInfo, err := r.clientLibs.Rgw.GetBucketInfo(ctx, admin.Bucket{Bucket: data.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("CreateBucket failed", err.Error())
		return
	}

	data = model.ToRgwBucket(bucketInfo)

	versioning, err := r.clientLibs.S3.GetBucketVersioning(&s3.GetBucketVersioningInput{
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
	policyJson, err := r.clientLibs.S3.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: data.Name.ValueStringPointer(),
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
	lifecyclePolicy, err := r.clientLibs.S3.GetBucketLifecycleConfiguration(&s3.GetBucketLifecycleConfigurationInput{
		Bucket: data.Name.ValueStringPointer(),
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

// Read refreshes the Terraform state with the latest data.
func (r *RgwBucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data model.RgwBucket

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var name = data.Name.ValueString()

	bucket, err := r.clientLibs.Rgw.GetBucketInfo(ctx, admin.Bucket{Bucket: name})

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

	versioning, err := r.clientLibs.S3.GetBucketVersioning(&s3.GetBucketVersioningInput{
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
	policyJson, err := r.clientLibs.S3.GetBucketPolicy(&s3.GetBucketPolicyInput{
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
	lifecyclePolicy, err := r.clientLibs.S3.GetBucketLifecycleConfiguration(&s3.GetBucketLifecycleConfigurationInput{
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

// Update updates the resource and sets the updated Terraform state on success.
func (r *RgwBucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state model.RgwBucket
	var desired model.RgwBucket

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &desired)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if desired.Name != state.Name {
		resp.Diagnostics.AddError("Update not supported", "Bucket name cannot be changed")
		return
	}

	if !desired.PlacementRule.IsNull() && !desired.PlacementRule.IsUnknown() && desired.PlacementRule != state.PlacementRule {
		resp.Diagnostics.AddError("Update not supported", "Bucket placement rule cannot be changed from "+state.PlacementRule.String()+" to "+desired.PlacementRule.String())
		return
	}

	if !desired.PlacementRule.IsNull() && !desired.PlacementRule.IsUnknown() && desired.Owner != state.Owner {
		resp.Diagnostics.AddError("Update not supported", "Bucket owner cannot be changed")
		return
	}

	var status string
	if desired.VersioningEnabled.ValueBool() {
		status = s3.BucketVersioningStatusEnabled
	} else {
		status = s3.BucketVersioningStatusSuspended
	}
	_, err := r.clientLibs.S3.PutBucketVersioning(&s3.PutBucketVersioningInput{
		Bucket: state.Name.ValueStringPointer(),
		VersioningConfiguration: &s3.VersioningConfiguration{
			Status: &status,
		},
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to enable versioning", err.Error())
		return
	}
	state.VersioningEnabled = desired.VersioningEnabled

	if len(desired.Permissions) > 0 {
		s3BucketPolicy := model.GenerateS3BucketPolicyFromBucket(&desired)
		s3BucketPolicyJson, err := model.MarshalBucketPolicy(&s3BucketPolicy)
		if err != nil {
			resp.Diagnostics.AddError("Failed to generate bucket policy", err.Error())
			return
		}
		_, err = r.clientLibs.S3.PutBucketPolicy(&s3.PutBucketPolicyInput{
			Bucket: state.Name.ValueStringPointer(),
			Policy: &s3BucketPolicyJson,
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to put bucket policy", err.Error())
			return
		}
	} else {
		_, err := r.clientLibs.S3.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
			Bucket: state.Name.ValueStringPointer(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to delete bucket policy", err.Error())
			return
		}
	}

	if len(desired.LifecycleDelete) > 0 {
		s3LifecyclePolicy := model.GenerateS3LifecyclePolicyFromBucket(&desired)
		_, err := r.clientLibs.S3.PutBucketLifecycleConfiguration(&s3.PutBucketLifecycleConfigurationInput{
			Bucket:                 state.Name.ValueStringPointer(),
			LifecycleConfiguration: &s3LifecyclePolicy,
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to put bucket lifecycle policy", err.Error())
			return
		}
	} else {
		_, err := r.clientLibs.S3.DeleteBucketLifecycle(&s3.DeleteBucketLifecycleInput{
			Bucket: state.Name.ValueStringPointer(),
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to delete bucket lifecycle policy", err.Error())
			return
		}
	}

	// Update the state
	state.Permissions = desired.Permissions
	state.LifecycleDelete = desired.LifecycleDelete

	// Set state (for now, set it to the state)
	diags := resp.State.Set(ctx, &state)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

// Delete deletes the resource and removes the Terraform state on success.
func (r *RgwBucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data model.RgwBucket

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.clientLibs.Rgw.RemoveBucket(ctx, admin.Bucket{Bucket: data.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("DeleteBucket failed", err.Error())
		return
	}
}

func (r *RgwBucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
