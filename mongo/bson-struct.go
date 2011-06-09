// Based on the Go json package.

// Copyright 2009 The Go Authors.  All rights reserved.
// Copyright 2009-2011 The gomongo Authors.  All rights reserved.
// Use of this source code is governed by the 3-clause BSD License
// that can be found in the LICENSE and LICENSE.GO files.

package mongo

import (
	"io"
	"reflect"
	"strings"
	"fmt"
	"os"
	"bytes"
	"time"
	"container/vector"
	"strconv"
)

type bsonError struct {
	msg string
}

func NewBsonError(format string, args ...interface{}) bsonError {
	return bsonError{fmt.Sprintf(format, args...)}
}

func (self bsonError) String() string {
	return self.msg
}

// Maps & interface values will not give you a reference to their underlying object.
// You can only update them through their Set methods.
type structBuilder struct {
	val reflect.Value

	// isSimple == bool: It's the top level structBuilder object and it has a simple value
	isSimple bool

	// if map_ != nil, write val to map_[key] when val is finalized, performed by Flush()
	map_ reflect.Value
	key  reflect.Value

	// if interface_ != nil, write val to interface_ when val is finalized. Performed by Flush()
	interface_ reflect.Value
}

func NewStructBuilder(val reflect.Value) *structBuilder {
	// Dereference pointers here so we don't have to handle this case everywhere else
	if v := val; v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.Zero(v.Type().Elem()).Addr())
		}
		val = v.Elem()
	}
	return &structBuilder{val: val}
}

func MapValueBuilder(val reflect.Value, map_ reflect.Value, key reflect.Value) *structBuilder {
	if v := val; v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.Zero(v.Type().Elem()).Addr())
		}
		map_.SetMapIndex(key, v)
		val = v.Elem()
		return &structBuilder{val: val}
	}
	return &structBuilder{val: val, map_: map_, key: key}
}

// Returns a valid unmarshalable structBuilder or an error
func TopLevelBuilder(val interface{}) (sb *structBuilder, err os.Error) {
	ival := reflect.ValueOf(val)
	v := ival
	if v.Kind() != reflect.Ptr {
		return nil, os.NewError(fmt.Sprintf("expecting pointer value, received %v", ival.Type()))
	}
	// We'll allow one level of indirection
	switch actual := v.Elem(); actual.Kind() {
	case reflect.Float32, reflect.Float64, reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Slice, reflect.Array:
		sb := NewStructBuilder(actual)
		sb.isSimple = true // Prepare to receive a simple value
		return sb, nil
	case reflect.Map, reflect.Struct, reflect.Interface:
		sb := NewStructBuilder(actual)
		sb.Object() // Allocate memory if necessary
		return sb, nil
	}
	return nil, os.NewError(fmt.Sprintf("unrecognized type %v", ival.Type()))
}

// Flush handles the final update for map & interface objects.
func (self *structBuilder) Flush() {
	if self.interface_ != nil {
		self.interface_.Set(self.val)
	}
	if self.map_ != nil {
		if self.interface_ != nil {
			self.map_.SetElem(self.key, self.interface_)
		} else {
			self.map_.SetElem(self.key, self.val)
		}
	}
}

// Defer update if it's an interface, handled by Flush().
func (self *structBuilder) DeferSet(val reflect.Value) {
	self.interface_ = self.val
	self.val = val
}

func (self *structBuilder) Int64(i int64) {
	switch v := self.val; v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(i))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(i))
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(i))
	default:
		panic(NewBsonError("unable to convert int64 %v to %s", i, self.val.Type()))
	}
}

func (self *structBuilder) Date(t *time.Time) {
	switch v := self.val; v.Kind() {
	case reflect.Struct:
		v.Set(reflect.ValueOf(*t))
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(*t))
	default:
		panic(NewBsonError("unable to convert time %v to %s", *t, self.val.Type()))
	}
}

func (self *structBuilder) Int32(i int32) {
	switch v := self.val; v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(i))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(uint32(i)))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(i))
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(i))
	default:
		panic(NewBsonError("unable to convert int32 %v to %s", i, self.val.Type()))
	}
}

