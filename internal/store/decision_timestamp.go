package store

import (
	"database/sql/driver"
	"errors"
	"strings"
	"time"

	"modernc.org/sqlite"
)

const decisionTimestampOrderFunction = "ndg_rfc3339nano_order_v1"

var errInvalidDecisionTimestamp = errors.New("store: invalid decision timestamp")

// decisionTimestampKey accepts only bounded ASCII RFC3339Nano timestamps.
// Go's parser accepts out-of-range numeric timezone offsets, so validate the
// wire grammar and offset before parsing. UTC normalization can also cross
// the four-digit year boundary; reject that rather than emit a non-sortable
// five-digit key.
func decisionTimestampKey(value string) (string, error) {
	if len(value) < 20 || len(value) > 35 ||
		value[4] != '-' || value[7] != '-' ||
		(value[10] != 'T' && value[10] != 't') ||
		value[13] != ':' || value[16] != ':' {
		return "", errInvalidDecisionTimestamp
	}
	for _, position := range [...]int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18} {
		if !isASCIIDigit(value[position]) {
			return "", errInvalidDecisionTimestamp
		}
	}

	zoneStart := 19
	if zoneStart < len(value) && value[zoneStart] == '.' {
		zoneStart++
		fractionStart := zoneStart
		for zoneStart < len(value) && isASCIIDigit(value[zoneStart]) {
			zoneStart++
		}
		fractionDigits := zoneStart - fractionStart
		if fractionDigits < 1 || fractionDigits > 9 {
			return "", errInvalidDecisionTimestamp
		}
	}
	if zoneStart >= len(value) {
		return "", errInvalidDecisionTimestamp
	}

	zone := value[zoneStart:]
	switch {
	case len(zone) == 1 && (zone[0] == 'Z' || zone[0] == 'z'):
	case len(zone) == 6 && (zone[0] == '+' || zone[0] == '-') && zone[3] == ':' &&
		isASCIIDigit(zone[1]) && isASCIIDigit(zone[2]) &&
		isASCIIDigit(zone[4]) && isASCIIDigit(zone[5]):
		hours := int(zone[1]-'0')*10 + int(zone[2]-'0')
		minutes := int(zone[4]-'0')*10 + int(zone[5]-'0')
		if hours > 23 || minutes > 59 {
			return "", errInvalidDecisionTimestamp
		}
	default:
		return "", errInvalidDecisionTimestamp
	}

	parseValue := value
	if value[10] == 't' || value[len(value)-1] == 'z' {
		parseValue = strings.Clone(value)
		if value[10] == 't' {
			parseValue = parseValue[:10] + "T" + parseValue[11:]
		}
		if value[len(value)-1] == 'z' {
			parseValue = parseValue[:len(parseValue)-1] + "Z"
		}
	}
	instant, err := time.Parse(time.RFC3339Nano, parseValue)
	if err != nil {
		return "", errInvalidDecisionTimestamp
	}
	instant = instant.UTC()
	if year := instant.Year(); year < 0 || year > 9999 {
		return "", errInvalidDecisionTimestamp
	}
	return instant.Format("2006-01-02T15:04:05.000000000Z"), nil
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

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
			key, err := decisionTimestampKey(value)
			if err != nil {
				return nil, errInvalidDecisionTimestamp
			}
			return key, nil
		})
}
