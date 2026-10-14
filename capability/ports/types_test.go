package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

func TestToolPermissionsValidate_MissingPermissionsIsError(t *testing.T) {
	err := (ToolPermissions{}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestToolPermissionsValidate_DelegatesToPermissionSet(t *testing.T) {
	valid := ToolPermissions{Permissions: &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemRead, Path: "/workspace/**"},
		},
		Network: []ucperms.NetworkPermission{
			{Direction: "egress", Protocol: "https", Host: "example.com"},
		},
	}}
	assert.NoError(t, valid.Validate(), "a well-formed permission set must validate")

	invalid := ToolPermissions{Permissions: &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemRead, Path: ""},
		},
	}}
	err := invalid.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path")

	escape := ToolPermissions{Permissions: &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{Action: ucperms.FileSystemRead, Path: "../escape"},
		},
	}}
	err = escape.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "..")
}