func (self *structBuilder) Float64(f float64) {
	switch v := self.val; v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(f))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(uint64(f))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(f)
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(f))
	default:
		panic(NewBsonError("unable to convert float64 %v to %s", f, self.val.Type()))
	}
}

func (self *structBuilder) Null() {}

func (self *structBuilder) String(s string) {
	switch v := self.val; v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(s))
	default:
		panic(NewBsonError("unable to convert string %v to %s", s, self.val.Type()))
	}
}

func (self *structBuilder) Regex(regex, options string) {
	// No special treatment
	self.String(regex)
}

func (self *structBuilder) Bool(tf bool) {
	switch v := self.val; v.Kind() {
	case reflect.Bool:
		v.SetBool(tf)
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(tf))
	default:
		panic(NewBsonError("unable to convert bool %v to %s", tf, self.val.Type()))
	}
}

func (self *structBuilder) OID(oid []byte) {
	self.Binary(oid)
}

func (self *structBuilder) Array() {
	switch v := self.val; v.Kind() {
	case reflect.Array:
		// no op
	case reflect.Slice:
		if v.IsNil() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 8))
		}
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(make([]interface{}, 0, 8)))
	default:
		panic(NewBsonError("unable to convert array to %s", self.val.Type()))
	}
}

func (self *structBuilder) Binary(bindata []byte) {
	switch v := self.val; v.Kind() {
	case reflect.Array:
		if v.Cap() < len(bindata) {
			panic(NewBsonError("insufficient space in array. Have: %v, Need: %v", v.Cap(), len(bindata)))
		}
		for i := 0; i < len(bindata); i++ {
			v.Index(i).SetUint(uint64(bindata[i]))
		}
	case reflect.Slice:
		if v.IsNil() {
			// Just point it to the bindata object
			v.Set(reflect.ValueOf(bindata))
			return
		}
		if v.Cap() < len(bindata) {
			nv := reflect.MakeSlice(v.Type(), len(bindata), len(bindata))
			v.Set(nv)
		}
		for i := 0; i < len(bindata); i++ {
			v.Index(i).SetUint(uint64(bindata[i]))
		}
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(bindata))
	default:
		panic(NewBsonError("unable to convert oid %v to %s", bindata, self.val.Type()))
	}
}

func (self *structBuilder) Elem(i int) Builder {
	if i < 0 {
		panic(NewBsonError("negative index %v for array element", i))
	}
	switch v := self.val; v.Kind() {
	case reflect.Array:
		if i < v.Len() {
			return NewStructBuilder(v.Index(i))
		} else {
			panic(NewBsonError("array index %v out of bounds", i))
		}
	case reflect.Slice:
		if i > v.Cap() {
			n := v.Cap()
			if n < 8 {
				n = 8
			}
			for n <= i {
				n *= 2
			}
			nv := reflect.MakeSlice(v.Type(), v.Len(), n)
			reflect.Copy(nv, v)
			v.Set(nv)
		}
		if v.Len() <= i && i < v.Cap() {
			v.SetLen(i + 1)
		}
		if i < v.Len() {
			return NewStructBuilder(v.Index(i))
		} else {
			panic(NewBsonError("internal error, realloc failed?"))
		}
	}
	panic(NewBsonError("unexpected type %s, expecting slice or array", self.val.Type()))
}

func (self *structBuilder) Object() {
	switch v := self.val; v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	case reflect.Struct:
		// no op
	case reflect.Interface:
		self.DeferSet(reflect.ValueOf(make(map[string]interface{})))
	default:
		panic(NewBsonError("unexpected type %s, expecting composite type", self.val.Type()))
	}
}

