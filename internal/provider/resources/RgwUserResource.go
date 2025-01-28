package resources

import (
	"context"
	"fmt"
	"strings"

	lib "terraform-provider-ceph/internal/provider/lib"
	model "terraform-provider-ceph/internal/provider/models"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &RgwUserResource{}
	_ resource.ResourceWithConfigure   = &RgwUserResource{}
	_ resource.ResourceWithImportState = &RgwUserResource{}
)

type RgwUserResource struct {
	clientLibs *lib.CephProviderClientLibs
}

func NewRgwUserResource() resource.Resource {
	return &RgwUserResource{}
}

// Metadata returns the resource type name.
func (r *RgwUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_user"
}

// Schema defines the schema for the resource.
func (r *RgwUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = model.GetRgwUserResourceSchema()
}

// Configure implements resource.ResourceWithConfigure.
func (r *RgwUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *RgwUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data model.RgwUser

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var displayName = data.Name.ValueString()
	if displayName == "" {
		displayName = data.Id.ValueString()
	}

	var generateAccessKey = data.AccessKey.IsNull() && data.SecretKey.IsNull()
	var accessKeys = []admin.UserKeySpec{}

	if !generateAccessKey {
		accessKeys = append(accessKeys, admin.UserKeySpec{
			AccessKey: data.AccessKey.ValueString(),
			SecretKey: data.SecretKey.ValueString(),
		})
	}

	var created, err = r.clientLibs.Rgw.CreateUser(ctx, admin.User{
		ID:          data.Id.ValueString(),
		DisplayName: displayName,
		MaxBuckets:  lib.ConvertInt32ToIntPointer(data.MaxBuckets.ValueInt32Pointer()),
		GenerateKey: lib.ConvertBoolToBoolPointer(&generateAccessKey),
		Keys:        accessKeys,
	})

	if err != nil {
		resp.Diagnostics.AddError("CreateUser failed", err.Error())
		return
	}

	data = model.ToRgwUser(created)

	// Set state
	diags := resp.State.Set(ctx, &data)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

// Read refreshes the Terraform state with the latest data.
func (r *RgwUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data model.RgwUser

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var uid = data.Id.ValueString()

	user, err := r.clientLibs.Rgw.GetUser(ctx, admin.User{ID: uid})

	if err != nil {
		// Check if the error is a "NoSuchUser" error
		if strings.HasPrefix(err.Error(), "NoSuchUser") {
			tflog.Debug(ctx, "User "+uid+" not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to get user info for user "+uid+" ("+err.Error()+")", err.Error())
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

// Update updates the resource and sets the updated Terraform state on success.
func (r *RgwUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state model.RgwUser
	var desired model.RgwUser

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &desired)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.Id != desired.Id {
		resp.Diagnostics.AddError("Update not supported", "ID cannot be changed")
		return
	}

	if !desired.Name.IsNull() {
		state.Name = desired.Name
	}

	if !desired.MaxBuckets.IsNull() {
		state.MaxBuckets = desired.MaxBuckets
	}

	var generateAccessKey = desired.AccessKey.IsNull() && desired.SecretKey.IsNull() && !state.AccessKey.IsNull() && !state.SecretKey.IsNull()

	var accessKeys = []admin.UserKeySpec{}

	if !generateAccessKey {
		accessKeys = append(accessKeys, admin.UserKeySpec{
			AccessKey: desired.AccessKey.ValueString(),
			SecretKey: desired.SecretKey.ValueString(),
		})
	}

	err := r.clientLibs.Rgw.RemoveKey(ctx, admin.UserKeySpec{
		UID:       state.Id.ValueString(),
		AccessKey: state.AccessKey.ValueString(),
	})

	if err != nil {
		resp.Diagnostics.AddError("RemoveKey failed", err.Error())
		return
	}

	modified, err := r.clientLibs.Rgw.ModifyUser(ctx, admin.User{
		ID:          state.Id.ValueString(),
		DisplayName: state.Name.ValueString(),
		MaxBuckets:  lib.ConvertInt32ToIntPointer(state.MaxBuckets.ValueInt32Pointer()),
		GenerateKey: lib.ConvertBoolToBoolPointer(&generateAccessKey),
		Keys:        accessKeys,
	})

	if err != nil {
		resp.Diagnostics.AddError("ModifyUser failed", err.Error())
		return
	}

	state = model.ToRgwUser(modified)

	// Set state (for now, set it to the state)
	diags := resp.State.Set(ctx, &state)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

}

// Delete deletes the resource and removes the Terraform state on success.
func (r *RgwUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data model.RgwUser

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.clientLibs.Rgw.RemoveUser(ctx, admin.User{ID: data.Id.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("DeleteUser failed", err.Error())
		return
	}
}

func (r *RgwUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
