package store

import (
	"database/sql/driver"
	"errors"
	"time"

	"modernc.org/sqlite"
)

const decisionTimestampOrderFunction = "ndg_rfc3339nano_order_v1"

var errInvalidDecisionTimestamp = errors.New("store: invalid decision timestamp")

func init() {
	// RFC3339Nano's variable-width fractional seconds and timezone offsets are
	// not lexically sortable. Return one fixed-width UTC representation while
	// leaving historical database values untouched. A malformed timestamp
	// fails closed with a fixed error that never includes stored data.
	sqlite.MustRegisterDeterministicScalarFunction(decisionTimestampOrderFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) != 1 {
				return nil, errInvalidDecisionTimestamp
			}
			value, ok := args[0].(string)
			if !ok {
				return nil, errInvalidDecisionTimestamp
			}
			instant, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return nil, errInvalidDecisionTimestamp
			}
			return instant.UTC().Format("2006-01-02T15:04:05.000000000Z"), nil
		})
}
