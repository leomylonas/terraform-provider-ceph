package lib

import (
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
)

type CephProviderClientLibs struct {
	S3  *s3.S3
	Rgw *admin.API
}
