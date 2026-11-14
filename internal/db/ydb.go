package db

import (
	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
)

// ydbIssueConflictWithExistingKey is the YQL issue code YDB attaches to
// PRECONDITION_FAILED when an INSERT hits an existing primary key or a unique
// index value ("Conflict with existing key").
const ydbIssueConflictWithExistingKey = 2012

// IsYDBConflict reports whether err is YDB's "Conflict with existing key" — an
// INSERT that collided with an existing primary key or GLOBAL UNIQUE index
// value. It is the YDB counterpart of IsUniqueViolation. YDB reports both
// kinds of collision identically, so a caller cannot tell which key collided.
func IsYDBConflict(err error) bool {
	if !ydb.IsOperationError(err, Ydb.StatusIds_PRECONDITION_FAILED) {
		return false
	}
	conflict := false
	ydb.IterateByIssues(err, func(_ string, code Ydb.StatusIds_StatusCode, _ uint32) {
		if code == ydbIssueConflictWithExistingKey {
			conflict = true
		}
	})
	return conflict
}
