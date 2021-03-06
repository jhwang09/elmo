package sql

import (
	"database/sql"
	"log"
	"reflect"
	"strconv"
	"strings"

	"github.com/jhwang09/elmo/errs"
)

type Result sql.Result

type TxFunc func(shard *Shard) errs.Err

type Conn interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func NewShard(DBName string, db *sql.DB) (*Shard, errs.Err) {
	err := db.Ping()
	if err != nil {
		return nil, errs.NewStdError(err)
	}
	return &Shard{DBName, db, db}, nil
}

type Shard struct {
	DBName string
	db     *sql.DB // Nil for transaction and autocommit shard structs
	conn   Conn
}

func (s *Shard) Transact(txFunc TxFunc) errs.Err {
	if s.db == nil {
		return errs.NewError("This Shard is missing db")
	}

	conn, stdErr := s.db.Begin()
	if stdErr != nil {
		return errs.NewStdError(stdErr)
	}
	defer func() {
		if panicErr := recover(); panicErr != nil {
			rbErr := conn.Rollback()
			log.Println(errs.NewStdErrorWithInfo(rbErr, errs.Info{
				"Description": "Panic during sql transcation",
				"PanicErr":    panicErr}))
		}
	}()

	err := txFunc(&Shard{s.DBName, nil, conn})
	if err != nil {
		rbErr := conn.Rollback()
		if rbErr != nil {
			return errs.NewErrorWithInfo("txFunc has error", errs.Info{"TxFuncError": err, "TxRollbackError": rbErr})
		} else {
			return err
		}
	} else {
		stdErr = conn.Commit()
		if stdErr != nil {
			return errs.NewStdErrorWithInfo(stdErr, errs.Info{"Description": "Could not commit transaction"})
		}
	}
	return nil
}

// Query with fixed args
func (s *Shard) Query(query string, args ...interface{}) (*sql.Rows, errs.Err) {
	fixArgs(args)
	rows, stdErr := s.conn.Query(query, args...)
	if stdErr != nil {
		return nil, errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
	}
	return rows, nil
}

// Execute with fixed args
func (s *Shard) Exec(query string, args ...interface{}) (sql.Result, errs.Err) {
	fixArgs(args)
	res, stdErr := s.conn.Exec(query, args...)
	if stdErr != nil {
		return nil, errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
	}
	return res, nil
}
func IsDuplicateExecError(err errs.Err) bool {
	str := err.StdErrorMessage()
	return strings.HasPrefix(str, "Error 1060: Duplicate column name") ||
		strings.HasPrefix(str, "Error 1061: Duplicate key name") ||
		strings.HasPrefix(str, "Error 1050: Table") ||
		strings.HasPrefix(str, "Error 1022: Can't write; duplicate key in table") ||
		strings.HasPrefix(str, "Error 1062: Duplicate entry")
}
func (s *Shard) ExecIgnoreDuplicateError(query string, args ...interface{}) (res sql.Result, err errs.Err) {
	res, err = s.Exec(query, args...)
	if err != nil && IsDuplicateExecError(err) {
		err = nil
	}
	return
}

/*
Fix args by converting them to values of their underlying kind.
This avoids problems in database/sql with e.g custom string types.
Without fixArgs, the following code:

	type Foo string
	...
	pool.Query("SELECT * WHERE Foo=?", Foo("bar"))

would give you the error:

	sql: converting Exec argument #1's type: unsupported type Foo, a string
*/
func fixArgs(args []interface{}) {
	for i, arg := range args {
		vArg := reflect.ValueOf(arg)
		switch vArg.Kind() {
		case reflect.String:
			args[i] = vArg.String()
			if args[i] == "" {
				args[i] = nil
			}
		}
	}
}

func (s *Shard) SelectInt(query string, args ...interface{}) (num int64, found bool, err errs.Err) {
	found, err = s.queryOne(query, args, &num)
	return
}

func (s *Shard) SelectString(query string, args ...interface{}) (str string, found bool, err errs.Err) {
	found, err = s.queryOne(query, args, &str)
	return
}

func (s *Shard) SelectUint(query string, args ...interface{}) (num uint, found bool, err errs.Err) {
	found, err = s.queryOne(query, args, &num)
	return
}

