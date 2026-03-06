package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPermissionSetValidateAcceptsNetworkOnly(t *testing.T) {
	perms := &PermissionSet{
		Network: []NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
		},
	}

	require.NoError(t, perms.Validate())
}

func TestPermissionSetValidateRejectsCompletelyEmpty(t *testing.T) {
	require.Error(t, (&PermissionSet{}).Validate())
}
