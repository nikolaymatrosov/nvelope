package adapters

import (
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// deref returns the value of a nullable text column, "" for NULL.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
