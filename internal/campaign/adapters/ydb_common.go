package adapters

import (
	"context"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
)

// ydbTenantParam binds the $tenant_id parameter.
func ydbTenantParam(tenantID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$tenant_id").Text(tenantID).Build())
}

// ydbTenantAndID binds the (tenant_id, id) primary key of an unscoped table.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbTenantAndPage binds $tenant_id, $limit and $offset.
func ydbTenantAndPage(tenantID string, limit, offset int) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$limit").Uint64(uint64(limit)).
		Param("$offset").Uint64(uint64(offset)).
		Build())
}

// ydbCount runs a single-row COUNT(*) query.
func ydbCount(ctx context.Context, tx query.TxActor, q string, opt query.ExecuteOption) (int, error) {
	row, err := tx.QueryRow(ctx, q, opt)
	if err != nil {
		return 0, err
	}
	var n uint64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return int(n), nil
}

// nullableJSONString maps an empty JSON document to nil for a nullable Utf8
// column that stores it whole.
func nullableJSONString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}

// jsonBytes returns the stored JSON document as bytes, nil when the column is
// NULL.
func jsonBytes(s *string) []byte {
	if s == nil {
		return nil
	}
	return []byte(*s)
}
