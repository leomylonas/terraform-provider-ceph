package models

import (
	"context"
	"encoding/json"
	"strings"
	"terraform-provider-ceph/internal/provider/lib"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	datasource "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	resource "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type RgwPermission struct {
	UserId      types.String `tfsdk:"user_id"`
	Permissions types.List   `tfsdk:"permissions"`
}

type RgwLifecycleDelete struct {
	Prefix    types.String `tfsdk:"object_prefix"`
	AfterDays types.Int64  `tfsdk:"after_days"`
	Id        types.String `tfsdk:"id"`
}

type RgwBucket struct {
	Name                      types.String         `tfsdk:"name"`
	PlacementRule             types.String         `tfsdk:"placement_rule"`
	Permissions               []RgwPermission      `tfsdk:"permission"`
	LifecycleDelete           []RgwLifecycleDelete `tfsdk:"lifecycle_delete"`
	LifecycleDeleteNonCurrent []RgwLifecycleDelete `tfsdk:"lifecycle_delete_noncurrent"`
	VersioningEnabled         types.Bool           `tfsdk:"versioning_enabled"`
}

func ToRgwBucket(bucket admin.Bucket) RgwBucket {
	return RgwBucket{
		Name:          types.StringValue(bucket.Bucket),
		PlacementRule: types.StringValue(bucket.PlacementRule),
	}
}

func GenerateS3BucketPolicyFromBucket(bucket *RgwBucket) S3BucketPolicy {
	policy := S3BucketPolicy{
		Version:   "2012-10-17",
		Statement: []Statement{},
	}

	for _, permission := range bucket.Permissions {
		var statement = Statement{
			Effect: "Allow",
			Principal: map[string]string{
				"AWS": "arn:aws:iam:::user/" + permission.UserId.ValueString(),
			},
			Resource: []string{"arn:aws:s3:::" + bucket.Name.ValueString(), "arn:aws:s3:::" + bucket.Name.ValueString() + "/*"},
		}

		permission.Permissions.ElementsAs(context.Background(), &statement.Action, false)

		policy.Statement = append(policy.Statement, statement)
	}

	return policy
}

func ReadS3BucketPolicyIntoBucket(bucket *RgwBucket, policy *S3BucketPolicy) {
	bucket.Permissions = []RgwPermission{}

	for _, statement := range policy.Statement {
		actions, _ := types.ListValueFrom(context.Background(), types.StringType, statement.Action)

		permission := RgwPermission{
			UserId:      types.StringValue(strings.Split(statement.Principal["AWS"], "/")[1]),
			Permissions: actions,
		}

		bucket.Permissions = append(bucket.Permissions, permission)
	}
}

func MarshalBucketPolicy(policy *S3BucketPolicy) (string, error) {
	policyJson, err := json.Marshal(policy)

	return string(policyJson), err
}

func UnmarshalBucketPolicy(policyJson string) (S3BucketPolicy, error) {
	var policy S3BucketPolicy

	err := json.Unmarshal([]byte(policyJson), &policy)

	return policy, err
}

func GenerateS3LifecyclePolicyFromBucket(bucket *RgwBucket) s3.BucketLifecycleConfiguration {

	rules := []*s3.LifecycleRule{}
	status := "Enabled"

	for _, lifecycleDelete := range bucket.LifecycleDelete {
		rule := s3.LifecycleRule{
			ID:     lifecycleDelete.Id.ValueStringPointer(),
			Status: &status,
			Filter: &s3.LifecycleRuleFilter{
				Prefix: lifecycleDelete.Prefix.ValueStringPointer(),
			},
			Expiration: &s3.LifecycleExpiration{
				Days: lib.ConvertInt64ToIntPointer(lifecycleDelete.AfterDays.ValueInt64Pointer()),
			},
		}
		rules = append(rules, &rule)
	}
	for _, lifecycleDeleteNonCurrent := range bucket.LifecycleDeleteNonCurrent {
		rule := s3.LifecycleRule{
			ID:     lifecycleDeleteNonCurrent.Id.ValueStringPointer(),
			Status: &status,
			Filter: &s3.LifecycleRuleFilter{
				Prefix: lifecycleDeleteNonCurrent.Prefix.ValueStringPointer(),
			},
			NoncurrentVersionExpiration: &s3.NoncurrentVersionExpiration{
				NoncurrentDays: lib.ConvertInt64ToIntPointer(lifecycleDeleteNonCurrent.AfterDays.ValueInt64Pointer()),
			},
		}
		rules = append(rules, &rule)
	}

	policy := s3.BucketLifecycleConfiguration{
		Rules: rules,
	}

	return policy
}

func ReadS3LifecyclePolicyRulesIntoBucket(bucket *RgwBucket, rules []*s3.LifecycleRule) {
	bucket.LifecycleDelete = []RgwLifecycleDelete{}

	for _, rule := range rules {
		var prefix string
		if rule.Filter != nil && rule.Filter.Prefix != nil {
			prefix = *rule.Filter.Prefix
		} else {
			prefix = ""
		}
		bucket.LifecycleDelete = append(bucket.LifecycleDelete, RgwLifecycleDelete{
			Id:        types.StringValue(*rule.ID),
			Prefix:    types.StringValue(prefix),
			AfterDays: types.Int64Value(*rule.Expiration.Days),
		})
	}
}

func GetRgwBucketDatasourceSchema() datasource.Schema {
	return datasource.Schema{
		Attributes: map[string]datasource.Attribute{
			"name": datasource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"placement_rule": datasource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"versioning_enabled": datasource.BoolAttribute{
				Optional: true,
				Computed: true,
			},
		},
		Blocks: map[string]datasource.Block{
			"permission": datasource.ListNestedBlock{
				NestedObject: datasource.NestedBlockObject{
					Attributes: map[string]datasource.Attribute{
						"user_id": datasource.StringAttribute{
							Required: true,
						},
						"permissions": datasource.ListAttribute{
							ElementType: types.StringType,
							Required:    true,
						},
					},
				},
			},
			"lifecycle_delete": datasource.ListNestedBlock{
				NestedObject: datasource.NestedBlockObject{
					Attributes: map[string]datasource.Attribute{
						"object_prefix": datasource.StringAttribute{
							Required: true,
						},
						"after_days": datasource.Int64Attribute{
							Required: true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
						},
						"id": datasource.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(10, 255),
							},
						},
					},
				},
			},
		},
	}
}

func GetRgwBucketResourceSchema() resource.Schema {
	return resource.Schema{
		Attributes: map[string]resource.Attribute{
			"name": resource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"placement_rule": resource.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"versioning_enabled": resource.BoolAttribute{
				Optional: true,
				Computed: true,
			},
		},
		Blocks: map[string]resource.Block{
			"permission": resource.ListNestedBlock{
				NestedObject: resource.NestedBlockObject{
					Attributes: map[string]resource.Attribute{
						"user_id": resource.StringAttribute{
							Required: true,
						},
						"permissions": resource.ListAttribute{
							ElementType: types.StringType,
							Required:    true,
						},
					},
				},
			},
			"lifecycle_delete": datasource.ListNestedBlock{
				NestedObject: datasource.NestedBlockObject{
					Attributes: map[string]datasource.Attribute{
						"object_prefix": datasource.StringAttribute{
							Required: true,
						},
						"after_days": datasource.Int64Attribute{
							Required: true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
						},
						"id": datasource.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(10, 255),
							},
						},
					},
				},
			},
		},
	}
}
