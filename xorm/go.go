// Copyright 2017 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"go/format"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/go-xorm/core"
)

var (
	supportComment bool
	GoLangTmpl     LangTmpl = LangTmpl{
		template.FuncMap{
			"Mapper":   mapper.Table2Obj,
			"Type":     typestring,
			"Tag":      tag,
			"UnTitle":  unTitle,
			"gt":       gt,
			"getCol":   getCol,
			"distinct": distinct,
		},
		formatGo,
		genGoImports,
	}
)

var (
	errBadComparisonType = errors.New("invalid type for comparison")
	errBadComparison     = errors.New("incompatible types for comparison")
	errNoComparison      = errors.New("missing argument for comparison")
)

type kind int

const (
	invalidKind kind = iota
	boolKind
	complexKind
	intKind
	floatKind
	integerKind
	stringKind
	sliceKind
	uintKind
)

func basicKind(v reflect.Value) (kind, error) {
	switch v.Kind() {
	case reflect.Bool:
		return boolKind, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intKind, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintKind, nil
	case reflect.Float32, reflect.Float64:
		return floatKind, nil
	case reflect.Complex64, reflect.Complex128:
		return complexKind, nil
	case reflect.String:
		return stringKind, nil
	case reflect.Slice:
		return sliceKind, nil
	}
	return invalidKind, errBadComparisonType
}

// eq evaluates the comparison a == b || a == c || ...
func eq(arg1 interface{}, arg2 ...interface{}) (bool, error) {
	v1 := reflect.ValueOf(arg1)
	k1, err := basicKind(v1)
	if err != nil {
		return false, err
	}
	if len(arg2) == 0 {
		return false, errNoComparison
	}
	for _, arg := range arg2 {
		v2 := reflect.ValueOf(arg)
		k2, err := basicKind(v2)
		if err != nil {
			return false, err
		}
		if k1 != k2 {
			return false, errBadComparison
		}
		truth := false
		switch k1 {
		case boolKind:
			truth = v1.Bool() == v2.Bool()
		case complexKind:
			truth = v1.Complex() == v2.Complex()
		case floatKind:
			truth = v1.Float() == v2.Float()
		case intKind:
			truth = v1.Int() == v2.Int()
		case stringKind:
			truth = v1.String() == v2.String()
		case uintKind:
			truth = v1.Uint() == v2.Uint()
		default:
			panic("invalid kind")
		}
		if truth {
			return true, nil
		}
	}
	return false, nil
}

// lt evaluates the comparison a < b.
func lt(arg1, arg2 interface{}) (bool, error) {
	v1 := reflect.ValueOf(arg1)
	k1, err := basicKind(v1)
	if err != nil {
		return false, err
	}
	v2 := reflect.ValueOf(arg2)
	k2, err := basicKind(v2)
	if err != nil {
		return false, err
	}
	if k1 != k2 {
		return false, errBadComparison
	}
	truth := false
	switch k1 {
	case boolKind, complexKind:
		return false, errBadComparisonType
	case floatKind:
		truth = v1.Float() < v2.Float()
	case intKind:
		truth = v1.Int() < v2.Int()
	case stringKind:
		truth = v1.String() < v2.String()
	case uintKind:
		truth = v1.Uint() < v2.Uint()
	default:
		panic("invalid kind")
	}
	return truth, nil
}

// le evaluates the comparison <= b.
func le(arg1, arg2 interface{}) (bool, error) {
	// <= is < or ==.
	lessThan, err := lt(arg1, arg2)
	if lessThan || err != nil {
		return lessThan, err
	}
	return eq(arg1, arg2)
}

