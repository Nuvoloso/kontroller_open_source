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

// Inspired by the code in
// https://github.com/eclipse/jetty.project/blob/master/jetty-util/src/main/java/org/eclipse/jetty/util/security/Password.java
//
// That code was copyrighted as follows:
//  Copyright (c) 1995-2017 Mort Bay Consulting Pty. Ltd.
//  All rights reserved. This program and the accompanying materials
//  are made available under the terms of the Eclipse Public License v1.0
//  and Apache License v2.0 which accompanies this distribution.
//
//      The Eclipse Public License is available at
//      http://www.eclipse.org/legal/epl-v10.html
//
//      The Apache License v2.0 is available at
//      http://www.opensource.org/licenses/apache2.0.php
//
//  You may elect to redistribute this code under either of these licenses.

package util

import (
	"strconv"
	"strings"
)

const obfPrefix = "OBF:"

// Obfuscate returns an obfuscated form of the input string. The empty string is not obfuscated.
func Obfuscate(s string) string {
	if len(s) == 0 {
		return s
	}
	b := []byte(s)
	var buf strings.Builder
	buf.WriteString(obfPrefix)

	for i, b1 := range b {
		b2 := b[len(b)-(i+1)]
		i1 := 255 + int(b1) + int(b2)
		i2 := 255 + int(b1) - int(b2)
		i0 := i1*512 + i2
		x := strconv.FormatInt(int64(i0), 36)

		// Smallest possible value for i0 is for (b1, b2) == (0, 0): 130815 == base36(2sxr)
		// Largest possible value for i0 is for (b1, b2) == (255, 255): 391935 = base64(8ef3)
		buf.WriteString(x)
	}
	return buf.String()
}

// DeObfuscate returns the de-obfuscated form of the input string. If the input string lacks the OBF: prefix, the string itself is returned
func DeObfuscate(s string) (string, error) {
	if !strings.HasPrefix(s, obfPrefix) {
		return s, nil
	}
	s = s[4:]
	var buf strings.Builder
	for i := 0; i < len(s); i += 4 {
		x := s[i:]
		if len(x) >= 4 {
			x = x[:4]
		}
		i0, err := strconv.ParseInt(x, 36, 0)
		if err != nil {
			return "", err
		}
		i1 := i0 / 512
		i2 := i0 % 512
		buf.WriteByte(byte((i1 + i2 - 510) / 2))
	}
	return buf.String(), nil
}
