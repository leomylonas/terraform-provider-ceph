package models

import (
	"github.com/ceph/go-ceph/rgw/admin"
	datasource "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	resource "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type RgwUser struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	MaxBuckets types.Int32  `tfsdk:"max_buckets"`
	AccessKey  types.String `tfsdk:"access_key"`
	SecretKey  types.String `tfsdk:"secret_key"`
}

func ToRgwUser(user admin.User) RgwUser {

	accessKey := ""
	secretKey := ""

	if len(user.Keys) > 0 {
		accessKey = user.Keys[0].AccessKey
		secretKey = user.Keys[0].SecretKey
	}

	return RgwUser{
		Id:         types.StringValue(user.ID),
		Name:       types.StringValue(user.DisplayName),
		MaxBuckets: types.Int32Value(int32(*user.MaxBuckets)),
		AccessKey:  types.StringValue(accessKey),
		SecretKey:  types.StringValue(secretKey),
	}
}

func GetRgwUserDatasourceSchema() datasource.Schema {
	return datasource.Schema{
		Attributes: map[string]datasource.Attribute{
			"id": datasource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"name": datasource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"max_buckets": datasource.Int32Attribute{
				Optional: true,
				Computed: true,
			},
			"access_key": datasource.StringAttribute{
				Optional:  true,
				Computed:  true,
				Sensitive: true,
			},
			"secret_key": datasource.StringAttribute{
				Optional:  true,
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

func GetRgwUserResourceSchema() resource.Schema {
	return resource.Schema{
		Attributes: map[string]resource.Attribute{
			"id": resource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"name": resource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"max_buckets": resource.Int32Attribute{
				Optional: true,
				Computed: true,
			},
			"access_key": resource.StringAttribute{
				Optional:  true,
				Computed:  true,
				Sensitive: true,
			},
			"secret_key": resource.StringAttribute{
				Optional:  true,
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}
