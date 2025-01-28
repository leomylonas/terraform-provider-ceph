package provider

import (
	"context"
	"os"

	"terraform-provider-ceph/internal/provider/datasources"
	"terraform-provider-ceph/internal/provider/lib"
	"terraform-provider-ceph/internal/provider/resources"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &CephProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CephProvider{
			version: version,
		}
	}
}

// CephProvider is the provider implementation.
type CephProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

type CephProviderModel struct {
	Endpoint  types.String `tfsdk:"endpoint"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
	Zone      types.String `tfsdk:"zone"`
}

// Metadata returns the provider type name.
func (p *CephProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ceph"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *CephProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional: true,
			},
			"access_key": schema.StringAttribute{
				Optional: true,
			},
			"secret_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
			},
			"zone": schema.StringAttribute{
				Optional: true,
			},
		},
	}
}

// Configure prepares a Ceph API client for data sources and resources.
func (p *CephProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Retrieve provider data from configuration
	var config CephProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.

	if config.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Unknown Ceph RGW Endpoint",
			"The provider cannot create the Ceph RGW client as there is an unknown configuration value for the Ceph RGW endpoint",
		)
	}

	if config.AccessKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("access_key"),
			"Unknown Ceph RGW Access Key",
			"The provider cannot create the Ceph RGW client as there is an unknown configuration value for the Ceph RGW access_key.",
		)
	}

	if config.SecretKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("secret_key"),
			"Unknown Ceph RGW SecretKey",
			"The provider cannot create the Ceph RGW client as there is an unknown configuration value for the Ceph RGW secret_key. ",
		)
	}

	if config.SecretKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("zone"),
			"Unknown Ceph RGW Zone",
			"The provider cannot create the Ceph RGW client as there is an unknown configuration value for the Ceph RGW zone. ",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	endpoint := os.Getenv("CEPH_RGW_ENDPOINT")
	accessKey := os.Getenv("CEPH_RGW_ACCESS_KEY")
	secretKey := os.Getenv("CEPH_RGW_SECRET_KEY")
	zone := os.Getenv("CEPH_RGW_ZONE")

	if !config.Endpoint.IsNull() {
		endpoint = config.Endpoint.ValueString()
	}

	if !config.AccessKey.IsNull() {
		accessKey = config.AccessKey.ValueString()
	}

	if !config.SecretKey.IsNull() {
		secretKey = config.SecretKey.ValueString()
	}

	if !config.Zone.IsNull() {
		zone = config.Zone.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Missing Ceph RGW Endpoint",
			"The provider cannot create the Ceph RGW client as there is a missing or empty value for the Ceph RGW endpoint. "+
				"Set the endpoint value in the configuration or use the CEPH_RGW_ENDPOINT environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if accessKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("access_key"),
			"Missing Ceph RGW Access Key",
			"The provider cannot create the Ceph RGW client as there is a missing or empty value for the Ceph RGW access_key. "+
				"Set the access_key value in the configuration or use the CEPH_RGW_ACCESS_KEY environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if secretKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("secret_key"),
			"Missing Ceph RGW Secret Key",
			"The provider cannot create the Ceph RGW client as there is a missing or empty value for the Ceph RGW secret_key. "+
				"Set the secret_key value in the configuration or use the CEPH_RGW_SECRET_KEY environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if zone == "" {
		zone = "default"
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Create a new client using the configuration values
	rgwClient, err := admin.New(endpoint, accessKey, secretKey, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Ceph RGW Client",
			"An unexpected error occurred when creating the Ceph RGW client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Error: "+err.Error(),
		)
		return
	}

	s3Session := session.Must(session.NewSession())
	s3Client := s3.New(
		s3Session,
		aws.NewConfig().
			WithRegion(zone).
			WithEndpoint(endpoint).
			WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")).
			WithS3ForcePathStyle(true),
	)

	clientLibs := &lib.CephProviderClientLibs{
		S3:  s3Client,
		Rgw: rgwClient,
	}

	// Make the client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = clientLibs
	resp.ResourceData = clientLibs
}

// DataSources defines the data sources implemented in the provider.
func (p *CephProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewRgwBucketsDataSource,
		datasources.NewRgwBucketDataSource,
		datasources.NewRgwUserDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *CephProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewRgwBucketResource,
		resources.NewRgwUserResource,
	}
}
