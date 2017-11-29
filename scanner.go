package scnr

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
)

// ErrTooManyColumns indicates that a select query returned multiple columns and
// Rows attempted to bind to a slice of a primitive type. For example, trying to bind
// `select col1, col2 from mytable`  to []string
var ErrTooManyColumns = errors.New("too many columns returned for primitive slice")

// Scalar is a wrapper for (sql.DB).Scan(value). It is here to provide consistency
// for users and offeres nothing more
func Scalar(v interface{}, scanner Scanner) error {
	return scanner.Scan(v)
}

// Row scans a single row into a single variable
func Row(v interface{}, rows RowsScanner) error {
	vType := reflect.TypeOf(v).Elem()
	vVal := reflect.ValueOf(v).Elem()

	sl := reflect.New(reflect.SliceOf(vType))
	err := Rows(sl.Interface(), rows)
	if err != nil {
		return err
	}

	sl = sl.Elem()

	if sl.Len() > 0 {
		vVal.Set(sl.Index(0))
	}

	return nil
}

// Rows scans sql rows into a slice (v)
func Rows(v interface{}, rows RowsScanner) error {
	vType := reflect.TypeOf(v)
	if k := vType.Kind(); k != reflect.Ptr {
		panic(k.String() + ": must be a pointer to a slice")
	}
	sliceType := vType.Elem()
	if reflect.Slice != sliceType.Kind() {
		panic(sliceType.String() + ": must be a pointer to a slice")
	}

	sliceVal := reflect.Indirect(reflect.ValueOf(v))
	itemType := sliceType.Elem()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	isPrimitive := itemType.Kind() != reflect.Struct

	for rows.Next() {
		sliceItem := reflect.New(itemType).Elem()

		var pointers []interface{}
		if isPrimitive {
			if len(cols) > 1 {
				return ErrTooManyColumns
			}
			pointers = []interface{}{sliceItem.Addr().Interface()}
		} else {
			pointers = structPointers(sliceItem, cols)
		}

		if len(pointers) == 0 {
			return nil
		}

		err := rows.Scan(pointers...)
		if err != nil {
			return err
		}
		sliceVal.Set(reflect.Append(sliceVal, sliceItem))
	}
	return rows.Err()
}

// fieldByName gets a struct's field by first looking up the db struct tag and falling
// back to the field's name in Title case.
func fieldByName(v reflect.Value, name string) reflect.Value {
	typ := v.Type()

	for i := 0; i < v.NumField(); i++ {
		tag, ok := typ.Field(i).Tag.Lookup("db")
		if ok && tag == name {
			return v.Field(i)
		}
	}
	return v.FieldByName(strings.Title(name))
}

func structPointers(stct reflect.Value, cols []string) []interface{} {
	pointers := make([]interface{}, 0, len(cols))
	for _, colName := range cols {
		fieldVal := fieldByName(stct, colName)
		if !fieldVal.IsValid() || !fieldVal.CanSet() {
			// have to add if we found a column because Scan() requires
			// len(cols) arguments or it will error. This way we can scan to
			// nowhere
			nothing := reflect.New(reflect.TypeOf("")).Elem().Addr().Interface()
			pointers = append(pointers, nothing)
			continue
		}

		pointers = append(pointers, fieldVal.Addr().Interface())
	}
	return pointers
}

// RowsScanner is a database scanner for many rows. It is most commonly the result of
// *(database/sql).DB.Query(...) but can be mocked or stubbed
type RowsScanner interface {
	Scan(dest ...interface{}) error
	Columns() ([]string, error)
	ColumnTypes() ([]*sql.ColumnType, error)
	Err() error
	Next() bool
	NextResultSet() bool
}

// Scanner is a single row scanner. It is most commonly the result of
// *(database/sql).DB.QueryRow(...) but can be mocked or stubbed
type Scanner interface {
	Scan(dest ...interface{}) error
}
