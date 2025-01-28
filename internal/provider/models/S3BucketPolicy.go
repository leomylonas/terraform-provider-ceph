package models

type Statement struct {
	Effect    string            `json:"Effect"`
	Principal map[string]string `json:"Principal"`
	Action    []string          `json:"Action"`
	Resource  []string          `json:"Resource"`
}

type S3BucketPolicy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}