func (s *Shard) queryOne(query string, args []interface{}, out interface{}) (found bool, err errs.Err) {
	rows, err := s.Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	if rows.Next() {
		stdErr := rows.Scan(out)
		if stdErr != nil {
			err = errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
			return
		}
		if rows.Next() {
			err = errs.NewErrorWithInfo("queryOne query returned too many rows", errInfo(query, args))
			return
		}
		found = true
	}

	stdErr := rows.Err()
	if stdErr != nil {
		err = errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
		return
	}

	return
}

func (s *Shard) UpdateOne(query string, args ...interface{}) (err errs.Err) {
	return s.UpdateNum(1, query, args...)
}

func (s *Shard) UpdateNum(num int64, query string, args ...interface{}) (err errs.Err) {
	rowsAffected, err := s.Update(query, args...)
	if err != nil {
		return err
	}
	if rowsAffected != num {
		return errs.NewErrorWithInfo("UpdateNum affected unexpected number of rows",
			errInfo(query, args, errs.Info{"ExpectedRows": num, "AffectedRows": rowsAffected}))
	}
	return
}

func (s *Shard) Update(query string, args ...interface{}) (rowsAffected int64, err errs.Err) {
	res, err := s.Exec(query, args...)
	if err != nil {
		return
	}

	rowsAffected, stdErr := res.RowsAffected()
	if stdErr != nil {
		err = errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
		return
	}
	return
}

func (s *Shard) InsertIgnoreID(query string, args ...interface{}) (err errs.Err) {
	_, err = s.Insert(query, args...)
	return
}

func IsDuplicateEntryError(err errs.Err) bool {
	str := err.StdErrorMessage()
	return strings.Contains(str, "Duplicate entry")
}

func (s *Shard) InsertIgnoreDuplicates(query string, args ...interface{}) (err errs.Err) {
	_, err = s.Insert(query, args...)
	if err != nil && IsDuplicateEntryError(err) {
		err = nil
	}
	return
}

func (s *Shard) Insert(query string, args ...interface{}) (id int64, err errs.Err) {
	res, err := s.Exec(query, args...)
	if err != nil {
		return
	}
	id, stdErr := res.LastInsertId()
	if stdErr != nil {
		err = errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
		return
	}
	return
}

func (s *Shard) Select(output interface{}, query string, args ...interface{}) errs.Err {
	// Check types
	var outputPtr = reflect.ValueOf(output)
	if outputPtr.Kind() != reflect.Ptr {
		return errs.NewErrorWithInfo("Select expects a pointer to a slice of items", errInfo(query, args))
	}
	var outputReflection = reflect.Indirect(outputPtr)
	if outputReflection.Kind() != reflect.Slice {
		return errs.NewErrorWithInfo("Select expects items to be a slice", errInfo(query, args))
	}
	if outputReflection.Len() != 0 {
		return errs.NewErrorWithInfo("Select expects items to be empty", errInfo(query, args))
	}
	outputReflection.Set(reflect.MakeSlice(outputReflection.Type(), 0, 0))

	// Query DB
	var rows, err = s.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns, stdErr := rows.Columns()
	if stdErr != nil {
		return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
	}

	valType := outputReflection.Type().Elem()
	isStruct := (valType.Kind() == reflect.Ptr && valType.Elem().Kind() == reflect.Struct)
	if isStruct {
		// Reflect onto structs
		for rows.Next() {
			structPtrVal := reflect.New(valType.Elem())
			outputItemStructVal := structPtrVal.Elem()
			err = structFromRow(outputItemStructVal, columns, rows, query, args)
			if err != nil {
				return err
			}
			outputReflection.Set(reflect.Append(outputReflection, structPtrVal))
		}
	} else {
		if len(columns) != 1 {
			return errs.NewErrorWithInfo("Select expected single column in select statement for slice of non-struct values", errInfo(query, args))
		}
		for rows.Next() {
			rawBytes := &sql.RawBytes{}
			stdErr = rows.Scan(rawBytes)
			if stdErr != nil {
				return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
			}
			outputValue := reflect.New(valType).Elem()
			err = scanColumnValue(columns[0], outputValue, rawBytes, query, args)
			if err != nil {
				return err
			}
			outputReflection.Set(reflect.Append(outputReflection, outputValue))
		}
	}

	stdErr = rows.Err()
	if err != nil {
		return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
	}
	return nil
}

const scanOneTypeError = "elmo/sql.SelectOne: expects a **struct, e.g var person *Person; c.SelectOne(&person, sql)"

