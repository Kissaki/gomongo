// Copyright 2009 The Go Authors.  All rights reserved.
// Copyright 2009,2010, The 'gomongo' Authors.  All rights reserved.
// Use of this source code is governed by the 3-clause BSD License
// that can be found in the LICENSE and LICENSE.GO files.

package mongo

import (
	"io"
	"os"
	"math"
	"reflect"
	"strings"
	"time"
	"bytes"
	"strconv"
)

type SimpleContainer struct {
	Val interface{}
}

// LenWriter records the current write postion on the buffer
// and can later be used to recor the number of bytes written
// in conformance to BSON spec
type LenWriter struct {
	buf        *bytes.Buffer
	len_offset int
}

func NewLenWriter(buf *bytes.Buffer) *LenWriter {
	len_offset := len(buf.Bytes())
	w32 := make([]byte, _WORD32)
	buf.Write(w32)
	return &LenWriter{buf, len_offset}
}

func (self *LenWriter) RecordLen() {
	buf := self.buf.Bytes()
	final_len := len(buf)
	w32 := buf[self.len_offset : self.len_offset+_WORD32]
	pack.PutUint32(w32, uint32(final_len-self.len_offset))
}

func MarshalToStream(writer io.Writer, val interface{}) (err os.Error) {
	var encoded []byte
	encoded, err = Marshal2(val)
	if err != nil {
		return err
	}
	_, err = writer.Write(encoded)
	return err
}

func Marshal2(val interface{}) (encoded []byte, err os.Error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(bsonError)
		}
	}()

	if val == nil {
		return nil, os.NewError("Cannot marshal empty object")
	}

	// Dereference pointer types
	switch v := reflect.ValueOf(val); v.Kind() {
	case reflect.Ptr:
		val = v.Elem().Interface()
	}

	buf := bytes.NewBuffer(make([]byte, 0, 32))
	switch fv := reflect.ValueOf(val); fv.Kind() {
	case reflect.Float32, reflect.Float64, reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Slice, reflect.Array:
		// Wrap simple types in a container
		val = SimpleContainer{fv.Interface()}
		EncodeStruct(buf, reflect.ValueOf(val))
	case reflect.Struct:
		EncodeStruct(buf, fv)
	case reflect.Map:
		EncodeMap(buf, fv)
	default:
		panic(NewBsonError("Unexpected type %v\n", fv.Type()))
	}
	return buf.Bytes(), err
}

func EncodeField(buf *bytes.Buffer, key string, val interface{}) {
	// MongoDB uses '_id' as the primary key, but this
	// name is private in Go. Use 'Id_' for this purpose
	// instead.
	if key == "id_" {
		key = "_id"
	}
	switch v := val.(type) {
	case time.Time:
		EncodePrefix(buf, '\x11', key)
		EncodeTime(buf, v)
	case []byte:
		EncodePrefix(buf, '\x05', key)
		EncodeBinary(buf, v)
	default:
		goto CompositeType
	}
	return

CompositeType:
	switch fv := reflect.ValueOf(val); fv.Kind() {
	case reflect.Float32, reflect.Float64:
		EncodePrefix(buf, '\x01', key)
		EncodeFloat64(buf, fv.Float())
	case reflect.String:
		EncodePrefix(buf, '\x02', key)
		EncodeString(buf, fv.String())
	case reflect.Bool:
		EncodePrefix(buf, '\x08', key)
		EncodeBool(buf, fv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		EncodePrefix(buf, '\x12', key)
		EncodeUint64(buf, uint64(fv.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		EncodePrefix(buf, '\x12', key)
		EncodeUint64(buf, fv.Uint())
	case reflect.Struct:
		EncodePrefix(buf, '\x03', key)
		EncodeStruct(buf, fv)
	case reflect.Map:
		EncodePrefix(buf, '\x03', key)
		EncodeMap(buf, fv)
	case reflect.Slice:
		EncodePrefix(buf, '\x04', key)
		EncodeSlice(buf, fv)
	case reflect.Ptr:
		EncodeField(buf, key, fv.Elem().Interface())
	default:
		panic(NewBsonError("don't know how to marshal %v\n", reflect.ValueOf(val).Type()))
	}
}

func EncodePrefix(buf *bytes.Buffer, etype byte, key string) {
	buf.WriteByte(etype)
	buf.WriteString(key)
	buf.WriteByte(0)
}

func EncodeFloat64(buf *bytes.Buffer, val float64) {
	bits := math.Float64bits(val)
	w64 := make([]byte, _WORD64)
	pack.PutUint64(w64, bits)
	buf.Write(w64)
}

func EncodeString(buf *bytes.Buffer, val string) {
	w32 := make([]byte, _WORD32)
	pack.PutUint32(w32, uint32(len(val)+1))
	buf.Write(w32)
	buf.WriteString(val)
	buf.WriteByte(0)
}

func EncodeBool(buf *bytes.Buffer, val bool) {
	if val {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func EncodeUint64(buf *bytes.Buffer, val uint64) {
	w64 := make([]byte, _WORD64)
	pack.PutUint64(w64, val)
	buf.Write(w64)
}

func EncodeTime(buf *bytes.Buffer, val time.Time) {
	w64 := make([]byte, _WORD64)
	mtime := val.Seconds() * 1000
	pack.PutUint64(w64, uint64(mtime))
	buf.Write(w64)
}

func EncodeBinary(buf *bytes.Buffer, val []byte) {
	w32 := make([]byte, _WORD32)
	pack.PutUint32(w32, uint32(len(val)))
	buf.Write(w32)
	buf.WriteByte(0)
	buf.Write(val)
}

func EncodeStruct(buf *bytes.Buffer, val reflect.Value) {
	lenWriter := NewLenWriter(buf)
	t := val.Type()
	for i := 0; i < t.NumField(); i++ {
		key := strings.ToLower(t.Field(i).Name)
		EncodeField(buf, key, val.Field(i).Interface())
	}
	buf.WriteByte(0)
	lenWriter.RecordLen()
}

func EncodeMap(buf *bytes.Buffer, val reflect.Value) {
	lenWriter := NewLenWriter(buf)
	mt := val.Type()
	if mt.Key() != reflect.TypeOf("") {
		panic(NewBsonError("can't marshall maps with non-string key types"))
	}
	keys := val.MapKeys()
	for _, k := range keys {
		key := k.String()
		EncodeField(buf, key, val.MapIndex(k).Interface())
	}
	buf.WriteByte(0)
	lenWriter.RecordLen()
}

func EncodeSlice(buf *bytes.Buffer, val reflect.Value) {
	lenWriter := NewLenWriter(buf)
	for i := 0; i < val.Len(); i++ {
		EncodeField(buf, strconv.Itoa(i), val.Index(i).Interface())
	}
	buf.WriteByte(0)
	lenWriter.RecordLen()
}
