terraform {
  backend "local" {
    path = "state.tfstate"
  }
  required_providers {
    ceph = {
      source = "leomylonas/ceph"
    }
  }
}

provider "ceph" {
  # endpoint = "http://192.168.70.51:9000"
  endpoint = "https://s3.eastcottage.industries"
  # These are coming from env vars
  # access_key = ""
  # secret_key = ""
}

resource "ceph_rgw_user" "test" {
  id = "tf-test"
}
output "resource_ceph_rgw_user_test" {
  value     = resource.ceph_rgw_user.test
  sensitive = true
}

resource "ceph_rgw_bucket" "hot" {
  name               = "tf-test-hot"
  versioning_enabled = true
  permission {
    user_id     = "tf-test"
    permissions = ["s3:GetObject", "s3:PutObject", "s3:ListBucket"]
  }
  lifecycle_delete {
    object_prefix = ""
    after_days    = 1
    id            = "delete-after-1-day"
  }
}
output "resource_ceph_rgw_bucket_hot" {
  value = resource.ceph_rgw_bucket.hot
}

resource "ceph_rgw_bucket" "cold" {
  name           = "tf-test-cold"
  placement_rule = "cold"
}
output "resource_ceph_rgw_bucket_cold" {
  value = resource.ceph_rgw_bucket.cold
}