func (s *Shard) SelectOne(output interface{}, query string, args ...interface{}) (found bool, err errs.Err) {
	return s.scanOne(output, query, args...)
}
func (s *Shard) scanOne(output interface{}, query string, args ...interface{}) (found bool, err errs.Err) {
	// Check types
	var outputReflectionPtr = reflect.ValueOf(output)
	if !outputReflectionPtr.IsValid() {
		err = errs.NewError(scanOneTypeError)
		return
	}
	if outputReflectionPtr.Kind() != reflect.Ptr {
		err = errs.NewError(scanOneTypeError)
		return
	}
	var outputReflection = outputReflectionPtr.Elem()
	if outputReflection.Kind() != reflect.Ptr {
		err = errs.NewError(scanOneTypeError)
		return
	}

	// Query DB
	rows, err := s.Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	// Reflect onto struct
	columns, stdErr := rows.Columns()
	if stdErr != nil {
		err = errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
		return
	}
	if !rows.Next() {
		return
	}

	var vStruct reflect.Value
	if outputReflection.IsNil() {
		structPtrVal := reflect.New(outputReflection.Type().Elem())
		outputReflection.Set(structPtrVal)
		vStruct = structPtrVal.Elem()
	} else {
		vStruct = outputReflection.Elem()
	}

	err = structFromRow(vStruct, columns, rows, query, args)
	if err != nil {
		return
	}

	if rows.Next() {
		err = errs.NewErrorWithInfo("scanOne got multiple rows", errInfo(query, args))
		return
	}

	stdErr = rows.Err()
	if stdErr != nil {
		err = errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
		return
	}

	found = true
	return
}

type scanError struct {
	err   error
	query string
}

func (s *scanError) Error() string {
	return s.err.Error() + " [SQL: " + s.query + "]"
}

func structFromRow(outputItemStructVal reflect.Value, columns []string, rows *sql.Rows, query string, args []interface{}) errs.Err {
	vals := make([]interface{}, len(columns))
	for i, _ := range columns {
		vals[i] = &sql.RawBytes{}
	}
	stdErr := rows.Scan(vals...)
	if stdErr != nil {
		return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args))
	}

	for i, column := range columns {
		structFieldValue := outputItemStructVal.FieldByName(column)
		if !structFieldValue.IsValid() {
			// commented out as this cause noise in the logs
			// fmt.Println("Warning: no corresponding struct field found for column: " + column)
			continue
		}
		err := scanColumnValue(column, structFieldValue, vals[i].(*sql.RawBytes), query, args)
		if err != nil {
			return err
		}
	}

	return nil
}

func scanColumnValue(column string, reflectVal reflect.Value, value *sql.RawBytes, query string, args []interface{}) errs.Err {
	bytes := []byte(*value)
	if bytes == nil {
		return nil // Leave struct field empty
	}
	switch reflectVal.Kind() {
	case reflect.String:
		reflectVal.SetString(string(bytes))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, stdErr := strconv.ParseUint(string(bytes), 10, 64)
		if stdErr != nil {
			return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args, errs.Info{"Bytes": bytes}))
		}
		reflectVal.SetUint(reflect.ValueOf(uintVal).Uint())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, stdErr := strconv.ParseInt(string(bytes), 10, 64)
		if stdErr != nil {
			return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args, errs.Info{"Bytes": bytes}))
		}
		reflectVal.SetInt(reflect.ValueOf(intVal).Int())
	case reflect.Bool:
		boolVal, stdErr := strconv.ParseBool(string(bytes))
		if stdErr != nil {
			return errs.NewStdErrorWithInfo(stdErr, errInfo(query, args, errs.Info{"Bytes": bytes}))
		}
		reflectVal.SetBool(reflect.ValueOf(boolVal).Bool())
	default:
		if reflectVal.Kind() == reflect.Slice { // && reflectVal. == reflect.Uint8 {
			// byte slice
			reflectVal.SetBytes(bytes)
		} else {
			return errs.NewErrorWithInfo("Bad row value for column "+column+": "+reflectVal.Kind().String(), errInfo(query, args))
		}
	}
	return nil
}

func errInfo(query string, args []interface{}, infos ...errs.Info) errs.Info {
	info := errs.Info{"Query": query, "Args": args}
	for _, moreInfo := range infos {
		for key, val := range moreInfo {
			info[key] = val
		}
	}
	return info
}
