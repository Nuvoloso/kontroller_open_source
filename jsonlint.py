#!/usr/bin/env python2.7
"""
Copyright 2019 Tad Lebeck

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

json syntax checker for use by build
"""
import fileinput
import json
import sys


def process(filename, contents):
    """ process one file """
    try:
        json.loads(contents)
    except ValueError, ex:
        print filename + ':', ex
        return 1
    return 0


def jsonlint():
    """ lint all files on command line or read from stdin """
    filename = ''
    contents = ''
    failures = 0
    for line in fileinput.input():
        if fileinput.isfirstline() and contents != '':
            failures += process(filename, contents)
            contents = ''
        filename = fileinput.filename()
        contents += line
    if contents != '':
        failures += process(filename, contents)
    return failures


if __name__ == '__main__':
    sys.exit(jsonlint())
