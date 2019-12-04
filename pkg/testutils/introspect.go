// Copyright 2019 Tad Lebeck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package testutils

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"regexp"
	"strconv"
)

// BasicConstants contains the constants of the file
type BasicConstants struct {
	CMap map[string]*ast.BasicLit
}

// FetchBasicConstantsFromGoFile returns the basic constant symbols from a Go source file
func FetchBasicConstantsFromGoFile(fileName string) (*BasicConstants, error) {
	data, err := ioutil.ReadFile(fileName)
	if err == nil {
		fSet := token.NewFileSet()
		var f *ast.File
		if f, err = parser.ParseFile(fSet, "src.go", data, 0); err == nil {
			// ast.Print(fSet, f)
			cMap := make(map[string]*ast.BasicLit)
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.ValueSpec:
					if len(x.Names) > 0 && len(x.Values) > 0 && x.Names[0].Obj.Kind == ast.Con {
						switch v := x.Values[0].(type) {
						case *ast.BasicLit:
							cMap[x.Names[0].Name] = v
						}
					}
				}
				return true
			})
			return &BasicConstants{CMap: cMap}, nil
		}
	}
	return nil, err
}

// GetStringConstants returns all string constants and their (parsed) value, optionally matching a name pattern
func (bc *BasicConstants) GetStringConstants(re *regexp.Regexp) map[string]string {
	sMap := make(map[string]string)
	for n, lit := range bc.CMap {
		if lit.Kind == token.STRING && (re == nil || re.MatchString(n)) {
			sl := len(lit.Value)
			sMap[n] = lit.Value[1 : sl-1]
		}
	}
	return sMap
}

// GetIntConstants returns all int constants and their (parsed) value, optionally matching a name pattern
func (bc *BasicConstants) GetIntConstants(re *regexp.Regexp) map[string]int {
	iMap := make(map[string]int)
	for n, lit := range bc.CMap {
		if lit.Kind == token.INT && (re == nil || re.MatchString(n)) {
			if i64, err := strconv.ParseInt(lit.Value, 0, 0); err == nil {
				iMap[n] = int(i64)
			}
		}
	}
	return iMap
}

const testConstInt = 3
const testConstString = "string"