// gt evaluates the comparison a > b.
func gt(arg1, arg2 interface{}) (bool, error) {
	// > is the inverse of <=.
	lessOrEqual, err := le(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessOrEqual, nil
}

func getCol(cols map[string]*core.Column, name string) *core.Column {
	return cols[strings.ToLower(name)]
}

func formatGo(src string) (string, error) {
	source, err := format.Source([]byte(src))
	if err != nil {
		return "", err
	}
	return string(source), nil
}

func genGoImports(tables []*core.Table) map[string]string {
	imports := make(map[string]string)

	for _, table := range tables {
		for _, col := range table.Columns() {
			if typestring(col) == "time.Time" {
				imports["time"] = "time"
			}
		}
	}
	return imports
}

func typestring(col *core.Column) string {
	st := col.SQLType
	t := core.SQLType2Type(st)
	s := t.String()
	if s == "[]uint8" {
		return "[]byte"
	}
	return s
}

func tag(table *core.Table, col *core.Column) string {
	// isNameId := (mapper.Table2Obj(col.Name) == "Id")
	// isIdPk := isNameId && typestring(col) == "int64"

	var res []string

	// SQLType
	nstr := col.SQLType.Name
	if col.Length != 0 {
		if col.Length2 != 0 {
			nstr += fmt.Sprintf("(%v,%v)", col.Length, col.Length2)
		} else {
			nstr += fmt.Sprintf("(%v)", col.Length)
		}
	} else if len(col.EnumOptions) > 0 { //enum
		nstr += "("
		opts := ""

		enumOptions := make([]string, 0, len(col.EnumOptions))
		for enumOption := range col.EnumOptions {
			enumOptions = append(enumOptions, enumOption)
		}
		sort.Strings(enumOptions)

		for _, v := range enumOptions {
			opts += fmt.Sprintf(",'%v'", v)
		}
		nstr += strings.TrimLeft(opts, ",")
		nstr += ")"
	} else if len(col.SetOptions) > 0 { //enum
		nstr += "("
		opts := ""

		setOptions := make([]string, 0, len(col.SetOptions))
		for setOption := range col.SetOptions {
			setOptions = append(setOptions, setOption)
		}
		sort.Strings(setOptions)

		for _, v := range setOptions {
			opts += fmt.Sprintf(",'%v'", v)
		}
		nstr += strings.TrimLeft(opts, ",")
		nstr += ")"
	}
	res = append(res, fmt.Sprintf("%-20s", nstr))

	// IsPrimaryKey
	if col.IsPrimaryKey {
		nstr = "pk"
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-4s", nstr))

	// IsAutoIncrement
	if col.IsAutoIncrement {
		nstr = "autoincr"
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-10s", nstr))

	// VERSION
	if strings.ToUpper(col.Name) == "VERSION" {
		nstr = "version"
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-10s", nstr))

	// Nullable
	if !col.Nullable {
		if !col.IsPrimaryKey {
			nstr = "not null"
		} else {
			nstr = " "
		}
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-10s", nstr))

	// Default
	if col.Default != "" {
		colDefault := col.Default
		if strings.Contains(colDefault, "character varying") {
			colDefault = "''"
		}
		nstr = "default " + colDefault
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-20s", nstr))

	// created
	if col.IsCreated {
		nstr = "created"
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-10s", nstr))

	// updated
	if col.IsUpdated {
		nstr = "updated"
	} else {
		nstr = " "
	}
	res = append(res, fmt.Sprintf("%-10s", nstr))

	// Indexes
	if len(col.Indexes) == 0 {
		nstr = " "
		res = append(res, fmt.Sprintf("%-20s", nstr))
	} else {
		names := make([]string, 0, len(col.Indexes))
		for name := range col.Indexes {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			index := table.Indexes[name]
			var uistr string
			if index.Type == core.UniqueType {
				uistr = "unique"
			} else if index.Type == core.IndexType {
				uistr = "index"
			}
			if len(index.Cols) > 1 {
				uistr += "(" + index.Name + ")"
			}
			res = append(res, fmt.Sprintf("%-20s", uistr))
		}
	}

	// postgres did not suppoert
	if supportComment && col.Comment != "" {
		comment := fmt.Sprintf("      comment('%s')", col.Comment)
		res = append(res, fmt.Sprintf("%20s", comment))
	}

	var tags []string
	if genJson {
		tags = append(tags, "json:\""+col.Name+"\"  ")
	}
	if len(res) > 0 {
		tags = append(tags, "xorm:\""+strings.Join(res, " ")+"\"")
	}
	if genComment {
		tags = append(tags, "  comment:\""+col.Comment+"\"")
	}

	if len(tags) > 0 {
		return "`" + strings.Join(tags, " ") + "`"
	} else {
		return ""
	}
}

func distinct(input []string) []string {
	u := make([]string, 0, len(input))
	m := make(map[string]bool)
	for _, val := range input {
		if _, ok := m[val]; !ok {
			m[val] = true
			u = append(u, val)
		}
	}
	return u
}