func (self *structBuilder) Key(k string) Builder {
	switch v := self.val; v.Kind() {
	case reflect.Struct:
		t := v.Type()
		// Case-insensitive field lookup.
		k = strings.ToLower(k)
		for i := 0; i < t.NumField(); i++ {
			if strings.ToLower(t.Field(i).Name) == k {
				return NewStructBuilder(v.Field(i))
			}
		}
	case reflect.Map:
		t := v.Type()
		if t.Key() != reflect.TypeOf(k) {
			break
		}
		key := reflect.ValueOf(k)
		elem := v.MapIndex(key)
		if !elem.IsValid() {
			v.SetMapIndex(key, reflect.Zero(t.Elem()))
			elem = v.MapIndex(key)
		}
		return MapValueBuilder(elem, v, key)
	case reflect.Slice, reflect.Array:
		if self.isSimple {
			self.isSimple = false
			return self
		}
		index, err := strconv.Atoi(k)
		if err != nil {
			panic(bsonError{err.String()})
		}
		return self.Elem(index)
	case reflect.Float32, reflect.Float64, reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Special case. We're unmarshaling into a simple type.
		if self.isSimple {
			self.isSimple = false
			return self
		}
	}
	panic(NewBsonError("%s not supported as a BSON document", self.val.Type()))
}

func Unmarshal(b []byte, val interface{}) (err os.Error) {
	sb, terr := TopLevelBuilder(val)
	if terr != nil {
		return terr
	}
	err = Parse(bytes.NewBuffer(b[4:len(b)]), sb)
	sb.Flush()
	return
}

func UnmarshalFromStream(reader io.Reader, val interface{}) (err os.Error) {
	lenbuf := make([]byte, 4)
	var n int
	n, err = reader.Read(lenbuf)
	if err != nil {
		return err
	}
	if n != 4 {
		return io.ErrUnexpectedEOF
	}
	length := pack.Uint32(lenbuf)
	buf := make([]byte, length)
	pack.PutUint32(buf, length)
	n, err = reader.Read(buf[4:])
	if err != nil {
		if err == os.EOF {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	if n != int(length-4) {
		return io.ErrUnexpectedEOF
	}
	return Unmarshal(buf, val)
}

func Marshal(val interface{}) (BSON, os.Error) {
	if val == nil {
		return Null, nil
	}

	switch v := val.(type) {
	case float64:
		return &_Number{v, _Null{}}, nil
	case string:
		return &_String{v, _Null{}}, nil
	case bool:
		return &_Boolean{v, _Null{}}, nil
	case int32:
		return &_Int{v, _Null{}}, nil
	case int64:
		return &_Long{v, _Null{}}, nil
	case int:
		return &_Long{int64(v), _Null{}}, nil
	case *time.Time:
		return &_Date{v, _Null{}}, nil
	case []byte:
		return &_Binary{v, _Null{}}, nil
	}

	var value reflect.Value
	switch nv := reflect.ValueOf(val); nv.Kind() {
	case reflect.Ptr:
		value = nv.Elem()
	default:
		value = nv
	}

	switch fv := value; fv.Kind() {
	case reflect.Struct:
		o := &_Object{map[string]BSON{}, _Null{}}
		t := fv.Type()
		for i := 0; i < t.NumField(); i++ {
			key := strings.ToLower(t.Field(i).Name)
			el, err := Marshal(fv.Field(i).Interface())
			if err != nil {
				return nil, err
			}
			// MongoDB uses '_id' as the primary key, but this
			// name is private in Go. Use 'Id_' for this purpose
			// instead.
			if key == "id_" {
				key = "_id"
			}
			o.value[key] = el
		}
		return o, nil
	case reflect.Map:
		o := &_Object{map[string]BSON{}, _Null{}}
		mt := fv.Type()
		if mt.Key() != reflect.TypeOf("") {
			return nil, os.NewError("can't marshall maps with non-string key types")
		}

		keys := fv.MapKeys()
		for _, k := range keys {
			sk := k.String()
			el, err := Marshal(fv.MapIndex(k).Interface())
			if err != nil {
				return nil, err
			}
			o.value[sk] = el
		}
		return o, nil
	case reflect.Slice:
		a := &_Array{new(vector.Vector), _Null{}}
		for i := 0; i < fv.Len(); i++ {
			el, err := Marshal(fv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			a.value.Push(el)
		}
		return a, nil
	default:
		return nil, os.NewError(fmt.Sprintf("don't know how to marshal %v\n", value.Type()))
	}

	return nil, nil
}
