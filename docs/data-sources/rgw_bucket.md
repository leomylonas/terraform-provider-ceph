---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "ceph_rgw_bucket Data Source - ceph"
subcategory: ""
description: |-
  
---

# ceph_rgw_bucket (Data Source)





<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `lifecycle_delete` (Block List) (see [below for nested schema](#nestedblock--lifecycle_delete))
- `name` (String)
- `permission` (Block List) (see [below for nested schema](#nestedblock--permission))
- `placement_rule` (String)
- `versioning_enabled` (Boolean)

<a id="nestedblock--lifecycle_delete"></a>
### Nested Schema for `lifecycle_delete`

Required:

- `after_days` (Number)
- `id` (String)
- `object_prefix` (String)


<a id="nestedblock--permission"></a>
### Nested Schema for `permission`

Required:

- `permissions` (List of String)
- `user_id` (String)
